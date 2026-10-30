package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/ohing504/devclean/internal/fstree"
	"github.com/ohing504/devclean/internal/model"
)

// artifactRule describes one cleanable artifact matched by the walk engine.
// Exactly one of the match fields is set:
//
//   - RelPath: exact, "/"-separated path relative to the nearest enclosing
//     project root of the rule's ecosystem — "node_modules" matches a direct
//     child of a project root, "ios/Pods" matches one level deeper.
//   - Name: directory name matched at any depth under the nearest enclosing
//     project root (Python __pycache__ lives at every package level).
//   - Suffix: directory-name suffix matched at any depth (Python *.egg-info).
type artifactRule struct {
	RelPath  string
	Name     string
	Suffix   string
	Category model.Category
	Safety   model.SafetyLevel
	// Describe optionally inspects the matched directory (e.g. which tool
	// populated it) to fill the result's fields below.
	Describe func(dir string) artifactNote
}

type artifactNote struct {
	Recommendation string
	CloneAware     bool
	CloneSource    string
}

// matches reports whether a directory matches this rule. rel is the
// "/"-separated path from the nearest project root of the rule's ecosystem,
// name is the directory's base name.
func (r artifactRule) matches(rel, name string) bool {
	switch {
	case r.RelPath != "":
		return rel == r.RelPath
	case r.Name != "":
		return name == r.Name
	case r.Suffix != "":
		return strings.HasSuffix(name, r.Suffix)
	}
	return false
}

// walkEcosystem describes how one ecosystem participates in the single-pass
// walk: which marker files identify a project root and which artifact rules
// apply beneath it.
type walkEcosystem struct {
	Name    string // scanner name shown in progress output, e.g. "node"
	Eco     model.Ecosystem
	Markers []string // any-of file names marking a project root, e.g. "package.json"
	Rules   []artifactRule
	// ExtraRules optionally contributes additional rules for a specific
	// project, decided when its root is detected (e.g. React Native compound
	// artifacts). entryNames holds the names of the root's direct entries.
	ExtraRules func(projectRoot string, entryNames map[string]bool) []artifactRule
	// SetProjectRoot populates ScanResult.ProjectRoot with the root of the
	// matched project context. Used by ecosystems whose artifacts sit at
	// arbitrary depth (python), where output grouping needs explicit
	// attribution.
	SetProjectRoot bool
	// PruneRoot reports whether dir is a tree that must be skipped entirely —
	// no artifact matching, no project context, no descent. Used to exclude an
	// ecosystem's own toolchain/SDK checkout (e.g. the Flutter SDK), whose
	// internal build/.dart_tool dirs are managed by the tool itself and must
	// never be offered for deletion. names holds dir's direct entries so cheap
	// gate checks avoid a stat on every directory. Trees that are no project
	// for any ecosystem go in isInstalledPackageTree instead.
	PruneRoot func(dir string, names map[string]bool) bool
}

// walkEcosystemTable is the canonical, ordered table of walk-based
// ecosystems. The order decides attribution when a directory matches rules
// of several ecosystems (first match wins) and the result ordering; it
// mirrors the registry order.
var walkEcosystemTable = []walkEcosystem{nodeWalkEcosystem, rustWalkEcosystem, rubyWalkEcosystem, pythonWalkEcosystem, goWalkEcosystem, flutterWalkEcosystem, androidWalkEcosystem}

// WalkScan runs the single-pass walk engine over root with the tables of the
// given ecosystems activated. Ecosystems without a walk table are ignored.
func WalkScan(ctx context.Context, root string, ecos ...model.Ecosystem) ([]model.ScanResult, error) {
	set := make(map[model.Ecosystem]bool, len(ecos))
	for _, e := range ecos {
		set[e] = true
	}

	var tables []walkEcosystem
	for _, t := range walkEcosystemTable {
		if set[t.Eco] {
			tables = append(tables, t)
		}
	}
	return runWalk(ctx, root, tables)
}

// walkScanner adapts one walkEcosystem table to the Scanner interface so it
// can be registered in the Registry next to stat-based scanners. The
// registry batches all walkScanners of a scan into a single filesystem pass.
type walkScanner struct{ table walkEcosystem }

func newWalkScanner(table walkEcosystem) *walkScanner { return &walkScanner{table: table} }

func (w *walkScanner) Name() string               { return w.table.Name }
func (w *walkScanner) Ecosystem() model.Ecosystem { return w.table.Eco }

func (w *walkScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	return runWalk(ctx, root, []walkEcosystem{w.table})
}

// projectContext is one entry of the walk's active-project stack: a detected
// project root plus the artifact rules applying beneath it.
type projectContext struct {
	root     string
	tableIdx int // index into the tables slice passed to runWalk
	rules    []artifactRule
}

// runWalk walks root once and dispatches every directory against the given
// ecosystem tables. Per directory: artifact rules are matched against the
// nearest enclosing project root of each ecosystem (in table order, first
// match wins); on a match the artifact is emitted and its subtree skipped;
// otherwise marker files may establish new project contexts before
// descending. Results are sorted by (table order, path) before returning.
func runWalk(ctx context.Context, root string, tables []walkEcosystem) ([]model.ScanResult, error) {
	if len(tables) == 0 {
		return nil, nil
	}

	// Hidden directories below the root are only descended into when an
	// active ecosystem lists the name as a single-segment or any-depth artifact
	// (e.g. ".next", ".venv"). Artifact matching happens before this check, so
	// compound rules ending in a hidden segment (android/.gradle) are unaffected.
	hiddenDescend := make(map[string]bool)
	for _, t := range tables {
		for _, r := range t.Rules {
			switch {
			case r.Name != "":
				hiddenDescend[r.Name] = true
			case r.RelPath != "" && !strings.Contains(r.RelPath, "/"):
				hiddenDescend[r.RelPath] = true
			}
		}
	}

	var mu sync.Mutex // guards results
	var results []model.ScanResult
	numTables := len(tables)
	items := scanItems(ctx)

	// fstree does the traversal, in parallel while walkTokens has a free slot;
	// the engine only decides what a directory is. Each directory hands its
	// subdirectories the ancestors' project contexts plus its own. Results are
	// sorted below, so their order does not depend on the split.
	walker := fstree.Walker[[]projectContext]{
		FollowRoot: true,
		Slots:      walkTokens,
		// Artifact match first, before the hidden-dir check, against the
		// ancestor contexts (dir's own context is added only if we descend).
		// Size is filled in afterwards by sizePending.
		Enter: func(dir string, info fs.FileInfo, stack []projectContext) bool {
			// A limited scan skips every directory that is neither an item nor
			// on the way to one. The rest go through the same checks, with
			// every entry read, as in an unlimited scan.
			if items != nil && !leadsToItem(dir, items) {
				return false
			}
			name := filepath.Base(dir)
			if rule, tableIdx, projRoot, ok := matchArtifact(stack, dir, name, numTables); ok {
				result := model.ScanResult{
					Path:      dir,
					Ecosystem: tables[tableIdx].Eco,
					Category:  rule.Category,
					LastMod:   info.ModTime(),
					Safety:    rule.Safety,
				}
				if tables[tableIdx].SetProjectRoot {
					result.ProjectRoot = projRoot
				}
				if rule.Describe != nil {
					n := rule.Describe(dir)
					result.Recommendation = n.Recommendation
					result.CloneAware, result.CloneSource = n.CloneAware, n.CloneSource
				}
				mu.Lock()
				results = append(results, result)
				ReportProgress(ctx, len(results))
				mu.Unlock()
				return false // matched artifact: do not descend
			}
			// After artifact matching, so a project's own node_modules is
			// still reported. Also for the root: a scan of an installed tree
			// reports nothing inside it.
			if installedPackageDirNames[name] {
				return false
			}
			// The root is always entered: the hidden-dir rule applies below it.
			return dir == root || !strings.HasPrefix(name, ".") || hiddenDescend[name]
		},
		Entries: func(dir string, entries []fs.DirEntry, stack []projectContext) ([]projectContext, bool) {
			names := make(map[string]bool, len(entries))
			for _, e := range entries {
				names[e.Name()] = true
			}

			// Prune before establishing any context or descending, so nothing
			// inside is ever a deletion target.
			if isInstalledPackageTree(dir, names) {
				return nil, false
			}
			for i := range tables {
				if p := tables[i].PruneRoot; p != nil && p(dir, names) {
					return nil, false
				}
			}
			// Clip so siblings walked in other goroutines never append
			// into one shared array.
			stack = slices.Clip(stack)

			// Marker check: establish the project contexts rooted at this directory.
			for i := range tables {
				t := &tables[i]
				rooted := false
				for _, m := range t.Markers {
					if names[m] {
						rooted = true
						break
					}
				}
				if !rooted {
					continue
				}

				rules := t.Rules
				if t.ExtraRules != nil {
					if extra := t.ExtraRules(dir, names); len(extra) > 0 {
						combined := make([]artifactRule, 0, len(t.Rules)+len(extra))
						combined = append(combined, t.Rules...)
						combined = append(combined, extra...)
						rules = combined
					}
				}
				stack = append(stack, projectContext{root: dir, tableIdx: i, rules: rules})
			}
			return stack, true
		},
	}
	if _, err := walker.Walk(ctx, root); err != nil {
		return nil, err
	}

	if err := sizePending(ctx, results); err != nil {
		return nil, err
	}

	ecoOrder := make(map[model.Ecosystem]int, len(tables))
	for i, t := range tables {
		if _, ok := ecoOrder[t.Eco]; !ok {
			ecoOrder[t.Eco] = i
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if ecoOrder[results[i].Ecosystem] != ecoOrder[results[j].Ecosystem] {
			return ecoOrder[results[i].Ecosystem] < ecoOrder[results[j].Ecosystem]
		}
		return results[i].Path < results[j].Path
	})
	return results, nil
}

// walkTokens bounds the extra goroutines runWalk starts to walk subtrees in
// parallel. It is separate from sizeTokens so the walk is not starved while
// the stat scanners size their caches alongside it.
var walkTokens = make(chan struct{}, walkSlots)

// walkSlots is the number of walkTokens.
const walkSlots = 8

// installedPackageDirNames hold only installed packages when the walk reaches
// them without matching an artifact rule (a global lib/node_modules, a tool's
// bundled dependencies, a Python environment's site-packages). Enter prunes
// them by name, so they are never read.
var installedPackageDirNames = map[string]bool{
	"node_modules":  true,
	"site-packages": true,
	"dist-packages": true,
}

// isInstalledPackageTree reports whether a directory roots a tree of installed
// packages identified by its entries:
//
//   - pnpm store version dir: v<N> with files/ and index/ (v10) or index.db
//     (v11). Deleting v11's links/ corrupts the store.
//   - macOS app bundle: *.app with Contents/.
//   - Go module cache: mod/ with cache/download/.
func isInstalledPackageTree(dir string, names map[string]bool) bool {
	name := filepath.Base(dir)
	switch {
	case strings.HasSuffix(name, ".app") && names["Contents"]:
		return true
	case name == "mod" && names["cache"]:
		return isDir(filepath.Join(dir, "cache", "download"))
	case isPnpmStoreVersionDir(name) && names["files"] && (names["index"] || names["index.db"]):
		return true
	}
	return false
}

func isPnpmStoreVersionDir(name string) bool {
	if len(name) < 2 || name[0] != 'v' {
		return false
	}
	for _, c := range name[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// leadsToItem reports whether dir is one of items or an ancestor of one.
func leadsToItem(dir string, items map[string]bool) bool {
	for it := range items {
		if isUnderRoot(it, dir) {
			return true
		}
	}
	return false
}

// sizeWorkerCap bounds concurrent sizing walks so a large scan does not
// traverse every artifact tree at once. sizeTokens enforces it across the
// pools of all scanners and the sub-walks inside one artifact.
const sizeWorkerCap = 8

// sizePending fills in the size fields of every result by running
// Measure concurrently across a bounded worker pool. The walk defers sizing
// (one in-process tree walk each) to here so the traversals and their I/O
// overlap. Returns ctx.Err() if the scan is cancelled mid-sizing.
func sizePending(ctx context.Context, results []model.ScanResult) error {
	if len(results) == 0 {
		return nil
	}
	// sizeTokens bounds the concurrent measures; the workers only hand them out.
	workers := min(sizeWorkerCap, len(results))

	// A limited scan (WithItems) sizes only the items.
	items := scanItems(ctx)
	sources := newCloneSources()
	idx := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range idx {
				r := &results[i]
				if items != nil && !items[r.Path] {
					continue
				}
				var clones *cloneSources
				if r.CloneAware && cloneSizingSupported {
					clones = sources
				}
				select {
				case sizeTokens <- struct{}{}:
				case <-ctx.Done():
					continue
				}
				st := measure(ctx, r.Path, clones, r.CloneSource)
				<-sizeTokens
				r.Size = st.Reclaim
				r.ApparentSize = st.Apparent
				r.Links = st.Links
				r.CloneShares = st.CloneShares
				if st.Reclaim < st.Disk {
					r.AllocatedSize = st.Disk
				}
			}
		})
	}

	var canceled bool
	for i := range results {
		// Check cancellation first: a bare select would pick randomly between
		// ctx.Done() and a ready worker, so tiny scans could finish without
		// ever observing an already-cancelled context.
		if ctx.Err() != nil {
			canceled = true
			break
		}
		select {
		case <-ctx.Done():
			canceled = true
		case idx <- i:
			continue
		}
		break
	}
	close(idx)
	wg.Wait()
	if canceled {
		return ctx.Err()
	}
	return nil
}

// matchArtifact matches a directory (path, base name) against the artifact
// rules of the active project contexts. A rule only matches relative to the
// nearest (deepest) project root of its own ecosystem; when several
// ecosystems match the same directory, the first one in table order wins.
// Returns the matched rule, its table index, and the matching project root.
func matchArtifact(stack []projectContext, path, name string, numTables int) (artifactRule, int, string, bool) {
	for tableIdx := range numTables {
		// Nearest (deepest) project root of this ecosystem.
		for _, pc := range slices.Backward(stack) {
			if pc.tableIdx != tableIdx {
				continue
			}
			rel, err := filepath.Rel(pc.root, path)
			if err != nil {
				break
			}
			rel = filepath.ToSlash(rel)
			for _, rule := range pc.rules {
				if rule.matches(rel, name) {
					return rule, tableIdx, pc.root, true
				}
			}
			break // only the nearest root of this ecosystem is considered
		}
	}
	return artifactRule{}, 0, "", false
}

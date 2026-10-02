package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"runtime"
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

	var results []model.ScanResult
	var stack []projectContext
	numTables := len(tables)

	// fstree does the traversal; the engine only decides what a directory is.
	walker := fstree.Walker{
		FollowRoot: true,
		// Artifact match first, before the hidden-dir check, against the
		// ancestor contexts (dir's own context is pushed only if we descend).
		// Size is filled in afterwards by sizePending.
		Enter: func(dir string, info fs.FileInfo) bool {
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
				results = append(results, result)
				ReportProgress(ctx, len(results))
				return false // matched artifact: do not descend
			}
			// The root is always entered: the hidden-dir rule applies below it.
			return dir == root || !strings.HasPrefix(name, ".") || hiddenDescend[name]
		},
		Entries: func(dir string, entries []fs.DirEntry) bool {
			names := make(map[string]bool, len(entries))
			for _, e := range entries {
				names[e.Name()] = true
			}

			// Prune before establishing any context or descending, so nothing
			// inside is ever a deletion target.
			if isInstalledPackageTree(filepath.Base(dir), names) {
				return false
			}
			for i := range tables {
				if p := tables[i].PruneRoot; p != nil && p(dir, names) {
					return false
				}
			}

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
			return true
		},
		// The contexts dir pushed are the ones on top rooted at dir.
		Leave: func(dir string) {
			for len(stack) > 0 && stack[len(stack)-1].root == dir {
				stack = stack[:len(stack)-1]
			}
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

// isInstalledPackageTree reports whether a directory roots a tree of installed
// packages, whose marker files belong to shipped packages, not projects.
// Checked for every ecosystem, so results don't depend on the --eco subset.
//
//   - pnpm store version dir: v<N> with files/ and index/ (v10) or index.db
//     (v11). v11's links/ unpacks packages whose dist/ would match; deleting
//     it corrupts the store every pnpm project hard-links from.
//   - macOS app bundle: *.app with Contents/ (e.g. Electron node_modules).
func isInstalledPackageTree(name string, names map[string]bool) bool {
	if strings.HasSuffix(name, ".app") && names["Contents"] {
		return true
	}
	return isPnpmStoreVersionDir(name) && names["files"] && (names["index"] || names["index.db"])
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

// sizeWorkerCap bounds concurrent sizing walks so a large scan does not
// traverse every artifact tree at once.
const sizeWorkerCap = 8

// sizePending fills in the size fields of every result by running
// Measure concurrently across a bounded worker pool. The walk finds artifacts
// serially but defers their sizing (one in-process tree walk each) to here so
// the traversals and their I/O overlap. Returns ctx.Err() if the scan is
// cancelled mid-sizing.
func sizePending(ctx context.Context, results []model.ScanResult) error {
	workers := min(runtime.NumCPU(), sizeWorkerCap)
	return sizePendingWorkers(ctx, results, workers)
}

// sizePendingWorkers is sizePending with an explicit pool size, split out so
// the worker count can be swept in benchmarks.
func sizePendingWorkers(ctx context.Context, results []model.ScanResult, workers int) error {
	if len(results) == 0 {
		return nil
	}
	if workers > len(results) {
		workers = len(results)
	}
	if workers < 1 {
		workers = 1
	}

	sources := newCloneSources()
	idx := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range idx {
				r := &results[i]
				var clones *cloneSources
				if r.CloneAware && cloneSizingSupported {
					clones = sources
				}
				st := measure(ctx, r.Path, clones, r.CloneSource)
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

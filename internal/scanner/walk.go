package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

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
}

// walkEcosystemTable is the canonical, ordered table of walk-based
// ecosystems. The order decides attribution when a directory matches rules
// of several ecosystems (first match wins) and the result ordering; it
// mirrors the registry order.
var walkEcosystemTable = []walkEcosystem{nodeWalkEcosystem, rustWalkEcosystem, rubyWalkEcosystem, pythonWalkEcosystem, goWalkEcosystem}

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

// walkReadDir is os.ReadDir, indirected so a test can assert every directory is
// read exactly once — the single-read property is the whole point of the
// engine, and a reintroduced double-read is otherwise invisible to output.
var walkReadDir = os.ReadDir

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

	// Hidden directories are only descended into when an active ecosystem
	// lists the name as a single-segment or any-depth artifact (e.g. ".next",
	// ".venv"). Artifact matching happens before this check, so compound
	// rules ending in a hidden segment (android/.gradle) are unaffected.
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

	// visit reads each directory exactly once and reuses its entries for both
	// marker detection and recursion. The previous filepath.WalkDir version
	// read every directory twice (WalkDir's own read to recurse, plus a second
	// os.ReadDir here for markers) — the dominant cost on large trees. name is
	// dir's base name, checked against the enclosing ecosystem rules; the stack
	// holds the ancestor project contexts, scoped by the recursion itself.
	var visit func(dir, name string) error
	visit = func(dir, name string) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Artifact match first, before the hidden-dir check, against the
		// ancestor contexts (dir's own context is pushed only if we descend).
		// Size is filled in afterwards by sizePending.
		if rule, tableIdx, projRoot, ok := matchArtifact(stack, dir, name, numTables); ok {
			result := model.ScanResult{
				Path:      dir,
				Ecosystem: tables[tableIdx].Eco,
				Category:  rule.Category,
				LastMod:   ModTime(dir),
				Safety:    rule.Safety,
			}
			if tables[tableIdx].SetProjectRoot {
				result.ProjectRoot = projRoot
			}
			results = append(results, result)
			ReportProgress(ctx, len(results))
			return nil // matched artifact: do not descend
		}

		if strings.HasPrefix(name, ".") && !hiddenDescend[name] {
			return nil
		}

		entries, err := walkReadDir(dir)
		if err != nil {
			return nil // unreadable dir is skipped, like the legacy scanners
		}
		names := make(map[string]bool, len(entries))
		for _, e := range entries {
			names[e.Name()] = true
		}

		// Marker check: establish the project contexts rooted at this directory.
		pushed := 0
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
			pushed++
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if err := visit(filepath.Join(dir, e.Name()), e.Name()); err != nil {
				stack = stack[:len(stack)-pushed]
				return err
			}
		}
		stack = stack[:len(stack)-pushed]
		return nil
	}

	if err := visit(root, filepath.Base(root)); err != nil {
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

// sizeWorkerCap bounds concurrent du forks so a large scan does not spawn a
// subprocess per artifact all at once.
const sizeWorkerCap = 8

// sizePending fills in Size for every result by running DirSize concurrently
// across a bounded worker pool. The walk finds artifacts serially but defers
// their sizing (one du fork each) to here so the forks and their I/O overlap.
// du is faster per artifact than an in-process traversal, so the speedup comes
// from overlapping the sizing, not from replacing du. Returns ctx.Err() if the
// scan is cancelled mid-sizing.
func sizePending(ctx context.Context, results []model.ScanResult) error {
	workers := runtime.NumCPU()
	if workers > sizeWorkerCap {
		workers = sizeWorkerCap
	}
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

	idx := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				results[i].Size = DirSize(results[i].Path)
			}
		}()
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
		for i := len(stack) - 1; i >= 0; i-- {
			pc := stack[i]
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

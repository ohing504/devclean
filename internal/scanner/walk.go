package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// artifactRule describes one cleanable artifact matched by the walk engine.
// RelPath is the exact, "/"-separated path of the artifact relative to the
// nearest enclosing project root of its ecosystem: "node_modules" matches a
// direct child of a project root, "ios/Pods" matches one level deeper.
type artifactRule struct {
	RelPath  string
	Category model.Category
	Safety   model.SafetyLevel
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
}

// walkEcosystemTable is the canonical, ordered table of walk-based
// ecosystems. The order decides attribution when a directory matches rules
// of several ecosystems (first match wins) and the result ordering; it
// mirrors the registry order.
var walkEcosystemTable = []walkEcosystem{nodeWalkEcosystem, rustWalkEcosystem, rubyWalkEcosystem, goWalkEcosystem}

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

	// Hidden directories are only descended into when an active ecosystem
	// lists the name as a single-segment artifact (e.g. ".next"). Artifact
	// matching happens before this check, so compound rules ending in a
	// hidden segment (android/.gradle) are unaffected.
	hiddenDescend := make(map[string]bool)
	for _, t := range tables {
		for _, r := range t.Rules {
			if !strings.Contains(r.RelPath, "/") {
				hiddenDescend[r.RelPath] = true
			}
		}
	}

	var results []model.ScanResult
	var stack []projectContext

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if walkErr != nil {
			return nil // unreadable entries are skipped, like the legacy scanners
		}
		if !d.IsDir() {
			return nil
		}

		// Drop contexts of subtrees we have left.
		for len(stack) > 0 && !isUnderRoot(path, stack[len(stack)-1].root) {
			stack = stack[:len(stack)-1]
		}

		// Artifact match first, before the hidden-dir check.
		if rule, tableIdx, ok := matchArtifact(stack, path, len(tables)); ok {
			results = append(results, model.ScanResult{
				Path:      path,
				Ecosystem: tables[tableIdx].Eco,
				Category:  rule.Category,
				Size:      DirSize(path),
				LastMod:   ModTime(path),
				Safety:    rule.Safety,
			})
			ReportProgress(ctx, len(results))
			return fs.SkipDir
		}

		if name := d.Name(); strings.HasPrefix(name, ".") && !hiddenDescend[name] {
			return fs.SkipDir
		}

		// Marker check: one ReadDir snapshot decides which ecosystems root a
		// project at this directory.
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return nil
		}
		names := make(map[string]bool, len(entries))
		for _, e := range entries {
			names[e.Name()] = true
		}

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
				if extra := t.ExtraRules(path, names); len(extra) > 0 {
					combined := make([]artifactRule, 0, len(t.Rules)+len(extra))
					combined = append(combined, t.Rules...)
					combined = append(combined, extra...)
					rules = combined
				}
			}
			stack = append(stack, projectContext{root: path, tableIdx: i, rules: rules})
		}
		return nil
	})

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
	return results, err
}

// matchArtifact matches path against the artifact rules of the active
// project contexts. A rule only matches relative to the nearest (deepest)
// project root of its own ecosystem; when several ecosystems match the same
// directory, the first one in table order wins.
func matchArtifact(stack []projectContext, path string, numTables int) (artifactRule, int, bool) {
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
				if rule.RelPath == rel {
					return rule, tableIdx, true
				}
			}
			break // only the nearest root of this ecosystem is considered
		}
	}
	return artifactRule{}, 0, false
}

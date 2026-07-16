package ui

// White-box tests: selection helpers live on the unexported treeModel, so this
// file uses the internal package instead of the usual ui_test convention.

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

// newSelectorFixture builds a model with one protected project (dirty repo:
// one tracked artifact plus one gitignored-but-safe artifact) and one
// unprotected project with a safe artifact.
func newSelectorFixture(t *testing.T) treeModel {
	t.Helper()
	results := []model.ScanResult{
		{
			Path:        "/proj/dirty/build",
			Ecosystem:   model.EcoNode,
			Size:        100,
			Safety:      model.SafetyProtected,
			Protected:   true,
			ProjectRoot: "/proj/dirty",
		},
		{
			Path:        "/proj/dirty/node_modules",
			Ecosystem:   model.EcoNode,
			Size:        50,
			Safety:      model.SafetySafe,
			ProjectRoot: "/proj/dirty",
		},
		{
			Path:        "/proj/clean/node_modules",
			Ecosystem:   model.EcoNode,
			Size:        200,
			Safety:      model.SafetySafe,
			ProjectRoot: "/proj/clean",
		},
	}
	items := BuildTreeItems(results)
	if len(items) == 0 {
		t.Fatal("BuildTreeItems returned no items")
	}
	return treeModel{items: items}
}

// selectedPaths returns the paths of selected artifact rows.
func selectedPaths(m treeModel) map[string]bool {
	out := make(map[string]bool)
	for _, it := range m.items {
		if it.Type == ItemArtifact && it.Selected && it.Result != nil {
			out[it.Result.Path] = true
		}
	}
	return out
}

func TestSelectAllSkipsProtectedProjects(t *testing.T) {
	m := newSelectorFixture(t)
	m.selectAll()

	var protectedSeen, unprotectedSeen bool
	for _, it := range m.items {
		if it.Type != ItemArtifact {
			continue
		}
		if m.items[it.Parent].Protected {
			protectedSeen = true
			if it.Selected {
				t.Errorf("selectAll selected %q under a protected project", it.Result.Path)
			}
		} else {
			unprotectedSeen = true
			if !it.Selected {
				t.Errorf("selectAll skipped %q under an unprotected project", it.Result.Path)
			}
		}
	}
	if !protectedSeen || !unprotectedSeen {
		t.Fatal("fixture must contain artifacts under both protected and unprotected projects")
	}

	// The protected project row must not end up checked either.
	for i, it := range m.items {
		if it.Type == ItemProject && it.Protected && it.Selected {
			t.Errorf("selectAll marked protected project row %d as selected", i)
		}
	}
}

func TestSelectAllConsistentWithSelectBySafety(t *testing.T) {
	// All non-protected artifacts in the fixture are safe, so both helpers
	// must produce the same selection: protection is checked on the parent.
	a := newSelectorFixture(t)
	a.selectAll()

	b := newSelectorFixture(t)
	b.selectBySafety(model.SafetySafe)

	got, want := selectedPaths(a), selectedPaths(b)
	if len(got) == 0 {
		t.Fatal("selectAll selected nothing")
	}
	if len(got) != len(want) {
		t.Fatalf("selectAll picked %d artifacts, selectBySafety picked %d", len(got), len(want))
	}
	for p := range want {
		if !got[p] {
			t.Errorf("selectBySafety selected %q but selectAll did not", p)
		}
	}
}

package ui

// White-box tests: selection helpers live on the unexported treeModel, so this
// file uses the internal package instead of the usual ui_test convention.

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// newRichFixture builds two unprotected projects: an active "app" with two
// artifacts (for partial-selection tests) and an entirely-dormant "old" with
// one. Sizes make app sort first, giving a stable layout:
// [0]header [1]app [2]node_modules [3].next [4]old [5]cache.
func newRichFixture(t *testing.T) treeModel {
	t.Helper()
	// GroupByProject derives a project's Activity from its most recent item, so
	// the results carry non-zero LastMod (the newest wins per project).
	ts := time.Unix(1_700_000_000, 0)
	results := []model.ScanResult{
		{Path: "/app/node_modules", Ecosystem: model.EcoNode, Size: 200, Safety: model.SafetySafe, Activity: model.StatusActive, LastMod: ts, ProjectRoot: "/app"},
		{Path: "/app/.next", Ecosystem: model.EcoNode, Size: 100, Safety: model.SafetySafe, Activity: model.StatusActive, LastMod: ts, ProjectRoot: "/app"},
		{Path: "/old/cache", Ecosystem: model.EcoNode, Size: 50, Safety: model.SafetySafe, Activity: model.StatusDormant, LastMod: ts, ProjectRoot: "/old"},
	}
	m := treeModel{items: BuildTreeItems(results)}
	if len(m.items) != 6 {
		t.Fatalf("expected 6 items, got %d: %+v", len(m.items), m.items)
	}
	return m
}

// firstProject returns the index of the first project row.
func firstProject(m treeModel) int {
	for i, it := range m.items {
		if it.Type == ItemProject {
			return i
		}
	}
	return -1
}

// protectedProject returns the index of the first protected project row.
func protectedProject(m treeModel) int {
	for i, it := range m.items {
		if it.Type == ItemProject && it.Protected {
			return i
		}
	}
	return -1
}

func TestToggleCurrent_ProjectPropagatesToChildren(t *testing.T) {
	m := newRichFixture(t)
	m.cursor = firstProject(m) // "app"

	m.toggleCurrent()
	proj := m.items[m.cursor]
	if !proj.Selected {
		t.Error("toggling a project should select it")
	}
	for _, ci := range proj.Children {
		if !m.items[ci].Selected {
			t.Errorf("child %d should follow the project into selection", ci)
		}
	}

	m.toggleCurrent()
	if m.items[m.cursor].Selected {
		t.Error("toggling again should deselect the project")
	}
	for _, ci := range m.items[m.cursor].Children {
		if m.items[ci].Selected {
			t.Errorf("child %d should follow the project out of selection", ci)
		}
	}
}

func TestToggleCurrent_ProtectedProjectIgnored(t *testing.T) {
	m := newSelectorFixture(t)
	m.cursor = protectedProject(m)
	if m.cursor < 0 {
		t.Fatal("fixture must contain a protected project")
	}

	m.toggleCurrent()
	if m.items[m.cursor].Selected {
		t.Error("a protected project must not become selected")
	}
	for _, ci := range m.items[m.cursor].Children {
		if m.items[ci].Selected {
			t.Errorf("child %d of a protected project must not be selected", ci)
		}
	}
}

func TestToggleCurrent_ArtifactUpdatesParentState(t *testing.T) {
	m := newRichFixture(t)
	projIdx := firstProject(m)
	children := m.items[projIdx].Children
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}

	// Select one child: parent is partially selected, not fully.
	m.cursor = children[0]
	m.toggleCurrent()
	if m.items[projIdx].Selected {
		t.Error("parent should not be fully selected with one child unselected")
	}
	if !m.isPartiallySelected(projIdx) {
		t.Error("parent should be partially selected with one of two children on")
	}

	// Select the second: parent becomes fully selected, no longer partial.
	m.cursor = children[1]
	m.toggleCurrent()
	if !m.items[projIdx].Selected {
		t.Error("parent should be fully selected once all children are on")
	}
	if m.isPartiallySelected(projIdx) {
		t.Error("parent should not be partial once all children are on")
	}
}

func TestSelectNone_ClearsEverything(t *testing.T) {
	m := newRichFixture(t)
	m.selectAll()
	if len(selectedPaths(m)) == 0 {
		t.Fatal("selectAll selected nothing")
	}

	m.selectNone()
	if got := len(selectedPaths(m)); got != 0 {
		t.Errorf("selectNone left %d artifacts selected", got)
	}
	for i, it := range m.items {
		if it.Type == ItemProject && it.Selected {
			t.Errorf("selectNone left project row %d selected", i)
		}
	}
}

func TestSelectByActivity_PicksOnlyMatching(t *testing.T) {
	m := newRichFixture(t)
	m.selectByActivity(model.StatusDormant)

	// selectByActivity works at the project level: the entirely-dormant "old"
	// project and its child are selected; the active "app" is left alone.
	got := selectedPaths(m)
	if !got["/old/cache"] {
		t.Error("the dormant project's artifact should be selected")
	}
	if got["/app/node_modules"] || got["/app/.next"] {
		t.Error("artifacts of an active project should not be selected by a dormant query")
	}
}

func TestMoveCursor_SkipsHeadersAndClamps(t *testing.T) {
	m := newRichFixture(t)
	m.cursor = 0 // eco header

	// Moving down from the header must land on a selectable row, never a header.
	m.moveCursor(1)
	if m.items[m.cursor].Type == ItemEcoHeader {
		t.Errorf("moveCursor down landed on a header at %d", m.cursor)
	}

	// Moving down past the end clamps to the last index.
	for range len(m.items) + 2 {
		m.moveCursor(1)
	}
	if m.cursor != len(m.items)-1 {
		t.Errorf("cursor = %d after clamping down, want %d", m.cursor, len(m.items)-1)
	}

	// Moving up past the start clamps to 0.
	for range len(m.items) + 2 {
		m.moveCursor(-1)
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d after clamping up, want 0", m.cursor)
	}
}

func TestJumpProject_MovesBetweenProjects(t *testing.T) {
	m := newRichFixture(t)
	first := firstProject(m)
	m.cursor = first

	m.jumpProject(1)
	if m.items[m.cursor].Type != ItemProject || m.cursor == first {
		t.Errorf("jumpProject(1) did not advance to the next project, cursor=%d", m.cursor)
	}
	second := m.cursor

	// No project below the last one: cursor stays put.
	m.jumpProject(1)
	if m.cursor != second {
		t.Errorf("jumpProject(1) past the last project moved cursor to %d, want %d", m.cursor, second)
	}

	m.jumpProject(-1)
	if m.cursor != first {
		t.Errorf("jumpProject(-1) returned to %d, want %d", m.cursor, first)
	}
}

func TestUpdate_KeyRouting(t *testing.T) {
	rune := func(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

	// space toggles the current project.
	m := newRichFixture(t)
	m.cursor = firstProject(m)
	nm, _ := m.Update(rune(' '))
	if !nm.(treeModel).items[m.cursor].Selected {
		t.Error("space should toggle the current project on")
	}

	// "a" selects all, "n" clears.
	nm, _ = nm.(treeModel).Update(rune('n'))
	if len(selectedPaths(nm.(treeModel))) != 0 {
		t.Error("n should clear the selection")
	}
	nm, _ = nm.(treeModel).Update(rune('a'))
	if len(selectedPaths(nm.(treeModel))) == 0 {
		t.Error("a should select all selectable artifacts")
	}

	// esc aborts and quits.
	aborted, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !aborted.(treeModel).aborted {
		t.Error("esc should set aborted")
	}
	if cmd == nil {
		t.Error("esc should return a quit command")
	}
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

// newManyArtifactsFixture builds a single unprotected project with n
// artifacts — enough rows to overflow a small terminal window.
func newManyArtifactsFixture(t *testing.T, n int) treeModel {
	t.Helper()
	results := make([]model.ScanResult, n)
	for i := range results {
		results[i] = model.ScanResult{
			Path:        "/proj/many/artifact" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Ecosystem:   model.EcoNode,
			Size:        int64(i + 1),
			Safety:      model.SafetySafe,
			ProjectRoot: "/proj/many",
		}
	}
	items := BuildTreeItems(results)
	if len(items) == 0 {
		t.Fatal("BuildTreeItems returned no items")
	}
	return treeModel{items: items}
}

// TestCursorStaysWithinViewport reproduces the reported bug: with more rows
// than fit on screen, moving the cursor down must scroll the viewport so the
// highlighted row is always inside [YOffset, YOffset+Height). Before the
// viewport-backed rendering was added, View() dumped every row unconditionally
// and the cursor could scroll off the visible terminal region entirely.
func TestCursorStaysWithinViewport(t *testing.T) {
	m := newManyArtifactsFixture(t, 40)

	// Small window: forces scrolling almost immediately.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 17})
	m = updated.(treeModel)
	if !m.ready {
		t.Fatal("model did not become ready after WindowSizeMsg")
	}
	if m.viewport.Height != 10 { // 17 - chromeLines(7)
		t.Fatalf("viewport height = %d, want 10", m.viewport.Height)
	}

	assertCursorVisible := func(t *testing.T, m treeModel) {
		t.Helper()
		_, starts, counts := m.layoutItems()
		top := starts[m.cursor]
		bottom := top + counts[m.cursor] - 1
		if top < m.viewport.YOffset || bottom > m.viewport.YOffset+m.viewport.Height-1 {
			t.Fatalf("cursor row [%d,%d] outside visible window [%d,%d]",
				top, bottom, m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height-1)
		}
	}

	sawScroll := false
	for i := 0; i < 39; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(treeModel)
		assertCursorVisible(t, m)
		if m.viewport.YOffset > 0 {
			sawScroll = true
		}
	}
	if !sawScroll {
		t.Fatal("viewport never scrolled despite cursor moving past the visible window")
	}

	// Scrolling back up must also keep the cursor visible, down to the top.
	for i := 0; i < 39; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = updated.(treeModel)
		assertCursorVisible(t, m)
	}
	if m.viewport.YOffset != 0 {
		t.Fatalf("YOffset = %d after scrolling back to the first row, want 0", m.viewport.YOffset)
	}
}

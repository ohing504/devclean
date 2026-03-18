package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/pathutil"
)

// ItemType distinguishes rows in the tree selector.
type ItemType int

const (
	ItemEcoHeader ItemType = iota
	ItemProject
	ItemArtifact
)

// TreeItem represents a single row in the tree selector.
type TreeItem struct {
	Type      ItemType
	Label     string
	Path      string
	Size      int64
	LastMod   time.Time
	Activity  model.ActivityStatus
	Protected bool
	Selected  bool
	Result    *model.ScanResult // non-nil for artifacts
	Children  []int             // for projects: indices of artifact children
	Parent    int               // for artifacts: index of parent project (-1 if none)
}

// TreeSelectorResult holds the selected scan results.
type TreeSelectorResult struct {
	Selected []model.ScanResult
	Aborted  bool
}

// treeModel is the bubbletea model for tree selection.
type treeModel struct {
	items       []TreeItem
	cursor      int
	done        bool
	aborted     bool
	windowWidth int
}

// BuildTreeItems constructs the flat item list from grouped scan results.
func BuildTreeItems(results []model.ScanResult) []TreeItem {
	var items []TreeItem

	// Group by ecosystem
	ecoMap := make(map[model.Ecosystem][]model.ScanResult)
	var ecoOrder []model.Ecosystem
	for _, r := range results {
		if _, exists := ecoMap[r.Ecosystem]; !exists {
			ecoOrder = append(ecoOrder, r.Ecosystem)
		}
		ecoMap[r.Ecosystem] = append(ecoMap[r.Ecosystem], r)
	}

	for _, eco := range ecoOrder {
		ecoResults := ecoMap[eco]
		projects := model.GroupByProject(ecoResults)

		// Eco header
		var ecoSize int64
		for _, p := range projects {
			ecoSize += p.TotalSize
		}
		items = append(items, TreeItem{
			Type:  ItemEcoHeader,
			Label: fmt.Sprintf("● %s %d projects · %s", eco, len(projects), model.HumanSize(ecoSize)),
			Size:  ecoSize,
		})

		for _, p := range projects {
			projectIdx := len(items)
			projectItem := TreeItem{
				Type:      ItemProject,
				Label:     p.Name,
				Path:      pathutil.ShortenHome(p.Path),
				Size:      p.TotalSize,
				LastMod:   p.LastMod,
				Activity:  p.Activity,
				Protected: p.Protected,
				Selected:  false, // start with nothing selected
				Parent:    -1,
			}

			items = append(items, projectItem)
			artifactStartIdx := len(items)

			for _, r := range p.Items {
				r := r
				relPath := r.Path
				if p.Path != "" {
					if rel, err := relPathFromRoot(r.Path, p.Path); err == nil {
						relPath = rel
					}
				}
				items = append(items, TreeItem{
					Type:     ItemArtifact,
					Label:    relPath,
					Size:     r.Size,
					Selected: false,
					Result:   &r,
					Parent:   projectIdx,
				})
			}

			// Set children indices on the project
			var childIndices []int
			for i := artifactStartIdx; i < len(items); i++ {
				childIndices = append(childIndices, i)
			}
			items[projectIdx].Children = childIndices
		}
	}

	return items
}

func relPathFromRoot(path, root string) (string, error) {
	if !strings.HasPrefix(path, root) {
		return path, fmt.Errorf("not a subpath")
	}
	rel := path[len(root):]
	return strings.TrimPrefix(rel, "/"), nil
}

// RunTreeSelector runs the interactive tree selector and returns selected items.
func RunTreeSelector(results []model.ScanResult) TreeSelectorResult {
	items := BuildTreeItems(results)
	if len(items) == 0 {
		return TreeSelectorResult{Aborted: true}
	}

	// Move cursor to first selectable item
	cursor := 0
	for i, item := range items {
		if item.Type == ItemProject {
			cursor = i
			break
		}
	}

	m := treeModel{
		items:  items,
		cursor: cursor,
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return TreeSelectorResult{Aborted: true}
	}

	fm := finalModel.(treeModel)
	if fm.aborted {
		return TreeSelectorResult{Aborted: true}
	}

	var selected []model.ScanResult
	for _, item := range fm.items {
		if item.Type == ItemArtifact && item.Selected && item.Result != nil {
			selected = append(selected, *item.Result)
		}
	}

	return TreeSelectorResult{Selected: selected}
}

func (m treeModel) Init() tea.Cmd {
	return nil
}

func (m treeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
		case "left", "h":
			m.jumpProject(-1)
		case "right", "l":
			m.jumpProject(1)
		case " ":
			m.toggleCurrent()
		case "enter":
			m.done = true
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		case "a":
			m.selectAll()
		case "n":
			m.selectNone()
		case "s":
			m.selectBySafety(model.SafetySafe)
		case "d":
			m.selectByActivity(model.StatusDormant)
		}
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
	}
	return m, nil
}

func (m *treeModel) moveCursor(dir int) {
	for {
		m.cursor += dir
		if m.cursor < 0 {
			m.cursor = 0
			return
		}
		if m.cursor >= len(m.items) {
			m.cursor = len(m.items) - 1
			return
		}
		// Skip eco headers
		if m.items[m.cursor].Type != ItemEcoHeader {
			return
		}
	}
}

func (m *treeModel) jumpProject(dir int) {
	start := m.cursor
	for {
		m.cursor += dir
		if m.cursor < 0 {
			m.cursor = start
			return
		}
		if m.cursor >= len(m.items) {
			m.cursor = start
			return
		}
		if m.items[m.cursor].Type == ItemProject {
			return
		}
	}
}

func (m *treeModel) toggleCurrent() {
	item := &m.items[m.cursor]
	switch item.Type {
	case ItemProject:
		if item.Protected {
			return
		}
		newState := !item.Selected
		item.Selected = newState
		for _, ci := range item.Children {
			m.items[ci].Selected = newState
		}
	case ItemArtifact:
		parent := &m.items[item.Parent]
		if parent.Protected {
			return
		}
		item.Selected = !item.Selected
		m.updateProjectState(item.Parent)
	}
}

func (m *treeModel) updateProjectState(projectIdx int) {
	project := &m.items[projectIdx]
	allSelected := true
	anySelected := false
	for _, ci := range project.Children {
		if m.items[ci].Selected {
			anySelected = true
		} else {
			allSelected = false
		}
	}
	project.Selected = allSelected
	_ = anySelected // used for partial state rendering
}

func (m *treeModel) isPartiallySelected(projectIdx int) bool {
	project := &m.items[projectIdx]
	anySelected := false
	allSelected := true
	for _, ci := range project.Children {
		if m.items[ci].Selected {
			anySelected = true
		} else {
			allSelected = false
		}
	}
	return anySelected && !allSelected
}

func (m *treeModel) selectAll() {
	for i := range m.items {
		if !m.items[i].Protected {
			m.items[i].Selected = true
		}
	}
}

func (m *treeModel) selectNone() {
	for i := range m.items {
		if m.items[i].Type != ItemEcoHeader {
			m.items[i].Selected = false
		}
	}
}

func (m *treeModel) selectBySafety(safety model.SafetyLevel) {
	m.selectNone()
	for i := range m.items {
		if m.items[i].Type == ItemArtifact && m.items[i].Result != nil {
			if m.items[i].Result.Safety == safety && !m.items[m.items[i].Parent].Protected {
				m.items[i].Selected = true
			}
		}
	}
	// Update project states
	for i := range m.items {
		if m.items[i].Type == ItemProject {
			m.updateProjectState(i)
		}
	}
}

func (m *treeModel) selectByActivity(activity model.ActivityStatus) {
	m.selectNone()
	for i := range m.items {
		if m.items[i].Type == ItemProject && m.items[i].Activity == activity && !m.items[i].Protected {
			m.items[i].Selected = true
			for _, ci := range m.items[i].Children {
				m.items[ci].Selected = true
			}
		}
	}
}

func (m treeModel) View() string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s move  %s jump project  %s toggle  %s confirm  %s cancel",
		DimStyle.Render("[↑↓]"), DimStyle.Render("[←→]"), DimStyle.Render("[space]"), DimStyle.Render("[enter]"), DimStyle.Render("[esc]"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s all  %s none  %s safe only  %s dormant only",
		DimStyle.Render("[a]"), DimStyle.Render("[n]"), DimStyle.Render("[s]"), DimStyle.Render("[d]"))
	b.WriteString("\n\n")

	var selectedCount int
	var selectedSize int64

	for i, item := range m.items {
		if item.Type == ItemArtifact && item.Selected {
			selectedCount++
			selectedSize += item.Size
		}

		isCursor := i == m.cursor
		line := m.renderItem(i, item, isCursor)
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(InfoStyle.Render(fmt.Sprintf("Selected: %d items (%s)", selectedCount, model.HumanSize(selectedSize))))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "%s  %s safe  %s caution  %s protected   %s Active  %s Recent  %s Stale  %s Dormant\n",
		DimStyle.Render("Legend:"),
		SafeStyle.Render("✔"), CautionStyle.Render("⚠"), ProtectedStyle.Render("✖"),
		ActiveStyle.Render("●"), RecentStyle.Render("●"), StaleStyle.Render("●"), DormantStyle.Render("●"))

	return b.String()
}

func (m treeModel) renderItem(idx int, item TreeItem, isCursor bool) string {
	cursor := "  "
	if isCursor {
		cursor = SafeStyle.Render("> ")
	}

	switch item.Type {
	case ItemEcoHeader:
		return fmt.Sprintf("\n%s%s", cursor, HeaderStyle.Render(item.Label))

	case ItemProject:
		checkbox := m.projectCheckbox(idx, item)
		badge := statusBadge(item.Activity)
		protectedBadge := ""
		if item.Protected {
			protectedBadge = " " + ProtectedStyle.Render("Protected")
		}
		name := ProjectStyle.Render(item.Label)
		info := InfoStyle.Render(fmt.Sprintf("%s · %s", model.HumanSize(item.Size), relativeTime(item.LastMod)))
		path := DimStyle.Render(item.Path)
		return fmt.Sprintf("%s%s %s%s %s %s\n%s    %s",
			cursor, checkbox, name, protectedBadge, badge, info,
			"    ", path)

	case ItemArtifact:
		checkbox := "[ ]"
		if item.Selected {
			checkbox = SafeStyle.Render("[✔]")
		}
		parent := m.items[item.Parent]
		if parent.Protected {
			checkbox = ProtectedStyle.Render("[✖]")
		}
		safetyIcon := safetyIndicator(item.Result.Safety)
		cat := DimStyle.Render("(" + string(item.Result.Category) + ")")
		size := DimStyle.Render(model.HumanSize(item.Size))
		return fmt.Sprintf("%s    %s %s %s %s  %s", cursor, checkbox, safetyIcon, item.Label, cat, size)
	}

	return ""
}

func (m treeModel) projectCheckbox(idx int, item TreeItem) string {
	if item.Protected {
		return ProtectedStyle.Render("[✖]")
	}
	if item.Selected {
		return SafeStyle.Render("[✔]")
	}
	if m.isPartiallySelected(idx) {
		return CautionStyle.Render("[-]")
	}
	return "[ ]"
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "just now"
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case d < 365*24*time.Hour:
		months := int(d.Hours() / 24 / 30)
		if months <= 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(d.Hours() / 24 / 365)
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

func safetyIndicator(s model.SafetyLevel) string {
	switch s {
	case model.SafetySafe:
		return SafeStyle.Render("✔")
	case model.SafetyCaution:
		return CautionStyle.Render("⚠")
	case model.SafetyProtected:
		return ProtectedStyle.Render("✖")
	default:
		return " "
	}
}

func statusBadge(s model.ActivityStatus) string {
	switch s {
	case model.StatusActive:
		return ActiveStyle.Render("Active")
	case model.StatusRecent:
		return RecentStyle.Render("Recent")
	case model.StatusStale:
		return StaleStyle.Render("Stale")
	case model.StatusDormant:
		return DormantStyle.Render("Dormant")
	default:
		return ""
	}
}

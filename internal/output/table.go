package output

import (
	"cmp"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/ui"
)

// TableOptions controls table output behavior.
type TableOptions struct {
	TopN      int    // limit output to top N projects (0 = all)
	Verbose   bool   // show all artifacts including small ones
	SortBy    string // project order and top-N key: size (default), time, name
	Ascending bool   // smallest, oldest or A→Z project first
}

const collapseThreshold = 1024 * 1024 // 1 MB — hide directories and artifacts below this in default mode

// WriteTable writes scan results as a colored table grouped by ecosystem and project.
func WriteTable(w io.Writer, results []model.ScanResult) {
	WriteTableWithOptions(w, results, TableOptions{})
}

// WriteTableWithOptions writes scan results with configurable options.
func WriteTableWithOptions(w io.Writer, results []model.ScanResult, opts TableOptions) {
	if len(results) == 0 {
		fmt.Fprintln(w, "No items found.")
		return
	}

	projectOrder := model.CompareProjects(opts.SortBy, opts.Ascending)
	if opts.TopN > 0 {
		results = model.TopProjects(results, opts.TopN, projectOrder)
	}
	ecoGroups := groupByEcosystem(results)
	sortGroupsBySize(ecoGroups)

	var grandCount int
	var allItems []model.ScanResult

	for _, eg := range ecoGroups {
		grandCount += len(eg.items)
		allItems = append(allItems, eg.items...)

		projects := model.GroupByProject(eg.items)
		slices.SortFunc(projects, projectOrder)

		fmt.Fprintf(
			w, "\n%s %s\n",
			ui.HeaderStyle.Render(fmt.Sprintf("● %s", eg.ecosystem)),
			ui.DimStyle.Render(fmt.Sprintf("%d projects · %s", len(projects), model.HumanSize(eg.totalSize))),
		)

		for _, p := range projects {
			// Project header: path + status badge + size + relative time
			badge := ui.StatusBadge(p.Activity)
			protectedBadge := ""
			if p.Protected {
				protectedBadge = " " + ui.ProtectedStyle.Render("Protected")
			}

			fmt.Fprintf(
				w, "  %s%s %s %s\n",
				ui.ProjectStyle.Render(ui.ProjectName(p)),
				protectedBadge,
				badge,
				ui.InfoStyle.Render(fmt.Sprintf("%s · %s", model.HumanSize(p.TotalSize), ui.RelativeTime(p.LastMod))),
			)

			renderDirs(w, groupByDir(p.Items, p.Path), opts)
		}
	}

	// Totals count blocks shared across artifacts once (see DedupedTotal), so
	// the total can differ from the row sum either way.
	grandTotal := model.DedupedTotal(allItems)
	var naiveTotal int64
	for _, r := range allItems {
		naiveTotal += r.Size
	}
	safeItems := model.FilterResults(allItems, func(r model.ScanResult) bool {
		return r.Safety == model.SafetySafe
	})
	safeTotal := model.DedupedTotal(safeItems)

	fmt.Fprintf(w, "\n%s", ui.TotalStyle.Render(fmt.Sprintf("Total: %s (%d items)", model.HumanSize(grandTotal), grandCount)))
	if grandTotal != naiveTotal {
		fmt.Fprintf(w, " %s", ui.DimStyle.Render("(blocks shared across items counted once)"))
	}
	fmt.Fprintln(w)
	if safeTotal > 0 {
		fmt.Fprintf(
			w, "%s\n",
			ui.SafeStyle.Render(fmt.Sprintf("Safe to clean: %s", model.HumanSize(safeTotal))),
		)
	}

	// Legend
	fmt.Fprintf(
		w, "\n%s  %s safe  %s caution  %s protected   %s Active  %s Recent  %s Stale  %s Dormant\n",
		ui.DimStyle.Render("Legend:"),
		ui.SafeStyle.Render("✔"), ui.CautionStyle.Render("⚠"), ui.ProtectedStyle.Render("✖"),
		ui.ActiveStyle.Render("●"), ui.RecentStyle.Render("●"), ui.StaleStyle.Render("●"), ui.DormantStyle.Render("●"),
	)
	fmt.Fprintf(w, "        %s\n", ui.DimStyle.Render("Run 'devclean list' for details"))
}

// --- Ecosystem grouping ---

type ecoGroup struct {
	ecosystem string
	totalSize int64 // hard-linked blocks counted once (DedupedTotal)
	items     []model.ScanResult
}

func groupByEcosystem(results []model.ScanResult) []ecoGroup {
	m := make(map[model.Ecosystem]*ecoGroup)
	for _, r := range results {
		g, ok := m[r.Ecosystem]
		if !ok {
			g = &ecoGroup{ecosystem: string(r.Ecosystem)}
			m[r.Ecosystem] = g
		}
		g.items = append(g.items, r)
	}

	var groups []ecoGroup
	for _, g := range m {
		g.totalSize = model.DedupedTotal(g.items)
		groups = append(groups, *g)
	}
	return groups
}

func sortGroupsBySize(groups []ecoGroup) {
	slices.SortFunc(groups, func(a, b ecoGroup) int {
		return cmp.Or(cmp.Compare(b.totalSize, a.totalSize), strings.Compare(a.ecosystem, b.ecosystem))
	})
}

// --- Directory grouping ---

// artifactDir holds a project's artifacts that sit in one directory.
type artifactDir struct {
	name      string // relative to the project path ("." for the project path itself)
	totalSize int64  // hard-linked blocks counted once (DedupedTotal)
	items     []model.ScanResult
}

// groupByDir groups a project's artifacts by the directory holding each one,
// largest directory first and largest artifact first within each.
func groupByDir(items []model.ScanResult, projectRoot string) []artifactDir {
	m := make(map[string]*artifactDir)

	for _, r := range items {
		dir := filepath.Dir(artifactRelPath(r.Path, projectRoot))
		d, ok := m[dir]
		if !ok {
			d = &artifactDir{name: dir}
			m[dir] = d
		}
		d.items = append(d.items, r)
	}

	var result []artifactDir
	for _, d := range m {
		d.totalSize = model.DedupedTotal(d.items)
		slices.SortFunc(d.items, model.CompareResults(model.SortBySize, false))
		result = append(result, *d)
	}
	slices.SortFunc(result, func(a, b artifactDir) int {
		return cmp.Or(cmp.Compare(b.totalSize, a.totalSize), strings.Compare(a.name, b.name))
	})
	return result
}

// collapses reports whether default mode folds the i-th entry of a size-sorted
// list into its "... and N more" line. The first, the largest, always shows,
// so a list never consists of the collapsed line alone.
func collapses(opts TableOptions, i int, size int64) bool {
	return !opts.Verbose && i > 0 && size < collapseThreshold
}

// renderDirs lists a project's artifacts under one header per directory. A
// project whose artifacts all sit in the project path itself gets no header.
func renderDirs(w io.Writer, dirs []artifactDir, opts TableOptions) {
	if len(dirs) == 1 && dirs[0].name == "." {
		renderArtifacts(w, dirs[0].items, "    ", 30, opts)
		return
	}

	var collapsed []model.ScanResult
	var collapsedDirs int
	for i, d := range dirs {
		if collapses(opts, i, d.totalSize) {
			collapsedDirs++
			collapsed = append(collapsed, d.items...)
			continue
		}
		name := d.name
		if name == "." {
			name = ". (root)"
		}
		fmt.Fprintf(
			w, "    %s %s\n",
			ui.ProjectStyle.Render(name),
			ui.DimStyle.Render(fmt.Sprintf("(%s)", model.HumanSize(d.totalSize))),
		)
		renderArtifacts(w, d.items, "      ", 24, opts)
	}

	if collapsedDirs > 0 {
		noun := "directories"
		if collapsedDirs == 1 {
			noun = "directory"
		}
		fmt.Fprintf(
			w, "    %s\n",
			ui.DimStyle.Render(fmt.Sprintf("  ... and %d more %s (%s)", collapsedDirs, noun, model.HumanSize(model.DedupedTotal(collapsed)))),
		)
	}
}

// renderArtifacts lists the artifacts of one directory, each named relative to
// that directory, which the header above already shows.
func renderArtifacts(w io.Writer, items []model.ScanResult, indent string, nameWidth int, opts TableOptions) {
	var collapsed []model.ScanResult
	for i, r := range items {
		if collapses(opts, i, r.Size) {
			collapsed = append(collapsed, r)
			continue
		}
		name := ui.ArtifactName(r, filepath.Dir(r.Path))
		cat := ui.DimStyle.Render("(" + string(r.Category) + ")")
		fmt.Fprintf(
			w, "%s%s %-*s %10s%s%s\n",
			indent,
			ui.SafetyIcon(r.Safety),
			nameWidth, name+" "+cat,
			ui.InfoStyle.Render(ui.ArtifactSize(r)),
			ui.LastUsedTag(r),
			ui.RecommendationTag(r),
		)
	}

	if len(collapsed) > 0 {
		fmt.Fprintf(
			w, "%s%s\n",
			indent,
			ui.DimStyle.Render(fmt.Sprintf("  ... and %d more (%s)", len(collapsed), model.HumanSize(model.DedupedTotal(collapsed)))),
		)
	}
}

// artifactRelPath returns the path of an artifact relative to its project root.
// e.g., "/monorepo/apps/web/.next" with root "/monorepo" → "apps/web/.next"
func artifactRelPath(artifactPath, projectRoot string) string {
	rel, err := filepath.Rel(projectRoot, artifactPath)
	if err != nil {
		return filepath.Base(artifactPath)
	}
	return rel
}

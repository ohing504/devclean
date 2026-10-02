package output

import (
	"cmp"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/pathutil"
	"github.com/ohing504/devclean/internal/ui"
)

// TableOptions controls table output behavior.
type TableOptions struct {
	TopN      int    // limit output to top N projects (0 = all)
	Verbose   bool   // show all artifacts including small ones
	SortBy    string // project order and top-N key: size (default), time, name
	Ascending bool   // smallest, oldest or A→Z project first
}

const collapseThreshold = 1024 * 1024 // 1 MB — hide artifacts below this in default mode

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
		results = topProjects(results, opts.TopN, projectOrder)
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
			// Project header: name + status badge + size + relative time
			badge := ui.StatusBadge(p.Activity)
			protectedBadge := ""
			if p.Protected {
				protectedBadge = " " + ui.ProtectedStyle.Render("Protected")
			}

			fmt.Fprintf(
				w, "  %s%s %s %s\n",
				ui.ProjectStyle.Render(p.Name),
				protectedBadge,
				badge,
				ui.InfoStyle.Render(fmt.Sprintf("%s · %s", model.HumanSize(p.TotalSize), ui.RelativeTime(p.LastMod))),
			)

			// Project path
			fmt.Fprintf(w, "  %s\n", ui.DimStyle.Render(pathutil.ShortenHome(p.Path)))

			// Group artifacts by sub-package
			subPkgs := groupBySubPackage(p.Items, p.Path)
			renderSubPackages(w, subPkgs, opts)
		}
	}

	// Totals dedup blocks shared across artifacts via hard links (e.g. a pnpm
	// store blob also linked into node_modules), so the figures reflect space
	// actually freed rather than an inflated sum of overlapping artifacts.
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
	if grandTotal < naiveTotal {
		fmt.Fprintf(w, " %s", ui.DimStyle.Render("(excludes hard-linked blocks shared across items)"))
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

// topProjects keeps the artifacts of the first topN projects in order. A
// project is one project root even when its artifacts span several
// ecosystems; it is ranked by all of them together.
func topProjects(results []model.ScanResult, topN int, order func(a, b model.ProjectGroup) int) []model.ScanResult {
	projects := model.GroupByProject(results)
	if topN >= len(projects) {
		return results
	}
	slices.SortFunc(projects, order)
	kept := make(map[string]bool, topN)
	for _, p := range projects[:topN] {
		kept[p.Path] = true
	}
	return model.FilterResults(results, func(r model.ScanResult) bool {
		return kept[r.ProjectKey()]
	})
}

func sortGroupsBySize(groups []ecoGroup) {
	slices.SortFunc(groups, func(a, b ecoGroup) int {
		return cmp.Or(cmp.Compare(b.totalSize, a.totalSize), strings.Compare(a.ecosystem, b.ecosystem))
	})
}

// --- Sub-package grouping ---

type subPackage struct {
	name      string // relative dir from project root ("." for root)
	totalSize int64  // hard-linked blocks counted once (DedupedTotal)
	items     []model.ScanResult
}

func groupBySubPackage(items []model.ScanResult, projectRoot string) []subPackage {
	m := make(map[string]*subPackage)

	for _, r := range items {
		rel := artifactRelPath(r.Path, projectRoot)
		dir := filepath.Dir(rel)

		sp, ok := m[dir]
		if !ok {
			sp = &subPackage{name: dir}
			m[dir] = sp
		}
		sp.items = append(sp.items, r)
	}

	var result []subPackage
	for _, sp := range m {
		sp.totalSize = model.DedupedTotal(sp.items)
		slices.SortFunc(sp.items, model.CompareResults(model.SortBySize, false))
		result = append(result, *sp)
	}

	// Sort sub-packages by size desc
	slices.SortFunc(result, func(a, b subPackage) int {
		return cmp.Or(cmp.Compare(b.totalSize, a.totalSize), strings.Compare(a.name, b.name))
	})

	return result
}

func renderSubPackages(w io.Writer, subPkgs []subPackage, opts TableOptions) {
	// If only one sub-package (not a monorepo), render flat
	if len(subPkgs) == 1 && subPkgs[0].name == "." {
		renderArtifactsFlat(w, subPkgs[0].items, subPkgs[0].items[0].ProjectRoot, opts)
		return
	}

	var collapsedPkgs int
	var collapsedItems []model.ScanResult

	for _, sp := range subPkgs {
		// Collapse small sub-packages in default mode
		if !opts.Verbose && sp.totalSize < collapseThreshold && len(subPkgs) > 2 {
			collapsedPkgs++
			collapsedItems = append(collapsedItems, sp.items...)
			continue
		}

		// Sub-package header
		displayName := sp.name
		if displayName == "." {
			displayName = ". (root)"
		}
		fmt.Fprintf(
			w, "    %s %s\n",
			ui.ProjectStyle.Render(displayName),
			ui.DimStyle.Render(fmt.Sprintf("(%s)", model.HumanSize(sp.totalSize))),
		)

		// Artifacts in this sub-package
		for _, r := range sp.items {
			icon := ui.SafetyIcon(r.Safety)
			// The sub-package header already shows the directory, so the
			// artifact is named relative to it.
			name := ui.ArtifactName(r, filepath.Dir(r.Path))
			cat := ui.DimStyle.Render("(" + string(r.Category) + ")")
			rec := ui.RecommendationTag(r)
			fmt.Fprintf(
				w, "      %s %-24s %10s%s%s\n",
				icon,
				name+" "+cat,
				ui.InfoStyle.Render(ui.ArtifactSize(r)),
				ui.LastUsedTag(r),
				rec,
			)
		}
	}

	if collapsedPkgs > 0 {
		fmt.Fprintf(
			w, "    %s\n",
			ui.DimStyle.Render(fmt.Sprintf("  ... and %d more packages (%s)", collapsedPkgs, model.HumanSize(model.DedupedTotal(collapsedItems)))),
		)
	}
}

func renderArtifactsFlat(w io.Writer, items []model.ScanResult, projectRoot string, opts TableOptions) {
	var collapsed []model.ScanResult

	for _, r := range items {
		if !opts.Verbose && r.Size < collapseThreshold && len(items) > 1 {
			collapsed = append(collapsed, r)
			continue
		}

		icon := ui.SafetyIcon(r.Safety)
		name := ui.ArtifactName(r, projectRoot)
		cat := ui.DimStyle.Render("(" + string(r.Category) + ")")
		rec := ui.RecommendationTag(r)
		fmt.Fprintf(
			w, "    %s %-30s %10s%s%s\n",
			icon,
			name+" "+cat,
			ui.InfoStyle.Render(ui.ArtifactSize(r)),
			ui.LastUsedTag(r),
			rec,
		)
	}

	if len(collapsed) > 0 {
		fmt.Fprintf(
			w, "    %s\n",
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

package output

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/pathutil"
	"github.com/ohing504/devclean/internal/ui"
)

// TableOptions controls table output behavior.
type TableOptions struct {
	TopN    int  // limit output to top N projects (0 = all)
	Verbose bool // show all artifacts including small ones
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

	ecoGroups := groupByEcosystem(results)
	sortGroupsBySize(ecoGroups)

	// Apply top N at project level across all ecosystems
	if opts.TopN > 0 {
		ecoGroups = applyTopN(ecoGroups, opts.TopN)
	}

	var grandTotal int64
	var grandCount int
	var safeTotal int64

	for _, eg := range ecoGroups {
		grandTotal += eg.totalSize
		grandCount += len(eg.items)

		projects := model.GroupByProject(eg.items)

		fmt.Fprintf(w, "\n%s %s\n",
			ui.HeaderStyle.Render(fmt.Sprintf("● %s", eg.ecosystem)),
			ui.DimStyle.Render(fmt.Sprintf("%d projects · %s", len(projects), model.HumanSize(eg.totalSize))),
		)

		for _, p := range projects {
			// Project header: name + status badge + size + relative time
			badge := statusBadge(p.Activity)
			protectedBadge := ""
			if p.Protected {
				protectedBadge = " " + ui.ProtectedStyle.Render("Protected")
			}

			fmt.Fprintf(w, "  %s%s %s %s\n",
				ui.ProjectStyle.Render(p.Name),
				protectedBadge,
				badge,
				ui.InfoStyle.Render(fmt.Sprintf("%s · %s", model.HumanSize(p.TotalSize), relativeTime(p.LastMod))),
			)

			// Project path
			fmt.Fprintf(w, "  %s\n", ui.DimStyle.Render(pathutil.ShortenHome(p.Path)))

			// Count safe items
			for _, r := range p.Items {
				if r.Safety == model.SafetySafe {
					safeTotal += r.Size
				}
			}

			// Group artifacts by sub-package
			subPkgs := groupBySubPackage(p.Items, p.Path)
			renderSubPackages(w, subPkgs, opts)
		}
	}

	fmt.Fprintf(w, "\n%s\n",
		ui.TotalStyle.Render(fmt.Sprintf("Total: %s (%d items)", model.HumanSize(grandTotal), grandCount)),
	)
	if safeTotal > 0 {
		fmt.Fprintf(w, "%s\n",
			ui.SafeStyle.Render(fmt.Sprintf("Safe to clean: %s", model.HumanSize(safeTotal))),
		)
	}

	// Legend
	fmt.Fprintf(w, "\n%s  %s safe  %s caution  %s protected   %s Active  %s Recent  %s Stale  %s Dormant\n",
		ui.DimStyle.Render("Legend:"),
		ui.SafeStyle.Render("✔"), ui.CautionStyle.Render("⚠"), ui.ProtectedStyle.Render("✖"),
		ui.ActiveStyle.Render("●"), ui.RecentStyle.Render("●"), ui.StaleStyle.Render("●"), ui.DormantStyle.Render("●"),
	)
	fmt.Fprintf(w, "        %s\n", ui.DimStyle.Render("Run 'devclean list' for details"))
}

// --- Ecosystem grouping ---

type ecoGroup struct {
	ecosystem string
	totalSize int64
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
		g.totalSize += r.Size
		g.items = append(g.items, r)
	}

	var groups []ecoGroup
	for _, g := range m {
		groups = append(groups, *g)
	}
	return groups
}

// applyTopN collects all projects across ecosystems, sorts by size,
// keeps only top N, then rebuilds ecosystem groups with the remaining items.
func applyTopN(ecoGroups []ecoGroup, topN int) []ecoGroup {
	// Collect all projects across ecosystems
	var allProjects []model.ProjectGroup
	for _, eg := range ecoGroups {
		projects := model.GroupByProject(eg.items)
		allProjects = append(allProjects, projects...)
	}

	// Already sorted by GroupByProject, but re-sort across ecosystems
	sort.Slice(allProjects, func(i, j int) bool {
		return allProjects[i].TotalSize > allProjects[j].TotalSize
	})

	if topN < len(allProjects) {
		allProjects = allProjects[:topN]
	}

	// Rebuild: collect kept project paths
	kept := make(map[string]bool)
	for _, p := range allProjects {
		kept[p.Path] = true
	}

	// Filter original results to only include items from kept projects
	var filtered []model.ScanResult
	for _, eg := range ecoGroups {
		for _, r := range eg.items {
			key := r.ProjectKey()
			if kept[key] {
				filtered = append(filtered, r)
			}
		}
	}

	return groupByEcosystem(filtered)
}

func sortGroupsBySize(groups []ecoGroup) {
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].totalSize > groups[j].totalSize
	})
}

// --- Formatting helpers ---

func statusBadge(s model.ActivityStatus) string {
	switch s {
	case model.StatusActive:
		return ui.ActiveStyle.Render("Active")
	case model.StatusRecent:
		return ui.RecentStyle.Render("Recent")
	case model.StatusStale:
		return ui.StaleStyle.Render("Stale")
	case model.StatusDormant:
		return ui.DormantStyle.Render("Dormant")
	default:
		return ""
	}
}

func safetyIcon(s model.SafetyLevel) string {
	switch s {
	case model.SafetySafe:
		return ui.SafeStyle.Render("✔")
	case model.SafetyCaution:
		return ui.CautionStyle.Render("⚠")
	case model.SafetyProtected:
		return ui.ProtectedStyle.Render("✖")
	default:
		return " "
	}
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

// --- Sub-package grouping ---

type subPackage struct {
	name      string // relative dir from project root ("." for root)
	totalSize int64
	items     []model.ScanResult
}

func groupBySubPackage(items []model.ScanResult, projectRoot string) []subPackage {
	m := make(map[string]*subPackage)

	for _, r := range items {
		rel := artifactRelPath(r.Path, projectRoot)
		dir := filepath.Dir(rel)
		if dir == "." {
			dir = "."
		}

		sp, ok := m[dir]
		if !ok {
			sp = &subPackage{name: dir}
			m[dir] = sp
		}
		sp.totalSize += r.Size
		sp.items = append(sp.items, r)
	}

	var result []subPackage
	for _, sp := range m {
		sort.Slice(sp.items, func(i, j int) bool {
			return sp.items[i].Size > sp.items[j].Size
		})
		result = append(result, *sp)
	}

	// Sort sub-packages by size desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].totalSize > result[j].totalSize
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
	var collapsedPkgSize int64

	for _, sp := range subPkgs {
		// Collapse small sub-packages in default mode
		if !opts.Verbose && sp.totalSize < collapseThreshold && len(subPkgs) > 2 {
			collapsedPkgs++
			collapsedPkgSize += sp.totalSize
			continue
		}

		// Sub-package header
		displayName := sp.name
		if displayName == "." {
			displayName = ". (root)"
		}
		fmt.Fprintf(w, "    %s %s\n",
			ui.ProjectStyle.Render(displayName),
			ui.DimStyle.Render(fmt.Sprintf("(%s)", model.HumanSize(sp.totalSize))),
		)

		// Artifacts in this sub-package
		for _, r := range sp.items {
			icon := safetyIcon(r.Safety)
			name := artifactDisplayName(r)
			cat := ui.DimStyle.Render("(" + string(r.Category) + ")")
			rec := recommendationTag(r)
			fmt.Fprintf(w, "      %s %-24s %10s%s\n",
				icon,
				name+" "+cat,
				ui.InfoStyle.Render(model.HumanSize(r.Size)),
				rec,
			)
		}
	}

	if collapsedPkgs > 0 {
		fmt.Fprintf(w, "    %s\n",
			ui.DimStyle.Render(fmt.Sprintf("  ... and %d more packages (%s)", collapsedPkgs, model.HumanSize(collapsedPkgSize))),
		)
	}
}

func renderArtifactsFlat(w io.Writer, items []model.ScanResult, projectRoot string, opts TableOptions) {
	var collapsed int
	var collapsedSize int64

	for _, r := range items {
		if !opts.Verbose && r.Size < collapseThreshold && len(items) > 1 {
			collapsed++
			collapsedSize += r.Size
			continue
		}

		icon := safetyIcon(r.Safety)
		name := artifactDisplayNameOr(r, artifactRelPath(r.Path, projectRoot))
		cat := ui.DimStyle.Render("(" + string(r.Category) + ")")
		rec := recommendationTag(r)
		fmt.Fprintf(w, "    %s %-30s %10s%s\n",
			icon,
			name+" "+cat,
			ui.InfoStyle.Render(model.HumanSize(r.Size)),
			rec,
		)
	}

	if collapsed > 0 {
		fmt.Fprintf(w, "    %s\n",
			ui.DimStyle.Render(fmt.Sprintf("  ... and %d more (%s)", collapsed, model.HumanSize(collapsedSize))),
		)
	}
}

// artifactDisplayName picks the best human-readable name for an artifact:
// Label if set, else basename.
func artifactDisplayName(r model.ScanResult) string {
	if r.Label != "" {
		return r.Label
	}
	return filepath.Base(r.Path)
}

// artifactDisplayNameOr uses Label if set, else the supplied fallback (typically a relative path).
func artifactDisplayNameOr(r model.ScanResult, fallback string) string {
	if r.Label != "" {
		return r.Label
	}
	return fallback
}

// recommendationTag formats r.Recommendation as a styled trailing tag, or "" if empty.
func recommendationTag(r model.ScanResult) string {
	if r.Recommendation == "" {
		return ""
	}
	return "  " + ui.RecommendStyle.Render("← "+r.Recommendation)
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

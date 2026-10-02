package model

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// Sort keys accepted by scan --sort.
const (
	SortBySize = "size"
	SortByTime = "time"
	SortByName = "name"
)

// IsSortKey reports whether s is a key scan --sort accepts.
func IsSortKey(s string) bool {
	return s == SortBySize || s == SortByTime || s == SortByName
}

// TopProjects keeps the artifacts of the first topN projects in order, keeping
// their order in results. A project is one project root even when its
// artifacts span several ecosystems; it is ranked by all of them together.
func TopProjects(results []ScanResult, topN int, order func(a, b ProjectGroup) int) []ScanResult {
	projects := GroupByProject(results)
	if topN >= len(projects) {
		return results
	}
	slices.SortFunc(projects, order)
	kept := make(map[string]bool, topN)
	for _, p := range projects[:topN] {
		kept[p.Path] = true
	}
	return FilterResults(results, func(r ScanResult) bool {
		return kept[r.ProjectKey()]
	})
}

// CompareResults orders scan results by a --sort key: largest or newest first,
// or path A→Z for name; ascending puts the smallest or oldest first instead.
// Equal keys fall back to path A→Z so the order is the same on every run.
// An unknown key sorts by size.
func CompareResults(sortBy string, ascending bool) func(a, b ScanResult) int {
	return func(a, b ScanResult) int {
		return compareKeys(sortBy, ascending,
			sortKeys{a.Size, a.LastMod, a.Path},
			sortKeys{b.Size, b.LastMod, b.Path})
	}
}

// CompareProjects orders project groups like CompareResults, using the
// project's TotalSize, LastMod and Path.
func CompareProjects(sortBy string, ascending bool) func(a, b ProjectGroup) int {
	return func(a, b ProjectGroup) int {
		return compareKeys(sortBy, ascending,
			sortKeys{a.TotalSize, a.LastMod, a.Path},
			sortKeys{b.TotalSize, b.LastMod, b.Path})
	}
}

// sortKeys holds the fields a --sort key can order by.
type sortKeys struct {
	size    int64
	lastMod time.Time
	path    string
}

// compareKeys compares a and b by sortBy: descending for size and time unless
// ascending is set. Name has no key besides the path tie-break, which is
// always A→Z.
func compareKeys(sortBy string, ascending bool, a, b sortKeys) int {
	var c int
	switch sortBy {
	case SortByTime:
		c = a.lastMod.Compare(b.lastMod)
	case SortByName:
	default:
		c = cmp.Compare(a.size, b.size)
	}
	if !ascending {
		c = -c
	}
	return cmp.Or(c, strings.Compare(a.path, b.path))
}

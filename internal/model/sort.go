package model

import (
	"cmp"
	"strings"
	"time"
)

// Sort keys accepted by scan --sort.
const (
	SortBySize = "size"
	SortByTime = "time"
	SortByName = "name"
)

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

package model

import (
	"cmp"
	"strings"
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
		var c int
		switch sortBy {
		case SortByTime:
			c = a.LastMod.Compare(b.LastMod)
		case SortByName:
		default:
			c = cmp.Compare(a.Size, b.Size)
		}
		return cmp.Or(directed(sortBy, ascending, c), strings.Compare(a.Path, b.Path))
	}
}

// CompareProjects orders project groups like CompareResults, using the
// project's TotalSize, LastMod and Path.
func CompareProjects(sortBy string, ascending bool) func(a, b ProjectGroup) int {
	return func(a, b ProjectGroup) int {
		var c int
		switch sortBy {
		case SortByTime:
			c = a.LastMod.Compare(b.LastMod)
		case SortByName:
		default:
			c = cmp.Compare(a.TotalSize, b.TotalSize)
		}
		return cmp.Or(directed(sortBy, ascending, c), strings.Compare(a.Path, b.Path))
	}
}

// directed turns an ascending comparison c into the default direction for
// sortBy: descending for size and time. Name has no key besides the path
// tie-break, which is always A→Z.
func directed(sortBy string, ascending bool, c int) int {
	if ascending || sortBy == SortByName {
		return c
	}
	return -c
}

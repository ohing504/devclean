package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/ohing504/devclean/internal/model"
)

// TestSortResultsTieBreaksByPath pins that results with equal sort keys are
// ordered by path in both directions, so scan output is the same on every run.
func TestSortResultsTieBreaksByPath(t *testing.T) {
	for _, asc := range []bool{false, true} {
		var results []model.ScanResult
		for _, p := range []string{"/e", "/c", "/a", "/d", "/b"} {
			results = append(results, model.ScanResult{Path: p, Size: 100})
		}

		sortResults(results, "size", asc)

		var got []string
		for _, r := range results {
			got = append(got, r.Path)
		}
		if want := "/a /b /c /d /e"; strings.Join(got, " ") != want {
			t.Errorf("ascending=%v: order = %v, want %s", asc, got, want)
		}
	}
}

// TestSortResultsAscending pins that --asc means ascending for every key:
// smallest, oldest or A→Z first.
func TestSortResultsAscending(t *testing.T) {
	old, mid, newest := time.Unix(100, 0), time.Unix(200, 0), time.Unix(300, 0)
	for _, tc := range []struct {
		sortBy string
		want   string
	}{
		{"size", "/b /c /a"},
		{"time", "/c /a /b"},
		{"name", "/a /b /c"},
	} {
		results := []model.ScanResult{
			{Path: "/b", Size: 1, LastMod: newest},
			{Path: "/a", Size: 3, LastMod: mid},
			{Path: "/c", Size: 2, LastMod: old},
		}

		sortResults(results, tc.sortBy, true)

		var got []string
		for _, r := range results {
			got = append(got, r.Path)
		}
		if strings.Join(got, " ") != tc.want {
			t.Errorf("--sort %s --asc: order = %v, want %s", tc.sortBy, got, tc.want)
		}
	}
}

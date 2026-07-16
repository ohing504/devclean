package model_test

import (
	"testing"
	"time"

	"github.com/ohing504/devclean/internal/model"
)

func TestEcosystemValues(t *testing.T) {
	tests := []struct {
		eco  model.Ecosystem
		want string
	}{
		{model.EcoXcode, "xcode"},
		{model.EcoAndroid, "android"},
		{model.EcoFlutter, "flutter"},
		{model.EcoNode, "node"},
		{model.EcoDocker, "docker"},
		{model.EcoPython, "python"},
		{model.EcoRust, "rust"},
		{model.EcoRuby, "ruby"},
		{model.EcoGlobal, "global"},
	}
	for _, tt := range tests {
		if string(tt.eco) != tt.want {
			t.Errorf("got %q, want %q", tt.eco, tt.want)
		}
	}
}

func TestAllEcosystems(t *testing.T) {
	ecos := model.AllEcosystems()
	if len(ecos) != 11 {
		t.Errorf("expected 11 ecosystems, got %d", len(ecos))
	}
}

func TestCategoryValues(t *testing.T) {
	tests := []struct {
		cat  model.Category
		want string
	}{
		{model.CatCache, "cache"},
		{model.CatBuild, "build"},
		{model.CatRuntime, "runtime"},
		{model.CatDeps, "deps"},
	}
	for _, tt := range tests {
		if string(tt.cat) != tt.want {
			t.Errorf("got %q, want %q", tt.cat, tt.want)
		}
	}
}

func TestAllCategories(t *testing.T) {
	cats := model.AllCategories()
	if len(cats) != 4 {
		t.Errorf("expected 4 categories, got %d", len(cats))
	}
}

func TestHumanSize(t *testing.T) {
	// HumanSize uses decimal SI (1 KB = 1000 B) to match macOS Finder
	// convention and the `--min-size` parser default.
	tests := []struct {
		size int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{999, "999 B"},
		{1000, "1.0 KB"},
		{1500, "1.5 KB"},
		{1024, "1.0 KB"},       // binary KiB still rounds to 1.0 KB
		{1048576, "1.0 MB"},    // 1 MiB ≈ 1.05 MB → "1.0 MB" rounded
		{1073741824, "1.1 GB"}, // 1 GiB ≈ 1.07 GB → "1.1 GB" rounded
		{5368709120, "5.4 GB"}, // 5 GiB ≈ 5.37 GB → "5.4 GB"
		{1_000_000_000_000, "1.0 TB"},
	}
	for _, tt := range tests {
		got := model.HumanSize(tt.size)
		if got != tt.want {
			t.Errorf("HumanSize(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}

func TestScanResultHumanSize(t *testing.T) {
	r := model.ScanResult{Size: 1048576}
	if got := r.HumanSize(); got != "1.0 MB" {
		t.Errorf("ScanResult.HumanSize() = %q, want %q", got, "1.0 MB")
	}
}

func TestSafetyLevelValues(t *testing.T) {
	tests := []struct {
		level model.SafetyLevel
		want  string
	}{
		{model.SafetySafe, "safe"},
		{model.SafetyCaution, "caution"},
		{model.SafetyProtected, "protected"},
	}
	for _, tt := range tests {
		if string(tt.level) != tt.want {
			t.Errorf("got %q, want %q", tt.level, tt.want)
		}
	}
}

func TestGroupByProject_SortsDescending(t *testing.T) {
	results := []model.ScanResult{
		{Path: "/projects/small/node_modules", ProjectRoot: "/projects/small", Size: 100, Ecosystem: model.EcoNode},
		{Path: "/projects/big/node_modules", ProjectRoot: "/projects/big", Size: 1000, Ecosystem: model.EcoNode},
		{Path: "/projects/big/.next", ProjectRoot: "/projects/big", Size: 500, Ecosystem: model.EcoNode},
	}

	groups := model.GroupByProject(results)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// First group should be bigger
	if groups[0].TotalSize <= groups[1].TotalSize {
		t.Errorf("expected groups sorted descending by size: got %d, %d", groups[0].TotalSize, groups[1].TotalSize)
	}
	if groups[0].TotalSize != 1500 {
		t.Errorf("expected first group total=1500, got %d", groups[0].TotalSize)
	}
}

func TestGroupByProject_PropagatesProtected(t *testing.T) {
	results := []model.ScanResult{
		{Path: "/proj/vendor", ProjectRoot: "/proj", Size: 100, Protected: true, Safety: model.SafetyProtected},
		{Path: "/proj/target", ProjectRoot: "/proj", Size: 200, Protected: false, Safety: model.SafetySafe},
	}

	groups := model.GroupByProject(results)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if !groups[0].Protected {
		t.Error("expected group to be Protected when any item is protected")
	}
}

func TestGroupByProject_Empty(t *testing.T) {
	groups := model.GroupByProject(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for nil input, got %d", len(groups))
	}
}

func TestProjectKey_WithProjectRoot(t *testing.T) {
	r := model.ScanResult{
		Path:        "/home/user/myapp/node_modules",
		ProjectRoot: "/home/user/myapp",
	}
	got := r.ProjectKey()
	if got != "/home/user/myapp" {
		t.Errorf("ProjectKey() = %q, want %q", got, "/home/user/myapp")
	}
}

func TestProjectKey_WithoutProjectRoot(t *testing.T) {
	r := model.ScanResult{
		Path: "/home/user/myapp/node_modules",
	}
	got := r.ProjectKey()
	want := "/home/user/myapp"
	if got != want {
		t.Errorf("ProjectKey() = %q, want %q", got, want)
	}
}

func TestFilterResults(t *testing.T) {
	now := time.Now()
	results := []model.ScanResult{
		{Path: "/a", Ecosystem: model.EcoNode, Category: model.CatDeps, LastMod: now},
		{Path: "/b", Ecosystem: model.EcoRust, Category: model.CatBuild, LastMod: now},
		{Path: "/c", Ecosystem: model.EcoNode, Category: model.CatBuild, LastMod: now},
	}

	// Filter by ecosystem
	nodeResults := model.FilterResults(results, func(r model.ScanResult) bool {
		return r.Ecosystem == model.EcoNode
	})
	if len(nodeResults) != 2 {
		t.Errorf("expected 2 node results, got %d", len(nodeResults))
	}

	// Filter by category
	buildResults := model.FilterResults(results, func(r model.ScanResult) bool {
		return r.Category == model.CatBuild
	})
	if len(buildResults) != 2 {
		t.Errorf("expected 2 build results, got %d", len(buildResults))
	}

	// Filter returning nothing
	none := model.FilterResults(results, func(r model.ScanResult) bool {
		return r.Ecosystem == model.EcoPython
	})
	if len(none) != 0 {
		t.Errorf("expected 0 results, got %d", len(none))
	}
}

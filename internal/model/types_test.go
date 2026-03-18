package model_test

import (
	"testing"

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
	if len(ecos) != 7 {
		t.Errorf("expected 7 ecosystems, got %d", len(ecos))
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
	tests := []struct {
		size int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{5368709120, "5.0 GB"},
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

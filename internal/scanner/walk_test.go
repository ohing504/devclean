package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

type stubScanner struct {
	name    string
	eco     model.Ecosystem
	results []model.ScanResult
}

func (s *stubScanner) Name() string               { return s.name }
func (s *stubScanner) Ecosystem() model.Ecosystem { return s.eco }

func (s *stubScanner) Scan(_ context.Context, _ string) ([]model.ScanResult, error) {
	return s.results, nil
}

// TestWalkScan_DedupCoverage pins the double-count fix for artifacts listed
// by several ecosystems: a directory matching rules of multiple active
// ecosystems is emitted once, attributed to the first table in order. With a
// subset of ecosystems active, attribution follows the first active table.
func TestWalkScan_DedupCoverage(t *testing.T) {
	root := t.TempDir()

	// A project that is both a Node and a Ruby project, with a coverage/
	// directory that node and ruby tables both list.
	proj := filepath.Join(root, "dualapp")
	if err := os.MkdirAll(filepath.Join(proj, "coverage"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, f := range []string{"package.json", "Gemfile", filepath.Join("coverage", "index.html")} {
		if err := os.WriteFile(filepath.Join(proj, f), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	// node + ruby active: exactly one result, attributed to node (table order).
	results, err := WalkScan(context.Background(), root, model.EcoNode, model.EcoRuby)
	if err != nil {
		t.Fatalf("WalkScan error: %v", err)
	}
	if len(results) != 1 {
		for _, r := range results {
			t.Logf("  %s %s", r.Ecosystem, r.Path)
		}
		t.Fatalf("expected exactly 1 result for node+ruby, got %d", len(results))
	}
	if results[0].Path != filepath.Join(proj, "coverage") {
		t.Errorf("unexpected path: %s", results[0].Path)
	}
	if results[0].Ecosystem != model.EcoNode {
		t.Errorf("expected node attribution (first table), got %s", results[0].Ecosystem)
	}

	// ruby alone: the same directory is attributed to ruby.
	results, err = WalkScan(context.Background(), root, model.EcoRuby)
	if err != nil {
		t.Fatalf("WalkScan error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result for ruby alone, got %d", len(results))
	}
	if results[0].Ecosystem != model.EcoRuby {
		t.Errorf("expected ruby attribution, got %s", results[0].Ecosystem)
	}
}

// TestScanWithProgress_MixedPartition pins the partition behavior: walk
// adapters are batched into a single walk that runs first (reported under the
// "projects" label), remaining scanners run sequentially after it, and batch
// results are ordered by (table order, path).
func TestScanWithProgress_MixedPartition(t *testing.T) {
	root := t.TempDir()

	// alpha project placed lexically after the beta project, so table-order
	// sorting is distinguishable from plain path sorting.
	alphaOut := filepath.Join(root, "z-alpha", "out_a")
	betaOut := filepath.Join(root, "a-beta", "out_b")
	for _, dir := range []string{alphaOut, betaOut} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	for _, marker := range []string{
		filepath.Join(root, "z-alpha", "alpha.mark"),
		filepath.Join(root, "a-beta", "beta.mark"),
	} {
		if err := os.WriteFile(marker, []byte("m"), 0o644); err != nil {
			t.Fatalf("write %s: %v", marker, err)
		}
	}

	alpha := newWalkScanner(walkEcosystem{
		Name:    "alpha",
		Eco:     model.Ecosystem("test-alpha"),
		Markers: []string{"alpha.mark"},
		Rules:   []artifactRule{{RelPath: "out_a", Category: model.CatBuild, Safety: model.SafetySafe}},
	})
	beta := newWalkScanner(walkEcosystem{
		Name:    "beta",
		Eco:     model.Ecosystem("test-beta"),
		Markers: []string{"beta.mark"},
		Rules:   []artifactRule{{RelPath: "out_b", Category: model.CatBuild, Safety: model.SafetySafe}},
	})
	stub := &stubScanner{
		name:    "stub",
		eco:     model.Ecosystem("test-stub"),
		results: []model.ScanResult{{Path: "/stub/result", Ecosystem: "test-stub", Size: 42}},
	}

	var labels []string
	reg := NewRegistry()
	// The stub is listed first, but the walk batch must still run first.
	results, err := reg.ScanWithProgress(context.Background(), root, []Scanner{stub, alpha, beta},
		func(eco string, _ int) {
			if eco != "" {
				labels = append(labels, eco)
			}
		})
	if err != nil {
		t.Fatalf("ScanWithProgress error: %v", err)
	}

	wantPaths := []string{alphaOut, betaOut, "/stub/result"}
	if len(results) != len(wantPaths) {
		for _, r := range results {
			t.Logf("  %s %s", r.Ecosystem, r.Path)
		}
		t.Fatalf("expected %d results, got %d", len(wantPaths), len(results))
	}
	for i, want := range wantPaths {
		if results[i].Path != want {
			t.Errorf("result[%d].Path: got %s, want %s", i, results[i].Path, want)
		}
	}
	if results[0].Ecosystem != "test-alpha" || results[1].Ecosystem != "test-beta" {
		t.Errorf("batch results misattributed: got %s, %s", results[0].Ecosystem, results[1].Ecosystem)
	}

	// One "projects" label for the whole batch, then the stub by name.
	wantLabels := []string{"projects", "stub"}
	if len(labels) != len(wantLabels) {
		t.Fatalf("expected labels %v, got %v", wantLabels, labels)
	}
	for i, want := range wantLabels {
		if labels[i] != want {
			t.Errorf("label[%d]: got %s, want %s", i, labels[i], want)
		}
	}
}

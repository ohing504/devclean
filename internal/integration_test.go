package internal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ohing504/devclean/internal/classifier"
	"github.com/ohing504/devclean/internal/cleaner"
	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/output"
	"github.com/ohing504/devclean/internal/scanner"
)

var update = flag.Bool("update", false, "update golden files")

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// setupTestWorkspace creates a temporary workspace with Node.js and Rust projects.
func setupTestWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Node.js project with node_modules and .next
	nodeProject := filepath.Join(root, "my-app")
	mustMkdir(t, filepath.Join(nodeProject, "node_modules", "react"))
	mustWriteFile(t, filepath.Join(nodeProject, "node_modules", "react", "index.js"), make([]byte, 4096))
	mustMkdir(t, filepath.Join(nodeProject, ".next", "cache"))
	mustWriteFile(t, filepath.Join(nodeProject, ".next", "cache", "data.json"), make([]byte, 2048))
	mustWriteFile(t, filepath.Join(nodeProject, "package.json"), []byte(`{}`))

	// Rust project with target/
	rustProject := filepath.Join(root, "cli-tool")
	mustMkdir(t, filepath.Join(rustProject, "target", "debug"))
	mustWriteFile(t, filepath.Join(rustProject, "target", "debug", "cli-tool"), make([]byte, 8192))
	mustWriteFile(t, filepath.Join(rustProject, "Cargo.toml"), []byte("[package]"))

	// Python project with __pycache__ and .pytest_cache
	pyProject := filepath.Join(root, "py-svc")
	mustMkdir(t, filepath.Join(pyProject, "__pycache__"))
	mustWriteFile(t, filepath.Join(pyProject, "__pycache__", "mod.cpython-312.pyc"), make([]byte, 1024))
	mustMkdir(t, filepath.Join(pyProject, ".pytest_cache"))
	mustWriteFile(t, filepath.Join(pyProject, ".pytest_cache", "lastfailed"), make([]byte, 512))
	mustWriteFile(t, filepath.Join(pyProject, "pyproject.toml"), []byte("[project]"))

	// Ruby project with vendor/bundle and .bundle
	rbProject := filepath.Join(root, "rb-app")
	mustMkdir(t, filepath.Join(rbProject, "vendor", "bundle", "ruby"))
	mustWriteFile(t, filepath.Join(rbProject, "vendor", "bundle", "ruby", "gem.rb"), make([]byte, 2048))
	mustMkdir(t, filepath.Join(rbProject, ".bundle"))
	mustWriteFile(t, filepath.Join(rbProject, ".bundle", "config"), make([]byte, 256))
	mustWriteFile(t, filepath.Join(rbProject, "Gemfile"), []byte("source 'https://rubygems.org'"))

	return root
}

func TestIntegration_ScanClassifyPipeline(t *testing.T) {
	root := setupTestWorkspace(t)

	// 1. Scan
	reg := scanner.DefaultRegistry()
	results, err := reg.ScanAll(context.Background(), root)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	// Should find artifacts from both ecosystems
	if len(results) < 3 {
		t.Fatalf("expected at least 3 results (node_modules, .next, target), got %d", len(results))
	}

	// 2. Classify
	classifier.ClassifyResults(results, classifier.DefaultThresholds())

	// All should be active (just created)
	for _, r := range results {
		if r.Activity != model.StatusActive {
			t.Errorf("expected active for %s, got %s", r.Path, r.Activity)
		}
	}

	// 3. Verify ecosystems
	ecoFound := make(map[model.Ecosystem]bool)
	for _, r := range results {
		ecoFound[r.Ecosystem] = true
	}
	for _, eco := range []model.Ecosystem{model.EcoNode, model.EcoRust, model.EcoPython, model.EcoRuby} {
		if !ecoFound[eco] {
			t.Errorf("expected %s ecosystem in results", eco)
		}
	}

	// 4. Group by project
	projects := model.GroupByProject(results)
	if len(projects) < 4 {
		t.Errorf("expected at least 4 projects (my-app, cli-tool, py-svc, rb-app), got %d", len(projects))
	}
}

func TestIntegration_ScanClassifyClean(t *testing.T) {
	root := setupTestWorkspace(t)

	// Scan
	reg := scanner.DefaultRegistry()
	results, err := reg.ScanAll(context.Background(), root)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	// Classify
	classifier.ClassifyResults(results, classifier.DefaultThresholds())

	// Clean (force, since TempDir won't map to Trash)
	c := cleaner.New(cleaner.Options{Force: true})
	for _, r := range results {
		if err := c.Clean(r); err != nil {
			t.Errorf("clean error for %s: %v", r.Path, err)
		}
	}

	// Verify artifacts are deleted
	for _, r := range results {
		if _, err := os.Stat(r.Path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be deleted", r.Path)
		}
	}

	// Verify project files still exist
	if _, err := os.Stat(filepath.Join(root, "my-app", "package.json")); os.IsNotExist(err) {
		t.Error("package.json should not be deleted")
	}
	if _, err := os.Stat(filepath.Join(root, "cli-tool", "Cargo.toml")); os.IsNotExist(err) {
		t.Error("Cargo.toml should not be deleted")
	}
}

func TestIntegration_FilterByEcosystem(t *testing.T) {
	root := setupTestWorkspace(t)

	reg := scanner.DefaultRegistry()

	// Scan only Node.js
	nodeScanners := reg.ForEcosystems([]model.Ecosystem{model.EcoNode})
	results, err := reg.ScanWith(context.Background(), root, nodeScanners)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	for _, r := range results {
		if r.Ecosystem != model.EcoNode {
			t.Errorf("expected only node ecosystem, got %s", r.Ecosystem)
		}
	}
}

// TestIntegration_GlobalScannerPipeline drives the Global stat scanner with an
// injected HOME, then classifies — exercising the full scan→classify path for a
// fixed-home scanner (the walk fixtures above only cover the tree-walking
// scanners). The scanner is constructed directly with an isolated TmpRoot and a
// stubbed process check so it never touches real machine temp state.
func TestIntegration_GlobalScannerPipeline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	npm := filepath.Join(home, ".npm")
	mustMkdir(t, npm)
	mustWriteFile(t, filepath.Join(npm, "cache.json"), make([]byte, 2048))

	s := &scanner.GlobalScanner{
		TmpRoot:        t.TempDir(), // isolate from real browser code-sign clones
		ProcessRunning: func(string) bool { return false },
	}
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	var npmResult *model.ScanResult
	for i := range results {
		if results[i].Ecosystem != model.EcoGlobal {
			t.Errorf("expected only global ecosystem, got %s for %s", results[i].Ecosystem, results[i].Path)
		}
		if results[i].Path == npm {
			npmResult = &results[i]
		}
	}
	if npmResult == nil {
		t.Fatalf("expected ~/.npm to be detected under injected HOME")
	}
	if npmResult.Safety != model.SafetySafe {
		t.Errorf("~/.npm: expected safety=safe, got %s", npmResult.Safety)
	}

	classifier.ClassifyResults(results, classifier.DefaultThresholds())
	if npmResult.Activity == "" {
		t.Error("classify should assign an activity status to the global cache")
	}
}

func TestIntegration_FilterResults(t *testing.T) {
	root := setupTestWorkspace(t)

	reg := scanner.DefaultRegistry()
	results, err := reg.ScanAll(context.Background(), root)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	// Filter by category
	deps := model.FilterResults(results, func(r model.ScanResult) bool {
		return r.Category == model.CatDeps
	})
	for _, r := range deps {
		if r.Category != model.CatDeps {
			t.Errorf("expected deps category, got %s", r.Category)
		}
	}

	// Filter by ecosystem
	rust := model.FilterResults(results, func(r model.ScanResult) bool {
		return r.Ecosystem == model.EcoRust
	})
	if len(rust) != 1 {
		t.Errorf("expected 1 rust result (target), got %d", len(rust))
	}
}

// --- Golden file tests ---

func TestGolden_JSONOutput(t *testing.T) {
	root := setupTestWorkspace(t)

	reg := scanner.DefaultRegistry()
	results, err := reg.ScanAll(context.Background(), root)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	classifier.ClassifyResults(results, classifier.DefaultThresholds())

	// Normalize for golden file: zero out volatile fields
	for i := range results {
		results[i].Path = filepath.Base(results[i].Path)
		results[i].Size = 0              // du returns block-aligned sizes
		results[i].LastMod = time.Time{} // zero out to make golden file stable
		results[i].ProjectRoot = ""      // holds the absolute temp path — machine-specific
	}

	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, results); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	goldenPath := filepath.Join("testdata", "scan_output.golden.json")

	if *update {
		mustMkdir(t, "testdata")
		mustWriteFile(t, goldenPath, buf.Bytes())
		t.Log("updated golden file")
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file: %v (run with -update to create)", err)
	}

	// Parse both to compare structure (ignore whitespace)
	var gotJSON, wantJSON output.ScanOutput
	if err := json.Unmarshal(buf.Bytes(), &gotJSON); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal(expected, &wantJSON); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}

	if gotJSON.TotalCount != wantJSON.TotalCount {
		t.Errorf("TotalCount: got %d, want %d", gotJSON.TotalCount, wantJSON.TotalCount)
	}
	if len(gotJSON.Results) != len(wantJSON.Results) {
		t.Fatalf("Results count: got %d, want %d\nGot:\n%s", len(gotJSON.Results), len(wantJSON.Results), buf.String())
	}

	// Compare actual fields per result (Ecosystem, Category, Safety)
	for i := range gotJSON.Results {
		got := gotJSON.Results[i]
		want := wantJSON.Results[i]
		if got.Path != want.Path {
			t.Errorf("result[%d].Path: got %q, want %q", i, got.Path, want.Path)
		}
		if got.Ecosystem != want.Ecosystem {
			t.Errorf("result[%d].Ecosystem: got %q, want %q", i, got.Ecosystem, want.Ecosystem)
		}
		if got.Category != want.Category {
			t.Errorf("result[%d].Category: got %q, want %q", i, got.Category, want.Category)
		}
		if got.Safety != want.Safety {
			t.Errorf("result[%d].Safety: got %q, want %q", i, got.Safety, want.Safety)
		}
	}
}

package scanner_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestNodeScanner_FindsNodeModules(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myapp")
	nmDir := filepath.Join(projDir, "node_modules", "lodash")
	mustMkdir(t, nmDir)
	mustWriteFile(t, filepath.Join(nmDir, "index.js"), make([]byte, 4096))
	mustWriteFile(t, filepath.Join(projDir, "package.json"), []byte(`{}`))

	s := scanner.NewNodeScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	r := results[0]
	if r.Ecosystem != model.EcoNode {
		t.Errorf("expected ecosystem=node, got %s", r.Ecosystem)
	}
	if r.Category != model.CatDeps {
		t.Errorf("expected category=deps, got %s", r.Category)
	}
	if r.Path != filepath.Join(projDir, "node_modules") {
		t.Errorf("unexpected path: %s", r.Path)
	}
	if r.Size < 4096 {
		t.Errorf("expected size >= 4096 (block-aligned), got %d", r.Size)
	}
	if r.Safety != model.SafetySafe {
		t.Errorf("expected safety=safe, got %s", r.Safety)
	}
}

func TestNodeScanner_FindsNextBuild(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "webapp")
	nextDir := filepath.Join(projDir, ".next", "cache")
	mustMkdir(t, nextDir)
	mustWriteFile(t, filepath.Join(nextDir, "data.json"), make([]byte, 2048))
	mustWriteFile(t, filepath.Join(projDir, "package.json"), []byte(`{}`))

	s := scanner.NewNodeScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Category != model.CatBuild {
		t.Errorf("expected category=build, got %s", r.Category)
	}
}

func TestNodeScanner_SkipsNestedNodeModules(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myapp")
	nested := filepath.Join(projDir, "node_modules", "pkg", "node_modules", "dep")
	mustMkdir(t, nested)
	mustWriteFile(t, filepath.Join(nested, "index.js"), []byte("x"))
	mustWriteFile(t, filepath.Join(projDir, "package.json"), []byte(`{}`))

	s := scanner.NewNodeScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	depsCount := 0
	for _, r := range results {
		if r.Category == model.CatDeps {
			depsCount++
		}
	}
	if depsCount != 1 {
		t.Errorf("expected 1 node_modules result, got %d", depsCount)
	}
}

func TestNodeScanner_IgnoresWithoutPackageJSON(t *testing.T) {
	root := t.TempDir()

	// node_modules without package.json should be ignored
	nmDir := filepath.Join(root, "random", "node_modules")
	mustMkdir(t, nmDir)

	s := scanner.NewNodeScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results without package.json, got %d", len(results))
	}
}

func TestNodeScanner_MultipleProjects(t *testing.T) {
	root := t.TempDir()

	// Project 1
	proj1 := filepath.Join(root, "app1")
	mustMkdir(t, filepath.Join(proj1, "node_modules"))
	mustWriteFile(t, filepath.Join(proj1, "package.json"), []byte(`{}`))

	// Project 2
	proj2 := filepath.Join(root, "app2")
	mustMkdir(t, filepath.Join(proj2, "node_modules"))
	mustMkdir(t, filepath.Join(proj2, ".next"))
	mustWriteFile(t, filepath.Join(proj2, "package.json"), []byte(`{}`))

	s := scanner.NewNodeScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	// app1: node_modules, app2: node_modules + .next = 3
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
		for _, r := range results {
			t.Logf("  %s %s", r.Category, r.Path)
		}
	}
}

func TestNodeScanner_NameAndEcosystem(t *testing.T) {
	s := scanner.NewNodeScanner()
	if s.Name() != "node" {
		t.Errorf("expected name=node, got %s", s.Name())
	}
	if s.Ecosystem() != model.EcoNode {
		t.Errorf("expected ecosystem=node, got %s", s.Ecosystem())
	}
}

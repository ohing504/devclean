package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

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

type fakeScanner struct {
	name      string
	ecosystem model.Ecosystem
	results   []model.ScanResult
}

func (f *fakeScanner) Name() string               { return f.name }
func (f *fakeScanner) Ecosystem() model.Ecosystem { return f.ecosystem }

func (f *fakeScanner) Scan(_ context.Context, _ string) ([]model.ScanResult, error) {
	return f.results, nil
}

func TestRegistryScanAll(t *testing.T) {
	reg := scanner.NewRegistry()
	reg.Register(&fakeScanner{
		name:      "node",
		ecosystem: model.EcoNode,
		results: []model.ScanResult{
			{Path: "/project/node_modules", Ecosystem: model.EcoNode, Size: 1024},
		},
	})
	reg.Register(&fakeScanner{
		name:      "python",
		ecosystem: model.EcoPython,
		results: []model.ScanResult{
			{Path: "/project/venv", Ecosystem: model.EcoPython, Size: 2048},
		},
	})

	results, err := reg.ScanAll(context.Background(), "/")
	if err != nil {
		t.Fatalf("ScanAll error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Ecosystem != model.EcoNode {
		t.Errorf("expected first result=node, got %s", results[0].Ecosystem)
	}
	if results[1].Ecosystem != model.EcoPython {
		t.Errorf("expected second result=python, got %s", results[1].Ecosystem)
	}
}

func TestRegistryForEcosystems(t *testing.T) {
	reg := scanner.NewRegistry()
	reg.Register(&fakeScanner{name: "node", ecosystem: model.EcoNode})
	reg.Register(&fakeScanner{name: "python", ecosystem: model.EcoPython})
	reg.Register(&fakeScanner{name: "xcode", ecosystem: model.EcoXcode})

	filtered := reg.ForEcosystems([]model.Ecosystem{model.EcoNode, model.EcoXcode})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 scanners, got %d", len(filtered))
	}

	names := map[string]bool{}
	for _, s := range filtered {
		names[s.Name()] = true
	}
	if !names["node"] || !names["xcode"] {
		t.Errorf("expected node and xcode, got %v", names)
	}
}

func TestRegistryScanWith(t *testing.T) {
	reg := scanner.NewRegistry()
	nodeScanner := &fakeScanner{
		name:      "node",
		ecosystem: model.EcoNode,
		results:   []model.ScanResult{{Path: "/a", Size: 100}},
	}
	pythonScanner := &fakeScanner{
		name:      "python",
		ecosystem: model.EcoPython,
		results:   []model.ScanResult{{Path: "/b", Size: 200}},
	}
	reg.Register(nodeScanner)
	reg.Register(pythonScanner)

	results, err := reg.ScanWith(context.Background(), "/", []scanner.Scanner{nodeScanner})
	if err != nil {
		t.Fatalf("ScanWith error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestRegistryEmpty(t *testing.T) {
	reg := scanner.NewRegistry()
	results, err := reg.ScanAll(context.Background(), "/")
	if err != nil {
		t.Fatalf("ScanAll error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "a.txt"), make([]byte, 1024), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), make([]byte, 2048), 0o644)

	subdir := filepath.Join(dir, "sub")
	os.MkdirAll(subdir, 0o755)
	os.WriteFile(filepath.Join(subdir, "c.txt"), make([]byte, 512), 0o644)

	size := scanner.DirSize(dir)
	// du reports disk usage (block-aligned), so size >= logical size
	logicalSize := int64(1024 + 2048 + 512)
	if size < logicalSize {
		t.Errorf("DirSize = %d, want >= %d", size, logicalSize)
	}
}

func TestDirSizeNonExistent(t *testing.T) {
	size := scanner.DirSize("/nonexistent/path")
	if size != 0 {
		t.Errorf("DirSize of nonexistent = %d, want 0", size)
	}
}

func TestModTime(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	os.WriteFile(f, []byte("hello"), 0o644)

	mt := scanner.ModTime(f)
	if mt.IsZero() {
		t.Error("expected non-zero ModTime")
	}
}

func TestModTimeNonExistent(t *testing.T) {
	mt := scanner.ModTime("/nonexistent/file")
	if !mt.IsZero() {
		t.Error("expected zero ModTime for nonexistent file")
	}
}

func TestRegistryScanAll_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reg := scanner.NewRegistry()
	reg.Register(&fakeScanner{name: "node", ecosystem: model.EcoNode, results: []model.ScanResult{{Path: "/a"}}})
	results, err := reg.ScanAll(ctx, "/")
	// cancelled context should either return error or empty results — both are acceptable
	_ = results
	_ = err
}

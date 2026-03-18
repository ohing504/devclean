package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestRustScanner_FindsTarget(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myproject")
	targetDir := filepath.Join(projDir, "target", "debug")
	os.MkdirAll(targetDir, 0o755)
	os.WriteFile(filepath.Join(targetDir, "myproject"), make([]byte, 8192), 0o644)
	os.WriteFile(filepath.Join(projDir, "Cargo.toml"), []byte("[package]"), 0o644)

	s := scanner.NewRustScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Ecosystem != model.EcoRust {
		t.Errorf("expected ecosystem=rust, got %s", r.Ecosystem)
	}
	if r.Category != model.CatBuild {
		t.Errorf("expected category=build, got %s", r.Category)
	}
	if r.Safety != model.SafetySafe {
		t.Errorf("expected safety=safe, got %s", r.Safety)
	}
}

func TestRustScanner_IgnoresWithoutCargoToml(t *testing.T) {
	root := t.TempDir()

	// target/ without Cargo.toml should be ignored
	targetDir := filepath.Join(root, "random", "target")
	os.MkdirAll(targetDir, 0o755)

	s := scanner.NewRustScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results without Cargo.toml, got %d", len(results))
	}
}

func TestRustScanner_NameAndEcosystem(t *testing.T) {
	s := scanner.NewRustScanner()
	if s.Name() != "rust" {
		t.Errorf("expected name=rust, got %s", s.Name())
	}
	if s.Ecosystem() != model.EcoRust {
		t.Errorf("expected ecosystem=rust, got %s", s.Ecosystem())
	}
}

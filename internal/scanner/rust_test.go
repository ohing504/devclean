package scanner_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestRustScanner_FindsTarget(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myproject")
	targetDir := filepath.Join(projDir, "target", "debug")
	mustMkdir(t, targetDir)
	mustWriteFile(t, filepath.Join(targetDir, "myproject"), make([]byte, 8192))
	mustWriteFile(t, filepath.Join(projDir, "Cargo.toml"), []byte("[package]"))

	results, err := scanner.WalkScan(context.Background(), root, model.EcoRust)
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
	mustMkdir(t, targetDir)

	results, err := scanner.WalkScan(context.Background(), root, model.EcoRust)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results without Cargo.toml, got %d", len(results))
	}
}

func TestRustScanner_NameAndEcosystem(t *testing.T) {
	for _, s := range scanner.DefaultRegistry().All() {
		if s.Name() != "rust" {
			continue
		}
		if s.Ecosystem() != model.EcoRust {
			t.Errorf("expected ecosystem=rust, got %s", s.Ecosystem())
		}
		return
	}
	t.Error(`expected a registered scanner named "rust"`)
}

func TestRustScanner_Workspace(t *testing.T) {
	root := t.TempDir()
	// Rust workspace: root Cargo.toml + member crate
	projDir := filepath.Join(root, "my-workspace")
	mustMkdir(t, filepath.Join(projDir, "target", "debug"))
	mustWriteFile(t, filepath.Join(projDir, "target", "debug", "bin"), make([]byte, 1024))
	mustWriteFile(t, filepath.Join(projDir, "Cargo.toml"), []byte("[workspace]"))
	// Member crate with its own Cargo.toml but NO target/
	mustMkdir(t, filepath.Join(projDir, "crates", "core"))
	mustWriteFile(t, filepath.Join(projDir, "crates", "core", "Cargo.toml"), []byte("[package]"))

	results, err := scanner.WalkScan(context.Background(), root, model.EcoRust)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	// Should find only 1 target/ (at workspace root)
	if len(results) != 1 {
		t.Errorf("expected 1 target, got %d", len(results))
	}
}

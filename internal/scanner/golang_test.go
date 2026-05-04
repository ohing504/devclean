package scanner_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestGoScanner_FindsVendor(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "myapp")
	mustMkdir(t, projDir)
	mustWriteFile(t, filepath.Join(projDir, "go.mod"), []byte("module example.com/myapp\n"))

	vendor := filepath.Join(projDir, "vendor", "github.com", "stretchr", "testify")
	mustMkdir(t, vendor)
	mustWriteFile(t, filepath.Join(vendor, "doc.go"), make([]byte, 4096))

	s := scanner.NewGoScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Ecosystem != model.EcoGo {
		t.Errorf("expected ecosystem=go, got %s", r.Ecosystem)
	}
	if r.Category != model.CatDeps {
		t.Errorf("expected category=deps, got %s", r.Category)
	}
	if r.Path != filepath.Join(projDir, "vendor") {
		t.Errorf("unexpected path: %s", r.Path)
	}
	if r.Safety != model.SafetyCaution {
		t.Errorf("expected safety=caution (vendor is opt-in), got %s", r.Safety)
	}
}

func TestGoScanner_IgnoresVendorWithoutGoMod(t *testing.T) {
	root := t.TempDir()
	// vendor/ at random place without go.mod sibling — must not be reported.
	vendor := filepath.Join(root, "stuff", "vendor")
	mustMkdir(t, vendor)
	mustWriteFile(t, filepath.Join(vendor, "a"), []byte("x"))

	s := scanner.NewGoScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestGoScanner_IgnoresRubyVendorBundle(t *testing.T) {
	// Ruby/Rails project's vendor/bundle should not be picked up by Go scanner —
	// only Gemfile present, no go.mod.
	root := t.TempDir()
	projDir := filepath.Join(root, "rails")
	mustMkdir(t, projDir)
	mustWriteFile(t, filepath.Join(projDir, "Gemfile"), []byte("source 'rubygems'"))

	bundle := filepath.Join(projDir, "vendor", "bundle", "ruby", "3.2", "gems")
	mustMkdir(t, bundle)
	mustWriteFile(t, filepath.Join(bundle, "x.rb"), []byte("x"))

	s := scanner.NewGoScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Go scanner must not pick up Ruby vendor/bundle, got %d results", len(results))
	}
}

func TestGoScanner_MultipleProjects(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		proj := filepath.Join(root, name)
		mustMkdir(t, proj)
		mustWriteFile(t, filepath.Join(proj, "go.mod"), []byte("module x\n"))
		mustMkdir(t, filepath.Join(proj, "vendor"))
		mustWriteFile(t, filepath.Join(proj, "vendor", "f"), []byte("v"))
	}

	s := scanner.NewGoScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestGoScanner_NameAndEcosystem(t *testing.T) {
	s := scanner.NewGoScanner()
	if s.Name() != "go" {
		t.Errorf("expected name=go, got %s", s.Name())
	}
	if s.Ecosystem() != model.EcoGo {
		t.Errorf("expected ecosystem=go, got %s", s.Ecosystem())
	}
}

package scanner_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestRubyScanner_NameAndEcosystem(t *testing.T) {
	s := scanner.NewRubyScanner()
	if s.Name() != "ruby" {
		t.Errorf("expected name=ruby, got %s", s.Name())
	}
	if s.Ecosystem() != model.EcoRuby {
		t.Errorf("expected ecosystem=ruby, got %s", s.Ecosystem())
	}
}

func TestRubyScanner_FindsVendorBundle(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myapp")
	bundleDir := filepath.Join(projDir, "vendor", "bundle", "ruby", "3.3.0", "gems")
	mustMkdir(t, bundleDir)
	mustWriteFile(t, filepath.Join(bundleDir, "rails.rb"), make([]byte, 4096))
	mustWriteFile(t, filepath.Join(projDir, "Gemfile"), []byte("source 'https://rubygems.org'"))

	s := scanner.NewRubyScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	r := results[0]
	if r.Ecosystem != model.EcoRuby {
		t.Errorf("expected ecosystem=ruby, got %s", r.Ecosystem)
	}
	if r.Category != model.CatDeps {
		t.Errorf("expected category=deps, got %s", r.Category)
	}
	if r.Path != filepath.Join(projDir, "vendor", "bundle") {
		t.Errorf("unexpected path: %s", r.Path)
	}
	if r.Safety != model.SafetySafe {
		t.Errorf("expected safety=safe, got %s", r.Safety)
	}
}

func TestRubyScanner_FindsBundleCache(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myapp")
	mustMkdir(t, filepath.Join(projDir, ".bundle"))
	mustWriteFile(t, filepath.Join(projDir, ".bundle", "config"), []byte("BUNDLE_PATH: vendor/bundle"))
	mustWriteFile(t, filepath.Join(projDir, "Gemfile"), []byte("source 'https://rubygems.org'"))

	s := scanner.NewRubyScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Category == model.CatCache && filepath.Base(r.Path) == ".bundle" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find .bundle cache artifact")
	}
}

func TestRubyScanner_FindsTmp(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "railsapp")
	tmpDir := filepath.Join(projDir, "tmp", "cache", "bootsnap")
	mustMkdir(t, tmpDir)
	mustWriteFile(t, filepath.Join(tmpDir, "compile.cache"), make([]byte, 2048))
	mustWriteFile(t, filepath.Join(projDir, "Gemfile"), []byte("source 'https://rubygems.org'"))

	s := scanner.NewRubyScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Category == model.CatCache && filepath.Base(r.Path) == "tmp" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find tmp artifact")
	}
}

func TestRubyScanner_FindsCoverage(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myapp")
	mustMkdir(t, filepath.Join(projDir, "coverage"))
	mustWriteFile(t, filepath.Join(projDir, "coverage", "index.html"), make([]byte, 1024))
	mustWriteFile(t, filepath.Join(projDir, "Gemfile"), []byte("source 'https://rubygems.org'"))

	s := scanner.NewRubyScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Category == model.CatBuild && filepath.Base(r.Path) == "coverage" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find coverage artifact")
	}
}

func TestRubyScanner_FindsRubyLSP(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myapp")
	mustMkdir(t, filepath.Join(projDir, ".ruby-lsp"))
	mustWriteFile(t, filepath.Join(projDir, ".ruby-lsp", "cache.json"), make([]byte, 512))
	mustWriteFile(t, filepath.Join(projDir, "Gemfile"), []byte("source 'https://rubygems.org'"))

	s := scanner.NewRubyScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Category == model.CatCache && filepath.Base(r.Path) == ".ruby-lsp" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find .ruby-lsp artifact")
	}
}

func TestRubyScanner_IgnoresWithoutGemfile(t *testing.T) {
	root := t.TempDir()

	// vendor/bundle without Gemfile should be ignored
	bundleDir := filepath.Join(root, "random", "vendor", "bundle")
	mustMkdir(t, bundleDir)

	s := scanner.NewRubyScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results without Gemfile, got %d", len(results))
	}
}

func TestRubyScanner_SkipsNestedArtifacts(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myapp")
	// vendor/bundle containing a nested tmp
	nestedTmp := filepath.Join(projDir, "vendor", "bundle", "tmp")
	mustMkdir(t, nestedTmp)
	mustWriteFile(t, filepath.Join(nestedTmp, "data"), []byte("x"))
	mustWriteFile(t, filepath.Join(projDir, "Gemfile"), []byte("source 'https://rubygems.org'"))

	s := scanner.NewRubyScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	// Should find vendor/bundle but NOT nested tmp inside it
	tmpCount := 0
	for _, r := range results {
		if filepath.Base(r.Path) == "tmp" {
			tmpCount++
		}
	}
	if tmpCount != 0 {
		t.Errorf("expected 0 nested tmp results, got %d", tmpCount)
	}
}

func TestRubyScanner_MultipleProjects(t *testing.T) {
	root := t.TempDir()

	// Project 1: Rails app
	proj1 := filepath.Join(root, "webapp")
	mustMkdir(t, filepath.Join(proj1, "vendor", "bundle"))
	mustMkdir(t, filepath.Join(proj1, "tmp"))
	mustMkdir(t, filepath.Join(proj1, "log"))
	mustWriteFile(t, filepath.Join(proj1, "Gemfile"), []byte("source 'https://rubygems.org'"))

	// Project 2: gem library
	proj2 := filepath.Join(root, "mygem")
	mustMkdir(t, filepath.Join(proj2, "coverage"))
	mustWriteFile(t, filepath.Join(proj2, "Gemfile"), []byte("source 'https://rubygems.org'"))

	s := scanner.NewRubyScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	// proj1: vendor/bundle + tmp + log = 3, proj2: coverage = 1 → total 4
	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
		for _, r := range results {
			t.Logf("  %s %s %s", r.Ecosystem, r.Category, r.Path)
		}
	}
}

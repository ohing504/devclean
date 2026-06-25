package scanner_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

// TestGlobalScanner_DetectsCaches points HOME at a temp dir, seeds a safe and a
// caution cache, and verifies classification + consequence note.
func TestGlobalScanner_DetectsCaches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	npm := filepath.Join(home, ".npm")
	mustMkdir(t, npm)
	mustWriteFile(t, filepath.Join(npm, "cache.json"), make([]byte, 2048))

	pnpm := filepath.Join(home, "Library", "pnpm", "store")
	mustMkdir(t, pnpm)
	mustWriteFile(t, filepath.Join(pnpm, "blob"), make([]byte, 4096))

	s := scanner.NewGlobalScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	byPath := make(map[string]model.ScanResult, len(results))
	for _, r := range results {
		if r.Ecosystem != model.EcoGlobal {
			t.Errorf("expected ecosystem=global, got %s for %s", r.Ecosystem, r.Path)
		}
		byPath[r.Path] = r
	}

	safe, ok := byPath[npm]
	if !ok {
		t.Fatalf("expected ~/.npm to be detected")
	}
	if safe.Safety != model.SafetySafe {
		t.Errorf("~/.npm: expected safety=safe, got %s", safe.Safety)
	}

	caution, ok := byPath[pnpm]
	if !ok {
		t.Fatalf("expected pnpm store to be detected")
	}
	if caution.Safety != model.SafetyCaution {
		t.Errorf("pnpm store: expected safety=caution, got %s", caution.Safety)
	}
	if caution.Recommendation == "" {
		t.Errorf("pnpm store: expected a consequence note in Recommendation, got empty")
	}
}

// TestGlobalScanner_SkipsMissingPaths ensures absent caches yield no results
// (an empty home produces nothing rather than phantom entries).
func TestGlobalScanner_SkipsMissingPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := scanner.NewGlobalScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results for empty home, got %d", len(results))
	}
}

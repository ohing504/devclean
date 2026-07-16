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

// TestGlobalScanner_DetectsExpandedCatalog seeds representative entries from the
// expanded catalog (uv, pipx, AI tools, Cursor cache subdirs) and verifies
// detection, safety classification, and consequence notes. Cursor is checked
// negatively too: only the cache subdirectories may be reported, never
// Application Support/Cursor itself (settings live there).
func TestGlobalScanner_DetectsExpandedCatalog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	uv := filepath.Join(home, ".cache", "uv")
	mustMkdir(t, uv)
	mustWriteFile(t, filepath.Join(uv, "wheel.bin"), make([]byte, 2048))

	pipx := filepath.Join(home, ".local", "pipx")
	mustMkdir(t, pipx)
	mustWriteFile(t, filepath.Join(pipx, "venv.cfg"), make([]byte, 512))

	claudeProjects := filepath.Join(home, ".claude", "projects")
	mustMkdir(t, claudeProjects)
	mustWriteFile(t, filepath.Join(claudeProjects, "session.jsonl"), make([]byte, 1024))

	cursorRoot := filepath.Join(home, "Library", "Application Support", "Cursor")
	cursorCache := filepath.Join(cursorRoot, "Cache")
	mustMkdir(t, cursorCache)
	mustWriteFile(t, filepath.Join(cursorCache, "blob"), make([]byte, 256))
	// Settings live next to the caches and must never be picked up.
	mustWriteFile(t, filepath.Join(cursorRoot, "settings.json"), []byte("{}"))

	s := scanner.NewGlobalScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	byPath := make(map[string]model.ScanResult, len(results))
	for _, r := range results {
		byPath[r.Path] = r
	}

	if r, ok := byPath[uv]; !ok {
		t.Errorf("expected ~/.cache/uv to be detected")
	} else if r.Safety != model.SafetySafe {
		t.Errorf("~/.cache/uv: expected safety=safe, got %s", r.Safety)
	}

	if _, ok := byPath[pipx]; !ok {
		t.Errorf("expected ~/.local/pipx to be detected")
	}

	if r, ok := byPath[claudeProjects]; !ok {
		t.Errorf("expected ~/.claude/projects to be detected")
	} else {
		if r.Safety != model.SafetyCaution {
			t.Errorf("~/.claude/projects: expected safety=caution, got %s", r.Safety)
		}
		if r.Recommendation == "" {
			t.Errorf("~/.claude/projects: expected a consequence note in Recommendation, got empty")
		}
	}

	if _, ok := byPath[cursorCache]; !ok {
		t.Errorf("expected Cursor Cache subdir to be detected")
	}
	if _, ok := byPath[cursorRoot]; ok {
		t.Errorf("Application Support/Cursor itself must not be reported (settings live there)")
	}
}

// TestGlobalScanner_ScopedRootExcludesHomeCaches documents the intended scoping:
// home-global caches are reported only when the scan root contains them (the
// default --path is ~). Scanning a subdirectory excludes them — same behavior
// as the xcode scanner's isUnderRoot guard.
func TestGlobalScanner_ScopedRootExcludesHomeCaches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	npm := filepath.Join(home, ".npm")
	mustMkdir(t, npm)
	mustWriteFile(t, filepath.Join(npm, "cache.json"), make([]byte, 2048))

	// Scan a subdirectory of home, not home itself.
	projects := filepath.Join(home, "projects")
	mustMkdir(t, projects)

	s := scanner.NewGlobalScanner()
	results, err := s.Scan(context.Background(), projects)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results when scanning a subdir of home, got %d", len(results))
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

package scanner_test

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
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
	s.TmpRoot = t.TempDir() // isolate from real code-sign clones on this machine
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
	s.TmpRoot = t.TempDir() // isolate from real code-sign clones on this machine
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

// TestGlobalScanner_DetectsBrowserCodeSignClones injects a fake system temp
// root, seeds a Chrome code-sign clone with two copies plus two non-matching
// neighbors, and verifies exactly the clone dir is reported with a browser
// label, copy count, safe classification, and a consequence note.
func TestGlobalScanner_DetectsBrowserCodeSignClones(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("browser code-sign clones are scanned on darwin only")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	tmpRoot := t.TempDir()
	clone := filepath.Join(tmpRoot, "aa", "bbb", "X", "com.google.Chrome.code_sign_clone")
	mustMkdir(t, filepath.Join(clone, "copy1"))
	mustMkdir(t, filepath.Join(clone, "copy2"))
	mustWriteFile(t, filepath.Join(clone, "copy1", "dummy"), make([]byte, 1024))

	// Negative cases: only the X letter dir and the .code_sign_clone suffix match.
	mustMkdir(t, filepath.Join(tmpRoot, "aa", "bbb", "T", "com.google.Chrome.code_sign_clone", "copy1"))
	mustMkdir(t, filepath.Join(tmpRoot, "aa", "bbb", "X", "com.google.Chrome.savedState"))

	s := scanner.NewGlobalScanner()
	s.TmpRoot = tmpRoot
	// Browser not running → the clones are safe zombies.
	s.ProcessRunning = func(string) bool { return false }
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result (the X-dir clone), got %d: %+v", len(results), results)
	}
	r := results[0]
	if r.Path != clone {
		t.Errorf("expected path %s, got %s", clone, r.Path)
	}
	if r.Safety != model.SafetySafe {
		t.Errorf("expected safety=safe, got %s", r.Safety)
	}
	if !strings.Contains(r.Label, "Chrome") {
		t.Errorf("expected browser name in label, got %q", r.Label)
	}
	if !strings.Contains(r.Label, "2 copies") {
		t.Errorf("expected copy count in label, got %q", r.Label)
	}
	if r.Recommendation == "" {
		t.Errorf("expected a consequence note in Recommendation, got empty")
	}
}

// TestGlobalScanner_CodeSignClonesCautionWhenBrowserRunning stubs the process
// check to report Chrome as running: its clones must be downgraded to caution
// with a "currently running" note (the newest copy may be in use). Unknown
// bundle IDs are caution too, without any process check — their process name
// is unknowable. The stub also records calls to verify the run-state check is
// memoized: one pgrep per browser, even with clones in multiple temp dirs.
func TestGlobalScanner_CodeSignClonesCautionWhenBrowserRunning(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("browser code-sign clones are scanned on darwin only")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	tmpRoot := t.TempDir()
	chrome := filepath.Join(tmpRoot, "aa", "bbb", "X", "com.google.Chrome.code_sign_clone")
	mustMkdir(t, filepath.Join(chrome, "copy1"))
	// Second Chrome clone under another per-user dir — must reuse the memoized check.
	chrome2 := filepath.Join(tmpRoot, "cc", "ddd", "X", "com.google.Chrome.code_sign_clone")
	mustMkdir(t, filepath.Join(chrome2, "copy1"))
	unknown := filepath.Join(tmpRoot, "aa", "bbb", "X", "com.example.Unknown.code_sign_clone")
	mustMkdir(t, filepath.Join(unknown, "copy1"))

	var checked []string
	s := scanner.NewGlobalScanner()
	s.TmpRoot = tmpRoot
	s.ProcessRunning = func(name string) bool {
		checked = append(checked, name)
		return name == "Google Chrome"
	}
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	byPath := make(map[string]model.ScanResult, len(results))
	for _, r := range results {
		byPath[r.Path] = r
	}

	for _, p := range []string{chrome, chrome2} {
		r, ok := byPath[p]
		if !ok {
			t.Fatalf("expected %s to be detected", p)
		}
		if r.Safety != model.SafetyCaution {
			t.Errorf("running Chrome clone %s: expected safety=caution, got %s", p, r.Safety)
		}
		if !strings.Contains(r.Recommendation, "currently running") {
			t.Errorf("running Chrome clone %s: expected a running note, got %q", p, r.Recommendation)
		}
	}

	u, ok := byPath[unknown]
	if !ok {
		t.Fatalf("expected unknown-bundle clone to be detected")
	}
	if u.Safety != model.SafetyCaution {
		t.Errorf("unknown bundle: expected safety=caution, got %s", u.Safety)
	}
	if !strings.Contains(u.Label, "com.example.Unknown") {
		t.Errorf("unknown bundle: expected raw bundle ID in label, got %q", u.Label)
	}
	if u.Recommendation == "" {
		t.Errorf("unknown bundle: expected a consequence note in Recommendation, got empty")
	}

	if len(checked) != 1 || checked[0] != "Google Chrome" {
		t.Errorf("expected exactly one memoized check for %q (never for unknown bundles), got %v", "Google Chrome", checked)
	}
}

// TestGlobalScanner_CodeSignClonesExcludedFromScopedScan mirrors the home-cache
// scoping rule: the system temp path lies outside home, so clones are reported
// only when the scan root covers the home directory (the default scan).
// Scanning a home subdirectory must not surface system temp entries.
func TestGlobalScanner_CodeSignClonesExcludedFromScopedScan(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("browser code-sign clones are scanned on darwin only")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	tmpRoot := t.TempDir()
	mustMkdir(t, filepath.Join(tmpRoot, "aa", "bbb", "X", "com.google.Chrome.code_sign_clone", "copy1"))

	projects := filepath.Join(home, "projects")
	mustMkdir(t, projects)

	s := scanner.NewGlobalScanner()
	s.TmpRoot = tmpRoot
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
	s.TmpRoot = t.TempDir() // isolate from real code-sign clones on this machine
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results for empty home, got %d", len(results))
	}
}

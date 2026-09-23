package scanner_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

	s := newIsolatedGlobalScanner(t)
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

	// Session history / project memory must NEVER be offered for deletion:
	// it is irreplaceable, not reclaimable cache. Seed it and assert below that
	// the scanner does not report it.
	claudeProjects := filepath.Join(home, ".claude", "projects")
	mustMkdir(t, claudeProjects)
	mustWriteFile(t, filepath.Join(claudeProjects, "session.jsonl"), make([]byte, 1024))
	codexDir := filepath.Join(home, ".codex")
	mustMkdir(t, codexDir)
	mustWriteFile(t, filepath.Join(codexDir, "history.jsonl"), make([]byte, 1024))
	geminiDir := filepath.Join(home, ".gemini")
	mustMkdir(t, geminiDir)
	mustWriteFile(t, filepath.Join(geminiDir, "history.jsonl"), make([]byte, 1024))
	// The whole ~/.claude tree is user state, not a deletion target — even its
	// "caches" (plugin cache, shell snapshots) must not be reported.
	claudePluginCache := filepath.Join(home, ".claude", "plugins", "cache")
	mustMkdir(t, claudePluginCache)
	mustWriteFile(t, filepath.Join(claudePluginCache, "blob"), make([]byte, 512))
	claudeCliCache := filepath.Join(home, "Library", "Caches", "claude-cli-nodejs")
	mustMkdir(t, claudeCliCache)
	mustWriteFile(t, filepath.Join(claudeCliCache, "log"), make([]byte, 512))
	// Config roots must not be deletion targets: ~/.gem holds the RubyGems
	// credential, ~/.cursor holds extensions & settings.
	gemDir := filepath.Join(home, ".gem")
	mustMkdir(t, gemDir)
	mustWriteFile(t, filepath.Join(gemDir, "credentials"), make([]byte, 128))
	cursorDir := filepath.Join(home, ".cursor")
	mustMkdir(t, cursorDir)
	mustWriteFile(t, filepath.Join(cursorDir, "settings.json"), []byte("{}"))

	cursorRoot := filepath.Join(home, "Library", "Application Support", "Cursor")
	cursorCache := filepath.Join(cursorRoot, "Cache")
	mustMkdir(t, cursorCache)
	mustWriteFile(t, filepath.Join(cursorCache, "blob"), make([]byte, 256))
	// Settings live next to the caches and must never be picked up.
	mustWriteFile(t, filepath.Join(cursorRoot, "settings.json"), []byte("{}"))

	s := newIsolatedGlobalScanner(t)
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

	// Irreplaceable session history / project memory must never be reported —
	// deleting it is unrecoverable data loss, not reclaimed cache.
	if _, ok := byPath[claudeProjects]; ok {
		t.Errorf("~/.claude/projects (session history/memory) must never be offered for deletion")
	}
	if _, ok := byPath[codexDir]; ok {
		t.Errorf("~/.codex (session history) must never be offered for deletion")
	}
	if _, ok := byPath[geminiDir]; ok {
		t.Errorf("~/.gemini (session history) must never be offered for deletion")
	}
	if _, ok := byPath[claudePluginCache]; ok {
		t.Errorf("~/.claude/** (user state) must never be offered for deletion, incl. its caches")
	}
	if _, ok := byPath[claudeCliCache]; ok {
		t.Errorf("~/Library/Caches/claude-cli-nodejs (Claude Code state) must never be offered for deletion")
	}
	if _, ok := byPath[gemDir]; ok {
		t.Errorf("~/.gem (holds RubyGems credentials) must never be offered for deletion")
	}
	if _, ok := byPath[cursorDir]; ok {
		t.Errorf("~/.cursor (extensions & settings) must never be offered for deletion")
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

	s := newIsolatedGlobalScanner(t)
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

	s := newIsolatedGlobalScanner(t)
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
	s := newIsolatedGlobalScanner(t)
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

	s := newIsolatedGlobalScanner(t)
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

	s := newIsolatedGlobalScanner(t)
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results for empty home, got %d", len(results))
	}
}

// TestGlobalScanner_VendorCleanups stubs PATH lookup so only brew and pip3 are
// "installed", and verifies VendorCleanups offers exactly the pip entry through
// its pip3 fallback. brew is never a vendor cleanup: its cleanup is a scan item.
func TestGlobalScanner_VendorCleanups(t *testing.T) {
	s := newIsolatedGlobalScanner(t, "brew", "pip3")

	vc, ok := any(s).(scanner.VendorCleaner)
	if !ok {
		t.Fatal("GlobalScanner should implement VendorCleaner")
	}
	actions := vc.VendorCleanups()
	if len(actions) != 1 {
		t.Fatalf("expected only the pip action, got %+v", actions)
	}
	pip := actions[0]
	if pip.ID != "pip-cache-purge" || pip.Kind != model.DeleteKindCommand || pip.Run == nil {
		t.Errorf("expected a runnable pip-cache-purge command, got %+v", pip)
	}
	if pip.Display != "pip3 cache purge" {
		t.Errorf("pip entry should use the pip3 fallback executable, got %q", pip.Display)
	}
}

// TestGlobalScanner_InstalledToolSafety: a cache whose owning tool is in PATH
// is in use — deleting it only makes the tool download the same content again
// — so it is raised to caution and --yes skips it. Without the tool the entry
// keeps its declared safety with a note that no installed tool uses it. Entries
// without an owning executable, and entries already declared caution, are
// unaffected by what is installed.
func TestGlobalScanner_InstalledToolSafety(t *testing.T) {
	cases := []struct {
		name      string
		relPath   string
		installed []string
		safety    model.SafetyLevel
		recHas    string
	}{
		{"owning tool installed", ".npm", []string{"npm"}, model.SafetyCaution, "npm is installed"},
		{"owning tool absent", ".npm", nil, model.SafetySafe, "npm not installed"},
		{"fallback executable installed", ".cache/pip", []string{"pip3"}, model.SafetyCaution, "pip3 is installed"},
		{"all candidates absent", ".cache/pip", nil, model.SafetySafe, "pip/pip3 not installed"},
		{"no owning executable", "Library/Caches/electron", []string{"npm", "node"}, model.SafetySafe, ""},
		{"declared caution keeps its note", "Library/pnpm/store", []string{"pnpm"}, model.SafetyCaution, "hard-linked"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, filepath.FromSlash(tc.relPath))
			mustMkdir(t, dir)
			mustWriteFile(t, filepath.Join(dir, "blob"), make([]byte, 1024))

			results, err := newIsolatedGlobalScanner(t, tc.installed...).Scan(context.Background(), home)
			if err != nil {
				t.Fatalf("Scan error: %v", err)
			}
			if len(results) != 1 || results[0].Path != dir {
				t.Fatalf("expected exactly %s, got %+v", dir, results)
			}
			r := results[0]
			if r.Safety != tc.safety {
				t.Errorf("safety = %s, want %s", r.Safety, tc.safety)
			}
			if !strings.Contains(r.Recommendation, tc.recHas) {
				t.Errorf("recommendation = %q, want it to contain %q", r.Recommendation, tc.recHas)
			}
		})
	}
}

// fakeBrew stubs RunCommand as a brew executable: `brew --cache` prints
// cacheDir, `brew cleanup ... --dry-run` prints dryRun (or fails with
// dryRunErr), and every call is recorded so a test can check what a delete
// runs.
type fakeBrew struct {
	cacheDir  string
	dryRun    string
	dryRunErr error
	calls     [][]string // env entries, then "brew", then args
}

func (f *fakeBrew) run(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append(append(append([]string{}, env...), filepath.Base(name)), args...))
	switch {
	case slices.Equal(args, []string{"--cache"}):
		return []byte(f.cacheDir + "\n"), nil
	case slices.Contains(args, "--dry-run"):
		return []byte(f.dryRun), f.dryRunErr
	}
	return nil, nil
}

// brewDryRun is `brew cleanup -s --dry-run` output in the shape Homebrew prints.
func brewDryRun(summarySize string) string {
	out := "Would remove: /opt/homebrew/Cellar/node/24.1.0 (2,345 files, 91.2MB)\n" +
		"Would remove: /Users/me/Library/Caches/Homebrew/downloads/abc--node--24.1.0.bottle.tar.gz (23.1MB)\n"
	if summarySize != "" {
		out += "==> This operation would free approximately " + summarySize + " of disk space.\n"
	}
	return out
}

// TestGlobalScanner_HomebrewCleanupItem: with brew installed, the Homebrew
// cache is reported as one item sized by brew's dry-run estimate (which covers
// old versions outside the cache directory) and deleted by running brew
// cleanup with autoremove disabled — never by removing the directory.
func TestGlobalScanner_HomebrewCleanupItem(t *testing.T) {
	sizes := []struct {
		printed string
		bytes   int64
	}{
		{"4.1GB", 4_100_000_000},
		{"34.6MB", 34_600_000},
		{"1KB", 1_000},
		{"512B", 512},
	}
	for _, sz := range sizes {
		t.Run(sz.printed, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			cache := filepath.Join(home, "Library", "Caches", "Homebrew")
			mustMkdir(t, cache)
			mustWriteFile(t, filepath.Join(cache, "blob"), make([]byte, 4096))

			brew := &fakeBrew{cacheDir: cache, dryRun: brewDryRun(sz.printed)}
			s := newIsolatedGlobalScanner(t, "brew")
			s.RunCommand = brew.run
			results, err := s.Scan(context.Background(), home)
			if err != nil {
				t.Fatalf("Scan error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected only the Homebrew cleanup item, got %+v", results)
			}
			r := results[0]
			if r.Path != cache || r.Size != sz.bytes || r.Safety != model.SafetySafe {
				t.Errorf("got path=%s size=%d safety=%s, want path=%s size=%d safety=safe", r.Path, r.Size, r.Safety, cache, sz.bytes)
			}
			if r.Delete == nil || r.Delete.Kind != model.DeleteKindCommand {
				t.Fatalf("expected a command delete method, got %+v", r.Delete)
			}

			brew.calls = nil
			if err := r.Delete.Run(context.Background()); err != nil {
				t.Fatalf("Delete.Run: %v", err)
			}
			if len(brew.calls) != 1 {
				t.Fatalf("expected one brew command, got %v", brew.calls)
			}
			call := brew.calls[0]
			if !slices.Contains(call, "HOMEBREW_NO_AUTOREMOVE=1") || !strings.HasSuffix(strings.Join(call, " "), "brew cleanup") {
				t.Errorf("delete ran %v, want HOMEBREW_NO_AUTOREMOVE=1 ... brew cleanup", call)
			}
		})
	}
}

// TestGlobalScanner_HomebrewWithoutCleanupItem covers the cases where brew
// yields no cleanup item. The cache directory is then reported as a measured
// path item: caution while brew is installed (brew uses it), carrying the
// error when the dry-run failed, and a plain safe catalog entry without brew.
func TestGlobalScanner_HomebrewWithoutCleanupItem(t *testing.T) {
	cases := []struct {
		name      string
		installed []string
		brew      *fakeBrew
		safety    model.SafetyLevel
		recHas    string
	}{
		{"nothing to free", []string{"brew"}, &fakeBrew{dryRun: brewDryRun("")}, model.SafetyCaution, "brew is installed"},
		{"dry-run fails", []string{"brew"}, &fakeBrew{dryRunErr: errors.New("exit status 1")}, model.SafetyCaution, "exit status 1"},
		{"brew absent", nil, nil, model.SafetySafe, "brew not installed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			cache := filepath.Join(home, "Library", "Caches", "Homebrew")
			mustMkdir(t, cache)
			mustWriteFile(t, filepath.Join(cache, "blob"), make([]byte, 4096))

			s := newIsolatedGlobalScanner(t, tc.installed...)
			if tc.brew != nil {
				tc.brew.cacheDir = cache
				s.RunCommand = tc.brew.run
			}
			results, err := s.Scan(context.Background(), home)
			if err != nil {
				t.Fatalf("Scan error: %v", err)
			}
			if len(results) != 1 || results[0].Path != cache {
				t.Fatalf("expected only %s, got %+v", cache, results)
			}
			r := results[0]
			if r.Delete != nil || r.Size == 0 || r.Safety != tc.safety {
				t.Errorf("got delete=%+v size=%d safety=%s, want a measured path item with safety=%s", r.Delete, r.Size, r.Safety, tc.safety)
			}
			if !strings.Contains(r.Recommendation, tc.recHas) {
				t.Errorf("recommendation = %q, want it to contain %q", r.Recommendation, tc.recHas)
			}
		})
	}
}

// TestGlobalScanner_HomebrewCacheElsewhere: when HOMEBREW_CACHE points outside
// home, the cleanup item is reported at brew's cache path and the directory in
// home is still reported on its own, not hidden behind the item.
func TestGlobalScanner_HomebrewCacheElsewhere(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	homeCache := filepath.Join(home, "Library", "Caches", "Homebrew")
	mustMkdir(t, homeCache)
	mustWriteFile(t, filepath.Join(homeCache, "blob"), make([]byte, 4096))
	external := t.TempDir()

	s := newIsolatedGlobalScanner(t, "brew")
	s.RunCommand = (&fakeBrew{cacheDir: external, dryRun: brewDryRun("1.2GB")}).run
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	byPath := make(map[string]model.ScanResult, len(results))
	for _, r := range results {
		byPath[r.Path] = r
	}
	if r, ok := byPath[external]; !ok || r.Delete == nil {
		t.Errorf("expected the cleanup item at %s, got %+v", external, results)
	}
	if r, ok := byPath[homeCache]; !ok || r.Delete != nil || r.Safety != model.SafetyCaution {
		t.Errorf("expected %s as a caution path item, got %+v", homeCache, results)
	}
}

// TestGlobalScanner_FindsToolOutsidePATH: a scan started without the user's
// login PATH (launchd, cron, an agent's non-login shell) must still see tools
// installed to user-level bin directories, or their in-use caches would be
// reported as safe.
func TestGlobalScanner_FindsToolOutsidePATH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")
	mustMkdir(t, filepath.Join(home, ".bun", "bin"))
	mustWriteFile(t, filepath.Join(home, ".bun", "bin", "bun"), []byte("#!/bin/sh\n"))
	if err := os.Chmod(filepath.Join(home, ".bun", "bin", "bun"), 0o755); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(home, ".bun", "install", "cache")
	mustMkdir(t, cache)
	mustWriteFile(t, filepath.Join(cache, "blob"), make([]byte, 1024))

	s := scanner.NewGlobalScanner()
	s.TmpRoot = t.TempDir()
	// brew in the Homebrew prefix is found too on a machine that has it;
	// fail its commands so the result does not depend on the host.
	s.RunCommand = func(context.Context, []string, string, ...string) ([]byte, error) {
		return nil, errors.New("not run in tests")
	}
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	for _, r := range results {
		if r.Path == cache {
			if r.Safety != model.SafetyCaution || !strings.Contains(r.Recommendation, "bun is installed") {
				t.Errorf("got safety=%s rec=%q, want caution with a bun note", r.Safety, r.Recommendation)
			}
			return
		}
	}
	t.Fatalf("expected %s to be detected, got %+v", cache, results)
}

// newIsolatedGlobalScanner returns a GlobalScanner that sees only the given
// tools as installed, finds no code-sign clones or running browsers, and fails
// the test on any external command, so results never depend on the host.
func newIsolatedGlobalScanner(t *testing.T, installed ...string) *scanner.GlobalScanner {
	t.Helper()
	s := scanner.NewGlobalScanner()
	s.TmpRoot = t.TempDir()
	s.ProcessRunning = func(string) bool { return false }
	s.LookPath = func(file string) (string, error) {
		if slices.Contains(installed, file) {
			return "/usr/local/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	}
	s.RunCommand = func(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
		t.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		return nil, errors.New("unexpected command")
	}
	return s
}

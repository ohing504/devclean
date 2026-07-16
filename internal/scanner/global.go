package scanner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// globalCache describes a shared, home-rooted cache that is not tied to any
// single project. Unlike the per-project scanners, these paths are fixed
// (home-relative) and span package managers and dev tools across ecosystems.
//
// rec carries the consequence-of-deletion note surfaced as ScanResult.Recommendation:
// for caution entries it states what breaks ("re-downloaded on next install"),
// so a user — or an AI agent reading --json — can decide without external knowledge.
//
// Paths owned by a dedicated scanner are intentionally excluded to avoid
// double-counting: Xcode's DerivedData / DeviceSupport / Archives / CoreSimulator
// belong to the xcode ecosystem, not here.
type globalCache struct {
	relPath     string
	category    model.Category
	safety      model.SafetyLevel
	description string
	rec         string
}

// globalCaches lists shared caches as home-relative paths. Entries whose path
// does not exist on the current platform are skipped at scan time, so macOS
// (~/Library/Caches/*) and Linux (~/.cache/*) variants can coexist here.
var globalCaches = []globalCache{
	// --- Package managers (cross-platform) ---
	{".npm", model.CatCache, model.SafetySafe, "npm package cache", ""},
	{".bun/install/cache", model.CatCache, model.SafetySafe, "Bun install cache", ""},
	{".gradle/caches", model.CatCache, model.SafetyCaution, "Gradle dependency & build cache", "Gradle re-downloads dependencies and rebuilds on next run"},
	{".gradle/wrapper/dists", model.CatCache, model.SafetyCaution, "Gradle wrapper distributions", "Gradle re-downloads its distribution on next run"},
	{".cargo/registry", model.CatCache, model.SafetyCaution, "Cargo registry cache", "Cargo re-downloads crate sources on next build"},
	{".cargo/git", model.CatCache, model.SafetyCaution, "Cargo git dependency cache", "Cargo re-clones git dependencies on next build"},
	{"go/pkg/mod", model.CatDeps, model.SafetyCaution, "Go module cache", "use 'go clean -modcache' — read-only files make --force fail; Go re-downloads on next build"},
	{".rustup/toolchains", model.CatRuntime, model.SafetyCaution, "Rust toolchains", "installed toolchains must be reinstalled with 'rustup toolchain install'"},
	{".nvm/versions", model.CatRuntime, model.SafetyCaution, "nvm-installed Node.js runtimes", "deletes installed Node.js versions, not a cache — reinstall with 'nvm install'"},
	{".pyenv/versions", model.CatRuntime, model.SafetyCaution, "pyenv-installed Python runtimes", "deletes installed Python versions, not a cache — reinstall with 'pyenv install'"},
	{".rbenv/versions", model.CatRuntime, model.SafetyCaution, "rbenv-installed Ruby runtimes", "deletes installed Ruby versions, not a cache — reinstall with 'rbenv install'"},
	{".local/pipx", model.CatDeps, model.SafetyCaution, "pipx-installed CLI tools", "deletes the installed CLI tools themselves — each must be reinstalled with 'pipx install'"},
	{".m2/repository", model.CatDeps, model.SafetyCaution, "Maven local repository", "shared by every Maven project; dependencies re-download on next build"},
	{".gem", model.CatCache, model.SafetySafe, "RubyGems cache", ""},
	{".cocoapods", model.CatCache, model.SafetySafe, "CocoaPods spec repos", ""},
	// uv and Puppeteer (v19+) use the XDG ~/.cache path on macOS too, so these
	// live here rather than in the Linux section.
	{".cache/uv", model.CatCache, model.SafetySafe, "uv package cache", ""},
	{".cache/puppeteer", model.CatCache, model.SafetySafe, "Puppeteer browser cache", ""},

	// --- Package managers / dev tools (macOS) ---
	{"Library/Caches/Yarn", model.CatCache, model.SafetySafe, "Yarn cache", ""},
	{"Library/pnpm/store", model.CatCache, model.SafetyCaution, "pnpm content-addressable store", "every project re-downloads dependencies on next install (store is hard-linked)"},
	{"Library/Caches/pnpm", model.CatCache, model.SafetySafe, "pnpm download cache", ""},
	{"Library/Caches/pip", model.CatCache, model.SafetySafe, "pip download cache", ""},
	{"Library/Caches/Homebrew", model.CatCache, model.SafetySafe, "Homebrew download cache", ""},
	{"Library/Caches/CocoaPods", model.CatCache, model.SafetySafe, "CocoaPods spec & pod cache", ""},
	{"Library/Caches/go-build", model.CatCache, model.SafetySafe, "Go build cache", ""},
	{"Library/Caches/ms-playwright", model.CatCache, model.SafetyCaution, "Playwright browser binaries", "Playwright re-downloads browsers on next install"},
	{"Library/Caches/electron", model.CatCache, model.SafetySafe, "Electron binary cache", ""},
	{"Library/Caches/node-gyp", model.CatCache, model.SafetySafe, "node-gyp header cache", ""},
	{"Library/Caches/typescript", model.CatCache, model.SafetySafe, "TypeScript installer cache", ""},
	{"Library/Caches/uv", model.CatCache, model.SafetySafe, "uv package cache", ""},
	{"Library/Caches/Cypress", model.CatCache, model.SafetySafe, "Cypress binary cache", ""},
	{"Library/Caches/deno", model.CatCache, model.SafetySafe, "Deno cache", ""},
	{"Library/Caches/pypoetry", model.CatCache, model.SafetySafe, "Poetry cache", ""},

	// --- Dev tools (Linux ~/.cache equivalents) ---
	{".cache/go-build", model.CatCache, model.SafetySafe, "Go build cache", ""},
	{".cache/pip", model.CatCache, model.SafetySafe, "pip download cache", ""},
	{".cache/ms-playwright", model.CatCache, model.SafetyCaution, "Playwright browser binaries", "Playwright re-downloads browsers on next install"},
	{".cache/node-gyp", model.CatCache, model.SafetySafe, "node-gyp header cache", ""},
	{".cache/yarn", model.CatCache, model.SafetySafe, "Yarn cache", ""},
	{".cache/pnpm", model.CatCache, model.SafetySafe, "pnpm download cache", ""},
	{".cache/electron", model.CatCache, model.SafetySafe, "Electron binary cache", ""},
	{".cache/Cypress", model.CatCache, model.SafetySafe, "Cypress binary cache", ""},
	{".cache/deno", model.CatCache, model.SafetySafe, "Deno cache", ""},
	{".cache/pypoetry", model.CatCache, model.SafetySafe, "Poetry cache", ""},

	// --- Android SDK (home-rooted, large; no dedicated scanner yet) ---
	{".android/avd", model.CatRuntime, model.SafetyCaution, "Android Virtual Devices", "emulator AVDs and their user data must be recreated"},
	{"Library/Android/sdk/system-images", model.CatRuntime, model.SafetyCaution, "Android emulator system images", "emulators won't start until images are re-downloaded"},
	{"Library/Android/sdk/ndk", model.CatDeps, model.SafetyCaution, "Android NDK installations", "NDK is re-downloaded on next native build"},
	{"Library/Android/sdk/build-tools", model.CatRuntime, model.SafetyCaution, "Android SDK build tools", "builds fail until build-tools are re-downloaded via the SDK manager"},

	// --- AI tools ---
	{".claude/projects", model.CatCache, model.SafetyCaution, "Claude Code session history & project memory", "Claude Code session history/memory — deleting breaks --resume and project memory"},
	{".claude/plugins/cache", model.CatCache, model.SafetySafe, "Claude Code plugin cache", ""},
	{".claude/shell-snapshots", model.CatCache, model.SafetySafe, "Claude Code shell snapshots", ""},
	{"Library/Caches/claude-cli-nodejs", model.CatCache, model.SafetySafe, "Claude Code CLI cache", ""},
	{".codex", model.CatCache, model.SafetyCaution, "Codex CLI data", "contains session history, not just caches"},
	{".gemini", model.CatCache, model.SafetyCaution, "Gemini CLI data", "contains session history, not just caches"},
	{".cursor", model.CatCache, model.SafetyCaution, "Cursor extensions & settings", "contains installed extensions and settings — Cursor must reinstall them"},
	// Only Cursor's cache subdirectories — Application Support/Cursor itself
	// holds settings and must never be offered for deletion.
	{"Library/Application Support/Cursor/Cache", model.CatCache, model.SafetySafe, "Cursor cache", ""},
	{"Library/Application Support/Cursor/CachedData", model.CatCache, model.SafetySafe, "Cursor cached data", ""},
	{"Library/Application Support/Cursor/Code Cache", model.CatCache, model.SafetySafe, "Cursor code cache", ""},
}

// GlobalScanner scans for shared, home-rooted developer caches that are not
// owned by any single project (package-manager and dev-tool caches). On macOS
// it additionally reports leftover browser code-sign clones under the per-user
// system temp root.
type GlobalScanner struct {
	// TmpRoot is the per-user system temp root searched for browser
	// code-sign clones (macOS: /private/var/folders). A field so tests can
	// point it at a fixture directory.
	TmpRoot string
	// ProcessRunning reports whether a process with exactly the given name
	// is currently running (default: pgrep -x). A field so tests can stub
	// browser run state.
	ProcessRunning func(processName string) bool
}

func NewGlobalScanner() *GlobalScanner {
	return &GlobalScanner{
		TmpRoot:        "/private/var/folders",
		ProcessRunning: processRunning,
	}
}

// processRunning reports whether a process with exactly the given name is
// running, via pgrep -x. Any failure (pgrep missing, no match) counts as not
// running.
func processRunning(name string) bool {
	return exec.Command("pgrep", "-x", name).Run() == nil
}

func (s *GlobalScanner) Name() string               { return "global" }
func (s *GlobalScanner) Ecosystem() model.Ecosystem { return model.EcoGlobal }

func (s *GlobalScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	var results []model.ScanResult
	for _, c := range globalCaches {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		full := filepath.Join(home, c.relPath)
		if !isUnderRoot(full, absRoot) {
			continue
		}
		info, err := os.Stat(full)
		if err != nil || !info.IsDir() {
			continue
		}

		results = append(results, model.ScanResult{
			Path:           full,
			Ecosystem:      model.EcoGlobal,
			Category:       c.category,
			Size:           DirSize(full),
			LastMod:        ModTime(full),
			Safety:         c.safety,
			ProjectRoot:    filepath.Dir(full),
			Label:          c.description,
			Recommendation: c.rec,
		})
		ReportProgress(ctx, len(results))
	}

	// Browser code-sign clones live outside home (per-user system temp), so
	// isUnderRoot on the path itself would never match. Gate them the same
	// way as the home caches: include only when the scan root covers the
	// home directory (the default scan root is ~). A --path scan of a home
	// subdirectory must not surface system temp entries.
	if runtime.GOOS == "darwin" && isUnderRoot(home, absRoot) {
		results = append(results, s.scanCodeSignClones(ctx, len(results))...)
	}
	return results, nil
}

// Recommendation notes for code-sign clones, keyed by browser run state:
// clones of a browser that is not running are true zombies (safe); while the
// browser runs, its newest copy may be in use (caution); for bundle IDs we
// cannot map to a process name the run state is unknowable (caution).
const (
	codeSignCloneRec        = "zombie copies from killed browser processes (headless automation like lighthouse/puppeteer); the browser cleans these on next normal exit. Size may overstate if copies are APFS clones."
	codeSignCloneRunningRec = "browser is currently running — newest copy may be in use; it cleans up leftovers on normal exit"
	codeSignCloneUnknownRec = "unrecognized browser — cannot check whether it is running (newest copy may be in use); it cleans up leftovers on normal exit"
)

// codeSignCloneBrowser describes a known Chromium-family browser: the display
// name used in labels and the process name checked (pgrep -x) to tell whether
// the browser is currently running.
type codeSignCloneBrowser struct {
	displayName string
	processName string
}

// codeSignCloneBrowsers maps Chromium-family bundle IDs to browser info.
// Unknown bundle IDs fall back to the raw bundle ID and a caution rating, so
// detection is never limited to this list — matching is done by the
// *.code_sign_clone glob.
var codeSignCloneBrowsers = map[string]codeSignCloneBrowser{
	"com.google.Chrome":          {"Chrome", "Google Chrome"},
	"com.google.Chrome.canary":   {"Chrome Canary", "Google Chrome Canary"},
	"com.brave.Browser":          {"Brave", "Brave Browser"},
	"com.microsoft.edgemac":      {"Edge", "Microsoft Edge"},
	"company.thebrowser.Browser": {"Arc", "Arc"},
	"com.vivaldi.Vivaldi":        {"Vivaldi", "Vivaldi"},
	"org.chromium.Chromium":      {"Chromium", "Chromium"},
	"com.naver.Whale":            {"Whale", "Whale"},
}

// scanCodeSignClones finds leftover browser code-sign clones under the
// per-user system temp root (macOS only). Chromium-family browsers copy their
// own bundle to /private/var/folders/<xx>/<yyy>/X/<bundle-id>.code_sign_clone/
// on launch to verify their code signature and remove the copy on normal exit.
// Force-killed processes — typically headless automation such as lighthouse or
// puppeteer — leave the copies behind, and they accumulate (observed in the
// wild: 92 copies / 156 GB).
//
// found is the number of results already reported so progress keeps counting up.
//
// Safety depends on run state: clones of a browser that is not running are
// safe zombies; while the browser runs (checked once per browser via
// ProcessRunning) or when the bundle ID is unrecognized, they are caution.
//
// Sizes come from du (DirSize) and may overstate real disk usage when the
// copies are APFS clones of the installed app bundle; clone-aware measurement
// is a planned follow-up.
func (s *GlobalScanner) scanCodeSignClones(ctx context.Context, found int) []model.ScanResult {
	matches, err := filepath.Glob(filepath.Join(s.TmpRoot, "*", "*", "X", "*.code_sign_clone"))
	if err != nil {
		return nil
	}

	// Memoize run-state checks: one pgrep per browser, not per clone dir.
	running := make(map[string]bool)
	isRunning := func(processName string) bool {
		if v, ok := running[processName]; ok {
			return v
		}
		v := s.ProcessRunning(processName)
		running[processName] = v
		return v
	}

	var out []model.ScanResult
	for _, m := range matches {
		select {
		case <-ctx.Done():
			return out
		default:
		}
		info, err := os.Stat(m)
		if err != nil || !info.IsDir() {
			continue
		}

		bundleID := strings.TrimSuffix(filepath.Base(m), ".code_sign_clone")
		browser, known := codeSignCloneBrowsers[bundleID]

		// Default: unrecognized bundle — no process name to check, so the
		// run state is unknowable; stay conservative.
		name := bundleID
		safety := model.SafetyCaution
		rec := codeSignCloneUnknownRec
		if known {
			name = browser.displayName
			if isRunning(browser.processName) {
				rec = codeSignCloneRunningRec
			} else {
				safety = model.SafetySafe
				rec = codeSignCloneRec
			}
		}

		out = append(out, model.ScanResult{
			Path:           m,
			Ecosystem:      model.EcoGlobal,
			Category:       model.CatCache,
			Size:           DirSize(m),
			LastMod:        ModTime(m),
			Safety:         safety,
			ProjectRoot:    filepath.Dir(m),
			Label:          codeSignCloneLabel(m, name),
			Recommendation: rec,
		})
		ReportProgress(ctx, found+len(out))
	}
	return out
}

// codeSignCloneLabel derives e.g. "Chrome code-sign clones (92 copies)" from
// the clone directory: browser display name plus the number of immediate child
// directories (one copy per killed process).
func codeSignCloneLabel(dir, name string) string {
	copies := 0
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				copies++
			}
		}
	}
	unit := "copies"
	if copies == 1 {
		unit = "copy"
	}
	return fmt.Sprintf("%s code-sign clones (%d %s)", name, copies, unit)
}

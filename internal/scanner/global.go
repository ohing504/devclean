package scanner

import (
	"context"
	"os"
	"path/filepath"

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
// owned by any single project (package-manager and dev-tool caches).
type GlobalScanner struct{}

func NewGlobalScanner() *GlobalScanner {
	return &GlobalScanner{}
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
	return results, nil
}

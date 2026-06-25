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
	{"go/pkg/mod", model.CatDeps, model.SafetyCaution, "Go module cache", "Go re-downloads all modules on next build (files are read-only)"},

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

	// --- Dev tools (Linux ~/.cache equivalents) ---
	{".cache/go-build", model.CatCache, model.SafetySafe, "Go build cache", ""},
	{".cache/pip", model.CatCache, model.SafetySafe, "pip download cache", ""},
	{".cache/ms-playwright", model.CatCache, model.SafetyCaution, "Playwright browser binaries", "Playwright re-downloads browsers on next install"},
	{".cache/node-gyp", model.CatCache, model.SafetySafe, "node-gyp header cache", ""},
	{".cache/yarn", model.CatCache, model.SafetySafe, "Yarn cache", ""},
	{".cache/pnpm", model.CatCache, model.SafetySafe, "pnpm download cache", ""},
	{".cache/electron", model.CatCache, model.SafetySafe, "Electron binary cache", ""},

	// --- Android SDK (home-rooted, large; no dedicated scanner yet) ---
	{".android/avd", model.CatRuntime, model.SafetyCaution, "Android Virtual Devices", "emulator AVDs and their user data must be recreated"},
	{"Library/Android/sdk/system-images", model.CatRuntime, model.SafetyCaution, "Android emulator system images", "emulators won't start until images are re-downloaded"},
	{"Library/Android/sdk/ndk", model.CatDeps, model.SafetyCaution, "Android NDK installations", "NDK is re-downloaded on next native build"},
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

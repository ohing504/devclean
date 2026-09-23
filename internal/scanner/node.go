package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ohing504/devclean/internal/model"
)

// nodeWalkEcosystem drives Node.js scanning in the single-pass walk engine.
var nodeWalkEcosystem = walkEcosystem{
	Name:    "node",
	Eco:     model.EcoNode,
	Markers: []string{"package.json"},
	Rules: []artifactRule{
		{RelPath: "node_modules", Category: model.CatDeps, Safety: model.SafetySafe, Recommend: pnpmStoreNote}, // NPM dependencies
		{RelPath: ".next", Category: model.CatBuild, Safety: model.SafetySafe},                                 // Next.js build cache
		{RelPath: ".nuxt", Category: model.CatBuild, Safety: model.SafetySafe},                                 // Nuxt.js build cache
		{RelPath: ".output", Category: model.CatBuild, Safety: model.SafetySafe},                               // Nuxt 3 output
		{RelPath: "dist", Category: model.CatBuild, Safety: model.SafetySafe},                                  // Build output
		{RelPath: ".turbo", Category: model.CatCache, Safety: model.SafetySafe},                                // Turborepo cache
		{RelPath: ".parcel-cache", Category: model.CatCache, Safety: model.SafetySafe},                         // Parcel cache
		{RelPath: "coverage", Category: model.CatBuild, Safety: model.SafetySafe},                              // Test coverage reports
		{RelPath: ".svelte-kit", Category: model.CatBuild, Safety: model.SafetySafe},                           // SvelteKit cache
	},
	ExtraRules: nodeExtraRules,
}

// pnpmStoreNote flags a node_modules installed by pnpm whose files likely
// share disk blocks with the pnpm store: pnpm clones (macOS) or hard-links
// (Linux) store files by default, so deleting node_modules alone frees little
// until the store drops the now-unreferenced packages. pnpm records the store
// it imported from in node_modules/.modules.yaml (a pnpm-lock.yaml alone does
// not prove pnpm populated node_modules). A store on another volume can be
// neither cloned nor hard-linked, so pnpm copies and no note is given; the
// packageImportMethod=copy setting is not recorded and cannot be detected.
func pnpmStoreNote(dir string) string {
	storeDir := modulesYAMLStoreDir(filepath.Join(dir, ".modules.yaml"))
	if storeDir == "" || !sameVolume(dir, storeDir) {
		return ""
	}
	return "files likely shared with pnpm store — run `pnpm store prune` after deleting to free space"
}

// modulesYAMLStoreDir returns the top-level storeDir value of a pnpm
// .modules.yaml, or "" when the file or key is missing.
func modulesYAMLStoreDir(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "storeDir:"); ok {
			return strings.Trim(strings.TrimSpace(v), `'"`)
		}
	}
	return ""
}

// sameVolume reports whether both paths exist on the same device.
func sameVolume(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	sa, okA := ia.Sys().(*syscall.Stat_t)
	sb, okB := ib.Sys().(*syscall.Stat_t)
	return okA && okB && sa.Dev == sb.Dev
}

// reactNativeRules are added to a Node project context when the project is
// detected as React Native. They live at multi-segment paths inside the
// project root (ios/Pods) or are RN tooling caches (.expo, .metro).
var reactNativeRules = []artifactRule{
	{RelPath: "ios/Pods", Category: model.CatDeps, Safety: model.SafetySafe},         // CocoaPods dependencies
	{RelPath: "ios/build", Category: model.CatBuild, Safety: model.SafetySafe},       // iOS build output
	{RelPath: "ios/DerivedData", Category: model.CatBuild, Safety: model.SafetySafe}, // iOS DerivedData
	{RelPath: "android/build", Category: model.CatBuild, Safety: model.SafetySafe},   // Android build output
	{RelPath: "android/.gradle", Category: model.CatCache, Safety: model.SafetySafe}, // Android Gradle cache
	{RelPath: ".expo", Category: model.CatCache, Safety: model.SafetySafe},           // Expo cache
	{RelPath: ".metro", Category: model.CatCache, Safety: model.SafetySafe},          // Metro bundler cache
}

var reactNativeMarkers = []string{
	"metro.config.js",
	"metro.config.ts",
	"metro.config.cjs",
	"metro.config.mjs",
}

// nodeExtraRules adds React Native artifact rules to Node projects that
// carry an RN marker (ios/Podfile or a metro config file).
func nodeExtraRules(projectRoot string, entryNames map[string]bool) []artifactRule {
	if !isReactNativeProject(projectRoot, entryNames) {
		return nil
	}
	return reactNativeRules
}

func isReactNativeProject(dir string, entryNames map[string]bool) bool {
	if hasFile(filepath.Join(dir, "ios"), "Podfile") {
		return true
	}
	for _, marker := range reactNativeMarkers {
		if entryNames[marker] {
			return true
		}
	}
	return false
}

func hasFile(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

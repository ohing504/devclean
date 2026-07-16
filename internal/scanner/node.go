package scanner

import (
	"os"
	"path/filepath"

	"github.com/ohing504/devclean/internal/model"
)

// nodeWalkEcosystem drives Node.js scanning in the single-pass walk engine.
var nodeWalkEcosystem = walkEcosystem{
	Name:    "node",
	Eco:     model.EcoNode,
	Markers: []string{"package.json"},
	Rules: []artifactRule{
		{RelPath: "node_modules", Category: model.CatDeps, Safety: model.SafetySafe},   // NPM dependencies
		{RelPath: ".next", Category: model.CatBuild, Safety: model.SafetySafe},         // Next.js build cache
		{RelPath: ".nuxt", Category: model.CatBuild, Safety: model.SafetySafe},         // Nuxt.js build cache
		{RelPath: ".output", Category: model.CatBuild, Safety: model.SafetySafe},       // Nuxt 3 output
		{RelPath: "dist", Category: model.CatBuild, Safety: model.SafetySafe},          // Build output
		{RelPath: ".turbo", Category: model.CatCache, Safety: model.SafetySafe},        // Turborepo cache
		{RelPath: ".parcel-cache", Category: model.CatCache, Safety: model.SafetySafe}, // Parcel cache
		{RelPath: "coverage", Category: model.CatBuild, Safety: model.SafetySafe},      // Test coverage reports
		{RelPath: ".svelte-kit", Category: model.CatBuild, Safety: model.SafetySafe},   // SvelteKit cache
	},
	ExtraRules: nodeExtraRules,
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

func safetyFromDef(a model.ArtifactDef) model.SafetyLevel {
	if a.AlwaysSafe {
		return model.SafetySafe
	}
	return model.SafetyCaution
}

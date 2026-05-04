package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

var nodeArtifacts = []model.ArtifactDef{
	{Pattern: "node_modules", Category: model.CatDeps, Description: "NPM dependencies", AlwaysSafe: true},
	{Pattern: ".next", Category: model.CatBuild, Description: "Next.js build cache", AlwaysSafe: true},
	{Pattern: ".nuxt", Category: model.CatBuild, Description: "Nuxt.js build cache", AlwaysSafe: true},
	{Pattern: ".output", Category: model.CatBuild, Description: "Nuxt 3 output", AlwaysSafe: true},
	{Pattern: "dist", Category: model.CatBuild, Description: "Build output", AlwaysSafe: true},
	{Pattern: ".turbo", Category: model.CatCache, Description: "Turborepo cache", AlwaysSafe: true},
	{Pattern: ".parcel-cache", Category: model.CatCache, Description: "Parcel cache", AlwaysSafe: true},
	{Pattern: "coverage", Category: model.CatBuild, Description: "Test coverage reports", AlwaysSafe: true},
	{Pattern: ".svelte-kit", Category: model.CatBuild, Description: "SvelteKit cache", AlwaysSafe: true},
}

// React Native artifacts live at multi-segment paths inside the project root.
// Discovered only when the project is detected as React Native (Podfile / metro.config).
var reactNativeArtifacts = []model.ArtifactDef{
	{Pattern: "ios/Pods", Category: model.CatDeps, Description: "CocoaPods dependencies (React Native)", AlwaysSafe: true},
	{Pattern: "ios/build", Category: model.CatBuild, Description: "iOS build output (React Native)", AlwaysSafe: true},
	{Pattern: "ios/DerivedData", Category: model.CatBuild, Description: "iOS DerivedData (React Native)", AlwaysSafe: true},
	{Pattern: "android/build", Category: model.CatBuild, Description: "Android build output (React Native)", AlwaysSafe: true},
	{Pattern: "android/.gradle", Category: model.CatCache, Description: "Android Gradle cache (React Native)", AlwaysSafe: true},
	{Pattern: ".expo", Category: model.CatCache, Description: "Expo cache", AlwaysSafe: true},
	{Pattern: ".metro", Category: model.CatCache, Description: "Metro bundler cache", AlwaysSafe: true},
}

var reactNativeMarkers = []string{
	"metro.config.js",
	"metro.config.ts",
	"metro.config.cjs",
	"metro.config.mjs",
}

// NodeScanner scans for Node.js project artifacts.
type NodeScanner struct{}

func NewNodeScanner() *NodeScanner {
	return &NodeScanner{}
}

func (s *NodeScanner) Name() string               { return "node" }
func (s *NodeScanner) Ecosystem() model.Ecosystem { return model.EcoNode }

func (s *NodeScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	var results []model.ScanResult
	artifactSet := make(map[string]model.ArtifactDef, len(nodeArtifacts))
	for _, a := range nodeArtifacts {
		artifactSet[a.Pattern] = a
	}

	skipPaths := make(map[string]bool)
	rnChecked := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !d.IsDir() {
			return nil
		}

		// Skip already-found artifact subtrees (and the artifact dir itself when revisited).
		for skip := range skipPaths {
			if path == skip || strings.HasPrefix(path, skip+string(os.PathSeparator)) {
				return fs.SkipDir
			}
		}

		// On first visit to a Node project directory, check if it's React Native and
		// register multi-segment artifacts (ios/Pods, android/build, etc.) directly.
		if !rnChecked[path] && hasFile(path, "package.json") {
			rnChecked[path] = true
			if isReactNativeProject(path) {
				for _, art := range reactNativeArtifacts {
					full := filepath.Join(path, art.Pattern)
					info, statErr := os.Stat(full)
					if statErr != nil || !info.IsDir() {
						continue
					}
					results = append(results, model.ScanResult{
						Path:      full,
						Ecosystem: model.EcoNode,
						Category:  art.Category,
						Size:      DirSize(full),
						LastMod:   ModTime(full),
						Safety:    safetyFromDef(art),
					})
					ReportProgress(ctx, len(results))
					skipPaths[full] = true
				}
			}
		}

		// Skip hidden dirs that aren't artifacts we care about
		name := d.Name()
		if strings.HasPrefix(name, ".") && artifactSet[name].Pattern == "" {
			return fs.SkipDir
		}

		// Check if this directory is a known artifact inside a Node project
		if art, ok := artifactSet[name]; ok {
			projectDir := filepath.Dir(path)
			if hasFile(projectDir, "package.json") {
				size := DirSize(path)
				results = append(results, model.ScanResult{
					Path:      path,
					Ecosystem: model.EcoNode,
					Category:  art.Category,
					Size:      size,
					LastMod:   ModTime(path),
					Safety:    safetyFromDef(art),
				})
				ReportProgress(ctx, len(results))
				skipPaths[path] = true
				return fs.SkipDir
			}
		}

		return nil
	})

	return results, err
}

func isReactNativeProject(dir string) bool {
	if hasFile(filepath.Join(dir, "ios"), "Podfile") {
		return true
	}
	for _, marker := range reactNativeMarkers {
		if hasFile(dir, marker) {
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

package scanner_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestNodeWalk(t *testing.T) {
	node := []model.Ecosystem{model.EcoNode}
	safe := func(path string, cat model.Category) artifact {
		return artifact{path, model.EcoNode, cat, model.SafetySafe, ""}
	}

	runWalkCases(t, []walkCase{
		{
			name: "artifacts of sibling projects",
			ecos: node,
			tree: []string{
				"app1/package.json", "app1/node_modules/lodash/index.js",
				"app2/package.json", "app2/node_modules/", "app2/.next/cache/data.json",
			},
			want: []artifact{
				safe("app1/node_modules", model.CatDeps),
				safe("app2/node_modules", model.CatDeps),
				safe("app2/.next", model.CatBuild),
			},
		},
		{
			name: "no package.json, no project",
			ecos: node,
			tree: []string{"random/node_modules/"},
		},
		{
			name: "no descent into a matched node_modules",
			ecos: node,
			tree: []string{"app/package.json", "app/node_modules/pkg/package.json", "app/node_modules/pkg/node_modules/dep/index.js"},
			want: []artifact{safe("app/node_modules", model.CatDeps)},
		},
		{
			name: "react native via ios/Podfile",
			ecos: node,
			tree: []string{
				"rn/package.json", "rn/ios/Podfile",
				"rn/ios/Pods/AFNetworking/lib.a", "rn/android/.gradle/lock", "rn/.expo/settings/data.json",
			},
			want: []artifact{
				safe("rn/ios/Pods", model.CatDeps),
				safe("rn/android/.gradle", model.CatCache),
				safe("rn/.expo", model.CatCache),
			},
		},
		{
			name: "react native via metro config",
			ecos: node,
			tree: []string{"expo/package.json", "expo/metro.config.js", "expo/.metro/cache/blob"},
			want: []artifact{safe("expo/.metro", model.CatCache)},
		},
		{
			name: "ios/ without an RN marker is not react native",
			ecos: node,
			tree: []string{"web/package.json", "web/ios/Pods/stuff/x"},
		},
		{
			name: "no descent into a matched ios/Pods",
			ecos: node,
			tree: []string{
				"rn/package.json", "rn/ios/Podfile",
				"rn/ios/Pods/react-native/package.json", "rn/ios/Pods/react-native/node_modules/x/x.js",
			},
			want: []artifact{safe("rn/ios/Pods", model.CatDeps)},
		},
	})
}

// TestNodeModulesPnpmNote: a node_modules installed by pnpm (it writes
// node_modules/.modules.yaml) shares its file blocks with the pnpm store, so
// deleting it alone frees little. The result must say so; npm/yarn-installed
// node_modules carry no note.
func TestNodeModulesPnpmNote(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"pnpm-app/package.json", "pnpm-app/node_modules/.modules.yaml", "npm-app/package.json", "npm-app/node_modules/lodash/index.js"} {
		full := filepath.Join(root, filepath.FromSlash(p))
		mustMkdir(t, filepath.Dir(full))
		mustWriteFile(t, full, nil)
	}

	results, err := scanner.WalkScan(context.Background(), root, model.EcoNode)
	if err != nil {
		t.Fatalf("WalkScan: %v", err)
	}
	got := make(map[string]string)
	for _, r := range results {
		rel, _ := filepath.Rel(root, r.Path)
		got[filepath.ToSlash(rel)] = r.Recommendation
	}
	if !strings.Contains(got["pnpm-app/node_modules"], "pnpm store prune") {
		t.Errorf("pnpm node_modules recommendation = %q, want a pnpm store prune note", got["pnpm-app/node_modules"])
	}
	if rec, ok := got["npm-app/node_modules"]; !ok || rec != "" {
		t.Errorf("npm node_modules recommendation = %q (found=%v), want empty", rec, ok)
	}
}

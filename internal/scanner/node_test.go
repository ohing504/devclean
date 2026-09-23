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
// node_modules/.modules.yaml with the storeDir it imported from) shares file
// blocks with the pnpm store only when the store is on the same volume — across
// volumes pnpm can neither clone nor hard-link, so it copies. The note appears
// only in the shared case; npm/yarn-installed node_modules carry no note.
func TestNodeModulesPnpmNote(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store", "v10")
	mustMkdir(t, store)
	files := map[string]string{
		"same-volume/package.json":               "",
		"same-volume/node_modules/.modules.yaml": "layoutVersion: 5\nstoreDir: " + store + "\n",
		"other-volume/package.json":              "",
		// /dev is devfs (macOS) or devtmpfs (Linux): never the temp dir's volume.
		"other-volume/node_modules/.modules.yaml": "storeDir: /dev\n",
		"store-gone/package.json":                 "",
		"store-gone/node_modules/.modules.yaml":   "storeDir: " + filepath.Join(root, "missing") + "\n",
		"npm-app/package.json":                    "",
		"npm-app/node_modules/lodash/index.js":    "",
	}
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		mustMkdir(t, filepath.Dir(full))
		mustWriteFile(t, full, []byte(content))
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
	if !strings.Contains(got["same-volume/node_modules"], "pnpm store prune") {
		t.Errorf("same-volume recommendation = %q, want a pnpm store prune note", got["same-volume/node_modules"])
	}
	for _, p := range []string{"other-volume/node_modules", "store-gone/node_modules", "npm-app/node_modules"} {
		if rec, ok := got[p]; !ok || rec != "" {
			t.Errorf("%s recommendation = %q (found=%v), want empty", p, rec, ok)
		}
	}
}

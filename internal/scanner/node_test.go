package scanner_test

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
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
			// Some Pods ship a package.json; the matched ios/Pods is not descended.
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

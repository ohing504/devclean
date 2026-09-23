package scanner_test

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

func TestAndroidWalk(t *testing.T) {
	android := []model.Ecosystem{model.EcoAndroid}
	safe := func(path string, cat model.Category) artifact {
		return artifact{path, model.EcoAndroid, cat, model.SafetySafe, ""}
	}

	runWalkCases(t, []walkCase{
		{
			name: "build and .gradle under build.gradle.kts",
			ecos: android,
			tree: []string{"app/build.gradle.kts", "app/build/outputs/app.apk", "app/.gradle/state"},
			want: []artifact{safe("app/build", model.CatBuild), safe("app/.gradle", model.CatCache)},
		},
		{
			name: "no gradle marker, no project",
			ecos: android,
			tree: []string{"random/build/", "random/.gradle/"},
		},
		{
			// Each module carries its own build.gradle, so one build rule
			// reclaims every module without enumerating module names.
			name: "multi-module",
			ecos: android,
			tree: []string{
				"proj/build.gradle", "proj/build/out",
				"proj/app/build.gradle", "proj/app/build/out",
				"proj/feature/build.gradle", "proj/feature/build/out",
			},
			want: []artifact{
				safe("proj/build", model.CatBuild),
				safe("proj/app/build", model.CatBuild),
				safe("proj/feature/build", model.CatBuild),
			},
		},
		{
			// node's RN rules win android/build (first in table order); android
			// still covers the deeper module build the RN rules don't list.
			name: "react native android/build attributed to node",
			ecos: []model.Ecosystem{model.EcoNode, model.EcoAndroid},
			tree: []string{
				"rn/package.json", "rn/metro.config.js",
				"rn/android/build.gradle", "rn/android/build/out",
				"rn/android/app/build.gradle", "rn/android/app/build/out",
			},
			want: []artifact{
				{"rn/android/build", model.EcoNode, model.CatBuild, model.SafetySafe, ""},
				safe("rn/android/app/build", model.CatBuild),
			},
		},
	})
}

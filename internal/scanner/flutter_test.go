package scanner_test

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

func TestFlutterWalk(t *testing.T) {
	flutter := []model.Ecosystem{model.EcoFlutter}
	build := func(path string) artifact {
		return artifact{path, model.EcoFlutter, model.CatBuild, model.SafetySafe, ""}
	}

	runWalkCases(t, []walkCase{
		{
			name: "build and .dart_tool under pubspec.yaml",
			ecos: flutter,
			tree: []string{"app/pubspec.yaml", "app/build/app/out", "app/.dart_tool/package_config.json"},
			want: []artifact{build("app/build"), build("app/.dart_tool")},
		},
		{
			name: "no pubspec.yaml, no project",
			ecos: flutter,
			tree: []string{"random/build/", "random/.dart_tool/"},
		},
		{
			// The SDK checkout is a git repo of pubspec.yaml roots whose
			// engine/src/flutter/build is committed GN source, not build output.
			name: "sdk checkout excluded",
			ecos: flutter,
			tree: []string{
				"flutter/bin/flutter", "flutter/bin/internal/engine.version", "flutter/pubspec.yaml",
				"flutter/engine/src/flutter/pubspec.yaml", "flutter/engine/src/flutter/build/BUILD.gn",
				"flutter/packages/flutter_tools/pubspec.yaml", "flutter/packages/flutter_tools/.dart_tool/x",
				"app/pubspec.yaml", "app/build/out",
			},
			want: []artifact{build("app/build")},
		},
	})
}

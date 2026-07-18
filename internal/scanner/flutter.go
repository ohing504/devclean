package scanner

import (
	"os"
	"path/filepath"

	"github.com/ohing504/devclean/internal/model"
)

// flutterWalkEcosystem drives Flutter/Dart scanning in the single-pass walk
// engine. `build/` and `.dart_tool/` are exactly what `flutter clean` removes;
// both regenerate on the next build. The global ~/.pub-cache lives in the
// global scanner's catalog, not here (per-project scanners handle local
// artifacts only).
//
// PruneRoot excludes the Flutter SDK checkout itself. The SDK is a git repo
// whose `pubspec.yaml` roots would otherwise match: its `.dart_tool` dirs are
// regenerable noise, but critically its `engine/src/build` and
// `engine/src/flutter/build` are committed GN build-system *source* trees, not
// build output — matching `build` by name would offer real source for deletion
// (and gitignore-aware protection misses it, since committed-clean files are
// not "protected"). Skipping the whole SDK subtree is the only safe answer.
var flutterWalkEcosystem = walkEcosystem{
	Name:      "flutter",
	Eco:       model.EcoFlutter,
	Markers:   []string{"pubspec.yaml"},
	PruneRoot: isFlutterSDKRoot,
	Rules: []artifactRule{
		{RelPath: "build", Category: model.CatBuild, Safety: model.SafetySafe},      // compiled output
		{RelPath: ".dart_tool", Category: model.CatBuild, Safety: model.SafetySafe}, // build_runner / tooling state
	},
}

// isFlutterSDKRoot reports whether dir is a Flutter SDK checkout root. The
// signature is the SDK's own invariant, location-independent bootstrap layout —
// the `flutter` launcher plus the pinned engine version file that the tool
// reads to locate itself — never a hardcoded install path. The names gate keeps
// the stat off every directory that has no `bin/` child.
func isFlutterSDKRoot(dir string, names map[string]bool) bool {
	if !names["bin"] {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "bin", "flutter")); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "bin", "internal", "engine.version"))
	return err == nil
}

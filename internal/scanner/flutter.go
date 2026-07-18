package scanner

import "github.com/ohing504/devclean/internal/model"

// flutterWalkEcosystem drives Flutter/Dart scanning in the single-pass walk
// engine. `build/` and `.dart_tool/` are exactly what `flutter clean` removes;
// both regenerate on the next build. The global ~/.pub-cache lives in the
// global scanner's catalog, not here (per-project scanners handle local
// artifacts only).
var flutterWalkEcosystem = walkEcosystem{
	Name:    "flutter",
	Eco:     model.EcoFlutter,
	Markers: []string{"pubspec.yaml"},
	Rules: []artifactRule{
		{RelPath: "build", Category: model.CatBuild, Safety: model.SafetySafe},      // compiled output
		{RelPath: ".dart_tool", Category: model.CatBuild, Safety: model.SafetySafe}, // build_runner / tooling state
	},
}

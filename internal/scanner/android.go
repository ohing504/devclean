package scanner

import "github.com/ohing504/devclean/internal/model"

// androidWalkEcosystem drives Android/Gradle scanning in the single-pass walk
// engine. Every Gradle module carries its own build.gradle(.kts), so the root
// project and each subproject (app/, feature/, ...) is detected as its own
// project root; the single `build` rule then reclaims every module's output
// without enumerating module names. `.gradle` is the per-project Gradle cache,
// regenerated on the next build.
//
// Scope is per-project only. The shared Gradle user home (~/.gradle/caches),
// AVD images, and NDK/system-images are home-rooted and belong to the Global
// Caches scanner's catalog, not here.
//
// No SDK PruneRoot is needed (unlike Flutter): the Android SDK ships no
// build.gradle, so it never establishes a project context and nothing inside it
// is ever matched. React Native projects nest an android/ Gradle tree, but
// node's RN rules match android/build first in table order, so those artifacts
// attribute to node; this scanner still covers the deeper module builds
// (android/app/build) that the RN rules do not list.
var androidWalkEcosystem = walkEcosystem{
	Name:    "android",
	Eco:     model.EcoAndroid,
	Markers: []string{"build.gradle", "build.gradle.kts"},
	Rules: []artifactRule{
		{RelPath: "build", Category: model.CatBuild, Safety: model.SafetySafe},   // compiled output (gradle clean)
		{RelPath: ".gradle", Category: model.CatCache, Safety: model.SafetySafe}, // per-project Gradle cache
	},
}

package scanner

import "github.com/ohing504/devclean/internal/model"

// rubyWalkEcosystem drives Ruby/Rails scanning in the single-pass walk engine.
var rubyWalkEcosystem = walkEcosystem{
	Name:    "ruby",
	Eco:     model.EcoRuby,
	Markers: []string{"Gemfile"},
	Rules: []artifactRule{
		{RelPath: "vendor/bundle", Category: model.CatDeps, Safety: model.SafetySafe}, // Bundled gems
		{RelPath: ".bundle", Category: model.CatCache, Safety: model.SafetySafe},      // Bundler cache
		{RelPath: "tmp", Category: model.CatCache, Safety: model.SafetySafe},          // Temporary files (bootsnap, cache, pids)
		{RelPath: "log", Category: model.CatBuild, Safety: model.SafetySafe},          // Development and test logs
		{RelPath: "coverage", Category: model.CatBuild, Safety: model.SafetySafe},     // Test coverage reports
		{RelPath: ".ruby-lsp", Category: model.CatCache, Safety: model.SafetySafe},    // Ruby LSP editor cache
	},
}

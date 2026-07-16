package scanner

import "github.com/ohing504/devclean/internal/model"

// rustWalkEcosystem drives Rust/Cargo scanning in the single-pass walk engine.
var rustWalkEcosystem = walkEcosystem{
	Name:    "rust",
	Eco:     model.EcoRust,
	Markers: []string{"Cargo.toml"},
	Rules: []artifactRule{
		{RelPath: "target", Category: model.CatBuild, Safety: model.SafetySafe}, // Rust build artifacts
	},
}

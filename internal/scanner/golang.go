package scanner

import "github.com/ohing504/devclean/internal/model"

// goWalkEcosystem drives Go scanning in the single-pass walk engine.
//
// Per-project Go artifacts only. Global caches (~/.cache/go-build, ~/go/pkg/mod)
// are scanned by the global scanner, since they belong to every Go project on
// the machine and need their own safety story.
var goWalkEcosystem = walkEcosystem{
	Name:    "go",
	Eco:     model.EcoGo,
	Markers: []string{"go.mod"},
	Rules: []artifactRule{
		// vendor/ is caution: `go mod vendor` is an opt-in choice — devs who
		// vendor often do so for offline builds or reproducibility.
		{RelPath: "vendor", Category: model.CatDeps, Safety: model.SafetyCaution}, // Vendored Go modules (regenerate with `go mod vendor`)
	},
}

package scanner_test

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

func TestRustWalk(t *testing.T) {
	rust := []model.Ecosystem{model.EcoRust}
	target := func(path string) artifact {
		return artifact{path, model.EcoRust, model.CatBuild, model.SafetySafe, ""}
	}

	runWalkCases(t, []walkCase{
		{
			name: "target under Cargo.toml",
			ecos: rust,
			tree: []string{"proj/Cargo.toml", "proj/target/debug/proj"},
			want: []artifact{target("proj/target")},
		},
		{
			name: "no Cargo.toml, no project",
			ecos: rust,
			tree: []string{"random/target/"},
		},
		{
			name: "workspace member without its own target",
			ecos: rust,
			tree: []string{"ws/Cargo.toml", "ws/target/debug/bin", "ws/crates/core/Cargo.toml"},
			want: []artifact{target("ws/target")},
		},
	})
}

package scanner_test

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

func TestGoWalk(t *testing.T) {
	goEco := []model.Ecosystem{model.EcoGo}
	// vendor/ is caution: vendoring is an opt-in the project committed to.
	vendor := func(path string) artifact {
		return artifact{path, model.EcoGo, model.CatDeps, model.SafetyCaution, ""}
	}

	runWalkCases(t, []walkCase{
		{
			name: "vendor of sibling modules",
			ecos: goEco,
			tree: []string{
				"a/go.mod", "a/vendor/github.com/stretchr/testify/doc.go",
				"b/go.mod", "b/vendor/f",
			},
			want: []artifact{vendor("a/vendor"), vendor("b/vendor")},
		},
		{
			name: "no go.mod, no project",
			ecos: goEco,
			tree: []string{"stuff/vendor/a"},
		},
		{
			name: "ruby vendor/bundle is not a go vendor",
			ecos: goEco,
			tree: []string{"rails/Gemfile", "rails/vendor/bundle/ruby/3.2/gems/x.rb"},
		},
	})
}

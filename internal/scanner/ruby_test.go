package scanner_test

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

func TestRubyWalk(t *testing.T) {
	ruby := []model.Ecosystem{model.EcoRuby}
	safe := func(path string, cat model.Category) artifact {
		return artifact{path, model.EcoRuby, cat, model.SafetySafe, ""}
	}

	runWalkCases(t, []walkCase{
		{
			name: "artifacts of sibling projects",
			ecos: ruby,
			tree: []string{
				"web/Gemfile", "web/vendor/bundle/ruby/3.3.0/gems/rails.rb", "web/.bundle/config",
				"web/tmp/cache/bootsnap/compile.cache", "web/log/", "web/.ruby-lsp/cache.json",
				"gem/Gemfile", "gem/coverage/index.html",
			},
			want: []artifact{
				safe("web/vendor/bundle", model.CatDeps),
				safe("web/.bundle", model.CatCache),
				safe("web/tmp", model.CatCache),
				safe("web/log", model.CatBuild),
				safe("web/.ruby-lsp", model.CatCache),
				safe("gem/coverage", model.CatBuild),
			},
		},
		{
			name: "no Gemfile, no project",
			ecos: ruby,
			tree: []string{"random/vendor/bundle/"},
		},
		{
			name: "no descent into a matched vendor/bundle",
			ecos: ruby,
			tree: []string{"app/Gemfile", "app/vendor/bundle/tmp/data"},
			want: []artifact{safe("app/vendor/bundle", model.CatDeps)},
		},
	})
}

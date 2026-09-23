package scanner_test

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

func TestPythonWalk(t *testing.T) {
	py := []model.Ecosystem{model.EcoPython}
	// Python artifacts sit at any depth, so results carry their project root.
	art := func(path string, cat model.Category, safety model.SafetyLevel, root string) artifact {
		return artifact{path, model.EcoPython, cat, safety, root}
	}

	cases := []walkCase{
		{
			name: "artifacts at any depth",
			ecos: py,
			tree: []string{
				"app/pyproject.toml", "app/src/mypkg/__pycache__/x.pyc",
				"app/.pytest_cache/", "app/.mypy_cache/", "app/.ruff_cache/", "app/__pycache__/",
				"app/mypackage.egg-info/PKG-INFO",
			},
			want: []artifact{
				art("app/src/mypkg/__pycache__", model.CatBuild, model.SafetySafe, "app"),
				art("app/.pytest_cache", model.CatCache, model.SafetySafe, "app"),
				art("app/.mypy_cache", model.CatCache, model.SafetySafe, "app"),
				art("app/.ruff_cache", model.CatCache, model.SafetySafe, "app"),
				art("app/__pycache__", model.CatBuild, model.SafetySafe, "app"),
				art("app/mypackage.egg-info", model.CatBuild, model.SafetySafe, "app"),
			},
		},
		{
			// A venv may hold packages not pinned anywhere (kondo#182).
			name: "venv is caution",
			ecos: py,
			tree: []string{"app/pyproject.toml", "app/.venv/lib/site.py"},
			want: []artifact{art("app/.venv", model.CatDeps, model.SafetyCaution, "app")},
		},
		{
			name: "no marker, no project",
			ecos: py,
			tree: []string{"stray/__pycache__/f.pyc"},
		},
		{
			name: "nested project attributes to the closest root",
			ecos: py,
			tree: []string{"outer/pyproject.toml", "outer/sub/pyproject.toml", "outer/sub/__pycache__/x.pyc"},
			want: []artifact{art("outer/sub/__pycache__", model.CatBuild, model.SafetySafe, "outer/sub")},
		},
	}
	for _, marker := range []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt", "Pipfile", "uv.lock"} {
		cases = append(cases, walkCase{
			name: "marker " + marker,
			ecos: py,
			tree: []string{"proj/" + marker, "proj/.pytest_cache/v"},
			want: []artifact{art("proj/.pytest_cache", model.CatCache, model.SafetySafe, "proj")},
		})
	}
	runWalkCases(t, cases)
}

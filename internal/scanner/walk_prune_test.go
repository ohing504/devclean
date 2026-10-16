package scanner_test

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

func TestWalkExcludesInstalledPackageTrees(t *testing.T) {
	node := []model.Ecosystem{model.EcoNode}

	runWalkCases(t, []walkCase{
		{
			name: "pnpm store v11",
			ecos: node,
			tree: []string{
				"pnpm/store/v11/files/00/blob", "pnpm/store/v11/index.db",
				"pnpm/store/v11/links/@/next/16.3.5/h/node_modules/next/package.json",
				"pnpm/store/v11/links/@/next/16.3.5/h/node_modules/next/dist/server.js",
				"app/package.json", "app/dist/",
			},
			want: []artifact{{"app/dist", model.EcoNode, model.CatBuild, model.SafetySafe, ""}},
		},
		{
			name: "pnpm store v10",
			ecos: node,
			tree: []string{
				"pnpm/store/v10/files/00/blob", "pnpm/store/v10/index/00/idx",
				"pnpm/store/v10/pkg/package.json", "pnpm/store/v10/pkg/dist/",
			},
		},
		{
			name: "app bundle",
			ecos: node,
			tree: []string{
				"update/Code.app/Contents/Resources/app/extensions/copilot/package.json",
				"update/Code.app/Contents/Resources/app/extensions/copilot/node_modules/",
				"app/package.json", "app/node_modules/",
			},
			want: []artifact{{"app/node_modules", model.EcoNode, model.CatDeps, model.SafetySafe, ""}},
		},
		{
			name: "node_modules outside a project",
			ecos: node,
			tree: []string{
				"sdk-cache/darwin-arm64/node_modules/hono/package.json",
				"sdk-cache/darwin-arm64/node_modules/hono/dist/index.js",
				"runner/node20/lib/node_modules/npm/package.json",
				"runner/node20/lib/node_modules/npm/node_modules/abbrev/",
				"app/package.json", "app/tools/node_modules/x/package.json", "app/tools/node_modules/x/dist/",
			},
		},
		{
			name: "python site-packages",
			ecos: []model.Ecosystem{model.EcoPython},
			tree: []string{
				"tool/.venv/lib/python3.12/site-packages/numpy/f2py/setup.py",
				"tool/.venv/lib/python3.12/site-packages/numpy/f2py/__pycache__/x.pyc",
				"usr/lib/python3/dist-packages/pkg/setup.py",
				"usr/lib/python3/dist-packages/pkg/__pycache__/x.pyc",
			},
		},
		{
			name: "go module cache",
			ecos: []model.Ecosystem{model.EcoGo},
			tree: []string{
				"go/pkg/mod/cache/download/x/@v/list",
				"go/pkg/mod/github.com/briandowns/spinner@v1.23.2/go.mod",
				"go/pkg/mod/github.com/briandowns/spinner@v1.23.2/vendor/",
			},
		},
		{
			name: "app bundle without node in the scan",
			ecos: []model.Ecosystem{model.EcoPython},
			tree: []string{
				"Tool.app/Contents/Resources/lib/requirements.txt",
				"Tool.app/Contents/Resources/lib/__pycache__/x.pyc",
			},
		},
	})
}

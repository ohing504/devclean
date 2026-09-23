package scanner_test

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

// TestWalkExcludesInstalledPackageTrees pins that installed-package trees are
// skipped by every ecosystem, whichever subset is scanned: their marker roots
// are shipped packages, not projects.
func TestWalkExcludesInstalledPackageTrees(t *testing.T) {
	node := []model.Ecosystem{model.EcoNode}

	runWalkCases(t, []walkCase{
		{
			// links/ unpacks packages whose shipped dist/ is package content;
			// deleting it corrupts the store every pnpm project hard-links from.
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
			// Electron apps (e.g. a VS Code update staged under Caches) ship
			// node_modules inside the bundle.
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
			name: "app bundle without node in the scan",
			ecos: []model.Ecosystem{model.EcoPython},
			tree: []string{
				"Tool.app/Contents/Resources/lib/requirements.txt",
				"Tool.app/Contents/Resources/lib/__pycache__/x.pyc",
			},
		},
	})
}

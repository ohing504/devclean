package scanner

import "github.com/ohing504/devclean/internal/model"

// pythonWalkEcosystem drives Python scanning in the single-pass walk engine.
//
// Unlike Node/Ruby, Python artifacts (notably __pycache__) appear at
// arbitrary depth inside a project, so the rules match by name or suffix
// anywhere under the nearest project root, and results carry ProjectRoot so
// output grouping can attribute them (nested roots win over parents).
var pythonWalkEcosystem = walkEcosystem{
	Name: "python",
	Eco:  model.EcoPython,
	Markers: []string{
		"pyproject.toml",
		"setup.py",
		"setup.cfg",
		"requirements.txt",
		"Pipfile",
		"uv.lock",
	},
	SetProjectRoot: true,
	Rules: []artifactRule{
		{Name: "__pycache__", Category: model.CatBuild, Safety: model.SafetySafe},        // Python bytecode cache
		{Name: ".pytest_cache", Category: model.CatCache, Safety: model.SafetySafe},      // pytest cache
		{Name: ".mypy_cache", Category: model.CatCache, Safety: model.SafetySafe},        // mypy type-checker cache
		{Name: ".ruff_cache", Category: model.CatCache, Safety: model.SafetySafe},        // ruff linter cache
		{Name: ".tox", Category: model.CatBuild, Safety: model.SafetySafe},               // tox testing environments
		{Name: ".nox", Category: model.CatBuild, Safety: model.SafetySafe},               // nox testing environments
		{Name: ".ipynb_checkpoints", Category: model.CatCache, Safety: model.SafetySafe}, // Jupyter checkpoint files
		{Name: "__pypackages__", Category: model.CatDeps, Safety: model.SafetySafe},      // PEP 582 local dependencies
		// .venv / venv are caution: virtual envs are often hand-curated and slow to recreate (kondo#182).
		{Name: ".venv", Category: model.CatDeps, Safety: model.SafetyCaution},     // Virtual environment
		{Name: "venv", Category: model.CatDeps, Safety: model.SafetyCaution},      // Virtual environment
		{Suffix: ".egg-info", Category: model.CatBuild, Safety: model.SafetySafe}, // Packaging metadata (*.egg-info)
	},
}

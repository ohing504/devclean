package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// pythonMarkers identify a directory as a Python project root.
var pythonMarkers = []string{
	"pyproject.toml",
	"setup.py",
	"setup.cfg",
	"requirements.txt",
	"Pipfile",
	"uv.lock",
}

var pythonArtifacts = []model.ArtifactDef{
	{Pattern: "__pycache__", Category: model.CatBuild, Description: "Python bytecode cache", AlwaysSafe: true},
	{Pattern: ".pytest_cache", Category: model.CatCache, Description: "pytest cache", AlwaysSafe: true},
	{Pattern: ".mypy_cache", Category: model.CatCache, Description: "mypy type-checker cache", AlwaysSafe: true},
	{Pattern: ".ruff_cache", Category: model.CatCache, Description: "ruff linter cache", AlwaysSafe: true},
	{Pattern: ".tox", Category: model.CatBuild, Description: "tox testing environments", AlwaysSafe: true},
	{Pattern: ".nox", Category: model.CatBuild, Description: "nox testing environments", AlwaysSafe: true},
	{Pattern: ".ipynb_checkpoints", Category: model.CatCache, Description: "Jupyter checkpoint files", AlwaysSafe: true},
	{Pattern: "__pypackages__", Category: model.CatDeps, Description: "PEP 582 local dependencies", AlwaysSafe: true},
	// .venv / venv are caution: virtual envs are often hand-curated and slow to recreate (kondo#182).
	{Pattern: ".venv", Category: model.CatDeps, Description: "Virtual environment (hand-curated setup)", AlwaysSafe: false},
	{Pattern: "venv", Category: model.CatDeps, Description: "Virtual environment (hand-curated setup)", AlwaysSafe: false},
}

// PythonScanner scans for Python project artifacts. Unlike Node/Ruby, Python
// artifacts (notably __pycache__) appear at arbitrary depth inside a project,
// so the scanner first identifies project roots (any pythonMarker present) and
// then matches artifact directory names anywhere under those roots.
type PythonScanner struct{}

func NewPythonScanner() *PythonScanner {
	return &PythonScanner{}
}

func (s *PythonScanner) Name() string               { return "python" }
func (s *PythonScanner) Ecosystem() model.Ecosystem { return model.EcoPython }

func (s *PythonScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	var results []model.ScanResult
	artifactSet := make(map[string]model.ArtifactDef, len(pythonArtifacts))
	for _, a := range pythonArtifacts {
		artifactSet[a.Pattern] = a
	}

	// Project roots discovered while walking; used as ancestor predicate for artifact matches.
	projectRoots := []string{}
	skipPaths := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !d.IsDir() {
			return nil
		}

		for skip := range skipPaths {
			if path == skip || strings.HasPrefix(path, skip+string(os.PathSeparator)) {
				return fs.SkipDir
			}
		}

		// Register project root if any marker is present.
		for _, marker := range pythonMarkers {
			if hasFile(path, marker) {
				projectRoots = append(projectRoots, path)
				break
			}
		}

		name := d.Name()

		// Skip hidden dirs that aren't tracked artifacts.
		if strings.HasPrefix(name, ".") && artifactSet[name].Pattern == "" {
			return fs.SkipDir
		}

		owner := pythonProjectFor(path, projectRoots)
		if owner == "" {
			return nil
		}

		// Exact-name artifact match.
		if art, ok := artifactSet[name]; ok {
			results = append(results, model.ScanResult{
				Path:        path,
				Ecosystem:   model.EcoPython,
				Category:    art.Category,
				Size:        DirSize(path),
				LastMod:     ModTime(path),
				Safety:      safetyFromDef(art),
				ProjectRoot: owner,
			})
			ReportProgress(ctx, len(results))
			skipPaths[path] = true
			return fs.SkipDir
		}

		// Suffix match: *.egg-info packaging metadata.
		if strings.HasSuffix(name, ".egg-info") {
			results = append(results, model.ScanResult{
				Path:        path,
				Ecosystem:   model.EcoPython,
				Category:    model.CatBuild,
				Size:        DirSize(path),
				LastMod:     ModTime(path),
				Safety:      model.SafetySafe,
				ProjectRoot: owner,
			})
			ReportProgress(ctx, len(results))
			skipPaths[path] = true
			return fs.SkipDir
		}

		return nil
	})

	return results, err
}

// pythonProjectFor returns the deepest known project root that contains path,
// or "" if none. The deepest match wins so nested projects (monorepo subpackages
// each with their own pyproject.toml) attribute artifacts to the closest root.
func pythonProjectFor(path string, roots []string) string {
	var best string
	for _, r := range roots {
		if path == r || strings.HasPrefix(path, r+string(os.PathSeparator)) {
			if len(r) > len(best) {
				best = r
			}
		}
	}
	return best
}

package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

var rubyArtifacts = []model.ArtifactDef{
	{Pattern: "vendor/bundle", Category: model.CatDeps, Description: "Bundled gems", AlwaysSafe: true},
	{Pattern: ".bundle", Category: model.CatCache, Description: "Bundler cache", AlwaysSafe: true},
	{Pattern: "tmp", Category: model.CatCache, Description: "Temporary files (bootsnap, cache, pids)", AlwaysSafe: true},
	{Pattern: "log", Category: model.CatBuild, Description: "Development and test logs", AlwaysSafe: true},
	{Pattern: "coverage", Category: model.CatBuild, Description: "Test coverage reports", AlwaysSafe: true},
	{Pattern: ".ruby-lsp", Category: model.CatCache, Description: "Ruby LSP editor cache", AlwaysSafe: true},
}

// RubyScanner scans for Ruby/Rails project artifacts.
type RubyScanner struct{}

func NewRubyScanner() *RubyScanner {
	return &RubyScanner{}
}

func (s *RubyScanner) Name() string               { return "ruby" }
func (s *RubyScanner) Ecosystem() model.Ecosystem { return model.EcoRuby }

func (s *RubyScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	var results []model.ScanResult

	// Build lookup sets.
	// Simple patterns match a single directory name.
	// Compound patterns like "vendor/bundle" need special handling.
	simpleArtifacts := make(map[string]model.ArtifactDef)
	type compoundArtifact struct {
		parent string
		child  string
		def    model.ArtifactDef
	}
	var compoundArtifacts []compoundArtifact

	for _, a := range rubyArtifacts {
		if strings.Contains(a.Pattern, "/") {
			parts := strings.SplitN(a.Pattern, "/", 2)
			compoundArtifacts = append(compoundArtifacts, compoundArtifact{
				parent: parts[0], child: parts[1], def: a,
			})
		} else {
			simpleArtifacts[a.Pattern] = a
		}
	}

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

		// Skip hidden dirs that aren't artifacts we care about
		name := d.Name()
		if strings.HasPrefix(name, ".") && simpleArtifacts[name].Pattern == "" {
			return fs.SkipDir
		}

		// Check compound patterns (e.g. vendor/bundle)
		for _, ca := range compoundArtifacts {
			if name == ca.child && filepath.Base(filepath.Dir(path)) == ca.parent {
				projectDir := filepath.Dir(filepath.Dir(path))
				if hasFile(projectDir, "Gemfile") {
					size := DirSize(path)
					results = append(results, model.ScanResult{
						Path:      path,
						Ecosystem: model.EcoRuby,
						Category:  ca.def.Category,
						Size:      size,
						LastMod:   ModTime(path),
						Safety:    safetyFromDef(ca.def),
					})
					ReportProgress(ctx, len(results))
					return fs.SkipDir
				}
			}
		}

		// Check simple patterns
		if art, ok := simpleArtifacts[name]; ok {
			projectDir := filepath.Dir(path)
			if hasFile(projectDir, "Gemfile") {
				size := DirSize(path)
				results = append(results, model.ScanResult{
					Path:      path,
					Ecosystem: model.EcoRuby,
					Category:  art.Category,
					Size:      size,
					LastMod:   ModTime(path),
					Safety:    safetyFromDef(art),
				})
				ReportProgress(ctx, len(results))
				return fs.SkipDir
			}
		}

		return nil
	})

	return results, err
}

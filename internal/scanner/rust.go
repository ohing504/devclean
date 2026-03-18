package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

var rustArtifacts = []model.ArtifactDef{
	{Pattern: "target", Category: model.CatBuild, Description: "Rust build artifacts", AlwaysSafe: true},
}

// RustScanner scans for Rust/Cargo project artifacts.
type RustScanner struct{}

func NewRustScanner() *RustScanner {
	return &RustScanner{}
}

func (s *RustScanner) Name() string               { return "rust" }
func (s *RustScanner) Ecosystem() model.Ecosystem { return model.EcoRust }

func (s *RustScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	var results []model.ScanResult
	artifactSet := make(map[string]model.ArtifactDef, len(rustArtifacts))
	for _, a := range rustArtifacts {
		artifactSet[a.Pattern] = a
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

		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return fs.SkipDir
		}

		if art, ok := artifactSet[name]; ok {
			projectDir := filepath.Dir(path)
			if hasFile(projectDir, "Cargo.toml") {
				size := DirSize(path)
				results = append(results, model.ScanResult{
					Path:      path,
					Ecosystem: model.EcoRust,
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

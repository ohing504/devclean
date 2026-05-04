package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// Per-project Go artifacts only. Global caches (~/.cache/go-build, ~/go/pkg/mod)
// are scanned by the upcoming global scanner, since they belong to every Go
// project on the machine and need their own safety story.
var goArtifacts = []model.ArtifactDef{
	// vendor/ is caution: `go mod vendor` is an opt-in choice — devs who
	// vendor often do so for offline builds or reproducibility.
	{Pattern: "vendor", Category: model.CatDeps, Description: "Vendored Go modules (regenerate with `go mod vendor`)", AlwaysSafe: false},
}

// GoScanner scans for Go project artifacts.
type GoScanner struct{}

func NewGoScanner() *GoScanner {
	return &GoScanner{}
}

func (s *GoScanner) Name() string               { return "go" }
func (s *GoScanner) Ecosystem() model.Ecosystem { return model.EcoGo }

func (s *GoScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	var results []model.ScanResult
	artifactSet := make(map[string]model.ArtifactDef, len(goArtifacts))
	for _, a := range goArtifacts {
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
			if hasFile(projectDir, "go.mod") {
				results = append(results, model.ScanResult{
					Path:      path,
					Ecosystem: model.EcoGo,
					Category:  art.Category,
					Size:      DirSize(path),
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

package scanner

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ohing504/devclean/internal/model"
)

// dockerArtifact is one fixed, home-relative Docker path plus how to classify
// it. Docker keeps every image, container and volume inside a single sparse
// disk image, so the file itself is never a deletion target — reclaiming space
// means pruning from inside Docker, not removing the file.
type dockerArtifact struct {
	relPath  string
	category model.Category
	safety   model.SafetyLevel
	label    string
	reason   string
	rec      string
}

// dockerArtifacts are the Docker Desktop disk images scanned on this machine.
// The path is the default Docker Desktop location; a non-default DOCKER data
// root is not tracked (only the default is scanned), mirroring how the Flutter
// scanner tracks only the default ~/.pub-cache.
var dockerArtifacts = []dockerArtifact{
	{
		relPath:  "Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw",
		category: model.CatRuntime,
		safety:   model.SafetyProtected,
		label:    "Docker Desktop VM disk image",
		reason:   "holds every Docker image, container and volume in one sparse file — deleting it destroys all Docker state",
		rec:      "reclaim from inside Docker (`docker system prune`), not by deleting this file; disk shows real usage, apparent shows the sparse image's declared size",
	},
}

// DockerScanner reports Docker Desktop disk images. It is a stat-based scanner
// (fixed paths, no project tree walk) like xcode and global. The disk image is
// reported as protected: it must never be path-deleted, and its space is
// reclaimed by Docker's own prune commands (a future --include-destructive
// vendor cleanup), not by this tool removing the file.
type DockerScanner struct{}

// NewDockerScanner constructs a Docker scanner.
func NewDockerScanner() *DockerScanner { return &DockerScanner{} }

func (s *DockerScanner) Name() string               { return "docker" }
func (s *DockerScanner) Ecosystem() model.Ecosystem { return model.EcoDocker }

// Scan reports each Docker disk image that exists and falls under the scan
// root. Unlike the global scanner these targets are files, not directories, so
// there is no IsDir gate — presence alone qualifies. Sizing is deferred to
// sizePending, which fills sparse-aware Size (allocated blocks) and ApparentSize
// (the image's declared size) so a 460G-declared image reads as its real ~8G.
func (s *DockerScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	var results []model.ScanResult
	for _, a := range dockerArtifacts {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		full := filepath.Join(home, a.relPath)
		if !isUnderRoot(full, absRoot) {
			continue
		}
		if _, err := os.Stat(full); err != nil {
			continue // Docker not installed, or a non-default data root
		}

		results = append(results, model.ScanResult{
			Path:           full,
			Ecosystem:      model.EcoDocker,
			Category:       a.category,
			LastMod:        ModTime(full),
			Safety:         a.safety,
			Protected:      a.safety == model.SafetyProtected,
			Reason:         a.reason,
			ProjectRoot:    filepath.Dir(full),
			Label:          a.label,
			Recommendation: a.rec,
		})
		ReportProgress(ctx, len(results))
	}

	if err := sizePending(ctx, results); err != nil {
		return results, err
	}
	return results, nil
}

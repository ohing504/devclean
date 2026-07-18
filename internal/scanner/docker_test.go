package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

const dockerRawRel = "Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw"

func TestDockerScanner_ReportsRawImageAsProtected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	raw := filepath.Join(home, dockerRawRel)
	mustMkdir(t, filepath.Dir(raw))
	mustWriteFile(t, raw, make([]byte, 4096))

	results, err := scanner.NewDockerScanner().Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (Docker.raw), got %d", len(results))
	}

	r := results[0]
	if r.Ecosystem != model.EcoDocker {
		t.Errorf("expected ecosystem=docker, got %s", r.Ecosystem)
	}
	if r.Category != model.CatRuntime {
		t.Errorf("expected category=runtime, got %s", r.Category)
	}
	if r.Safety != model.SafetyProtected {
		t.Errorf("expected safety=protected, got %s", r.Safety)
	}
	if !r.Protected {
		t.Error("Docker.raw must be marked Protected so the cleaner refuses to delete it")
	}
	if r.Reason == "" {
		t.Error("protected result should carry a Reason")
	}
}

func TestDockerScanner_IgnoresMissingImage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	results, err := scanner.NewDockerScanner().Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results without Docker installed, got %d", len(results))
	}
}

// TestDockerScanner_ScopedRootExcludesImage pins the scope rule: a --path scan
// of a home subdirectory that does not cover the Docker image must not surface
// it (isUnderRoot), matching the global scanner's home-cache behavior.
func TestDockerScanner_ScopedRootExcludesImage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	raw := filepath.Join(home, dockerRawRel)
	mustMkdir(t, filepath.Dir(raw))
	mustWriteFile(t, raw, make([]byte, 4096))

	// Scan an unrelated subdirectory of home.
	scoped := filepath.Join(home, "workspace")
	mustMkdir(t, scoped)

	results, err := scanner.NewDockerScanner().Scan(context.Background(), scoped)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for a scoped root excluding the image, got %d", len(results))
	}
}

// TestDockerScanner_SparseAwareSize pins that the scanner routes sizing through
// the sparse-aware measurer: a sparse image's on-disk Size stays below its
// declared ApparentSize. This is the whole reason Docker.raw reads as ~8G, not
// its 460G declared size.
func TestDockerScanner_SparseAwareSize(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	raw := filepath.Join(home, dockerRawRel)
	mustMkdir(t, filepath.Dir(raw))

	// Create a sparse file: a large declared size with almost no allocated
	// blocks (truncate writes no data). If the host filesystem does not support
	// sparse files, disk and apparent may match — assert non-strict so the test
	// is portable, but require apparent to reflect the declared size.
	const declared = 512 * 1024 * 1024 // 512 MiB
	f, err := os.Create(raw)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := f.Truncate(declared); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	results, err := scanner.NewDockerScanner().Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.ApparentSize != declared {
		t.Errorf("expected apparent size %d (declared), got %d", declared, r.ApparentSize)
	}
	if r.Size > r.ApparentSize {
		t.Errorf("disk size %d must not exceed apparent size %d", r.Size, r.ApparentSize)
	}
}

func TestDockerScanner_NameAndEcosystem(t *testing.T) {
	for _, s := range scanner.DefaultRegistry().All() {
		if s.Name() != "docker" {
			continue
		}
		if s.Ecosystem() != model.EcoDocker {
			t.Errorf("expected ecosystem=docker, got %s", s.Ecosystem())
		}
		return
	}
	t.Error(`expected a registered scanner named "docker"`)
}

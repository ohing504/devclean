package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

// TestWalkScanPopulatesSize verifies the deferred parallel sizing actually
// fills Size — the walk no longer sizes inline, so a regression here would
// silently report every artifact as 0 bytes.
func TestWalkScanPopulatesSize(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	nm := filepath.Join(proj, "node_modules", "pkg")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nm, "big.js"), make([]byte, 300000), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := WalkScan(context.Background(), root, model.EcoNode)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range results {
		if filepath.Base(r.Path) == "node_modules" {
			found = true
			if r.Size <= 0 {
				t.Errorf("node_modules Size = %d, want > 0", r.Size)
			}
		}
	}
	if !found {
		t.Fatalf("node_modules not found in results: %+v", results)
	}
}

// TestSizePendingCanceled asserts a cancelled context stops sizing and
// surfaces the error, matching the walk's cancellation contract. Looped
// because the small-slice case is exactly where a regressed bare select
// (dropping the ctx.Err()-first guard) would non-deterministically return nil.
func TestSizePendingCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := []model.ScanResult{{Path: t.TempDir()}, {Path: t.TempDir()}}
	for range 100 {
		if err := sizePending(ctx, results); err == nil {
			t.Fatal("sizePending(cancelled) = nil, want ctx.Err()")
		}
	}
}

// TestSizePendingEmpty is a no-op guard: no results, no workers, no panic.
func TestSizePendingEmpty(t *testing.T) {
	if err := sizePending(context.Background(), nil); err != nil {
		t.Errorf("sizePending(nil) = %v, want nil", err)
	}
}

// TestDirSizeMissingPath locks the deferred-sizing TOCTOU contract: an artifact
// deleted between walk discovery and sizing must yield 0, not a panic. The
// walk→size gap widened when sizing moved to a post-walk phase.
func TestDirSizeMissingPath(t *testing.T) {
	got := DirSize(filepath.Join(t.TempDir(), "does-not-exist"))
	if got != 0 {
		t.Errorf("DirSize(missing) = %d, want 0", got)
	}
}

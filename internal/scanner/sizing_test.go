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

// TestDirSizeReflectsContents checks DirSize actually measures bytes, not just
// returns a positive number: the reported size must cover the logical content
// (du rounds up to block boundaries, so it is a lower bound) and must grow when
// more data is added. The golden test zeroes Size out for portability, so this
// is where the sizing arithmetic itself is pinned.
func TestDirSizeReflectsContents(t *testing.T) {
	dir := t.TempDir()

	empty := DirSize(dir)

	const fileSize = 100_000
	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, fileSize), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	withThree := DirSize(dir)
	if withThree < 3*fileSize {
		t.Errorf("DirSize with 300KB of files = %d, want >= %d", withThree, 3*fileSize)
	}
	if withThree <= empty {
		t.Errorf("DirSize did not grow after adding files: empty=%d, withThree=%d", empty, withThree)
	}

	// Adding a nested file must increase the measured size further.
	sub := filepath.Join(dir, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "d"), make([]byte, fileSize), 0o644); err != nil {
		t.Fatal(err)
	}
	withFour := DirSize(dir)
	if withFour <= withThree {
		t.Errorf("DirSize did not grow after adding a nested file: withThree=%d, withFour=%d", withThree, withFour)
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

// TestMeasureSparseFile is the core A1 case: a sparse file's disk usage
// (allocated blocks) must stay far below its apparent (logical) size. This is
// the Docker.raw scenario in miniature — the old du-only path already got this
// right, and the in-process Measure must not regress it.
func TestMeasureSparseFile(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "sparse.img"))
	if err != nil {
		t.Fatal(err)
	}
	const apparent = 100 << 20 // 100 MiB hole, no data written
	if err := f.Truncate(apparent); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	st := Measure(dir)
	if st.Apparent < apparent {
		t.Errorf("Apparent = %d, want >= %d (logical size of the hole)", st.Apparent, int64(apparent))
	}
	if st.Disk > 1<<20 {
		t.Errorf("Disk = %d, want < 1MiB (sparse file allocates almost no blocks)", st.Disk)
	}
	if st.Disk >= st.Apparent {
		t.Errorf("Disk (%d) should be far below Apparent (%d) for a sparse file", st.Disk, st.Apparent)
	}
}

// TestMeasureNormalFileBothPositive: a fully-written file reports both apparent
// and disk > 0, and disk covers the logical content (block rounding makes it a
// lower bound, never below).
func TestMeasureNormalFileBothPositive(t *testing.T) {
	dir := t.TempDir()
	const size = 200_000
	if err := os.WriteFile(filepath.Join(dir, "data.bin"), make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	st := Measure(dir)
	if st.Apparent < size {
		t.Errorf("Apparent = %d, want >= %d", st.Apparent, int64(size))
	}
	if st.Disk < size {
		t.Errorf("Disk = %d, want >= %d (allocated blocks cover the content)", st.Disk, int64(size))
	}
}

// TestMeasureHardlinkIntraDedup: two directory entries pointing at the same
// inode within one artifact must be counted once for both apparent and disk,
// and the shared inode recorded in Links (so DedupedTotal can net it across
// artifacts).
func TestMeasureHardlinkIntraDedup(t *testing.T) {
	dir := t.TempDir()
	const size = 300_000
	orig := filepath.Join(dir, "orig")
	if err := os.WriteFile(orig, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(orig, filepath.Join(dir, "hardlink")); err != nil {
		t.Fatal(err)
	}

	st := Measure(dir)
	// Apparent counts the inode once, not once per link (matches du -A).
	if st.Apparent >= 2*size {
		t.Errorf("Apparent = %d, want ~%d (hard link counted once, not doubled)", st.Apparent, int64(size))
	}
	if st.Apparent < size {
		t.Errorf("Apparent = %d, want >= %d", st.Apparent, int64(size))
	}
	if len(st.Links) != 1 {
		t.Fatalf("Links = %v, want exactly 1 shared inode", st.Links)
	}
	for _, blocks := range st.Links {
		if blocks <= 0 {
			t.Errorf("recorded shared inode blocks = %d, want > 0", blocks)
		}
	}
}

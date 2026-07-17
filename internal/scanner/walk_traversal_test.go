package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestWalkScan_SiblingContextPop_NameRule guards the recursion-scoped context
// pop for any-depth Name rules — the one rule kind where a leaked context is
// silent (matches() ignores the relative path). A marker-less sibling visited
// after a Python project must not inherit its context.
func TestWalkScan_SiblingContextPop_NameRule(t *testing.T) {
	root := t.TempDir()
	// "a-proj" sorts before "b-plain", so the Python context is pushed then
	// must be popped before the sibling is visited.
	touch(t, filepath.Join(root, "a-proj", "pyproject.toml"))
	mkdirAll(t, filepath.Join(root, "a-proj", "sub", "__pycache__"))
	mkdirAll(t, filepath.Join(root, "b-plain", "__pycache__")) // no marker → must NOT match

	results, err := WalkScan(context.Background(), root, model.EcoPython)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (only a-proj's __pycache__): %+v", len(results), paths(results))
	}
	if got := results[0].Path; got != filepath.Join(root, "a-proj", "sub", "__pycache__") {
		t.Errorf("matched %s, want a-proj/sub/__pycache__", got)
	}
}

// TestWalkScan_ContextCanceled pins the walk's own cancellation contract. The
// tree is artifact-free so sizePending([]) is a no-op and the error can only
// come from visit's top-of-frame ctx.Err() check.
func TestWalkScan_ContextCanceled(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a/b/c", "a/b/d", "e/f"} {
		mkdirAll(t, filepath.Join(root, d))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := WalkScan(ctx, root, model.EcoNode); err == nil {
		t.Error("WalkScan(cancelled) = nil error, want ctx.Err()")
	}
}

// TestWalkScan_ReadsEachDirOnce converts the single-read property into a
// deterministic gate: a reintroduced double-read is invisible to output but
// fails here.
func TestWalkScan_ReadsEachDirOnce(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "proj", "package.json"))
	mkdirAll(t, filepath.Join(root, "proj", "src", "deep"))
	mkdirAll(t, filepath.Join(root, "other"))

	counts := make(map[string]int)
	orig := walkReadDir
	walkReadDir = func(dir string) ([]os.DirEntry, error) {
		counts[dir]++
		return orig(dir)
	}
	defer func() { walkReadDir = orig }()

	if _, err := WalkScan(context.Background(), root, model.EcoNode); err != nil {
		t.Fatal(err)
	}
	for dir, n := range counts {
		if n != 1 {
			t.Errorf("dir %s read %d times, want 1", dir, n)
		}
	}
	if counts[root] != 1 {
		t.Errorf("root read %d times, want exactly 1", counts[root])
	}
}

// TestWalkScan_DoesNotFollowSymlinkedArtifact locks the no-follow contract for
// interior symlinks (now an implicit consequence of e.IsDir()): a symlink named
// like an artifact is not matched, and a self-referential symlink does not loop.
func TestWalkScan_DoesNotFollowSymlinkedArtifact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	touch(t, filepath.Join(proj, "package.json"))
	real := filepath.Join(root, "real")
	touch(t, filepath.Join(real, "file.js"))

	if err := os.Symlink(real, filepath.Join(proj, "node_modules")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := os.Symlink(proj, filepath.Join(proj, "loop")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	// Must terminate (no infinite loop) and not match the symlinked node_modules.
	results, err := WalkScan(context.Background(), root, model.EcoNode)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0 (symlinked node_modules not followed): %+v", len(results), paths(results))
	}
}

func paths(rs []model.ScanResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Path
	}
	return out
}

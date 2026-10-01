package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// TestWalkScan_DoesNotFollowSymlinkedArtifact locks the no-follow policy of the
// shared traversal (fstree): a symlink named like an artifact
// is not matched, the walk never descends *through* a symlinked directory to
// match an artifact inside it, and a self-referential symlink does not loop.
func TestWalkScan_DoesNotFollowSymlinkedArtifact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	touch(t, filepath.Join(proj, "package.json"))
	real := filepath.Join(root, "real")
	touch(t, filepath.Join(real, "file.js"))

	// A directory reachable only through a symlink (kept outside the scan root so
	// it is not walked directly), holding a real artifact: it must stay invisible,
	// proving the walk does not descend through the link.
	linked := filepath.Join(t.TempDir(), "linked")
	touch(t, filepath.Join(linked, "package.json"))
	if err := os.MkdirAll(filepath.Join(linked, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(real, filepath.Join(proj, "node_modules")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := os.Symlink(linked, filepath.Join(proj, "via-link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := os.Symlink(proj, filepath.Join(proj, "loop")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	// Must terminate (no infinite loop), not match the symlinked node_modules,
	// and not reach the node_modules inside the symlinked "via-link" dir.
	results, err := WalkScan(context.Background(), root, model.EcoNode)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0 (no symlink followed): %+v", len(results), paths(results))
	}
}

func paths(rs []model.ScanResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Path
	}
	return out
}

// TestWalkScan_SymlinkedRoot pins that the engine reads through a scan root
// given as a symlink (a --path that is a link), reporting artifacts under the
// link path.
func TestWalkScan_SymlinkedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	target := t.TempDir()
	touch(t, filepath.Join(target, "proj", "package.json"))
	mkdirAll(t, filepath.Join(target, "proj", "node_modules"))
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	results, err := WalkScan(context.Background(), link, model.EcoNode)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := paths(results), []string{filepath.Join(link, "proj", "node_modules")}; !slices.Equal(got, want) {
		t.Errorf("results = %v, want %v", got, want)
	}
}

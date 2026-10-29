package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
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

// TestWalkScan_HiddenNamedRootIsScanned pins that the scan root is always
// entered: the hidden-dir rule applies only below it, so a root named like a
// hidden directory (".dotproj") is still scanned.
func TestWalkScan_HiddenNamedRootIsScanned(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".dotproj")
	cache := filepath.Join(root, ".mypy_cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	results, err := WalkScan(context.Background(), root, model.EcoPython)
	if err != nil {
		t.Fatalf("WalkScan error: %v", err)
	}
	if len(results) != 1 || results[0].Path != cache {
		t.Errorf("expected only %s, got %+v", cache, results)
	}
}

// TestWalkScan_InstalledPackageRootIsPruned pins that a scan root named like
// an installed-package tree is not read: a package's own node_modules inside
// it is not a deletion target.
func TestWalkScan_InstalledPackageRootIsPruned(t *testing.T) {
	root := filepath.Join(t.TempDir(), "node_modules")
	touch(t, filepath.Join(root, "foo", "package.json"))
	touch(t, filepath.Join(root, "foo", "node_modules", "bar", "index.js"))

	results, err := WalkScan(context.Background(), root, model.EcoNode)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("got %v, want no results", paths(results))
	}
}

// TestWalkScan_SplitMatchesSerial scans a tree of nested projects twice: with
// every walk slot taken (one goroutine walks it all) and with the slots free
// (subtrees are walked in parallel, each with its ancestors' project
// contexts). Both must report the same artifacts, attributed to the same
// project roots. Sibling projects under a three-deep stack add their contexts
// at the same depth, so they must not share the stack's spare capacity.
func TestWalkScan_SplitMatchesSerial(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{
		"mono/package.json", "mono/node_modules/x",
		"mono/packages/a/package.json", "mono/packages/a/node_modules/x", "mono/packages/a/dist/x",
		"mono/packages/b/src/__pycache__/x", "mono/packages/b/pyproject.toml",
		"mono/packages/b/.venv/pyvenv.cfg", "mono/packages/b/sub/lib/__pycache__/x",
		"rs/Cargo.toml", "rs/target/x", "rs/crates/c/Cargo.toml", "rs/crates/c/target/x",
		"plain/deep/er/__pycache__/x", ".hidden/package.json", ".hidden/node_modules/x",
		"n1/package.json", "n1/n2/package.json", "n1/n2/n3/package.json",
	} {
		touch(t, filepath.Join(root, filepath.FromSlash(p)))
	}
	for i := range 8 {
		sib := filepath.Join(root, "n1", "n2", "n3", "s"+strconv.Itoa(i))
		touch(t, filepath.Join(sib, "package.json"))
		touch(t, filepath.Join(sib, "node_modules", "x"))
	}
	scan := func() []string {
		results, err := runWalk(context.Background(), root, walkEcosystemTable)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range results {
			out = append(out, r.Path+" "+string(r.Ecosystem)+" "+r.ProjectRoot)
		}
		return out
	}

	var serial []string
	withSlotsTaken(t, walkTokens, func() { serial = scan() })
	split := scan()

	if len(serial) == 0 || !slices.Equal(serial, split) {
		t.Errorf("split:\n%v\nserial:\n%v", split, serial)
	}
}

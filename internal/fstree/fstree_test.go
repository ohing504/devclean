package fstree

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// fakeDevices places the listed directories on device 2 and everything else
// on device 1; a path mapped to 0 has no determinable device.
func fakeDevices(t *testing.T, devs map[string]uint64) {
	t.Helper()
	orig := deviceOf
	deviceOf = func(path string, _ fs.FileInfo) (uint64, bool) {
		if d, ok := devs[path]; ok {
			return d, d != 0
		}
		return 1, true
	}
	t.Cleanup(func() { deviceOf = orig })
}

// entered walks root and returns every directory Enter saw, in order.
func entered(t *testing.T, w Walker, root string) ([]string, []string) {
	t.Helper()
	var got []string
	w.Enter = func(dir string, _ fs.FileInfo) bool {
		got = append(got, dir)
		return true
	}
	skipped, err := w.Walk(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return got, skipped
}

func TestWalk_DoesNotEnterOtherFilesystem(t *testing.T) {
	root := t.TempDir()
	mounted := filepath.Join(root, "mnt")
	mkdirAll(t, filepath.Join(mounted, "inner"))
	mkdirAll(t, filepath.Join(root, "local"))
	fakeDevices(t, map[string]uint64{mounted: 2})

	got, skipped := entered(t, Walker{}, root)

	if want := []string{root, filepath.Join(root, "local")}; !slices.Equal(got, want) {
		t.Errorf("entered = %v, want %v", got, want)
	}
	if want := []string{mounted}; !slices.Equal(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

func TestWalk_DoesNotEnterDirWithoutDevice(t *testing.T) {
	root := t.TempDir()
	unknown := filepath.Join(root, "unknown")
	mkdirAll(t, filepath.Join(unknown, "inner"))
	fakeDevices(t, map[string]uint64{unknown: 0})

	got, skipped := entered(t, Walker{}, root)

	if want := []string{root}; !slices.Equal(got, want) {
		t.Errorf("entered = %v, want %v", got, want)
	}
	if want := []string{unknown}; !slices.Equal(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

// TestWalk_ReadsEachDirOnce converts the single-read property into a
// deterministic gate: a reintroduced double-read is invisible to callers but
// fails here.
func TestWalk_ReadsEachDirOnce(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "a", "b"))
	mkdirAll(t, filepath.Join(root, "c"))

	counts := make(map[string]int)
	orig := readDir
	readDir = func(dir string) ([]os.DirEntry, error) {
		counts[dir]++
		return orig(dir)
	}
	t.Cleanup(func() { readDir = orig })

	entered(t, Walker{Entries: func(string, []fs.DirEntry) bool { return true }}, root)

	if len(counts) != 4 {
		t.Errorf("read %d dirs, want 4: %v", len(counts), counts)
	}
	for dir, n := range counts {
		if n != 1 {
			t.Errorf("dir %s read %d times, want 1", dir, n)
		}
	}
}

func TestWalk_DoesNotFollowSymlinkedDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := t.TempDir()
	real := filepath.Join(t.TempDir(), "real")
	mkdirAll(t, filepath.Join(real, "inner"))
	if err := os.Symlink(real, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	// A self-referential link must not loop either.
	if err := os.Symlink(root, filepath.Join(root, "self")); err != nil {
		t.Fatal(err)
	}

	got, _ := entered(t, Walker{}, root)

	if want := []string{root}; !slices.Equal(got, want) {
		t.Errorf("entered = %v, want %v", got, want)
	}
}

// TestWalk_SymlinkedRoot pins the root-link switch: a symlinked root is read
// through only with FollowRoot (a --path given as a link); otherwise it is the
// link itself, as deleting or sizing that path touches only the link.
func TestWalk_SymlinkedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	target := t.TempDir()
	mkdirAll(t, filepath.Join(target, "inner"))
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	got, _ := entered(t, Walker{FollowRoot: true}, link)
	if want := []string{link, filepath.Join(link, "inner")}; !slices.Equal(got, want) {
		t.Errorf("FollowRoot: entered = %v, want %v", got, want)
	}

	got, _ = entered(t, Walker{}, link)
	if want := []string{link}; !slices.Equal(got, want) {
		t.Errorf("no FollowRoot: entered = %v, want %v", got, want)
	}
}

// TestWalk_SymlinkedRootOnOtherFilesystem checks the boundary against a real
// second filesystem: a root that links onto another volume is judged by its
// target, so its children are walked. Needs a writable dir on a filesystem
// other than the temp dir's (/dev/shm on Linux).
func TestWalk_SymlinkedRootOnOtherFilesystem(t *testing.T) {
	tmpInfo, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tmpDev, ok := deviceOf("", tmpInfo)
	if !ok {
		t.Skip("platform exposes no device IDs")
	}
	shmInfo, err := os.Stat("/dev/shm")
	if err != nil {
		t.Skip("no /dev/shm")
	}
	if dev, ok := deviceOf("", shmInfo); !ok || dev == tmpDev {
		t.Skip("/dev/shm is not a separate filesystem")
	}
	target, err := os.MkdirTemp("/dev/shm", "devclean-test-")
	if err != nil {
		t.Skip("/dev/shm not writable")
	}
	t.Cleanup(func() { _ = os.RemoveAll(target) })
	mkdirAll(t, filepath.Join(target, "inner"))
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	got, skipped := entered(t, Walker{FollowRoot: true}, link)

	if want := []string{link, filepath.Join(link, "inner")}; !slices.Equal(got, want) {
		t.Errorf("entered = %v, want %v (skipped %v)", got, want, skipped)
	}
}

func TestWalk_EnterFalseSkipsSubtree(t *testing.T) {
	root := t.TempDir()
	skip := filepath.Join(root, "skip")
	mkdirAll(t, filepath.Join(skip, "inner"))

	var got []string
	w := Walker{Enter: func(dir string, _ fs.FileInfo) bool {
		got = append(got, dir)
		return dir != skip
	}}
	if _, err := w.Walk(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if want := []string{root, skip}; !slices.Equal(got, want) {
		t.Errorf("entered = %v, want %v", got, want)
	}
}

func TestWalk_LeaveAfterChildren(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	mkdirAll(t, child)

	var events []string
	w := Walker{
		Enter: func(dir string, _ fs.FileInfo) bool { events = append(events, "enter "+dir); return true },
		Leave: func(dir string) { events = append(events, "leave "+dir) },
	}
	if _, err := w.Walk(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	want := []string{"enter " + root, "enter " + child, "leave " + child, "leave " + root}
	if !slices.Equal(events, want) {
		t.Errorf("events = %v, want %v", events, want)
	}
}

func TestWalk_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Walker{}).Walk(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestCheckOneFilesystem(t *testing.T) {
	root := t.TempDir()
	mounted := filepath.Join(root, "a", "mnt")
	mkdirAll(t, mounted)

	if err := CheckOneFilesystem(context.Background(), root); err != nil {
		t.Errorf("plain tree: err = %v, want nil", err)
	}

	fakeDevices(t, map[string]uint64{mounted: 2})
	err := CheckOneFilesystem(context.Background(), root)
	se, ok := errors.AsType[*SpansFilesystemsError](err)
	if !ok {
		t.Fatalf("err = %v, want *SpansFilesystemsError", err)
	}
	if want := []string{mounted}; se.Path != root || !slices.Equal(se.Dirs, want) {
		t.Errorf("err = %+v, want Path %s Dirs %v", se, root, want)
	}
}

// TestWalk_ReportsUncheckableDir pins that a subdirectory whose lstat fails
// (here: its parent lacks search permission) counts as skipped, not as
// vanished — its filesystem was never checked.
func TestWalk_ReportsUncheckableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	mkdirAll(t, sub)
	if err := os.Chmod(root, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	_, skipped := entered(t, Walker{}, root)

	if want := []string{sub}; !slices.Equal(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

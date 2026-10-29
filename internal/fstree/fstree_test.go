package fstree

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
)

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// fakeDevices puts the listed paths on the given device (0 = unknown) and
// everything else on device 1.
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

// plain is a Walker that hands nothing to subdirectories.
type plain = Walker[struct{}]

// entered walks root and returns every directory Enter saw, in order.
func entered(t *testing.T, w plain, root string) ([]string, []string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	w.Enter = func(dir string, _ fs.FileInfo, _ struct{}) bool {
		mu.Lock()
		got = append(got, dir)
		mu.Unlock()
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

	got, skipped := entered(t, plain{}, root)

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

	got, skipped := entered(t, plain{}, root)

	if want := []string{root}; !slices.Equal(got, want) {
		t.Errorf("entered = %v, want %v", got, want)
	}
	if want := []string{unknown}; !slices.Equal(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

// fakeDataless marks directories with the given base name dataless.
func fakeDataless(t *testing.T, name string) {
	t.Helper()
	orig := dataless
	dataless = func(info fs.FileInfo) bool { return info.Name() == name }
	t.Cleanup(func() { dataless = orig })
}

func TestWalk_DoesNotReadDatalessDir(t *testing.T) {
	root := t.TempDir()
	cloud := filepath.Join(root, "cloud")
	mkdirAll(t, filepath.Join(cloud, "inner"))
	mkdirAll(t, filepath.Join(root, "local"))
	fakeDataless(t, "cloud")

	got, skipped := entered(t, plain{}, root)

	if want := []string{root, filepath.Join(root, "local")}; !slices.Equal(got, want) {
		t.Errorf("entered = %v, want %v", got, want)
	}
	if want := []string{cloud}; !slices.Equal(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

func TestWalk_DatalessRootIsUnchecked(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cloud")
	mkdirAll(t, filepath.Join(root, "inner"))
	fakeDataless(t, "cloud")

	got, skipped := entered(t, plain{}, root)

	if len(got) != 0 {
		t.Errorf("entered = %v, want none", got)
	}
	if want := []string{root}; !slices.Equal(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

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

	entered(t, plain{Entries: func(string, []fs.DirEntry, struct{}) (struct{}, bool) { return struct{}{}, true }}, root)

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

	got, _ := entered(t, plain{}, root)

	if want := []string{root}; !slices.Equal(got, want) {
		t.Errorf("entered = %v, want %v", got, want)
	}
}

// TestWalk_SymlinkedRoot: a symlinked root is read through only with FollowRoot.
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

	got, _ := entered(t, plain{FollowRoot: true}, link)
	if want := []string{link, filepath.Join(link, "inner")}; !slices.Equal(got, want) {
		t.Errorf("FollowRoot: entered = %v, want %v", got, want)
	}

	got, _ = entered(t, plain{}, link)
	if want := []string{link}; !slices.Equal(got, want) {
		t.Errorf("no FollowRoot: entered = %v, want %v", got, want)
	}
}

// TestWalk_SymlinkedRootOnOtherFilesystem: a root linking onto another real
// filesystem (/dev/shm on Linux) is judged by its target.
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

	got, skipped := entered(t, plain{FollowRoot: true}, link)

	if want := []string{link, filepath.Join(link, "inner")}; !slices.Equal(got, want) {
		t.Errorf("entered = %v, want %v (skipped %v)", got, want, skipped)
	}
}

func TestWalk_EnterFalseSkipsSubtree(t *testing.T) {
	root := t.TempDir()
	skip := filepath.Join(root, "skip")
	mkdirAll(t, filepath.Join(skip, "inner"))

	var got []string
	w := plain{Enter: func(dir string, _ fs.FileInfo, _ struct{}) bool {
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

func TestWalk_EntriesStateReachesOnlySubdirs(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "a", "inner"))
	mkdirAll(t, filepath.Join(root, "b"))

	got := make(map[string]string)
	w := Walker[string]{
		Enter: func(dir string, _ fs.FileInfo, parent string) bool {
			got[dir] = parent
			return true
		},
		Entries: func(dir string, _ []fs.DirEntry, parent string) (string, bool) {
			return parent + "/" + filepath.Base(dir), true
		},
	}
	if _, err := w.Walk(context.Background(), filepath.Join(root, "a")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Walk(context.Background(), filepath.Join(root, "b")); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		filepath.Join(root, "a"):          "",
		filepath.Join(root, "a", "inner"): "/a",
		filepath.Join(root, "b"):          "",
	}
	if !maps.Equal(got, want) {
		t.Errorf("parent states = %v, want %v", got, want)
	}
}

// TestWalk_SlotsMatchSerial: walking subdirectories in other goroutines
// enters and skips the same directories, each once.
func TestWalk_SlotsMatchSerial(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a/b/c", "a/d", "e/f", "g", "mnt/x"} {
		mkdirAll(t, filepath.Join(root, d))
	}
	fakeDevices(t, map[string]uint64{filepath.Join(root, "mnt"): 2})

	serial, serialSkipped := entered(t, plain{}, root)
	parallel, parallelSkipped := entered(t, plain{Slots: make(chan struct{}, 4)}, root)

	slices.Sort(serial)
	slices.Sort(parallel)
	if !slices.Equal(parallel, serial) {
		t.Errorf("entered = %v, want %v", parallel, serial)
	}
	if !slices.Equal(parallelSkipped, serialSkipped) {
		t.Errorf("skipped = %v, want %v", parallelSkipped, serialSkipped)
	}
}

func TestWalk_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (plain{}).Walk(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
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
	se, ok := errors.AsType[*UncheckedError](err)
	if !ok {
		t.Fatalf("err = %v, want *UncheckedError", err)
	}
	if want := []string{mounted}; se.Path != root || !slices.Equal(se.Dirs, want) {
		t.Errorf("err = %+v, want Path %s Dirs %v", se, root, want)
	}
}

// TestWalk_ReportsUncheckableDir: a subdirectory whose lstat fails (parent
// without search permission) counts as skipped.
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

	_, skipped := entered(t, plain{}, root)

	if want := []string{sub}; !slices.Equal(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

// TestWalk_ReportsUnreadableDir: a directory that cannot be read counts as
// skipped.
func TestWalk_ReportsUnreadableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	mkdirAll(t, filepath.Join(locked, "inner"))
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	_, skipped := entered(t, plain{}, root)

	if want := []string{locked}; !slices.Equal(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

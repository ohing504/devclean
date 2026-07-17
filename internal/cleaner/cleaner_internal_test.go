package cleaner

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

// moveByCopy is the EXDEV fallback for moveToDir. EXDEV itself can't be forced
// portably in a unit test, so these exercise the fallback's data-moving logic
// directly: a fully-copied tree, the original removed, and — on copy failure —
// the original left intact with the partial copy cleaned up.

func TestMoveByCopy_CopiesTreeAndRemovesOriginal(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(filepath.Join(src, "pkg", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "pkg", "index.js"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "trash", "node_modules")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := moveByCopy(src, dest); err != nil {
		t.Fatalf("moveByCopy: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("original should be removed after a successful copy")
	}
	got, err := os.ReadFile(filepath.Join(dest, "pkg", "index.js"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("copied content = %q, want %q", got, "hello")
	}
	// Permission bits are preserved.
	info, err := os.Stat(filepath.Join(dest, "pkg", "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("copied mode = %o, want 600", info.Mode().Perm())
	}
}

// TestCopyTree_PreservesGroupOtherWriteBits guards against umask stripping: a
// group/other-writable mode (0775 dir, 0664 file) must survive the copy exactly.
// A plain os.OpenFile/MkdirAll with the source mode would lose these bits under
// the common umask 022 — only an explicit Chmod preserves them.
func TestCopyTree_PreservesGroupOtherWriteBits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	old := syscall.Umask(0o022) // force the common umask so the test is deterministic
	defer syscall.Umask(old)

	dir := t.TempDir()
	src := filepath.Join(dir, "art")
	if err := os.MkdirAll(src, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0o775); err != nil { // MkdirAll itself is umask-masked
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "f"), nil, 0o664); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(src, "f"), 0o664); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "copy")

	if err := copyTree(src, dest); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	di, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o775 {
		t.Errorf("copied dir mode = %o, want 775", di.Mode().Perm())
	}
	fi, err := os.Stat(filepath.Join(dest, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o664 {
		t.Errorf("copied file mode = %o, want 664", fi.Mode().Perm())
	}
}

func TestCopyTree_PreservesSymlinkAsLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is restricted on Windows")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "art")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	// A symlink pointing outside the tree must be recreated as a link, never
	// followed (else the fallback could copy a huge or cyclic target).
	if err := os.Symlink("/nonexistent/target", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "copy")

	if err := copyTree(src, dest); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	fi, err := os.Lstat(filepath.Join(dest, "link"))
	if err != nil {
		t.Fatalf("lstat copied link: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink should be copied as a symlink, not its target")
	}
	target, err := os.Readlink(filepath.Join(dest, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "/nonexistent/target" {
		t.Errorf("symlink target = %q, want /nonexistent/target", target)
	}
}

func TestMoveByCopy_CopyFailureLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "art")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "keep.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// dest's parent is a regular file, so MkdirAll(dest) inside copyTree fails —
	// simulating any mid-copy error without needing a second device.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(blocker, "art")

	if err := moveByCopy(src, dest); err == nil {
		t.Fatal("expected error when copy destination is unwritable")
	}

	// Original must survive a failed cross-device move — data loss is the worst
	// outcome here.
	if _, err := os.Stat(filepath.Join(src, "keep.txt")); err != nil {
		t.Errorf("original must remain intact on copy failure: %v", err)
	}
}

// TestMoveByCopy_PartialCopyIsCleanedUp exercises a failure *mid-tree*: after a
// sibling has already been copied into dest, a later entry fails, so dest holds
// a partial copy that moveByCopy must remove — the invariant the simpler
// first-call-failure test can't reach.
func TestMoveByCopy_PartialCopyIsCleanedUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unreadable-dir failure injection relies on POSIX permissions")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "art")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	// os.ReadDir returns entries sorted: "a_ok.txt" copies first, then reading
	// "z_locked" (mode 0000) fails with EACCES — a genuine mid-tree failure.
	if err := os.WriteFile(filepath.Join(src, "a_ok.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(src, "z_locked")
	if err := os.MkdirAll(filepath.Join(locked, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(locked, 0o755) }() // let t.TempDir clean up

	dest := filepath.Join(dir, "trash", "art")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := moveByCopy(src, dest); err == nil {
		t.Fatal("expected error when a subtree is unreadable")
	}

	// The partial copy (dest + the already-copied a_ok.txt) must be gone.
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("partial copy at dest should be removed, got err=%v", err)
	}
	// Original intact.
	if _, err := os.Stat(filepath.Join(src, "a_ok.txt")); err != nil {
		t.Errorf("original must remain intact: %v", err)
	}
}

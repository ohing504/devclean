//go:build darwin

package scanner_test

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

const mib = 1 << 20

// writeRandom writes n random bytes, so no two files share content by chance.
func writeRandom(t *testing.T, path string, n int) {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Dir(path))
	mustWriteFile(t, path, b)
}

// mustClone makes dst an APFS clone of src: a new inode sharing src's blocks.
func mustClone(t *testing.T, src, dst string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(dst))
	if err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW); err != nil {
		t.Skipf("clonefile unsupported here: %v", err)
	}
}

// TestPnpmNodeModulesSizeExcludesStoreClones: pnpm on macOS clones store files
// into node_modules, so deleting node_modules frees only the files that are
// not clones of the store. Size is that reclaimable amount; AllocatedSize keeps
// the allocated blocks counted per file.
func TestPnpmNodeModulesSizeExcludesStoreClones(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store", "v10")
	writeRandom(t, filepath.Join(store, "files", "00", "blob"), mib)

	nm := filepath.Join(root, "app", "node_modules")
	mustMkdir(t, nm)
	mustWriteFile(t, filepath.Join(root, "app", "package.json"), nil)
	mustWriteFile(t, filepath.Join(nm, ".modules.yaml"), []byte("storeDir: "+store+"\n"))
	mustClone(t, filepath.Join(store, "files", "00", "blob"), filepath.Join(nm, "pkg", "index.js"))
	writeRandom(t, filepath.Join(nm, "pkg", "own.bin"), mib)

	results, err := scanner.WalkScan(context.Background(), root, model.EcoNode)
	if err != nil {
		t.Fatalf("WalkScan: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d: %+v", len(results), results)
	}
	r := results[0]
	if r.Size < mib || r.Size >= mib+mib/2 {
		t.Errorf("Size = %d, want about 1 MiB (only the file not cloned from the store)", r.Size)
	}
	if r.AllocatedSize < 2*mib {
		t.Errorf("AllocatedSize = %d, want at least 2 MiB (blocks counted per file)", r.AllocatedSize)
	}
}

// TestCodeSignClonesSizeCountsSharedBlocksOnce: browser code-sign copies are
// APFS clones of each other and of the installed app. Deleting them frees only
// the blocks no file outside them uses, each counted once.
func TestCodeSignClonesSizeCountsSharedBlocksOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	apps := t.TempDir()
	installed := filepath.Join(apps, "Google Chrome.app", "Contents", "MacOS", "Google Chrome")
	writeRandom(t, installed, mib)

	s := newIsolatedGlobalScanner(t)
	s.AppDirs = []string{apps}
	clone := filepath.Join(s.TmpRoot, "aa", "bbb", "X", "com.google.Chrome.code_sign_clone")
	// copy1 holds an older build no longer installed; copy2 clones copy1.
	writeRandom(t, filepath.Join(clone, "copy1", "bin"), mib)
	mustClone(t, filepath.Join(clone, "copy1", "bin"), filepath.Join(clone, "copy2", "bin"))
	// copy3 clones the installed app.
	mustClone(t, installed, filepath.Join(clone, "copy3", "bin"))

	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d: %+v", len(results), results)
	}
	r := results[0]
	if r.Size < mib || r.Size >= mib+mib/2 {
		t.Errorf("Size = %d, want about 1 MiB (old build counted once, installed app's blocks excluded)", r.Size)
	}
	if r.AllocatedSize < 3*mib {
		t.Errorf("AllocatedSize = %d, want at least 3 MiB (blocks counted per file)", r.AllocatedSize)
	}
}

// TestCloneSizeWithoutReferenceCountsSharedBlocksOnce: with the installed app
// missing, copies that clone each other still count their blocks once.
func TestCloneSizeWithoutReferenceCountsSharedBlocksOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := newIsolatedGlobalScanner(t)
	s.AppDirs = []string{t.TempDir()} // no Chrome installed
	clone := filepath.Join(s.TmpRoot, "aa", "bbb", "X", "com.google.Chrome.code_sign_clone")
	writeRandom(t, filepath.Join(clone, "copy1", "bin"), mib)
	mustClone(t, filepath.Join(clone, "copy1", "bin"), filepath.Join(clone, "copy2", "bin"))

	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if r := results[0]; r.Size < mib || r.Size >= mib+mib/2 {
		t.Errorf("Size = %d, want about 1 MiB", r.Size)
	}
}

// TestCloneSizeCountsRewrittenPartOfClone: rewriting part of a clone gives it
// new blocks for that part only; the rest stays shared with the source.
func TestCloneSizeCountsRewrittenPartOfClone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	apps := t.TempDir()
	installed := filepath.Join(apps, "Google Chrome.app", "Contents", "MacOS", "Google Chrome")
	writeRandom(t, installed, mib)

	s := newIsolatedGlobalScanner(t)
	s.AppDirs = []string{apps}
	copied := filepath.Join(s.TmpRoot, "aa", "bbb", "X", "com.google.Chrome.code_sign_clone", "copy1", "bin")
	mustClone(t, installed, copied)
	f, err := os.OpenFile(copied, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	patch := make([]byte, 64<<10)
	if _, err := rand.Read(patch); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(patch, 256<<10); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if r := results[0]; r.Size < 64<<10 || r.Size > 128<<10 {
		t.Errorf("Size = %d, want about 64 KiB (only the rewritten range)", r.Size)
	}
}

// writePnpmProject makes <root>/<name> a pnpm project whose node_modules names
// store as its pnpm store, and returns the node_modules path.
func writePnpmProject(t *testing.T, root, name, store string) string {
	t.Helper()
	nm := filepath.Join(root, name, "node_modules")
	mustMkdir(t, nm)
	mustWriteFile(t, filepath.Join(root, name, "package.json"), nil)
	mustWriteFile(t, filepath.Join(nm, ".modules.yaml"), []byte("storeDir: "+store+"\n"))
	return nm
}

// TestCloneSizeSkipsSparseHoles: a hole in a sparse file has no blocks, so it
// adds nothing to what deleting the file frees.
func TestCloneSizeSkipsSparseHoles(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store")
	mustMkdir(t, store)
	nm := writePnpmProject(t, root, "app", store)
	p := filepath.Join(nm, "pkg", "sparse.bin")
	mustMkdir(t, filepath.Dir(p))
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(data, 64*mib); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	// Make it a clone candidate: the clone is the file measured, the original
	// stays in the store.
	mustClone(t, p, filepath.Join(store, "blob"))
	if err := os.Remove(filepath.Join(store, "blob")); err != nil {
		t.Fatal(err)
	}

	results, err := scanner.WalkScan(context.Background(), root, model.EcoNode)
	if err != nil {
		t.Fatalf("WalkScan: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if r := results[0]; r.Size > mib {
		t.Errorf("Size = %d, want under 1 MiB (the 64 MiB hole holds no blocks)", r.Size)
	}
}

// TestCloneSizeCountsGroupAndRewrittenCloneOnce: two pure clones of an old
// build plus a third copy rewritten in part all share most blocks; deleting
// them frees the shared blocks once plus the rewritten range.
func TestCloneSizeCountsGroupAndRewrittenCloneOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := newIsolatedGlobalScanner(t)
	clone := filepath.Join(s.TmpRoot, "aa", "bbb", "X", "com.google.Chrome.code_sign_clone")
	first := filepath.Join(clone, "copy1", "bin")
	writeRandom(t, first, mib)
	mustClone(t, first, filepath.Join(clone, "copy2", "bin"))
	third := filepath.Join(clone, "copy3", "bin")
	mustClone(t, first, third)
	f, err := os.OpenFile(third, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	patch := make([]byte, 64<<10)
	if _, err := rand.Read(patch); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(patch, 256<<10); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if r := results[0]; r.Size < mib || r.Size > mib+128<<10 {
		t.Errorf("Size = %d, want about 1 MiB + 64 KiB", r.Size)
	}
}

// TestCloneTotalCountsClonesSplitAcrossArtifacts: two projects whose files are
// pure clones of each other (the store copy pruned) each free nothing alone,
// but deleting both frees the blocks once.
func TestCloneTotalCountsClonesSplitAcrossArtifacts(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store")
	mustMkdir(t, store)
	a := writePnpmProject(t, root, "a", store)
	b := writePnpmProject(t, root, "b", store)
	writeRandom(t, filepath.Join(a, "pkg", "lib.js"), mib)
	mustClone(t, filepath.Join(a, "pkg", "lib.js"), filepath.Join(b, "pkg", "lib.js"))

	results, err := scanner.WalkScan(context.Background(), root, model.EcoNode)
	if err != nil {
		t.Fatalf("WalkScan: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Size >= mib/2 {
			t.Errorf("%s Size = %d, want near 0 (its blocks are shared with the other project)", r.Path, r.Size)
		}
	}
	if total := model.DedupedTotal(results); total < mib || total >= mib+mib/2 {
		t.Errorf("DedupedTotal = %d, want about 1 MiB (shared blocks counted once)", total)
	}
}

// TestCodeSignClonesFindInstalledAppByBundleName: the installed app is found by
// the bundle name inside the copies, which can differ from the process name.
func TestCodeSignClonesFindInstalledAppByBundleName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	apps := t.TempDir()
	installed := filepath.Join(apps, "Naver Whale.app", "Contents", "MacOS", "Whale")
	writeRandom(t, installed, mib)

	s := newIsolatedGlobalScanner(t)
	s.AppDirs = []string{apps}
	copied := filepath.Join(s.TmpRoot, "aa", "bbb", "X", "com.naver.Whale.code_sign_clone",
		"code_sign_clone.abc", "Naver Whale.app.bundle", "Contents", "MacOS", "Whale")
	mustClone(t, installed, copied)
	// Rewrite part of the copy so it is not a pure clone and its extents are
	// compared against the installed app.
	f, err := os.OpenFile(copied, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(make([]byte, 64<<10), 0); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if r := results[0]; r.Size > 128<<10 {
		t.Errorf("Size = %d, want about 64 KiB (rest shared with the installed app)", r.Size)
	}
}

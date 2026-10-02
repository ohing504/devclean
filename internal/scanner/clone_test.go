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

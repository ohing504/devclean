//go:build darwin

package scanner_test

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

const mib = 1 << 20

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func writeRandom(t *testing.T, path string, n int) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	mustWriteFile(t, path, randomBytes(t, n))
}

// mustClone makes dst an APFS clone of src.
func mustClone(t *testing.T, src, dst string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(dst))
	if err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW); err != nil {
		t.Skipf("clonefile unsupported here: %v", err)
	}
}

// writeAt overwrites n random bytes of path at off.
func writeAt(t *testing.T, path string, off int64, n int) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(randomBytes(t, n), off); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// writePnpmProject makes <root>/<name> a pnpm project installed from store and
// returns its node_modules.
func writePnpmProject(t *testing.T, root, name, store string) string {
	t.Helper()
	nm := filepath.Join(root, name, "node_modules")
	mustMkdir(t, nm)
	mustWriteFile(t, filepath.Join(root, name, "package.json"), nil)
	mustWriteFile(t, filepath.Join(nm, ".modules.yaml"), []byte("storeDir: "+store+"\n"))
	return nm
}

func walkNode(t *testing.T, root string, want int) []model.ScanResult {
	t.Helper()
	results, err := scanner.WalkScan(context.Background(), root, model.EcoNode)
	if err != nil {
		t.Fatalf("WalkScan: %v", err)
	}
	if len(results) != want {
		t.Fatalf("want %d results, got %d: %+v", want, len(results), results)
	}
	return results
}

// codeSignScan returns a scanner whose fake temp root holds a <bundleID>
// code-sign clone dir, that dir, and the scan's home.
func codeSignScan(t *testing.T, bundleID string, appDirs ...string) (*scanner.GlobalScanner, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := newIsolatedGlobalScanner(t)
	s.AppDirs = appDirs
	return s, filepath.Join(s.TmpRoot, "aa", "bbb", "X", bundleID+".code_sign_clone"), home
}

func scanOne(t *testing.T, s *scanner.GlobalScanner, home string) model.ScanResult {
	t.Helper()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d: %+v", len(results), results)
	}
	return results[0]
}

func TestPnpmNodeModulesSizeExcludesStoreClones(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store", "v10")
	writeRandom(t, filepath.Join(store, "blob"), mib)
	nm := writePnpmProject(t, root, "app", store)
	mustClone(t, filepath.Join(store, "blob"), filepath.Join(nm, "pkg", "index.js"))
	writeRandom(t, filepath.Join(nm, "pkg", "own.bin"), mib)

	r := walkNode(t, root, 1)[0]
	if r.Size < mib || r.Size >= mib+mib/2 {
		t.Errorf("Size = %d, want about 1 MiB (only the file not cloned from the store)", r.Size)
	}
	if r.AllocatedSize < 2*mib {
		t.Errorf("AllocatedSize = %d, want at least 2 MiB (blocks counted per file)", r.AllocatedSize)
	}
}

func TestCloneTotalCountsClonesSplitAcrossArtifacts(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store")
	mustMkdir(t, store)
	a := writePnpmProject(t, root, "a", store)
	b := writePnpmProject(t, root, "b", store)
	writeRandom(t, filepath.Join(a, "pkg", "lib.js"), mib)
	mustClone(t, filepath.Join(a, "pkg", "lib.js"), filepath.Join(b, "pkg", "lib.js"))

	results := walkNode(t, root, 2)
	for _, r := range results {
		if r.Size >= mib/2 {
			t.Errorf("%s Size = %d, want near 0 (shared with the other project)", r.Path, r.Size)
		}
	}
	if total := model.DedupedTotal(results); total < mib || total >= mib+mib/2 {
		t.Errorf("DedupedTotal = %d, want about 1 MiB", total)
	}
}

func TestCloneSizeSkipsSparseHoles(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store")
	mustMkdir(t, store)
	nm := writePnpmProject(t, root, "app", store)
	sparse := filepath.Join(nm, "pkg", "sparse.bin")
	mustMkdir(t, filepath.Dir(sparse))
	writeAt(t, sparse, 64*mib, 4096)
	// A clone, since removed, marks the file as possibly sharing blocks.
	mustClone(t, sparse, filepath.Join(store, "blob"))
	if err := os.Remove(filepath.Join(store, "blob")); err != nil {
		t.Fatal(err)
	}

	if r := walkNode(t, root, 1)[0]; r.Size > mib {
		t.Errorf("Size = %d, want under 1 MiB (the 64 MiB hole holds no blocks)", r.Size)
	}
}

func TestCodeSignClonesSizeCountsSharedBlocksOnce(t *testing.T) {
	apps := t.TempDir()
	installed := filepath.Join(apps, "Google Chrome.app", "Contents", "MacOS", "Google Chrome")
	writeRandom(t, installed, mib)
	s, clone, home := codeSignScan(t, "com.google.Chrome", apps)
	// copy1 is an old build, copy2 its clone; copy3 clones the installed app.
	writeRandom(t, filepath.Join(clone, "copy1", "bin"), mib)
	mustClone(t, filepath.Join(clone, "copy1", "bin"), filepath.Join(clone, "copy2", "bin"))
	mustClone(t, installed, filepath.Join(clone, "copy3", "bin"))

	r := scanOne(t, s, home)
	if r.Size < mib || r.Size >= mib+mib/2 {
		t.Errorf("Size = %d, want about 1 MiB", r.Size)
	}
	if r.AllocatedSize < 3*mib {
		t.Errorf("AllocatedSize = %d, want at least 3 MiB", r.AllocatedSize)
	}
}

func TestCloneSizeCountsGroupAndRewrittenCloneOnce(t *testing.T) {
	s, clone, home := codeSignScan(t, "com.google.Chrome")
	first := filepath.Join(clone, "copy1", "bin")
	writeRandom(t, first, mib)
	mustClone(t, first, filepath.Join(clone, "copy2", "bin"))
	third := filepath.Join(clone, "copy3", "bin")
	mustClone(t, first, third)
	writeAt(t, third, 256<<10, 64<<10)

	if r := scanOne(t, s, home); r.Size < mib || r.Size > mib+128<<10 {
		t.Errorf("Size = %d, want about 1 MiB + 64 KiB", r.Size)
	}
}

// The installed app is found by the bundle name inside the copies, which can
// differ from the process name.
func TestCodeSignClonesFindInstalledAppByBundleName(t *testing.T) {
	apps := t.TempDir()
	installed := filepath.Join(apps, "Naver Whale.app", "Contents", "MacOS", "Whale")
	writeRandom(t, installed, mib)
	s, clone, home := codeSignScan(t, "com.naver.Whale", apps)
	copied := filepath.Join(clone, "code_sign_clone.abc", "Naver Whale.app.bundle", "Contents", "MacOS", "Whale")
	mustClone(t, installed, copied)
	writeAt(t, copied, 0, 64<<10) // no longer a pure clone: compared by extent

	if r := scanOne(t, s, home); r.Size > 128<<10 {
		t.Errorf("Size = %d, want about 64 KiB (rest shared with the installed app)", r.Size)
	}
}

// Without the installed app, blocks shared with it count as freed, so the
// result says its size may overstate.
func TestCodeSignClonesNoteOverstatedSizeWithoutInstalledApp(t *testing.T) {
	apps := t.TempDir()
	writeRandom(t, filepath.Join(apps, "Google Chrome.app", "Contents", "MacOS", "Google Chrome"), mib)
	for name, appDirs := range map[string][]string{"found": {apps}, "missing": nil} {
		t.Run(name, func(t *testing.T) {
			s, clone, home := codeSignScan(t, "com.google.Chrome", appDirs...)
			writeRandom(t, filepath.Join(clone, "copy1", "bin"), mib)
			got := strings.Contains(scanOne(t, s, home).Recommendation, "may overstate")
			if want := name == "missing"; got != want {
				t.Errorf("overstate note = %v, want %v", got, want)
			}
		})
	}
}

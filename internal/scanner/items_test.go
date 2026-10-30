package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

// itemsFixture builds a home with walk artifacts in every position the walk
// treats differently (nested project, any-depth rule, hidden ancestor, pruned
// tree, artifact inside an artifact) and stat-scanner caches, and returns a
// registry over it whose brew is stubbed. dryRuns counts brew dry-runs.
func itemsFixture(t *testing.T) (home string, reg *Registry, dryRuns *atomic.Int32) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	for _, rel := range []string{
		"workspace/app/package.json",
		"workspace/app/node_modules/x/index.js",
		"workspace/app/node_modules/pkg/dist/index.js",
		"workspace/app/dist/main.js",
		"workspace/app/src/main.ts",
		"workspace/app/sub/package.json",
		"workspace/app/sub/node_modules/y/index.js",
		"workspace/py/pyproject.toml",
		"workspace/py/pkg/a/__pycache__/m.pyc",
		"workspace/py/.venv/lib/site.py",
		"workspace/.hidden/proj/package.json",
		"workspace/.hidden/proj/node_modules/z/index.js",
		"Library/pnpm/store/v10/index/i",
		"Library/pnpm/store/v10/files/pkg/package.json",
		"Library/pnpm/store/v10/files/pkg/node_modules/w/index.js",
		".npm/_cacache/c",
		"Library/Caches/Homebrew/downloads/d",
		".cache/huggingface/hub/models--org--name/snapshots/s/f",
	} {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dryRuns = &atomic.Int32{}
	global := NewGlobalScanner()
	global.TmpRoot = filepath.Join(home, "tmp")
	global.AppDirs = nil
	global.LookPath = func(file string) (string, error) {
		if file == "brew" {
			return "/stub/brew", nil
		}
		return "", fs.ErrNotExist
	}
	global.RunCommand = func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
		if args[len(args)-1] == "--cache" {
			return []byte(filepath.Join(home, homebrewCacheRelPath) + "\n"), nil
		}
		dryRuns.Add(1)
		return []byte("This operation would free approximately 1.5MB of disk space.\n"), nil
	}

	reg = NewRegistry()
	for _, w := range walkEcosystemTable {
		reg.Register(newWalkScanner(w))
	}
	reg.Register(NewDockerScanner())
	reg.Register(global)
	reg.Register(NewLLMScanner())
	return home, reg, dryRuns
}

// withoutFuncs drops the function fields, which reflect.DeepEqual cannot
// compare unless nil.
func withoutFuncs(rs []model.ScanResult) []model.ScanResult {
	out := make([]model.ScanResult, len(rs))
	for i, r := range rs {
		if r.Delete != nil {
			d := *r.Delete
			d.Run = nil
			r.Delete = &d
		}
		r.Release = nil
		out[i] = r
	}
	return out
}

func resultsAt(rs []model.ScanResult, path string) []model.ScanResult {
	var out []model.ScanResult
	for _, r := range rs {
		if r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// TestScanWithItemsMatchesFullScan verifies a scan limited to one path finds
// the artifact at it exactly as a full scan does, and nothing where a full scan
// finds nothing, for every directory of the fixture.
func TestScanWithItemsMatchesFullScan(t *testing.T) {
	home, reg, _ := itemsFixture(t)
	full, err := reg.ScanAll(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture is only a test if the full scan finds these and skips the
	// hidden, pruned and nested node_modules.
	if len(full) != 9 {
		t.Fatalf("full scan found %d artifacts, want 9: %v", len(full), full)
	}

	var dirs []string
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for _, dir := range dirs {
		t.Run(strings.TrimPrefix(dir, home), func(t *testing.T) {
			limited, err := reg.ScanAll(WithItems(t.Context(), []string{dir}), home)
			if err != nil {
				t.Fatal(err)
			}
			want, got := withoutFuncs(resultsAt(full, dir)), withoutFuncs(resultsAt(limited, dir))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("limited scan at %s:\n got %+v\nwant %+v", dir, got, want)
			}
		})
	}
}

// TestScanWithItemsSkipsBrewDryRun verifies a limited scan runs brew's slow
// dry-run only when an item is the Homebrew item.
func TestScanWithItemsSkipsBrewDryRun(t *testing.T) {
	home, reg, dryRuns := itemsFixture(t)
	for _, tc := range []struct {
		item string
		want int32
	}{
		{filepath.Join(home, ".npm"), 0},
		{filepath.Join(home, homebrewCacheRelPath), 1},
	} {
		dryRuns.Store(0)
		if _, err := reg.ScanAll(WithItems(t.Context(), []string{tc.item}), home); err != nil {
			t.Fatal(err)
		}
		if got := dryRuns.Load(); got != tc.want {
			t.Errorf("item %s: %d brew dry-runs, want %d", tc.item, got, tc.want)
		}
	}
}

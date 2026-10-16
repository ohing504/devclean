package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/pathutil"
)

// TestSelectForYes verifies the non-interactive (--yes) safety gate: only safe
// items are deleted by default, caution is opt-in, and protected is never
// deleted — the guard that stops a mis-classified entry from silently vanishing.
func TestSelectForYes(t *testing.T) {
	results := []model.ScanResult{
		{Path: "/safe", Safety: model.SafetySafe},
		{Path: "/caution", Safety: model.SafetyCaution},
		{Path: "/protected-safety", Safety: model.SafetyProtected},
		{Path: "/git-protected", Safety: model.SafetySafe, Protected: true},
	}

	t.Run("default deletes only safe", func(t *testing.T) {
		toClean, skipped := selectForYes(results, false)
		if len(toClean) != 1 || toClean[0].Path != "/safe" {
			t.Errorf("expected only /safe, got %v", paths(toClean))
		}
		if skipped != 1 {
			t.Errorf("expected 1 skipped caution, got %d", skipped)
		}
	})

	t.Run("include-caution adds caution but not protected", func(t *testing.T) {
		toClean, skipped := selectForYes(results, true)
		if got := paths(toClean); len(got) != 2 || !contains(got, "/safe") || !contains(got, "/caution") {
			t.Errorf("expected /safe and /caution, got %v", got)
		}
		if skipped != 0 {
			t.Errorf("expected 0 skipped, got %d", skipped)
		}
		// Protected safety and git-protected must never appear.
		for _, r := range toClean {
			if r.Path == "/protected-safety" || r.Path == "/git-protected" {
				t.Errorf("protected item %s must never be selected", r.Path)
			}
		}
	})

	t.Run("git-protected safe item is never deleted", func(t *testing.T) {
		toClean, _ := selectForYes(results, true)
		if contains(paths(toClean), "/git-protected") {
			t.Error("a git-protected (dirty) item must not be deleted even if safety=safe")
		}
	})
}

// nodeProject creates a node project with node_modules and dist under a temp
// root and returns the root and the project dir.
func nodeProject(t *testing.T) (root, proj string) {
	t.Helper()
	root = t.TempDir()
	proj = filepath.Join(root, "app")
	for _, f := range []string{"package.json", "node_modules/x/index.js", "dist/main.js"} {
		p := filepath.Join(proj, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, proj
}

func runClean(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newCleanCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	return cmd.Execute()
}

// TestCleanItemDeletesOnlyNamedArtifact verifies --item narrows a --yes clean
// to the named artifact and leaves the project's other artifacts in place.
func TestCleanItemDeletesOnlyNamedArtifact(t *testing.T) {
	root, proj := nodeProject(t)

	err := runClean(t, "--eco", "node", "--path", root, "--item", filepath.Join(proj, "dist"), "--yes", "--force")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(proj, "dist")); !os.IsNotExist(err) {
		t.Errorf("dist should be deleted, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "node_modules")); err != nil {
		t.Errorf("node_modules must be kept: %v", err)
	}
}

// TestCleanItemUnmatchedFailsBeforeDeleting verifies an --item that names no
// scanned artifact fails the command and deletes nothing, even alongside a
// matching one.
func TestCleanItemUnmatchedFailsBeforeDeleting(t *testing.T) {
	root, proj := nodeProject(t)
	missing := filepath.Join(proj, "build")

	err := runClean(t, "--eco", "node", "--path", root,
		"--item", filepath.Join(proj, "dist"), "--item", missing, "--yes", "--force")
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("want an error naming %s, got %v", missing, err)
	}
	if _, err := os.Stat(filepath.Join(proj, "dist")); err != nil {
		t.Errorf("dist must not be deleted when another --item is unmatched: %v", err)
	}
}

// TestCleanItemGatedBySafetyFails verifies a named caution artifact that --yes
// would skip fails the command with the reason and deletes nothing.
func TestCleanItemGatedBySafetyFails(t *testing.T) {
	root := t.TempDir()
	venv := filepath.Join(root, "app", ".venv")
	if err := os.MkdirAll(filepath.Join(venv, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "pyproject.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runClean(t, "--eco", "python", "--path", root, "--item", venv, "--yes", "--force")
	if err == nil || !strings.Contains(err.Error(), "--include-caution") {
		t.Fatalf("want an error naming --include-caution, got %v", err)
	}
	if _, err := os.Stat(venv); err != nil {
		t.Errorf(".venv must not be deleted: %v", err)
	}
}

// TestCleanItemKeepsCommaInPath verifies --item takes a path with a comma as
// one path.
func TestCleanItemKeepsCommaInPath(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "a,b")
	dist := filepath.Join(proj, "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runClean(t, "--eco", "node", "--path", root, "--item", dist, "--yes", "--force"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dist); !os.IsNotExist(err) {
		t.Errorf("dist should be deleted, stat err = %v", err)
	}
}

// TestKeepItemsResolvesHomeAndRelativePaths verifies a "~/" item and a
// relative item match the absolute scan paths they name, and an unmatched item
// is reported as given.
func TestKeepItemsResolvesHomeAndRelativePaths(t *testing.T) {
	home := pathutil.CachedHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	cwd := t.TempDir()
	t.Chdir(cwd)
	results := []model.ScanResult{
		{Path: filepath.Join(home, ".npm")},
		{Path: filepath.Join(cwd, "app", "dist")},
		{Path: filepath.Join(cwd, "app", "node_modules")},
	}

	kept, missing := keepItems(results, []string{"~/.npm", "app/dist/", "nope"})

	want := []string{filepath.Join(home, ".npm"), filepath.Join(cwd, "app", "dist")}
	if !slices.Equal(paths(kept), want) {
		t.Errorf("kept = %v, want %v", paths(kept), want)
	}
	if !slices.Equal(missing, []string{"nope"}) {
		t.Errorf("missing = %v, want [nope]", missing)
	}
}

func paths(rs []model.ScanResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Path
	}
	return out
}

func contains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

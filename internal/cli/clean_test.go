package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
		if n := countReason(skipped, reasonCaution); n != 1 {
			t.Errorf("expected 1 skipped caution, got %d", n)
		}
	})

	t.Run("include-caution adds caution but not protected", func(t *testing.T) {
		toClean, skipped := selectForYes(results, true)
		if got := paths(toClean); len(got) != 2 || !contains(got, "/safe") || !contains(got, "/caution") {
			t.Errorf("expected /safe and /caution, got %v", got)
		}
		if n := countReason(skipped, reasonCaution); n != 0 {
			t.Errorf("expected 0 skipped caution, got %d", n)
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

func countReason(skipped []skippedItem, reason string) int {
	n := 0
	for _, sk := range skipped {
		if sk.Reason == reason {
			n++
		}
	}
	return n
}

// writeFiles creates each "/"-separated rel path under root as a file, or as
// a directory when it ends in "/".
func writeFiles(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// nodeProject creates a node project named name with node_modules and dist
// under a temp root and returns the root and the project dir.
func nodeProject(t *testing.T, name string) (root, proj string) {
	t.Helper()
	root = t.TempDir()
	writeFiles(t, root, name+"/package.json", name+"/node_modules/x/index.js", name+"/dist/main.js")
	return root, filepath.Join(root, name)
}

func assertGone(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("%s should be deleted, stat err = %v", p, err)
	}
}

func assertExists(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Errorf("%s must be kept: %v", p, err)
	}
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
	root, proj := nodeProject(t, "app")

	if err := runClean(t, "--eco", "node", "--path", root, "--item", filepath.Join(proj, "dist"), "--yes", "--force"); err != nil {
		t.Fatal(err)
	}
	assertGone(t, filepath.Join(proj, "dist"))
	assertExists(t, filepath.Join(proj, "node_modules"))
}

// TestCleanItemUnmatchedFailsBeforeDeleting verifies an --item that names no
// scanned artifact fails the command and deletes nothing, even alongside a
// matching one.
func TestCleanItemUnmatchedFailsBeforeDeleting(t *testing.T) {
	root, proj := nodeProject(t, "app")
	missing := filepath.Join(proj, "build")

	err := runClean(t, "--eco", "node", "--path", root,
		"--item", filepath.Join(proj, "dist"), "--item", missing, "--yes", "--force")
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("want an error naming %s, got %v", missing, err)
	}
	assertExists(t, filepath.Join(proj, "dist"))
}

// TestCleanItemGatedBySafetyFails verifies a named caution artifact that --yes
// would skip fails the command with the reason and deletes nothing.
func TestCleanItemGatedBySafetyFails(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "app/pyproject.toml", "app/.venv/lib/")
	venv := filepath.Join(root, "app", ".venv")

	err := runClean(t, "--eco", "python", "--path", root, "--item", venv, "--yes", "--force")
	if err == nil || !strings.Contains(err.Error(), "--include-caution") {
		t.Fatalf("want an error naming --include-caution, got %v", err)
	}
	assertExists(t, venv)
}

// TestCleanItemKeepsCommaInPath verifies --item takes a path with a comma as
// one path.
func TestCleanItemKeepsCommaInPath(t *testing.T) {
	root, proj := nodeProject(t, "a,b")

	if err := runClean(t, "--eco", "node", "--path", root, "--item", filepath.Join(proj, "dist"), "--yes", "--force"); err != nil {
		t.Fatal(err)
	}
	assertGone(t, filepath.Join(proj, "dist"))
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

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r := redirectStd(t, &os.Stdout, true)
	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&out, r)
		close(done)
	}()
	fn()
	os.Stdout.Close()
	<-done
	return out.String()
}

// TestCleanShowsLabelLikeScan pins that clean names an artifact that has a
// Label (a DerivedData folder → its source workspace) by that Label in the
// dry-run preview and in the results, as scan does, not by its folder name.
func TestCleanShowsLabelLikeScan(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Xcode DerivedData is scanned only on macOS")
	}
	for _, mode := range []string{"--dry-run", "--force"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			ws := filepath.Join(home, "workspace", "app", "ios", "Runner.xcworkspace")
			dd := filepath.Join(home, "Library", "Developer", "Xcode", "DerivedData", "Runner-abc")
			writeFiles(t, home, "workspace/app/ios/Runner.xcworkspace/", "Library/Developer/Xcode/DerivedData/Runner-abc/Build/x")
			plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>WorkspacePath</key><string>` + ws + `</string></dict></plist>`
			if err := os.WriteFile(filepath.Join(dd, "info.plist"), []byte(plist), 0o644); err != nil {
				t.Fatal(err)
			}

			var err error
			out := captureStdout(t, func() {
				err = runClean(t, "--eco", "xcode", "--item", dd, "--yes", mode)
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "workspace/app/ios/Runner.xcworkspace") || strings.Contains(out, "Runner-abc") {
				t.Errorf("want the item named by its source workspace, not its folder; got:\n%s", out)
			}
		})
	}
}

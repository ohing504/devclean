package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// newPythonProject creates <tmp>/proj with a Python marker and a .mypy_cache
// artifact, isolates HOME, and returns the symlink-resolved project path (so it
// matches the working directory after chdir on macOS, where /var → /private/var).
func newPythonProject(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(proj, ".mypy_cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "pyproject.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".mypy_cache", "a"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	return proj
}

var (
	ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	spinnerRow = regexp.MustCompile(`(?m)^.*Scanning.*$\n?`)
)

// runCLI executes devclean with args and returns its stdout with ANSI escapes
// and spinner frames removed.
func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()

	cmd := NewRootCmd(BuildInfo{})
	cmd.SetArgs(args)
	runErr := cmd.Execute()

	os.Stdout = orig
	_ = w.Close()
	out := <-done
	if runErr != nil {
		t.Fatalf("devclean %s: %v", strings.Join(args, " "), runErr)
	}
	out = bytes.ReplaceAll(out, []byte("\r"), []byte("\n"))
	return spinnerRow.ReplaceAllString(ansiEscape.ReplaceAllString(string(out), ""), "")
}

// TestRelativePathMatchesAbsolute verifies a relative --path ("." and
// "../proj") produces the same scan JSON and clean dry-run output as the
// absolute path, with the absolute project path in both.
func TestRelativePathMatchesAbsolute(t *testing.T) {
	proj := newPythonProject(t)

	commands := map[string][]string{
		"scan":  {"scan", "--eco", "python", "--json"},
		"clean": {"clean", "--eco", "python", "--dry-run", "--yes"},
	}
	for name, base := range commands {
		t.Run(name, func(t *testing.T) {
			want := runCLI(t, append(base, "--path", proj)...)
			if !strings.Contains(want, proj) || !strings.Contains(want, ".mypy_cache") {
				t.Fatalf("absolute --path output lacks %s/.mypy_cache:\n%s", proj, want)
			}
			t.Chdir(proj)
			for _, rel := range []string{".", "../proj"} {
				if got := runCLI(t, append(base, "--path", rel)...); got != want {
					t.Errorf("--path %s output differs from absolute path\n got: %s\nwant: %s", rel, got, want)
				}
			}
		})
	}
}

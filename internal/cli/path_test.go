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
	spinnerRow = regexp.MustCompile(`(?m)^.*(Scanning|Classifying).*$\n?`)
)

// runCLI executes devclean with args and returns its stdout with ANSI escapes
// and spinner frames removed.
func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()

	// Restore stdout and close the pipe even if Execute panics, so later
	// tests keep their output and the reader goroutine exits.
	runErr := func() error {
		orig := os.Stdout
		os.Stdout = w
		defer func() {
			os.Stdout = orig
			_ = w.Close()
		}()
		cmd := NewRootCmd(BuildInfo{})
		cmd.SetArgs(args)
		return cmd.Execute()
	}()
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
			for _, rel := range []string{".", "../proj"} {
				t.Run(rel, func(t *testing.T) {
					t.Chdir(proj)
					if got := runCLI(t, append(base, "--path", rel)...); got != want {
						t.Errorf("output differs from absolute path\n got: %s\nwant: %s", got, want)
					}
				})
			}
		})
	}
}

package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// redirectStd replaces *std with the write or read end of a pipe for the test,
// so the command sees a real non-terminal file descriptor.
func redirectStd(t *testing.T, std **os.File, writeEnd bool) (other *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := *std
	if writeEnd {
		*std, other = w, r
	} else {
		*std, other = r, w
	}
	t.Cleanup(func() {
		*std = orig
		r.Close()
		w.Close()
	})
	return other
}

// TestScanPipedStdoutHasNoSpinner pins that piped scan output carries no
// spinner frames or line-clearing escape codes.
func TestScanPipedStdoutHasNoSpinner(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "app")
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := redirectStd(t, &os.Stdout, true)
	var piped bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&piped, r)
		close(done)
	}()

	_, err := runScan(t, "--path", root)
	os.Stdout.Close()
	<-done
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got := piped.String(); strings.Contains(got, "\r") || strings.Contains(got, "\033[K") {
		t.Errorf("piped stdout contains spinner control codes: %q", got)
	}
}

// TestCleanWithoutTerminalStdinRequiresYes pins that clean fails instead of
// reporting a cancellation when it cannot prompt and --yes is absent.
func TestCleanWithoutTerminalStdinRequiresYes(t *testing.T) {
	redirectStd(t, &os.Stdin, false)

	cmd := newCleanCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--eco", "node", "--path", t.TempDir(), "--dry-run"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("want an error naming --yes, got %v", err)
	}
}

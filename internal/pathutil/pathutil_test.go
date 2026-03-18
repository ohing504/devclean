package pathutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/pathutil"
)

func TestCachedHomeDir(t *testing.T) {
	home := pathutil.CachedHomeDir()
	if home == "" {
		t.Error("expected non-empty home dir")
	}
	expected, _ := os.UserHomeDir()
	if home != expected {
		t.Errorf("got %q, want %q", home, expected)
	}
	// Call again to verify caching doesn't break
	if pathutil.CachedHomeDir() != home {
		t.Error("cached home dir changed between calls")
	}
}

func TestShortenHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		input string
		want  string
	}{
		{home + "/workspace/project", "~/workspace/project"},
		{home, "~"},
		{"/tmp/other", "/tmp/other"},
		{"relative/path", "relative/path"},
		{"", ""},
	}
	for _, tt := range tests {
		got := pathutil.ShortenHome(tt.input)
		if got != tt.want {
			t.Errorf("ShortenHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestModTime(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	os.WriteFile(f, []byte("hello"), 0o644)

	mt := pathutil.ModTime(f)
	if mt.IsZero() {
		t.Error("expected non-zero ModTime for existing file")
	}
}

func TestModTimeNonExistent(t *testing.T) {
	mt := pathutil.ModTime("/nonexistent/path/xyz")
	if !mt.IsZero() {
		t.Error("expected zero ModTime for nonexistent path")
	}
}

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohing504/devclean/internal/output"
)

// runScan runs the scan command with args, limited to the node ecosystem so it
// walks only the given temp tree, and returns its stdout.
func runScan(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newScanCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"--eco", "node"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

// TestScanRejectsUnknownSortKey pins that an unknown --sort value is an error
// instead of a silent size sort.
func TestScanRejectsUnknownSortKey(t *testing.T) {
	_, err := runScan(t, "--path", t.TempDir(), "--sort", "date", "--json")
	if err == nil || !strings.Contains(err.Error(), "--sort") {
		t.Errorf("want an error naming --sort, got %v", err)
	}
}

// TestScanJSONAppliesTopN pins that --json output keeps only the -n projects.
func TestScanJSONAppliesTopN(t *testing.T) {
	root := t.TempDir()
	for i, name := range []string{"small", "big", "mid"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		blob := make([]byte, (i+1)*64*1024)
		if err := os.WriteFile(filepath.Join(dir, "node_modules", "blob"), blob, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	stdout, err := runScan(t, "--path", root, "--json", "-n", "1")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var got output.ScanOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, stdout)
	}
	if got.TotalCount != 1 || !strings.HasSuffix(got.Results[0].Path, filepath.Join("mid", "node_modules")) {
		t.Errorf("-n 1 --json should keep only the largest project mid; got %d results: %s", got.TotalCount, stdout)
	}
}

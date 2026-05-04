//go:build darwin

package scanner_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestXcodeScanner_NameAndEcosystem(t *testing.T) {
	s := scanner.NewXcodeScanner()
	if s.Name() != "xcode" {
		t.Errorf("expected name=xcode, got %s", s.Name())
	}
	if s.Ecosystem() != model.EcoXcode {
		t.Errorf("expected ecosystem=xcode, got %s", s.Ecosystem())
	}
}

func TestXcodeScanner_FindsAllArtifacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Non-expanding artifacts: directory itself becomes one result.
	flatPaths := []string{
		"Library/Developer/Xcode/DerivedData",
		"Library/Developer/Xcode/Archives",
		"Library/Developer/CoreSimulator/Caches",
		"Library/Logs/CoreSimulator",
	}
	for _, p := range flatPaths {
		dir := filepath.Join(home, p)
		mustMkdir(t, dir)
		mustWriteFile(t, filepath.Join(dir, "data.bin"), make([]byte, 1024))
	}

	// Expanding artifacts: each child directory becomes its own result.
	expandPaths := map[string][]string{
		"Library/Developer/Xcode/iOS DeviceSupport":     {"16.4 (20E247)", "17.2 (21C62)"},
		"Library/Developer/Xcode/watchOS DeviceSupport": {"10.4 (21S365)"},
		"Library/Developer/Xcode/tvOS DeviceSupport":    {"17.2 (21K364)"},
		"Library/Developer/CoreSimulator/Devices":       {"UUID-A", "UUID-B"},
	}
	expandedCount := 0
	for parent, children := range expandPaths {
		mustMkdir(t, filepath.Join(home, parent))
		for _, c := range children {
			child := filepath.Join(home, parent, c)
			mustMkdir(t, child)
			mustWriteFile(t, filepath.Join(child, "data.bin"), make([]byte, 1024))
			expandedCount++
		}
	}

	wantTotal := len(flatPaths) + expandedCount

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != wantTotal {
		t.Fatalf("expected %d results, got %d", wantTotal, len(results))
	}

	for _, r := range results {
		if r.Ecosystem != model.EcoXcode {
			t.Errorf("expected ecosystem=xcode, got %s for %s", r.Ecosystem, r.Path)
		}
		if r.Size == 0 {
			t.Errorf("expected non-zero size for %s", r.Path)
		}
	}
}

func TestXcodeScanner_ExpandsDeviceSupportPerVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	parent := filepath.Join(home, "Library/Developer/Xcode/iOS DeviceSupport")
	versions := []string{"16.4 (20E247)", "17.2 (21C62)", "18.0 (22A123)"}
	for _, v := range versions {
		dir := filepath.Join(parent, v)
		mustMkdir(t, dir)
		mustWriteFile(t, filepath.Join(dir, "Symbols.bin"), make([]byte, 4096))
	}

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != len(versions) {
		t.Fatalf("expected %d per-version results, got %d", len(versions), len(results))
	}

	for _, r := range results {
		if r.ProjectRoot != parent {
			t.Errorf("expected ProjectRoot=%s, got %s", parent, r.ProjectRoot)
		}
		if filepath.Dir(r.Path) != parent {
			t.Errorf("expected child of %s, got %s", parent, r.Path)
		}
		if r.Category != model.CatRuntime || r.Safety != model.SafetySafe {
			t.Errorf("unexpected category/safety for %s: %s/%s", r.Path, r.Category, r.Safety)
		}
	}
}

func TestXcodeScanner_ExpandsSimulatorDevices(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	parent := filepath.Join(home, "Library/Developer/CoreSimulator/Devices")
	uuids := []string{"027AEA9C-DE17-49C8-9F50-4FC1B94BE3F8", "079DDB1D-4286-472A-90F9-0FB9F9B00C6F"}
	for _, u := range uuids {
		dir := filepath.Join(parent, u)
		mustMkdir(t, dir)
		mustWriteFile(t, filepath.Join(dir, "device.plist"), []byte("<plist/>"))
	}

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != len(uuids) {
		t.Fatalf("expected %d per-device results, got %d", len(uuids), len(results))
	}

	for _, r := range results {
		if r.Safety != model.SafetyCaution {
			t.Errorf("expected safety=caution for simulator device, got %s", r.Safety)
		}
	}
}

func TestXcodeScanner_EmptyExpandableYieldsNoResults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Parent exists but has no children — should not produce a parent-level result.
	mustMkdir(t, filepath.Join(home, "Library/Developer/Xcode/iOS DeviceSupport"))

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty expandable parent, got %d", len(results))
	}
}

func TestXcodeScanner_SkipsMissingPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Only DerivedData exists.
	dd := filepath.Join(home, "Library/Developer/Xcode/DerivedData")
	mustMkdir(t, dd)
	mustWriteFile(t, filepath.Join(dd, "build.bin"), make([]byte, 2048))

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if filepath.Base(results[0].Path) != "DerivedData" {
		t.Errorf("expected DerivedData, got %s", results[0].Path)
	}
	if results[0].Category != model.CatBuild {
		t.Errorf("expected category=build, got %s", results[0].Category)
	}
}

func TestXcodeScanner_CategoryAndSafety(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Each artifact's classification is fixed; verify a representative sample.
	// Only flat (non-expanding) artifacts here — expanded ones are covered by dedicated tests.
	cases := map[string]struct {
		category model.Category
		safety   model.SafetyLevel
	}{
		"Library/Developer/Xcode/DerivedData":    {model.CatBuild, model.SafetySafe},
		"Library/Developer/Xcode/Archives":       {model.CatBuild, model.SafetyCaution},
		"Library/Developer/CoreSimulator/Caches": {model.CatCache, model.SafetySafe},
		"Library/Logs/CoreSimulator":             {model.CatCache, model.SafetySafe},
	}
	for p := range cases {
		dir := filepath.Join(home, p)
		mustMkdir(t, dir)
		mustWriteFile(t, filepath.Join(dir, "f"), []byte("x"))
	}

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	got := make(map[string]model.ScanResult, len(results))
	for _, r := range results {
		rel, _ := filepath.Rel(home, r.Path)
		got[rel] = r
	}
	for relPath, want := range cases {
		r, ok := got[relPath]
		if !ok {
			t.Errorf("missing result for %s", relPath)
			continue
		}
		if r.Category != want.category {
			t.Errorf("%s: category=%s, want %s", relPath, r.Category, want.category)
		}
		if r.Safety != want.safety {
			t.Errorf("%s: safety=%s, want %s", relPath, r.Safety, want.safety)
		}
	}
}

func TestXcodeScanner_FiltersByRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dd := filepath.Join(home, "Library/Developer/Xcode/DerivedData")
	mustMkdir(t, dd)
	mustWriteFile(t, filepath.Join(dd, "f"), []byte("x"))

	// Root outside of HOME — Xcode paths should not match.
	otherRoot := t.TempDir()

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), otherRoot)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when scanning outside HOME, got %d", len(results))
	}
}

func TestXcodeScanner_NoArtifactsInEmptyHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results in empty home, got %d", len(results))
	}
}

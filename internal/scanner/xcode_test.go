//go:build darwin

package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		"Library/Developer/Xcode/DerivedData":           {"Runner-abcdef", "ModuleCache.noindex"},
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

func TestXcodeScanner_FlagsOldDeviceSupportBuilds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	parent := filepath.Join(home, "Library/Developer/Xcode/iOS DeviceSupport")

	// Same device+version with three different builds; older mtimes should be flagged.
	type build struct {
		dir   string
		mtime time.Time
	}
	builds := []build{
		{"iPhone13,3 26.5 (23F5043k)", time.Now().Add(-30 * 24 * time.Hour)},
		{"iPhone13,3 26.5 (23F5059e)", time.Now().Add(-15 * 24 * time.Hour)},
		{"iPhone13,3 26.5 (23F5069b)", time.Now().Add(-1 * 24 * time.Hour)}, // newest
		// Different device — should not be compared with the iPhone13,3 group.
		{"iPad11,1 26.3 (23D5114d)", time.Now().Add(-40 * 24 * time.Hour)},
	}
	for _, b := range builds {
		dir := filepath.Join(parent, b.dir)
		mustMkdir(t, dir)
		mustWriteFile(t, filepath.Join(dir, "Symbols.bin"), make([]byte, 4096))
		if err := os.Chtimes(dir, b.mtime, b.mtime); err != nil {
			t.Fatalf("chtimes %s: %v", dir, err)
		}
	}

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	recs := make(map[string]string)
	for _, r := range results {
		recs[filepath.Base(r.Path)] = r.Recommendation
	}

	if recs["iPhone13,3 26.5 (23F5069b)"] != "" {
		t.Errorf("newest build should not be flagged, got %q", recs["iPhone13,3 26.5 (23F5069b)"])
	}
	for _, old := range []string{"iPhone13,3 26.5 (23F5043k)", "iPhone13,3 26.5 (23F5059e)"} {
		if !strings.Contains(recs[old], "superseded") {
			t.Errorf("expected %s to be flagged as superseded, got %q", old, recs[old])
		}
	}
	// Lone iPad entry has no peer, should not be flagged.
	if recs["iPad11,1 26.3 (23D5114d)"] != "" {
		t.Errorf("solo build should not be flagged, got %q", recs["iPad11,1 26.3 (23D5114d)"])
	}
}

// Older Xcode names DeviceSupport folders without a model ("16.4 (build)").
// Such a version-only key must group builds within its own platform but never
// collide across platforms.
func TestXcodeScanner_DeviceSupportKeyIsPlatformScoped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	type entry struct {
		platform string
		dir      string
		mtime    time.Time
	}
	entries := []entry{
		// iOS: two builds of the same version — older is superseded.
		{"iOS DeviceSupport", "16.4 (20E247)", time.Now().Add(-20 * 24 * time.Hour)},
		{"iOS DeviceSupport", "16.4 (20E252)", time.Now().Add(-1 * 24 * time.Hour)}, // newest
		// tvOS: same version-only name, different platform — must not be flagged.
		{"tvOS DeviceSupport", "16.4 (20K672)", time.Now().Add(-40 * 24 * time.Hour)},
	}
	for _, e := range entries {
		dir := filepath.Join(home, "Library/Developer/Xcode", e.platform, e.dir)
		mustMkdir(t, dir)
		mustWriteFile(t, filepath.Join(dir, "Symbols.bin"), make([]byte, 4096))
		if err := os.Chtimes(dir, e.mtime, e.mtime); err != nil {
			t.Fatalf("chtimes %s: %v", dir, err)
		}
	}

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	recs := make(map[string]string)
	for _, r := range results {
		recs[r.Path] = r.Recommendation
	}
	base := filepath.Join(home, "Library/Developer/Xcode")

	if got := recs[filepath.Join(base, "iOS DeviceSupport", "16.4 (20E247)")]; !strings.Contains(got, "superseded") {
		t.Errorf("older iOS build should be superseded, got %q", got)
	}
	if got := recs[filepath.Join(base, "iOS DeviceSupport", "16.4 (20E252)")]; got != "" {
		t.Errorf("newest iOS build should not be flagged, got %q", got)
	}
	if got := recs[filepath.Join(base, "tvOS DeviceSupport", "16.4 (20K672)")]; got != "" {
		t.Errorf("tvOS build must not collide with iOS key, got %q", got)
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

func TestXcodeScanner_LabelsKnownDerivedDataChildren(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	parent := filepath.Join(home, "Library/Developer/Xcode/DerivedData")
	children := []string{"Runner-abc123", "ModuleCache.noindex", "SymbolCache.noindex"}
	for _, c := range children {
		dir := filepath.Join(parent, c)
		mustMkdir(t, dir)
		mustWriteFile(t, filepath.Join(dir, "f"), []byte("x"))
	}

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	labels := make(map[string]string)
	for _, r := range results {
		labels[filepath.Base(r.Path)] = r.Label
	}

	if !strings.Contains(labels["ModuleCache.noindex"], "module cache") {
		t.Errorf("expected ModuleCache.noindex label to mention module cache, got %q", labels["ModuleCache.noindex"])
	}
	if !strings.Contains(labels["SymbolCache.noindex"], "Symbol cache") {
		t.Errorf("expected SymbolCache.noindex label to mention Symbol cache, got %q", labels["SymbolCache.noindex"])
	}
	if labels["Runner-abc123"] != "" {
		t.Errorf("Runner-abc123 should have no label, got %q", labels["Runner-abc123"])
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

	// Only Archives exists (a flat artifact).
	dir := filepath.Join(home, "Library/Developer/Xcode/Archives")
	mustMkdir(t, dir)
	mustWriteFile(t, filepath.Join(dir, "build.bin"), make([]byte, 2048))

	s := scanner.NewXcodeScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if filepath.Base(results[0].Path) != "Archives" {
		t.Errorf("expected Archives, got %s", results[0].Path)
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

	dd := filepath.Join(home, "Library/Developer/Xcode/Archives")
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

func TestXcodeScanner_VendorCleanups(t *testing.T) {
	s := scanner.NewXcodeScanner()
	vc, ok := any(s).(scanner.VendorCleaner)
	if !ok {
		t.Fatal("XcodeScanner should implement VendorCleaner on darwin")
	}
	actions := vc.VendorCleanups()
	if len(actions) == 0 {
		t.Fatal("expected at least one vendor cleanup action")
	}

	found := false
	for _, a := range actions {
		if a.ID == "simctl-delete-unavailable" {
			found = true
			if a.Run == nil {
				t.Error("Run must not be nil")
			}
			if a.Command == "" {
				t.Error("Command should be populated for display")
			}
		}
	}
	if !found {
		t.Error("expected simctl-delete-unavailable action")
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

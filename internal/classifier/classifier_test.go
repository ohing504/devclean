package classifier_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ohing504/devclean/internal/classifier"
	"github.com/ohing504/devclean/internal/model"
)

func TestClassifyActivity(t *testing.T) {
	th := classifier.DefaultThresholds()
	now := time.Now()

	tests := []struct {
		name    string
		lastMod time.Time
		want    model.ActivityStatus
	}{
		{"today", now, model.StatusActive},
		{"3 days ago", now.AddDate(0, 0, -3), model.StatusActive},
		{"6 days ago", now.AddDate(0, 0, -6), model.StatusActive},
		{"10 days ago", now.AddDate(0, 0, -10), model.StatusRecent},
		{"29 days ago", now.AddDate(0, 0, -29), model.StatusRecent},
		{"45 days ago", now.AddDate(0, 0, -45), model.StatusStale},
		{"89 days ago", now.AddDate(0, 0, -89), model.StatusStale},
		{"90 days ago", now.AddDate(0, 0, -90), model.StatusDormant},
		{"365 days ago", now.AddDate(0, 0, -365), model.StatusDormant},
		{"zero time", time.Time{}, model.StatusDormant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifier.ClassifyActivity(tt.lastMod, th)
			if got != tt.want {
				t.Errorf("ClassifyActivity(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestClassifyActivityCustomThresholds(t *testing.T) {
	th := classifier.ActivityThresholds{
		Active:  3,
		Recent:  14,
		Dormant: 60,
	}
	now := time.Now()

	got := classifier.ClassifyActivity(now.AddDate(0, 0, -5), th)
	if got != model.StatusRecent {
		t.Errorf("expected recent with custom threshold, got %q", got)
	}

	got = classifier.ClassifyActivity(now.AddDate(0, 0, -65), th)
	if got != model.StatusDormant {
		t.Errorf("expected dormant with custom threshold, got %q", got)
	}
}

func TestClassifyResults(t *testing.T) {
	now := time.Now()
	results := []model.ScanResult{
		{Path: "/a", LastMod: now},
		{Path: "/b", LastMod: now.AddDate(0, 0, -100)},
	}

	classifier.ClassifyResults(results, classifier.DefaultThresholds())

	if results[0].Activity != model.StatusActive {
		t.Errorf("expected active, got %q", results[0].Activity)
	}
	if results[1].Activity != model.StatusDormant {
		t.Errorf("expected dormant, got %q", results[1].Activity)
	}
}

func TestHasUncommittedChanges_CleanRepo(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	f := filepath.Join(dir, "file.txt")
	mustWriteFile(t, f, []byte("hello"))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	if classifier.HasUncommittedChanges(dir) {
		t.Error("expected no uncommitted changes in clean repo")
	}
}

func TestHasUncommittedChanges_DirtyRepo(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	f := filepath.Join(dir, "file.txt")
	mustWriteFile(t, f, []byte("hello"))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	mustWriteFile(t, f, []byte("modified"))

	if !classifier.HasUncommittedChanges(dir) {
		t.Error("expected uncommitted changes after modification")
	}
}

func TestHasUncommittedChanges_NonGit(t *testing.T) {
	dir := t.TempDir()
	if classifier.HasUncommittedChanges(dir) {
		t.Error("non-git directory should return false")
	}
}

func TestApplyGitInfo_UsesProjectDirMtime(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	f := filepath.Join(dir, "file.txt")
	mustWriteFile(t, f, []byte("hello"))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	// Create artifact with old mtime
	nmDir := filepath.Join(dir, "node_modules")
	mustMkdir(t, nmDir)
	oldTime := time.Now().AddDate(0, -7, 0)

	results := []model.ScanResult{
		{Path: nmDir, LastMod: oldTime, Safety: model.SafetySafe},
	}

	classifier.ApplyGitInfo(results)

	// LastMod should be updated to the project dir mtime (most recent),
	// not stay at the old artifact mtime
	if results[0].LastMod.Equal(oldTime) {
		t.Error("expected LastMod to be updated from project dir mtime, but it stayed at old artifact time")
	}
}

func TestApplyGitInfo_GitignoredArtifactNotProtected(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	// Create .gitignore that ignores node_modules
	mustWriteFile(t, filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"))
	mustWriteFile(t, filepath.Join(dir, "file.txt"), []byte("hello"))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	// Make repo dirty (uncommitted changes in tracked file)
	mustWriteFile(t, filepath.Join(dir, "file.txt"), []byte("dirty"))

	// Create gitignored artifact
	nmDir := filepath.Join(dir, "node_modules")
	mustMkdir(t, nmDir)

	results := []model.ScanResult{
		{Path: nmDir, Safety: model.SafetySafe},
	}

	classifier.ApplyGitInfo(results)

	// node_modules is gitignored → should NOT be protected even though repo is dirty
	if results[0].Protected {
		t.Error("gitignored artifact should not be protected")
	}
	if results[0].Safety != model.SafetySafe {
		t.Errorf("expected safety=safe for gitignored artifact, got %s", results[0].Safety)
	}
}

func TestApplyGitInfo_TrackedArtifactProtected(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	// Create a tracked directory (vendor/)
	vendorDir := filepath.Join(dir, "vendor")
	mustMkdir(t, vendorDir)
	mustWriteFile(t, filepath.Join(vendorDir, "lib.go"), []byte("package lib"))
	mustWriteFile(t, filepath.Join(dir, "file.txt"), []byte("hello"))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	// Make repo dirty
	mustWriteFile(t, filepath.Join(dir, "file.txt"), []byte("dirty"))

	results := []model.ScanResult{
		{Path: vendorDir, Safety: model.SafetyCaution},
	}

	classifier.ApplyGitInfo(results)

	// vendor/ is tracked → should be protected when repo is dirty
	if !results[0].Protected {
		t.Error("tracked artifact should be protected when repo is dirty")
	}
	if results[0].Safety != model.SafetyProtected {
		t.Errorf("expected safety=protected, got %s", results[0].Safety)
	}
}

// TestApplyGitInfo_TrackedArtifactWithoutGitignore verifies that a non-gitignored
// artifact (node_modules not listed in .gitignore) in a dirty repo is marked
// protected, because ApplyGitInfo cannot confirm it is safely ignorable.
func TestApplyGitInfo_TrackedArtifactWithoutGitignore(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	f := filepath.Join(dir, "file.txt")
	mustWriteFile(t, f, []byte("hello"))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	// Make it dirty
	mustWriteFile(t, f, []byte("dirty"))

	// Create a fake artifact dir (no .gitignore, so not explicitly ignored)
	nmDir := filepath.Join(dir, "node_modules")
	mustMkdir(t, nmDir)

	results := []model.ScanResult{
		{Path: nmDir, Safety: model.SafetySafe},
	}

	classifier.ApplyGitInfo(results)

	if !results[0].Protected {
		t.Error("expected protected=true for dirty repo")
	}
	if results[0].Safety != model.SafetyProtected {
		t.Errorf("expected safety=protected, got %s", results[0].Safety)
	}
	if results[0].Reason != "uncommitted changes" {
		t.Errorf("expected reason='uncommitted changes', got %q", results[0].Reason)
	}
}

// TestApplyGitInfo_MultipleReposParallel exercises the concurrent root
// resolution and per-root git-info path: several distinct repos in one call,
// mixing dirty/clean state and two artifacts sharing one root. Each artifact
// must be attributed to its own repo's git info regardless of scheduling, so a
// mis-mapping in the parallel map surfaces as wrong protection here. Run under
// -race, it also guards the distinct-index writes against data races.
func TestApplyGitInfo_MultipleReposParallel(t *testing.T) {
	// dirty repo with a tracked artifact → protected
	dirtyRepo := t.TempDir()
	gitInit(t, dirtyRepo)
	dirtyVendor := filepath.Join(dirtyRepo, "vendor")
	mustMkdir(t, dirtyVendor)
	mustWriteFile(t, filepath.Join(dirtyVendor, "lib.go"), []byte("package lib"))
	mustWriteFile(t, filepath.Join(dirtyRepo, "file.txt"), []byte("hello"))
	gitRun(t, dirtyRepo, "add", ".")
	gitRun(t, dirtyRepo, "commit", "-m", "init")
	mustWriteFile(t, filepath.Join(dirtyRepo, "file.txt"), []byte("dirty"))

	// clean repo with two artifacts sharing the same root → never protected
	cleanRepo := t.TempDir()
	gitInit(t, cleanRepo)
	mustWriteFile(t, filepath.Join(cleanRepo, "file.txt"), []byte("hello"))
	gitRun(t, cleanRepo, "add", ".")
	gitRun(t, cleanRepo, "commit", "-m", "init")
	cleanA := filepath.Join(cleanRepo, "node_modules")
	cleanB := filepath.Join(cleanRepo, "dist")
	mustMkdir(t, cleanA)
	mustMkdir(t, cleanB)

	results := []model.ScanResult{
		{Path: dirtyVendor, Safety: model.SafetyCaution},
		{Path: cleanA, Safety: model.SafetySafe},
		{Path: cleanB, Safety: model.SafetySafe},
	}

	classifier.ApplyGitInfo(results)

	if !results[0].Protected {
		t.Error("tracked artifact in dirty repo should be protected")
	}
	if results[1].Protected || results[2].Protected {
		t.Error("artifacts in clean repo should not be protected")
	}
	// git rev-parse resolves symlinks (macOS /var → /private/var), so compare
	// against the resolved paths rather than the raw t.TempDir() values.
	wantClean := evalSymlinks(t, cleanRepo)
	wantDirty := evalSymlinks(t, dirtyRepo)
	if results[1].ProjectRoot != wantClean || results[2].ProjectRoot != wantClean {
		t.Errorf("shared-root artifacts should both map to %s, got %q and %q",
			wantClean, results[1].ProjectRoot, results[2].ProjectRoot)
	}
	if results[0].ProjectRoot != wantDirty {
		t.Errorf("dirty artifact should map to %s, got %q", wantDirty, results[0].ProjectRoot)
	}
}

func evalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlinks %s: %v", path, err)
	}
	return resolved
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "test")
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

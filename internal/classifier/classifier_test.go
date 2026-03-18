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
	os.WriteFile(f, []byte("hello"), 0o644)
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
	os.WriteFile(f, []byte("hello"), 0o644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	os.WriteFile(f, []byte("modified"), 0o644)

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
	os.WriteFile(f, []byte("hello"), 0o644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	// Create artifact with old mtime
	nmDir := filepath.Join(dir, "node_modules")
	os.MkdirAll(nmDir, 0o755)
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
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	// Make repo dirty (uncommitted changes in tracked file)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("dirty"), 0o644)

	// Create gitignored artifact
	nmDir := filepath.Join(dir, "node_modules")
	os.MkdirAll(nmDir, 0o755)

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
	os.MkdirAll(vendorDir, 0o755)
	os.WriteFile(filepath.Join(vendorDir, "lib.go"), []byte("package lib"), 0o644)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	// Make repo dirty
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("dirty"), 0o644)

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

func TestApplyGitInfo(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("hello"), 0o644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")

	// Make it dirty
	os.WriteFile(f, []byte("dirty"), 0o644)

	// Create a fake artifact dir
	nmDir := filepath.Join(dir, "node_modules")
	os.MkdirAll(nmDir, 0o755)

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

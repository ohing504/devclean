package cleaner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/cleaner"
	"github.com/ohing504/devclean/internal/model"
)

func TestForceDelete(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "node_modules")
	os.MkdirAll(filepath.Join(target, "pkg"), 0o755)
	os.WriteFile(filepath.Join(target, "pkg", "index.js"), []byte("x"), 0o644)

	c := cleaner.New(cleaner.Options{Force: true})
	result := model.ScanResult{Path: target, Size: 100, Safety: model.SafetySafe}

	err := c.Clean(result)
	if err != nil {
		t.Fatalf("Clean error: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected target to be deleted")
	}
}

func TestDryRun(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "node_modules")
	os.MkdirAll(target, 0o755)

	c := cleaner.New(cleaner.Options{DryRun: true})
	result := model.ScanResult{Path: target, Safety: model.SafetySafe}

	err := c.Clean(result)
	if err != nil {
		t.Fatalf("Clean error: %v", err)
	}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		t.Error("expected target to still exist in dry-run mode")
	}
}

func TestProtectedNotDeleted(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "src")
	os.MkdirAll(target, 0o755)

	c := cleaner.New(cleaner.Options{Force: true})
	result := model.ScanResult{Path: target, Protected: true, Safety: model.SafetyProtected}

	err := c.Clean(result)
	if err == nil {
		t.Error("expected error when cleaning protected item")
	}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		t.Error("protected target should not be deleted")
	}
}

func TestTrashDelete(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "node_modules")
	os.MkdirAll(filepath.Join(target, "pkg"), 0o755)
	os.WriteFile(filepath.Join(target, "pkg", "index.js"), []byte("x"), 0o644)

	// Use custom trash dir for testing (not actual system Trash)
	trashDir := filepath.Join(dir, "trash")
	os.MkdirAll(trashDir, 0o755)

	c := cleaner.New(cleaner.Options{TrashDir: trashDir})
	result := model.ScanResult{Path: target, Size: 100, Safety: model.SafetySafe}

	err := c.Clean(result)
	if err != nil {
		t.Fatalf("Clean error: %v", err)
	}

	// Original should be gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected target to be moved from original location")
	}

	// Should exist in trash
	trashed := filepath.Join(trashDir, "node_modules")
	if _, err := os.Stat(trashed); os.IsNotExist(err) {
		t.Error("expected target to exist in trash dir")
	}
}

func TestCleanAll(t *testing.T) {
	dir := t.TempDir()

	t1 := filepath.Join(dir, "a")
	t2 := filepath.Join(dir, "b")
	os.MkdirAll(t1, 0o755)
	os.MkdirAll(t2, 0o755)

	c := cleaner.New(cleaner.Options{Force: true})
	results := []model.ScanResult{
		{Path: t1, Size: 100, Safety: model.SafetySafe},
		{Path: t2, Size: 200, Safety: model.SafetySafe},
	}

	cleanResults := c.CleanAll(results)

	if len(cleanResults) != 2 {
		t.Fatalf("expected 2 results, got %d", len(cleanResults))
	}

	for _, cr := range cleanResults {
		if cr.Error != nil {
			t.Errorf("unexpected error: %v", cr.Error)
		}
	}
}

func TestCleanAllSkipsProtected(t *testing.T) {
	dir := t.TempDir()

	safe := filepath.Join(dir, "safe")
	protected := filepath.Join(dir, "protected")
	os.MkdirAll(safe, 0o755)
	os.MkdirAll(protected, 0o755)

	c := cleaner.New(cleaner.Options{Force: true})
	results := []model.ScanResult{
		{Path: safe, Size: 100, Safety: model.SafetySafe},
		{Path: protected, Size: 200, Safety: model.SafetyProtected, Protected: true},
	}

	cleanResults := c.CleanAll(results)

	// First should succeed
	if cleanResults[0].Error != nil {
		t.Errorf("expected safe item to succeed: %v", cleanResults[0].Error)
	}

	// Second should fail (protected)
	if cleanResults[1].Error == nil {
		t.Error("expected protected item to fail")
	}

	// Protected dir should still exist
	if _, err := os.Stat(protected); os.IsNotExist(err) {
		t.Error("protected dir should still exist")
	}
}

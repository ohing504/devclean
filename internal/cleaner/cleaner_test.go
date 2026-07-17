package cleaner_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/cleaner"
	"github.com/ohing504/devclean/internal/model"
)

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

func TestForceDelete(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "node_modules")
	mustMkdir(t, filepath.Join(target, "pkg"))
	mustWriteFile(t, filepath.Join(target, "pkg", "index.js"), []byte("x"))

	c := cleaner.New(cleaner.Options{Force: true})
	result := model.ScanResult{Path: target, Size: 100, Safety: model.SafetySafe}

	err := c.Clean(t.Context(), result)
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
	mustMkdir(t, target)

	c := cleaner.New(cleaner.Options{DryRun: true})
	result := model.ScanResult{Path: target, Safety: model.SafetySafe}

	err := c.Clean(t.Context(), result)
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
	mustMkdir(t, target)

	c := cleaner.New(cleaner.Options{Force: true})
	result := model.ScanResult{Path: target, Protected: true, Safety: model.SafetyProtected}

	err := c.Clean(t.Context(), result)
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
	mustMkdir(t, filepath.Join(target, "pkg"))
	mustWriteFile(t, filepath.Join(target, "pkg", "index.js"), []byte("x"))

	// Use custom trash dir for testing (not actual system Trash)
	trashDir := filepath.Join(dir, "trash")
	mustMkdir(t, trashDir)

	c := cleaner.New(cleaner.Options{TrashDir: trashDir})
	result := model.ScanResult{Path: target, Size: 100, Safety: model.SafetySafe}

	err := c.Clean(t.Context(), result)
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

func TestTrashDelete_NameConflict(t *testing.T) {
	dir := t.TempDir()
	trashDir := filepath.Join(dir, "trash")
	mustMkdir(t, trashDir)
	// Pre-create conflicting name in trash
	mustMkdir(t, filepath.Join(trashDir, "node_modules"))

	target := filepath.Join(dir, "node_modules")
	mustMkdir(t, target)

	c := cleaner.New(cleaner.Options{TrashDir: trashDir})
	result := model.ScanResult{Path: target, Safety: model.SafetySafe}
	err := c.Clean(t.Context(), result)
	if err != nil {
		t.Fatalf("Clean error: %v", err)
	}

	// Should create node_modules_1 in trash
	if _, err := os.Stat(filepath.Join(trashDir, "node_modules_1")); os.IsNotExist(err) {
		t.Error("expected node_modules_1 in trash for name conflict")
	}
}

func TestCleanAll(t *testing.T) {
	dir := t.TempDir()

	t1 := filepath.Join(dir, "a")
	t2 := filepath.Join(dir, "b")
	mustMkdir(t, t1)
	mustMkdir(t, t2)

	c := cleaner.New(cleaner.Options{Force: true})
	results := []model.ScanResult{
		{Path: t1, Size: 100, Safety: model.SafetySafe},
		{Path: t2, Size: 200, Safety: model.SafetySafe},
	}

	cleanResults := c.CleanAll(t.Context(), results)

	if len(cleanResults) != 2 {
		t.Fatalf("expected 2 results, got %d", len(cleanResults))
	}

	for _, cr := range cleanResults {
		if cr.Error != nil {
			t.Errorf("unexpected error: %v", cr.Error)
		}
	}

	// Verify all items were actually deleted
	for _, r := range results {
		if _, err := os.Stat(r.Path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be deleted after CleanAll", r.Path)
		}
	}
}

func TestCleanAllSkipsProtected(t *testing.T) {
	dir := t.TempDir()

	safe := filepath.Join(dir, "safe")
	protected := filepath.Join(dir, "protected")
	mustMkdir(t, safe)
	mustMkdir(t, protected)

	c := cleaner.New(cleaner.Options{Force: true})
	results := []model.ScanResult{
		{Path: safe, Size: 100, Safety: model.SafetySafe},
		{Path: protected, Size: 200, Safety: model.SafetyProtected, Protected: true},
	}

	cleanResults := c.CleanAll(t.Context(), results)

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

// deleteMethodItem builds a ScanResult whose reclaim goes through a
// DeleteMethod, backed by a real path so tests can prove path removal
// was NOT taken.
func deleteMethodItem(t *testing.T, ran *bool) model.ScanResult {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "vendor-managed")
	mustMkdir(t, target)
	return model.ScanResult{
		Path:   target,
		Size:   100,
		Safety: model.SafetySafe,
		Delete: &model.DeleteMethod{
			Kind:    model.DeleteKindCommand,
			Display: "vendor prune",
			Run: func(context.Context) error {
				*ran = true
				return nil
			},
		},
	}
}

func TestDeleteMethodRun(t *testing.T) {
	var ran bool
	result := deleteMethodItem(t, &ran)

	c := cleaner.New(cleaner.Options{Force: true})
	if err := c.Clean(t.Context(), result); err != nil {
		t.Fatalf("Clean error: %v", err)
	}

	if !ran {
		t.Error("expected DeleteMethod.Run to be called")
	}
	// Path removal must not have been taken, even with Force set.
	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("path must be untouched when a DeleteMethod is attached: %v", err)
	}
}

func TestDeleteMethodDryRun(t *testing.T) {
	var ran bool
	result := deleteMethodItem(t, &ran)

	c := cleaner.New(cleaner.Options{DryRun: true})
	if err := c.Clean(t.Context(), result); err != nil {
		t.Fatalf("Clean error: %v", err)
	}

	if ran {
		t.Error("dry-run must not execute DeleteMethod.Run")
	}
}

func TestDeleteMethodProtected(t *testing.T) {
	var ran bool
	result := deleteMethodItem(t, &ran)
	result.Protected = true

	c := cleaner.New(cleaner.Options{})
	if err := c.Clean(t.Context(), result); err == nil {
		t.Error("expected error for protected item")
	}

	if ran {
		t.Error("protected item must not execute DeleteMethod.Run")
	}
}

func TestDeleteMethodRunError(t *testing.T) {
	wantErr := errors.New("vendor tool failed")
	result := model.ScanResult{
		Path:   "/nonexistent",
		Safety: model.SafetySafe,
		Delete: &model.DeleteMethod{
			Kind: model.DeleteKindCommand,
			Run:  func(context.Context) error { return wantErr },
		},
	}

	c := cleaner.New(cleaner.Options{})
	if err := c.Clean(t.Context(), result); !errors.Is(err, wantErr) {
		t.Errorf("expected Run error to propagate, got %v", err)
	}
}

func TestDeleteMethodNilRun(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "misconfigured")
	mustMkdir(t, target)
	result := model.ScanResult{
		Path:   target,
		Safety: model.SafetySafe,
		Delete: &model.DeleteMethod{Kind: model.DeleteKindCommand},
	}

	c := cleaner.New(cleaner.Options{Force: true})
	if err := c.Clean(t.Context(), result); err == nil {
		t.Error("expected error for DeleteMethod without Run")
	}

	// No fallback to path removal for a misconfigured method.
	if _, err := os.Stat(target); err != nil {
		t.Errorf("path must survive a misconfigured DeleteMethod: %v", err)
	}
}

func TestCleanAllMixedStrategies(t *testing.T) {
	dir := t.TempDir()
	pathItem := filepath.Join(dir, "by-path")
	mustMkdir(t, pathItem)

	var ran bool
	methodItem := deleteMethodItem(t, &ran)

	c := cleaner.New(cleaner.Options{Force: true})
	cleanResults := c.CleanAll(t.Context(), []model.ScanResult{
		{Path: pathItem, Safety: model.SafetySafe},
		methodItem,
	})

	for _, cr := range cleanResults {
		if cr.Error != nil {
			t.Errorf("unexpected error for %s: %v", cr.Item.Path, cr.Error)
		}
	}
	if _, err := os.Stat(pathItem); !os.IsNotExist(err) {
		t.Error("path item should be removed via path strategy")
	}
	if !ran {
		t.Error("method item should be reclaimed via its Run")
	}
	if _, err := os.Stat(methodItem.Path); err != nil {
		t.Error("method item's path must be untouched")
	}
}

package scanner_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestFlutterScanner_FindsBuildAndDartTool(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myapp")
	mustMkdir(t, filepath.Join(projDir, "build", "app"))
	mustWriteFile(t, filepath.Join(projDir, "build", "app", "out"), make([]byte, 8192))
	mustMkdir(t, filepath.Join(projDir, ".dart_tool"))
	mustWriteFile(t, filepath.Join(projDir, ".dart_tool", "package_config.json"), make([]byte, 1024))
	mustWriteFile(t, filepath.Join(projDir, "pubspec.yaml"), []byte("name: myapp"))

	results, err := scanner.WalkScan(context.Background(), root, model.EcoFlutter)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results (build + .dart_tool), got %d", len(results))
	}

	for _, r := range results {
		if r.Ecosystem != model.EcoFlutter {
			t.Errorf("expected ecosystem=flutter, got %s", r.Ecosystem)
		}
		if r.Category != model.CatBuild {
			t.Errorf("expected category=build, got %s", r.Category)
		}
		if r.Safety != model.SafetySafe {
			t.Errorf("expected safety=safe, got %s", r.Safety)
		}
	}
}

func TestFlutterScanner_IgnoresWithoutPubspec(t *testing.T) {
	root := t.TempDir()

	// build/ + .dart_tool/ without pubspec.yaml should be ignored
	mustMkdir(t, filepath.Join(root, "random", "build"))
	mustMkdir(t, filepath.Join(root, "random", ".dart_tool"))

	results, err := scanner.WalkScan(context.Background(), root, model.EcoFlutter)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results without pubspec.yaml, got %d", len(results))
	}
}

func TestFlutterScanner_NameAndEcosystem(t *testing.T) {
	for _, s := range scanner.DefaultRegistry().All() {
		if s.Name() != "flutter" {
			continue
		}
		if s.Ecosystem() != model.EcoFlutter {
			t.Errorf("expected ecosystem=flutter, got %s", s.Ecosystem())
		}
		return
	}
	t.Error(`expected a registered scanner named "flutter"`)
}

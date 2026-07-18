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

func TestFlutterScanner_ExcludesSDKCheckout(t *testing.T) {
	root := t.TempDir()

	// A Flutter SDK checkout: bin/flutter + bin/internal/engine.version, with a
	// root pubspec.yaml and internal build/.dart_tool that must NOT be reported.
	sdk := filepath.Join(root, "flutter")
	mustMkdir(t, filepath.Join(sdk, "bin", "internal"))
	mustWriteFile(t, filepath.Join(sdk, "bin", "flutter"), []byte("#!/bin/sh"))
	mustWriteFile(t, filepath.Join(sdk, "bin", "internal", "engine.version"), []byte("abc123"))
	mustWriteFile(t, filepath.Join(sdk, "pubspec.yaml"), []byte("name: _flutter_packages"))
	// engine source tree named "build" (committed source, not build output)
	mustMkdir(t, filepath.Join(sdk, "engine", "src", "build"))
	mustWriteFile(t, filepath.Join(sdk, "engine", "src", "build", "BUILD.gn"), make([]byte, 4096))
	// internal package .dart_tool
	mustMkdir(t, filepath.Join(sdk, "packages", "flutter_tools", ".dart_tool"))
	mustWriteFile(t, filepath.Join(sdk, "packages", "flutter_tools", ".dart_tool", "x"), make([]byte, 4096))

	// A real app project alongside the SDK, which MUST still be reported.
	app := filepath.Join(root, "myapp")
	mustMkdir(t, filepath.Join(app, "build"))
	mustWriteFile(t, filepath.Join(app, "build", "out"), make([]byte, 4096))
	mustWriteFile(t, filepath.Join(app, "pubspec.yaml"), []byte("name: myapp"))

	results, err := scanner.WalkScan(context.Background(), root, model.EcoFlutter)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	for _, r := range results {
		if filepath.Base(filepath.Dir(r.Path)) == "src" || // engine/src/build
			r.Path == filepath.Join(sdk, "engine", "src", "build") {
			t.Errorf("SDK engine source tree must be excluded, got %s", r.Path)
		}
		if want := sdk; len(r.Path) >= len(want) && r.Path[:len(want)] == want {
			t.Errorf("no artifact under the SDK checkout may be reported, got %s", r.Path)
		}
	}

	// The app's build/ must survive the SDK exclusion.
	var foundApp bool
	for _, r := range results {
		if r.Path == filepath.Join(app, "build") {
			foundApp = true
		}
	}
	if !foundApp {
		t.Errorf("app build/ should still be reported alongside an excluded SDK; results=%v", results)
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

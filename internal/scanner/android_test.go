package scanner_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestAndroidScanner_FindsBuildAndGradle(t *testing.T) {
	root := t.TempDir()

	projDir := filepath.Join(root, "myapp")
	mustMkdir(t, filepath.Join(projDir, "build", "outputs"))
	mustWriteFile(t, filepath.Join(projDir, "build", "outputs", "app.apk"), make([]byte, 8192))
	mustMkdir(t, filepath.Join(projDir, ".gradle"))
	mustWriteFile(t, filepath.Join(projDir, ".gradle", "state"), make([]byte, 1024))
	mustWriteFile(t, filepath.Join(projDir, "build.gradle.kts"), []byte("plugins {}"))

	results, err := scanner.WalkScan(context.Background(), root, model.EcoAndroid)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results (build + .gradle), got %d", len(results))
	}
	for _, r := range results {
		if r.Ecosystem != model.EcoAndroid {
			t.Errorf("expected ecosystem=android, got %s", r.Ecosystem)
		}
		if r.Safety != model.SafetySafe {
			t.Errorf("expected safety=safe, got %s", r.Safety)
		}
	}
}

func TestAndroidScanner_IgnoresWithoutGradleMarker(t *testing.T) {
	root := t.TempDir()

	// build/ + .gradle/ without a build.gradle marker should be ignored.
	mustMkdir(t, filepath.Join(root, "random", "build"))
	mustMkdir(t, filepath.Join(root, "random", ".gradle"))

	results, err := scanner.WalkScan(context.Background(), root, model.EcoAndroid)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results without a gradle marker, got %d", len(results))
	}
}

// TestAndroidScanner_MultiModule pins the core design: each module carries its
// own build.gradle and is detected as its own project root, so a single `build`
// rule reclaims every module's output without enumerating module names.
func TestAndroidScanner_MultiModule(t *testing.T) {
	root := t.TempDir()

	// Root project with settings + build script, plus two subproject modules.
	mustWriteFile(t, filepath.Join(root, "build.gradle"), []byte("// root"))
	mustMkdir(t, filepath.Join(root, "build"))
	mustWriteFile(t, filepath.Join(root, "build", "out"), make([]byte, 4096))

	for _, mod := range []string{"app", "feature"} {
		modDir := filepath.Join(root, mod)
		mustMkdir(t, filepath.Join(modDir, "build"))
		mustWriteFile(t, filepath.Join(modDir, "build.gradle"), []byte("// "+mod))
		mustWriteFile(t, filepath.Join(modDir, "build", "out"), make([]byte, 4096))
	}

	results, err := scanner.WalkScan(context.Background(), root, model.EcoAndroid)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	want := map[string]bool{
		filepath.Join(root, "build"):            false,
		filepath.Join(root, "app", "build"):     false,
		filepath.Join(root, "feature", "build"): false,
	}
	for _, r := range results {
		if _, ok := want[r.Path]; ok {
			want[r.Path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("expected module build/ to be reported: %s (results=%v)", path, results)
		}
	}
}

// TestAndroidScanner_ReactNativeAttributedToNode pins that a React Native
// project's android/build is attributed to node (first in table order), not
// duplicated by the android scanner, while the android scanner still covers the
// deeper module build the RN rules do not list.
func TestAndroidScanner_ReactNativeAttributedToNode(t *testing.T) {
	root := t.TempDir()

	rn := filepath.Join(root, "rnapp")
	mustMkdir(t, rn)
	mustWriteFile(t, filepath.Join(rn, "package.json"), []byte("{}"))
	mustWriteFile(t, filepath.Join(rn, "metro.config.js"), []byte("module.exports = {}"))

	androidDir := filepath.Join(rn, "android")
	mustMkdir(t, filepath.Join(androidDir, "build"))
	mustWriteFile(t, filepath.Join(androidDir, "build.gradle"), []byte("// android"))
	mustWriteFile(t, filepath.Join(androidDir, "build", "out"), make([]byte, 4096))

	appDir := filepath.Join(androidDir, "app")
	mustMkdir(t, filepath.Join(appDir, "build"))
	mustWriteFile(t, filepath.Join(appDir, "build.gradle"), []byte("// app"))
	mustWriteFile(t, filepath.Join(appDir, "build", "out"), make([]byte, 4096))

	results, err := scanner.WalkScan(context.Background(), root, model.EcoNode, model.EcoAndroid)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	byPath := make(map[string]model.Ecosystem, len(results))
	for _, r := range results {
		if prev, dup := byPath[r.Path]; dup {
			t.Errorf("path reported twice: %s (%s and %s)", r.Path, prev, r.Ecosystem)
		}
		byPath[r.Path] = r.Ecosystem
	}

	if got := byPath[filepath.Join(androidDir, "build")]; got != model.EcoNode {
		t.Errorf("android/build should attribute to node, got %q", got)
	}
	if got := byPath[filepath.Join(appDir, "build")]; got != model.EcoAndroid {
		t.Errorf("android/app/build should attribute to android, got %q", got)
	}
}

func TestAndroidScanner_NameAndEcosystem(t *testing.T) {
	for _, s := range scanner.DefaultRegistry().All() {
		if s.Name() != "android" {
			continue
		}
		if s.Ecosystem() != model.EcoAndroid {
			t.Errorf("expected ecosystem=android, got %s", s.Ecosystem())
		}
		return
	}
	t.Error(`expected a registered scanner named "android"`)
}

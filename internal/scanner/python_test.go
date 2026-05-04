package scanner_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func TestPythonScanner_FindsPycache(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "myapp")
	mustMkdir(t, projDir)
	mustWriteFile(t, filepath.Join(projDir, "pyproject.toml"), []byte(`[project]`))

	pyc := filepath.Join(projDir, "src", "mypkg", "__pycache__")
	mustMkdir(t, pyc)
	mustWriteFile(t, filepath.Join(pyc, "x.cpython-312.pyc"), make([]byte, 1024))

	s := scanner.NewPythonScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	for _, r := range results {
		if r.Path == pyc {
			if r.Category != model.CatBuild {
				t.Errorf("expected category=build, got %s", r.Category)
			}
			if r.ProjectRoot != projDir {
				t.Errorf("expected project_root=%s, got %s", projDir, r.ProjectRoot)
			}
			if r.Safety != model.SafetySafe {
				t.Errorf("expected safety=safe, got %s", r.Safety)
			}
			return
		}
	}
	t.Fatalf("expected __pycache__ result at %s", pyc)
}

func TestPythonScanner_VenvIsCaution(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "myapp")
	mustMkdir(t, projDir)
	mustWriteFile(t, filepath.Join(projDir, "pyproject.toml"), []byte(`[project]`))

	venv := filepath.Join(projDir, ".venv", "lib")
	mustMkdir(t, venv)
	mustWriteFile(t, filepath.Join(venv, "site.py"), make([]byte, 4096))

	s := scanner.NewPythonScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	want := filepath.Join(projDir, ".venv")
	for _, r := range results {
		if r.Path == want {
			if r.Safety != model.SafetyCaution {
				t.Errorf(".venv: expected safety=caution (kondo#182), got %s", r.Safety)
			}
			if r.Category != model.CatDeps {
				t.Errorf(".venv: expected category=deps, got %s", r.Category)
			}
			return
		}
	}
	t.Fatalf("expected .venv result, got %d results", len(results))
}

func TestPythonScanner_DetectsAllMarkers(t *testing.T) {
	markers := []string{
		"pyproject.toml",
		"setup.py",
		"setup.cfg",
		"requirements.txt",
		"Pipfile",
		"uv.lock",
	}
	for _, marker := range markers {
		t.Run(marker, func(t *testing.T) {
			root := t.TempDir()
			projDir := filepath.Join(root, "proj")
			mustMkdir(t, projDir)
			mustWriteFile(t, filepath.Join(projDir, marker), []byte(""))

			cache := filepath.Join(projDir, ".pytest_cache")
			mustMkdir(t, cache)
			mustWriteFile(t, filepath.Join(cache, "v"), []byte("1"))

			s := scanner.NewPythonScanner()
			results, err := s.Scan(context.Background(), root)
			if err != nil {
				t.Fatalf("Scan error: %v", err)
			}
			if len(results) != 1 {
				t.Errorf("marker %q: expected 1 result, got %d", marker, len(results))
			}
		})
	}
}

func TestPythonScanner_IgnoresWithoutMarker(t *testing.T) {
	root := t.TempDir()
	// __pycache__ without any Python marker → should be ignored
	pyc := filepath.Join(root, "stray", "__pycache__")
	mustMkdir(t, pyc)
	mustWriteFile(t, filepath.Join(pyc, "f.pyc"), []byte("z"))

	s := scanner.NewPythonScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results without marker, got %d", len(results))
	}
}

func TestPythonScanner_EggInfoSuffix(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "pkg")
	mustMkdir(t, projDir)
	mustWriteFile(t, filepath.Join(projDir, "setup.py"), []byte(""))

	egg := filepath.Join(projDir, "mypackage.egg-info")
	mustMkdir(t, egg)
	mustWriteFile(t, filepath.Join(egg, "PKG-INFO"), make([]byte, 256))

	s := scanner.NewPythonScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	for _, r := range results {
		if r.Path == egg && r.Category == model.CatBuild {
			return
		}
	}
	t.Fatalf("expected mypackage.egg-info result, got %d results", len(results))
}

func TestPythonScanner_NestedProjectsAttributeToClosestRoot(t *testing.T) {
	root := t.TempDir()
	outer := filepath.Join(root, "outer")
	inner := filepath.Join(outer, "subpkg")
	mustMkdir(t, inner)
	mustWriteFile(t, filepath.Join(outer, "pyproject.toml"), []byte(""))
	mustWriteFile(t, filepath.Join(inner, "pyproject.toml"), []byte(""))

	pyc := filepath.Join(inner, "__pycache__")
	mustMkdir(t, pyc)
	mustWriteFile(t, filepath.Join(pyc, "x.pyc"), []byte("y"))

	s := scanner.NewPythonScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	for _, r := range results {
		if r.Path == pyc {
			if r.ProjectRoot != inner {
				t.Errorf("expected project_root=%s (closest), got %s", inner, r.ProjectRoot)
			}
			return
		}
	}
	t.Fatalf("expected __pycache__ result")
}

func TestPythonScanner_MultipleArtifactsInOneProject(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "proj")
	mustMkdir(t, projDir)
	mustWriteFile(t, filepath.Join(projDir, "pyproject.toml"), []byte(""))

	mustMkdir(t, filepath.Join(projDir, ".pytest_cache"))
	mustMkdir(t, filepath.Join(projDir, ".mypy_cache"))
	mustMkdir(t, filepath.Join(projDir, ".ruff_cache"))
	mustMkdir(t, filepath.Join(projDir, "__pycache__"))

	s := scanner.NewPythonScanner()
	results, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
		for _, r := range results {
			t.Logf("  %s", r.Path)
		}
	}
}

func TestPythonScanner_NameAndEcosystem(t *testing.T) {
	s := scanner.NewPythonScanner()
	if s.Name() != "python" {
		t.Errorf("expected name=python, got %s", s.Name())
	}
	if s.Ecosystem() != model.EcoPython {
		t.Errorf("expected ecosystem=python, got %s", s.Ecosystem())
	}
}

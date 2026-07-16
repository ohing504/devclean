package scanner_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

// TestLLMScanner_DetectsModelStores points HOME at a temp dir, seeds two LM
// Studio models, one Hugging Face hub model, and an Ollama store, and verifies
// per-model results, label decoding, safety, and a populated LastUsedAt.
func TestLLMScanner_DetectsModelStores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	lmA := filepath.Join(home, ".lmstudio", "models", "mistralai", "magistral-small")
	mustMkdir(t, lmA)
	mustWriteFile(t, filepath.Join(lmA, "weights.gguf"), make([]byte, 4096))

	lmB := filepath.Join(home, ".lmstudio", "models", "qwen", "qwen3-8b")
	mustMkdir(t, lmB)
	mustWriteFile(t, filepath.Join(lmB, "weights.gguf"), make([]byte, 2048))

	hf := filepath.Join(home, ".cache", "huggingface", "hub", "models--a--b")
	mustMkdir(t, hf)
	mustWriteFile(t, filepath.Join(hf, "blob"), make([]byte, 1024))

	ollama := filepath.Join(home, ".ollama", "models")
	mustMkdir(t, ollama)
	mustWriteFile(t, filepath.Join(ollama, "manifest"), make([]byte, 512))

	s := scanner.NewLLMScanner()
	results, err := s.Scan(context.Background(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results (2 LM Studio models + 1 HF model + Ollama store), got %d: %+v", len(results), results)
	}

	byLabel := make(map[string]model.ScanResult, len(results))
	for _, r := range results {
		if r.Ecosystem != model.EcoLLM {
			t.Errorf("expected ecosystem=llm, got %s for %s", r.Ecosystem, r.Path)
		}
		if r.Safety != model.SafetySafe {
			t.Errorf("expected safety=safe, got %s for %s", r.Safety, r.Path)
		}
		if r.LastUsedAt.IsZero() {
			t.Errorf("expected non-zero LastUsedAt for %s", r.Path)
		}
		if r.Recommendation == "" {
			t.Errorf("expected a re-download note in Recommendation for %s", r.Path)
		}
		byLabel[r.Label] = r
	}

	if r, ok := byLabel["mistralai/magistral-small"]; !ok {
		t.Errorf("expected LM Studio model label 'mistralai/magistral-small', got labels %v", labels(results))
	} else if r.Path != lmA {
		t.Errorf("mistralai/magistral-small: expected path %s, got %s", lmA, r.Path)
	}
	if _, ok := byLabel["qwen/qwen3-8b"]; !ok {
		t.Errorf("expected LM Studio model label 'qwen/qwen3-8b', got labels %v", labels(results))
	}

	if r, ok := byLabel["a/b"]; !ok {
		t.Errorf("expected HF hub label 'a/b' decoded from models--a--b, got labels %v", labels(results))
	} else if r.Path != hf {
		t.Errorf("a/b: expected path %s, got %s", hf, r.Path)
	}

	if r, ok := byLabel["Ollama model store"]; !ok {
		t.Errorf("expected 'Ollama model store', got labels %v", labels(results))
	} else if !strings.Contains(r.Recommendation, "ollama rm") {
		t.Errorf("Ollama store: expected 'ollama rm' hint in Recommendation, got %q", r.Recommendation)
	}
}

// TestLLMScanner_ScopedRootExcludesHomeStores mirrors the global scanner's
// scoping rule: home-rooted model stores are reported only when the scan root
// contains them. Scanning a subdirectory of home excludes them.
func TestLLMScanner_ScopedRootExcludesHomeStores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	lm := filepath.Join(home, ".lmstudio", "models", "org", "model")
	mustMkdir(t, lm)
	mustWriteFile(t, filepath.Join(lm, "weights.gguf"), make([]byte, 1024))

	projects := filepath.Join(home, "projects")
	mustMkdir(t, projects)

	s := scanner.NewLLMScanner()
	results, err := s.Scan(context.Background(), projects)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results when scanning a subdir of home, got %d", len(results))
	}
}

func labels(results []model.ScanResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Label
	}
	return out
}

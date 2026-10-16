package scanner_test

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
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

// hfHub builds a Hugging Face hub cache with a shared blob store the way
// huggingface_hub lays it out: payloads under blobs/<2 hex>/<hash>, repo
// blobs/<etag> symlinks to them, and a <hash>.refs manifest per payload.
type hfHub struct {
	t   *testing.T
	dir string
}

func newHFHub(t *testing.T, home string) hfHub {
	t.Helper()
	dir := filepath.Join(home, ".cache", "huggingface", "hub")
	mustMkdir(t, filepath.Join(dir, "blobs"))
	mustWriteFile(t, filepath.Join(dir, "blobs", ".huggingface-shared-blobs"), []byte("1\n"))
	return hfHub{t: t, dir: dir}
}

// payload writes a store payload whose hash is c repeated and returns its path.
func (h hfHub) payload(c string, size int) string {
	h.t.Helper()
	hash := strings.Repeat(c, 64)
	p := filepath.Join(h.dir, "blobs", hash[:2], hash)
	mustMkdir(h.t, filepath.Dir(p))
	mustWriteFile(h.t, p, bytes.Repeat([]byte{1}, size))
	return p
}

// link points repo's blobs/<etag> at payload and, with manifest, lists it in
// the payload's .refs.
func (h hfHub) link(repo, etag, payload string, manifest bool) {
	h.t.Helper()
	blobs := filepath.Join(h.dir, repo, "blobs")
	mustMkdir(h.t, blobs)
	rel, err := filepath.Rel(blobs, payload)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.Symlink(rel, filepath.Join(blobs, etag)); err != nil {
		h.t.Fatal(err)
	}
	if !manifest {
		return
	}
	f, err := os.OpenFile(payload+".refs", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666)
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(repo + "/blobs/" + etag + "\n"); err != nil {
		h.t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func scanLLM(t *testing.T, home string) map[string]model.ScanResult {
	t.Helper()
	results, err := scanner.NewLLMScanner().Scan(t.Context(), home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	byLabel := make(map[string]model.ScanResult, len(results))
	for _, r := range results {
		byLabel[r.Label] = r
	}
	return byLabel
}

// TestLLMScanner_HFSharedBlobSizes: a model's size includes the payloads only
// it references; a payload two models share counts once in the total, and
// only when both are present; a payload a dataset also references is never
// counted; payloads no repo references form one result of their own.
func TestLLMScanner_HFSharedBlobSizes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHFHub(t, home)

	own := hub.payload("a", 64*1024)
	shared := hub.payload("b", 32*1024)
	withDataset := hub.payload("c", 16*1024)
	orphan := hub.payload("d", 8*1024)
	hub.link("models--o--m1", "e1", own, true)
	hub.link("models--o--m1", "e2", shared, true)
	hub.link("models--o--m2", "e2", shared, true)
	hub.link("models--o--m2", "e3", withDataset, true)
	hub.link("datasets--o--d", "e3", withDataset, true)
	mustWriteFile(t, orphan+".refs", nil)

	got := scanLLM(t, home)
	m1, m2 := got["o/m1"], got["o/m2"]

	if want := scanner.Measure(m1.Path).Disk + scanner.DirSize(own); m1.Size != want {
		t.Errorf("m1 size = %d, want its directory plus its own payload = %d", m1.Size, want)
	}
	if want := scanner.Measure(m2.Path).Disk; m2.Size != want {
		t.Errorf("m2 size = %d, want its directory only = %d", m2.Size, want)
	}
	if got := model.DedupedTotal([]model.ScanResult{m1}); got != m1.Size {
		t.Errorf("m1 alone totals %d, want %d: the shared payload stays while m2 references it", got, m1.Size)
	}
	if got, want := model.DedupedTotal([]model.ScanResult{m1, m2}), m1.Size+m2.Size+scanner.DirSize(shared); got != want {
		t.Errorf("m1+m2 total = %d, want %d: shared payload once, dataset payload never", got, want)
	}

	o, ok := got["Hugging Face unreferenced shared blobs"]
	if !ok {
		t.Fatalf("expected an unreferenced shared blobs result, got labels %v", slices.Collect(maps.Keys(got)))
	}
	if o.Size != scanner.DirSize(orphan) {
		t.Errorf("unreferenced blobs size = %d, want %d", o.Size, scanner.DirSize(orphan))
	}
	if o.DeleteStrategy() != model.DeleteKindAPI {
		t.Errorf("unreferenced blobs delete strategy = %s, want api (the store directory itself must stay)", o.DeleteStrategy())
	}
}

// TestLLMScanner_HFReleaseSweepsUnreferencedPayloads: after a model folder is
// removed, its Release deletes the payloads and manifests nothing references
// any more and leaves the rest — still referenced, or with no manifest.
func TestLLMScanner_HFReleaseSweepsUnreferencedPayloads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHFHub(t, home)

	own := hub.payload("a", 4096)
	shared := hub.payload("b", 4096)
	unlisted := hub.payload("c", 4096)
	hub.link("models--o--m1", "e1", own, true)
	hub.link("models--o--m1", "e2", shared, true)
	hub.link("models--o--m1", "e3", unlisted, false)
	hub.link("models--o--m2", "e2", shared, true)

	got := scanLLM(t, home)
	release := func(r model.ScanResult) {
		t.Helper()
		if err := os.RemoveAll(r.Path); err != nil {
			t.Fatal(err)
		}
		if err := r.Release(os.RemoveAll); err != nil {
			t.Fatalf("Release: %v", err)
		}
	}

	release(got["o/m1"])
	if exists(own) || exists(own+".refs") {
		t.Error("m1's own payload and manifest should be deleted with m1")
	}
	if !exists(shared) {
		t.Error("shared payload must stay while m2 references it")
	}
	if !exists(unlisted) {
		t.Error("a payload without a manifest must never be deleted")
	}

	release(got["o/m2"])
	if exists(shared) || exists(shared+".refs") {
		t.Error("shared payload should be deleted once no model references it")
	}
}

// TestLLMScanner_HFOrphanDelete: deleting the unreferenced blobs result
// removes those payloads and manifests, but leaves a payload whose
// huggingface_hub lock is held (a download in progress).
func TestLLMScanner_HFOrphanDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHFHub(t, home)

	free := hub.payload("a", 4096)
	locked := hub.payload("b", 4096)
	mustWriteFile(t, free+".refs", nil)
	mustWriteFile(t, locked+".refs", nil)

	lock, err := os.OpenFile(locked+".lock", os.O_WRONLY|os.O_CREATE, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	o := scanLLM(t, home)["Hugging Face unreferenced shared blobs"]
	if err := o.Delete.Run(t.Context()); err != nil {
		t.Fatalf("Delete.Run: %v", err)
	}
	if exists(free) || exists(free+".refs") {
		t.Error("unreferenced payload and manifest should be deleted")
	}
	if !exists(locked) {
		t.Error("a payload whose lock is held must be left in place")
	}
	if !exists(hub.dir + "/blobs/.huggingface-shared-blobs") {
		t.Error("the store marker must stay")
	}
}

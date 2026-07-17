package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// Recommendation notes for LLM model stores. All entries are re-downloadable
// weights, so the note states how the models come back after deletion.
const (
	lmStudioRec  = "model weights — re-downloadable in LM Studio"
	hfHubRec     = "cached model weights — huggingface_hub re-downloads them on next use"
	ollamaRec    = "model weights — re-downloadable with 'ollama pull'; prefer 'ollama rm <model>' to remove individual models"
	llamafileRec = "downloaded llamafiles — re-downloadable"
)

// LLMScanner scans local LLM model stores at fixed home paths: LM Studio and
// Hugging Face hub models (per-model results), plus the Ollama and llamafile
// stores as a whole. Model weights dominate these directories (often tens of
// GB) and are always re-downloadable, so everything is `safe`.
//
// LastUsedAt is derived from the model directory mtime (store mtime for
// Ollama/llamafile) — a rough but scanner-local signal; log-based usage
// analysis is a possible future refinement.
type LLMScanner struct{}

func NewLLMScanner() *LLMScanner {
	return &LLMScanner{}
}

func (s *LLMScanner) Name() string               { return "llm" }
func (s *LLMScanner) Ecosystem() model.Ecosystem { return model.EcoLLM }

func (s *LLMScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	var results []model.ScanResult
	add := func(path, label, rec string) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		results = append(results, sized(model.ScanResult{
			Path:           path,
			Ecosystem:      model.EcoLLM,
			Category:       model.CatCache,
			LastMod:        ModTime(path),
			Safety:         model.SafetySafe,
			ProjectRoot:    filepath.Dir(path),
			Label:          label,
			Recommendation: rec,
			LastUsedAt:     ModTime(path),
		}))
		ReportProgress(ctx, len(results))
		return true
	}

	// LM Studio: one result per model directory (~/.lmstudio/models/<org>/<model>).
	lmRoot := filepath.Join(home, ".lmstudio", "models")
	if isUnderRoot(lmRoot, absRoot) {
		for _, dir := range subdirs(lmRoot) {
			for _, modelDir := range subdirs(dir) {
				label := filepath.Base(dir) + "/" + filepath.Base(modelDir)
				if !add(modelDir, label, lmStudioRec) {
					return results, ctx.Err()
				}
			}
		}
	}

	// Hugging Face hub: one result per model directory
	// (~/.cache/huggingface/hub/models--<org>--<name>).
	hfRoot := filepath.Join(home, ".cache", "huggingface", "hub")
	if isUnderRoot(hfRoot, absRoot) {
		for _, dir := range subdirs(hfRoot) {
			base := filepath.Base(dir)
			if !strings.HasPrefix(base, "models--") {
				continue
			}
			if !add(dir, hfModelLabel(base), hfHubRec) {
				return results, ctx.Err()
			}
		}
	}

	// Ollama and llamafile: the store as a whole (per-model breakdown needs
	// manifest parsing — the vendor CLI is the right tool for single models).
	wholeStores := []struct {
		relPath string
		label   string
		rec     string
	}{
		{filepath.Join(".ollama", "models"), "Ollama model store", ollamaRec},
		{".llamafile", "llamafile store", llamafileRec},
	}
	for _, store := range wholeStores {
		full := filepath.Join(home, store.relPath)
		if !isUnderRoot(full, absRoot) {
			continue
		}
		info, err := os.Stat(full)
		if err != nil || !info.IsDir() {
			continue
		}
		if !add(full, store.label, store.rec) {
			return results, ctx.Err()
		}
	}

	return results, nil
}

// subdirs returns the immediate subdirectories of dir (empty on any error).
func subdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(dir, e.Name()))
		}
	}
	return dirs
}

// hfModelLabel decodes a Hugging Face hub cache directory name into the repo
// id: "models--<org>--<name>" → "<org>/<name>". Hub repo ids cannot contain
// consecutive hyphens, so "--" is an unambiguous separator.
func hfModelLabel(base string) string {
	return strings.ReplaceAll(strings.TrimPrefix(base, "models--"), "--", "/")
}

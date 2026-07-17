package cli

import (
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

// TestSelectForYes verifies the non-interactive (--yes) safety gate: only safe
// items are deleted by default, caution is opt-in, and protected is never
// deleted — the guard that stops a mis-classified entry from silently vanishing.
func TestSelectForYes(t *testing.T) {
	results := []model.ScanResult{
		{Path: "/safe", Safety: model.SafetySafe},
		{Path: "/caution", Safety: model.SafetyCaution},
		{Path: "/protected-safety", Safety: model.SafetyProtected},
		{Path: "/git-protected", Safety: model.SafetySafe, Protected: true},
	}

	t.Run("default deletes only safe", func(t *testing.T) {
		toClean, skipped := selectForYes(results, false)
		if len(toClean) != 1 || toClean[0].Path != "/safe" {
			t.Errorf("expected only /safe, got %v", paths(toClean))
		}
		if skipped != 1 {
			t.Errorf("expected 1 skipped caution, got %d", skipped)
		}
	})

	t.Run("include-caution adds caution but not protected", func(t *testing.T) {
		toClean, skipped := selectForYes(results, true)
		if got := paths(toClean); len(got) != 2 || !contains(got, "/safe") || !contains(got, "/caution") {
			t.Errorf("expected /safe and /caution, got %v", got)
		}
		if skipped != 0 {
			t.Errorf("expected 0 skipped, got %d", skipped)
		}
		// Protected safety and git-protected must never appear.
		for _, r := range toClean {
			if r.Path == "/protected-safety" || r.Path == "/git-protected" {
				t.Errorf("protected item %s must never be selected", r.Path)
			}
		}
	})

	t.Run("git-protected safe item is never deleted", func(t *testing.T) {
		toClean, _ := selectForYes(results, true)
		if contains(paths(toClean), "/git-protected") {
			t.Error("a git-protected (dirty) item must not be deleted even if safety=safe")
		}
	})
}

func paths(rs []model.ScanResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Path
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

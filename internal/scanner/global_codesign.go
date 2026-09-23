package scanner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// processRunning reports whether a process named exactly name is running,
// using pgrep -x. It is GlobalScanner.ProcessRunning's default.
func processRunning(name string) bool {
	return exec.Command("pgrep", "-x", name).Run() == nil
}

// Recommendation notes for code-sign clones, keyed by browser run state:
// clones of a browser that is not running are true zombies (safe); while the
// browser runs, its newest copy may be in use (caution); for bundle IDs we
// cannot map to a process name the run state is unknowable (caution).
const (
	codeSignCloneRec        = "zombie copies from killed browser processes (headless automation like lighthouse/puppeteer); the browser cleans these on next normal exit. Size may overstate if copies are APFS clones."
	codeSignCloneRunningRec = "browser is currently running — newest copy may be in use; it cleans up leftovers on normal exit"
	codeSignCloneUnknownRec = "unrecognized browser — cannot check whether it is running (newest copy may be in use); it cleans up leftovers on normal exit"
)

// codeSignCloneBrowser describes a known Chromium-family browser: the display
// name used in labels and the process name checked (pgrep -x) to tell whether
// the browser is currently running.
type codeSignCloneBrowser struct {
	displayName string
	processName string
}

// codeSignCloneBrowsers maps Chromium-family bundle IDs to browser info.
// Unknown bundle IDs fall back to the raw bundle ID and a caution rating, so
// detection is never limited to this list — matching is done by the
// *.code_sign_clone glob.
var codeSignCloneBrowsers = map[string]codeSignCloneBrowser{
	"com.google.Chrome":          {"Chrome", "Google Chrome"},
	"com.google.Chrome.canary":   {"Chrome Canary", "Google Chrome Canary"},
	"com.brave.Browser":          {"Brave", "Brave Browser"},
	"com.microsoft.edgemac":      {"Edge", "Microsoft Edge"},
	"company.thebrowser.Browser": {"Arc", "Arc"},
	"com.vivaldi.Vivaldi":        {"Vivaldi", "Vivaldi"},
	"org.chromium.Chromium":      {"Chromium", "Chromium"},
	"com.naver.Whale":            {"Whale", "Whale"},
}

// scanCodeSignClones finds leftover browser code-sign clones under the
// per-user system temp root (macOS only). Chromium-family browsers copy their
// own bundle to /private/var/folders/<xx>/<yyy>/X/<bundle-id>.code_sign_clone/
// on launch to verify their code signature and remove the copy on normal exit.
// Force-killed processes — typically headless automation such as lighthouse or
// puppeteer — leave the copies behind, and they accumulate (observed in the
// wild: 92 copies / 156 GB).
//
// found is the number of results already reported so progress keeps counting up.
//
// Safety depends on run state: clones of a browser that is not running are
// safe zombies; while the browser runs (checked once per browser via
// ProcessRunning) or when the bundle ID is unrecognized, they are caution.
//
// Sizes come from allocated blocks (Measure) and still overstate real disk
// usage when the copies are APFS clones of the installed app bundle: clones
// have distinct inodes and each reports full blocks, so neither block counting
// nor inode dedup catches the sharing. Clone-aware measurement is a planned
// follow-up (needs APFS extent-level accounting).
func (s *GlobalScanner) scanCodeSignClones(ctx context.Context, found int) []model.ScanResult {
	matches, err := filepath.Glob(filepath.Join(s.TmpRoot, "*", "*", "X", "*.code_sign_clone"))
	if err != nil {
		return nil
	}

	// Memoize run-state checks: one pgrep per browser, not per clone dir.
	running := make(map[string]bool)
	isRunning := func(processName string) bool {
		if v, ok := running[processName]; ok {
			return v
		}
		v := s.ProcessRunning(processName)
		running[processName] = v
		return v
	}

	var out []model.ScanResult
	for _, m := range matches {
		select {
		case <-ctx.Done():
			return out
		default:
		}
		info, err := os.Stat(m)
		if err != nil || !info.IsDir() {
			continue
		}

		bundleID := strings.TrimSuffix(filepath.Base(m), ".code_sign_clone")
		browser, known := codeSignCloneBrowsers[bundleID]

		// Default: unrecognized bundle — no process name to check, so the
		// run state is unknowable; stay conservative.
		name := bundleID
		safety := model.SafetyCaution
		rec := codeSignCloneUnknownRec
		if known {
			name = browser.displayName
			if isRunning(browser.processName) {
				rec = codeSignCloneRunningRec
			} else {
				safety = model.SafetySafe
				rec = codeSignCloneRec
			}
		}

		out = append(out, model.ScanResult{
			Path:           m,
			Ecosystem:      model.EcoGlobal,
			Category:       model.CatCache,
			LastMod:        ModTime(m),
			Safety:         safety,
			ProjectRoot:    filepath.Dir(m),
			Label:          codeSignCloneLabel(m, name),
			Recommendation: rec,
		})
		ReportProgress(ctx, found+len(out))
	}
	return out
}

// codeSignCloneLabel derives e.g. "Chrome code-sign clones (92 copies)" from
// the clone directory: browser display name plus the number of immediate child
// directories (one copy per killed process).
func codeSignCloneLabel(dir, name string) string {
	copies := 0
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				copies++
			}
		}
	}
	unit := "copies"
	if copies == 1 {
		unit = "copy"
	}
	return fmt.Sprintf("%s code-sign clones (%d %s)", name, copies, unit)
}

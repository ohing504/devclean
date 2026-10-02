package scanner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// RunState is the result of checking whether a browser process is running.
// The zero value is RunStateUnknown, so an unset state never reads as safe.
type RunState int

const (
	// RunStateUnknown means the check itself failed (pgrep missing or
	// erroring), so the browser may be running.
	RunStateUnknown RunState = iota
	RunStateNotRunning
	RunStateRunning
)

// processRunState checks for a process named exactly name with pgrep -x. Only
// pgrep's "no match" exit (1) means not running; any other failure is unknown.
// It is GlobalScanner.ProcessRunState's default.
func processRunState(name string) RunState {
	err := exec.Command("pgrep", "-x", name).Run()
	if err == nil {
		return RunStateRunning
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return RunStateNotRunning
	}
	return RunStateUnknown
}

// Recommendation notes for code-sign clones, keyed by browser run state:
// clones of a browser that is not running are true zombies (safe); while the
// browser runs, its newest copy may be in use (caution); when the run-state
// check fails, or for bundle IDs we cannot map to a process name, the run state
// is unknowable (caution).
const (
	codeSignCloneRec             = "zombie copies from killed browser processes (headless automation like lighthouse/puppeteer); the browser cleans these on next normal exit"
	codeSignCloneRunningRec      = "browser is currently running — newest copy may be in use; it cleans up leftovers on normal exit"
	codeSignCloneUnrecognizedRec = "unrecognized browser — cannot check whether it is running (newest copy may be in use); it cleans up leftovers on normal exit"
	codeSignCloneCheckFailedRec  = "cannot check whether the browser is running (pgrep failed) — newest copy may be in use; it cleans up leftovers on normal exit"
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
// wild: 92 copies summing to 156 GB of allocated blocks, most of it shared).
//
// found is the number of results already reported so progress keeps counting up.
//
// Safety depends on run state: clones of a browser that is not running are
// safe zombies; while the browser runs (checked once per browser via
// ProcessRunState), when that check fails, or when the bundle ID is
// unrecognized, they are caution.
//
// The copies are APFS clones of each other and of the installed app bundle,
// so sizing is clone-aware with that bundle as clone source (see clone.go):
// Size is what deleting the copies frees.
func (s *GlobalScanner) scanCodeSignClones(ctx context.Context, found int) []model.ScanResult {
	matches, err := filepath.Glob(filepath.Join(s.TmpRoot, "*", "*", "X", "*.code_sign_clone"))
	if err != nil {
		return nil
	}

	// Memoize run-state checks: one pgrep per browser, not per clone dir.
	states := make(map[string]RunState)
	runState := func(processName string) RunState {
		if v, ok := states[processName]; ok {
			return v
		}
		v := s.ProcessRunState(processName)
		states[processName] = v
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
		rec := codeSignCloneUnrecognizedRec
		if known {
			name = browser.displayName
			switch runState(browser.processName) {
			case RunStateNotRunning:
				safety = model.SafetySafe
				rec = codeSignCloneRec
			case RunStateRunning:
				rec = codeSignCloneRunningRec
			default:
				rec = codeSignCloneCheckFailedRec
			}
		}

		source := ""
		if known {
			source = s.installedApp(browser.processName)
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
			CloneAware:     true,
			CloneSource:    source,
		})
		ReportProgress(ctx, found+len(out))
	}
	return out
}

// installedApp returns the installed bundle "<appName>.app" from AppDirs, or
// "" when none is found. Each browser's app bundle is named after its process.
func (s *GlobalScanner) installedApp(appName string) string {
	for _, d := range s.AppDirs {
		p := filepath.Join(d, appName+".app")
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return ""
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

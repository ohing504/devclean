package scanner

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ohing504/devclean/internal/model"
)

// homebrewCacheRelPath is Homebrew's default download cache, relative to home.
// It is a catalog entry like any other cache; with brew installed, the
// Homebrew cleanup item replaces it when brew reports reclaimable space there.
const homebrewCacheRelPath = "Library/Caches/Homebrew"

// homebrewCleanupArgs is the cleanup the Homebrew item runs: the same cleanup
// brew runs by itself after install and upgrade (HOMEBREW_CLEANUP_PERIODIC),
// except that casks are cleaned only by that periodic run. It removes formula
// versions older than the installed one and downloads older than
// HOMEBREW_CLEANUP_MAX_AGE_DAYS. -s (scrub) is deliberately not passed: it
// also deletes the formula API cache and downloads brew still uses, which it
// fetches again on the next command. The dry-run appends --dry-run to the
// same arguments so the reported size matches what the delete frees.
var homebrewCleanupArgs = []string{"cleanup"}

// homebrewTimeout bounds each brew command of the scan, so a brew that waits
// on a lock cannot stall the scan indefinitely.
const homebrewTimeout = 2 * time.Minute

// homebrewEnv applies to both the dry-run and the cleanup.
// HOMEBREW_NO_AUTOREMOVE=1 disables the autoremove step `brew cleanup` runs by
// default, which uninstalls formulae that were installed only as dependencies
// and are no longer needed: that removes installed software, not cache, and
// the dry-run size does not include it. The other two keep the output plain.
var homebrewEnv = []string{
	"HOMEBREW_NO_AUTOREMOVE=1",
	"HOMEBREW_NO_COLOR=1",
	"HOMEBREW_NO_ENV_HINTS=1",
}

// homebrewFreeLine matches the dry-run summary, e.g. "This operation would
// free approximately 2.3GB of disk space." brew prints it only when the size
// is nonzero, formatting with decimal units and at most one decimal place.
var homebrewFreeLine = regexp.MustCompile(`would free approximately ([0-9]+(?:\.[0-9]+)?)(B|KB|MB|GB|TB) of disk space`)

// homebrewUnitBytes maps brew's decimal size units to bytes.
var homebrewUnitBytes = map[string]float64{
	"B":  1,
	"KB": 1e3,
	"MB": 1e6,
	"GB": 1e9,
	"TB": 1e12,
}

// homebrewScan is the outcome of the Homebrew dry-run: an item when brew
// reports reclaimable space, nil item and nil err when there is nothing to
// free, or err when brew could not be run or its output not understood.
type homebrewScan struct {
	item *model.ScanResult
	err  error
}

// startHomebrewScan runs the Homebrew dry-run in a goroutine and delivers its
// outcome on the returned channel (buffered, so an abandoned scan does not
// leak the goroutine).
func (s *GlobalScanner) startHomebrewScan(ctx context.Context, brew string) <-chan homebrewScan {
	ch := make(chan homebrewScan, 1)
	go func() {
		item, err := s.scanHomebrew(ctx, brew)
		ch <- homebrewScan{item: item, err: err}
	}()
	return ch
}

// scanHomebrew builds the Homebrew cleanup item from `brew cleanup
// --dry-run`. The size is brew's own estimate of what the cleanup frees: old
// formula versions under the Homebrew prefix, stale downloads and logs. The
// prefix lies outside the cache directory, so measuring the directory would
// both miss those and count downloads brew keeps.
func (s *GlobalScanner) scanHomebrew(ctx context.Context, brew string) (*model.ScanResult, error) {
	ctx, cancel := context.WithTimeout(ctx, homebrewTimeout)
	defer cancel()

	dryRunArgs := append(append([]string{}, homebrewCleanupArgs...), "--dry-run")
	out, err := s.runCommand(ctx, homebrewEnv, brew, dryRunArgs...)
	if err != nil {
		return nil, err
	}
	size, found, err := parseHomebrewFreeSize(string(out))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil // brew has nothing to clean up
	}

	// The item's path is brew's cache directory, which HOMEBREW_CACHE can
	// move out of home.
	out, err = s.runCommand(ctx, homebrewEnv, brew, "--cache")
	if err != nil {
		return nil, err
	}
	cacheDir := strings.TrimSpace(string(out))
	if cacheDir == "" || !filepath.IsAbs(cacheDir) {
		return nil, fmt.Errorf("brew --cache printed %q, want an absolute path", cacheDir)
	}

	run := s.runCommand
	args := homebrewCleanupArgs
	return &model.ScanResult{
		Path:           cacheDir,
		Ecosystem:      model.EcoGlobal,
		Category:       model.CatCache,
		Size:           size,
		ApparentSize:   size,
		LastMod:        ModTime(cacheDir),
		Safety:         model.SafetySafe,
		ProjectRoot:    filepath.Dir(cacheDir),
		Label:          "Homebrew cleanup (old formula versions, stale downloads, logs)",
		Recommendation: "size is brew's own dry-run estimate; the same cleanup brew runs after upgrades — installed versions and current downloads are kept, dependency autoremove is disabled",
		Delete: &model.DeleteMethod{
			Kind:    model.DeleteKindCommand,
			Display: "HOMEBREW_NO_AUTOREMOVE=1 brew " + strings.Join(args, " "),
			Run: func(ctx context.Context) error {
				_, err := run(ctx, homebrewEnv, brew, args...)
				return err
			},
		},
	}, nil
}

// mergeHomebrew adds the Homebrew dry-run outcome to the sized catalog
// results. The cleanup item replaces the catalog entry for the same
// directory, since both would reclaim it; a cache directory elsewhere stays
// as its own entry. When the dry-run failed, the catalog entry for brew's
// default cache (defaultCache) carries the error, so the user sees why no cleanup item is
// offered.
func mergeHomebrew(results []model.ScanResult, hs homebrewScan, defaultCache string) []model.ScanResult {
	if hs.err != nil {
		for i := range results {
			if results[i].Path == defaultCache {
				results[i].Recommendation = fmt.Sprintf("brew cleanup dry-run failed (%v); deleting this directory removes every Homebrew download, which brew downloads again when it needs one", hs.err)
			}
		}
		return results
	}
	if hs.item == nil {
		return results
	}
	for i := range results {
		if results[i].Path == hs.item.Path {
			results[i] = *hs.item
			return results
		}
	}
	return append(results, *hs.item)
}

// parseHomebrewFreeSize extracts the byte count from brew cleanup dry-run
// output. found is false when the output has no summary line, which brew
// omits when nothing would be freed.
func parseHomebrewFreeSize(out string) (size int64, found bool, err error) {
	m := homebrewFreeLine.FindStringSubmatch(out)
	if m == nil {
		return 0, false, nil
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false, fmt.Errorf("brew cleanup dry-run size %q: %w", m[1], err)
	}
	// Round, not truncate: 4.1 * 1e9 is 4099999999.9999995 in floating point.
	return int64(math.Round(n * homebrewUnitBytes[m[2]])), true, nil
}

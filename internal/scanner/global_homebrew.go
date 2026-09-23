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
const homebrewCacheRelPath = "Library/Caches/Homebrew"

// homebrewCleanupArgs is the cleanup brew runs by itself after upgrades.
// No -s: it also deletes the API cache and downloads brew still uses.
var homebrewCleanupArgs = []string{"cleanup"}

// homebrewTimeout bounds each brew command so a lock wait cannot stall the scan.
const homebrewTimeout = 2 * time.Minute

// homebrewEnv: HOMEBREW_NO_AUTOREMOVE=1 stops `brew cleanup` from
// uninstalling unused dependency formulae, which is not cache and is not in
// the dry-run size.
var homebrewEnv = []string{
	"HOMEBREW_NO_AUTOREMOVE=1",
	"HOMEBREW_NO_COLOR=1",
	"HOMEBREW_NO_ENV_HINTS=1",
}

// homebrewFreeLine matches the dry-run summary, printed only when the size is
// nonzero: "This operation would free approximately 2.3GB of disk space."
var homebrewFreeLine = regexp.MustCompile(`would free approximately ([0-9]+(?:\.[0-9]+)?)(B|KB|MB|GB|TB) of disk space`)

// homebrewUnitBytes maps brew's decimal size units to bytes.
var homebrewUnitBytes = map[string]float64{
	"B":  1,
	"KB": 1e3,
	"MB": 1e6,
	"GB": 1e9,
	"TB": 1e12,
}

// homebrewScan is the dry-run outcome; item is nil when there is nothing to free.
type homebrewScan struct {
	item *model.ScanResult
	err  error
}

// startHomebrewScan runs scanHomebrew concurrently with catalog sizing.
func (s *GlobalScanner) startHomebrewScan(ctx context.Context, brew string) <-chan homebrewScan {
	ch := make(chan homebrewScan, 1)
	go func() {
		item, err := s.scanHomebrew(ctx, brew)
		ch <- homebrewScan{item: item, err: err}
	}()
	return ch
}

// scanHomebrew builds the cleanup item sized by brew's dry-run estimate, which
// includes old versions under the Homebrew prefix outside the cache directory.
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

// mergeHomebrew replaces the catalog entry at the item's path with the item,
// or on a dry-run error notes the error on the defaultCache entry.
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

// parseHomebrewFreeSize returns found=false when there is no summary line.
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

package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ohing504/devclean/internal/classifier"
	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
	"github.com/ohing504/devclean/internal/ui"
)

// toEcosystems converts CLI ecosystem names to model values.
func toEcosystems(names []string) []model.Ecosystem {
	ecosystems := make([]model.Ecosystem, 0, len(names))
	for _, n := range names {
		ecosystems = append(ecosystems, model.Ecosystem(n))
	}
	return ecosystems
}

// selectScanners returns the scanners for the given ecosystems, or all
// registered scanners when none are specified.
func selectScanners(reg *scanner.Registry, ecosystems []model.Ecosystem) []scanner.Scanner {
	if len(ecosystems) == 0 {
		return reg.All()
	}
	return reg.ForEcosystems(ecosystems)
}

// ScanPipelineOptions configures the shared scan pipeline.
type ScanPipelineOptions struct {
	Path     string
	Ecos     []string
	Status   string
	Category string
	SafeOnly bool
	MinSize  int64 // drop artifacts smaller than this many bytes (0 = no filter)
	Quiet    bool  // skip spinner (for JSON output)
}

// runScanPipeline scans, classifies, and filters results according to opts.
func runScanPipeline(opts ScanPipelineOptions) ([]model.ScanResult, error) {
	scanPath := opts.Path
	if scanPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		scanPath = home
	}

	reg := scanner.DefaultRegistry()
	scanners := selectScanners(reg, toEcosystems(opts.Ecos))

	// Ctrl-C / SIGTERM cancels the context so scanners' ctx.Done() checks
	// actually abort the walk instead of being dead code.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var sp *ui.Spinner
	if !opts.Quiet {
		sp = ui.NewSpinner("Scanning...")
	}

	currentEco := ""
	results, err := reg.ScanWithProgress(ctx, scanPath, scanners, func(eco string, found int) {
		if eco != "" {
			currentEco = eco
		}
		if sp != nil {
			sp.Update(fmt.Sprintf("Scanning %s... (%d items found)", currentEco, found))
		}
	})
	if sp != nil {
		sp.Stop()
	}
	if err != nil {
		return nil, err
	}

	if !opts.Quiet {
		sp = ui.NewSpinner("Classifying...")
	}
	classifier.ApplyGitInfo(results)
	classifier.ClassifyResults(results, classifier.DefaultThresholds())
	if sp != nil {
		sp.Stop()
	}

	// Apply filters
	if opts.Status != "" {
		results = model.FilterResults(results, func(r model.ScanResult) bool {
			return r.Activity == model.ActivityStatus(opts.Status)
		})
	}
	if opts.Category != "" {
		results = model.FilterResults(results, func(r model.ScanResult) bool {
			return r.Category == model.Category(opts.Category)
		})
	}
	if opts.SafeOnly {
		results = model.FilterResults(results, func(r model.ScanResult) bool {
			return r.Safety == model.SafetySafe
		})
	}
	if opts.MinSize > 0 {
		results = model.FilterResults(results, func(r model.ScanResult) bool {
			return r.Size >= opts.MinSize
		})
	}

	return results, nil
}

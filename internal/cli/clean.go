package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/ohing504/devclean/internal/cleaner"
	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/pathutil"
	"github.com/ohing504/devclean/internal/scanner"
	"github.com/ohing504/devclean/internal/ui"
	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	var (
		scanPath       string
		ecos           []string
		status         string
		minSizeStr     string
		safeOnly       bool
		force          bool
		dryRun         bool
		yes            bool
		includeCaution bool
		vendorCleanup  bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean reclaimable disk space",
		Example: `  devclean clean --eco node
  devclean clean --eco node --status dormant --yes
  devclean clean --eco node --safe --dry-run
  devclean clean --eco node --force
  devclean clean --eco xcode --vendor-cleanup --yes  # also runs 'xcrun simctl delete unavailable'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			minSize, err := parseMinSize(minSizeStr)
			if err != nil {
				return err
			}
			results, err := runScanPipeline(ScanPipelineOptions{
				Path:     scanPath,
				Ecos:     ecos,
				Status:   status,
				SafeOnly: safeOnly,
				MinSize:  minSize,
			})
			if err != nil {
				return err
			}

			if len(results) == 0 {
				fmt.Println("No items to clean.")
				if vendorCleanup {
					runVendorCleanups(vendorEcos(ecos, nil), dryRun)
				}
				return nil
			}

			var toClean []model.ScanResult

			if yes {
				var skippedCaution int
				toClean, skippedCaution = selectForYes(results, includeCaution)
				if skippedCaution > 0 {
					fmt.Printf(
						"%s\n",
						ui.DimStyle.Render(fmt.Sprintf(
							"Skipped %d caution item(s) — pass --include-caution to remove them with --yes.",
							skippedCaution,
						)),
					)
				}
			} else {
				// Interactive tree selection
				result := ui.RunTreeSelector(results)
				if result.Aborted {
					fmt.Println("Cancelled.")
					return nil
				}
				toClean = result.Selected
			}

			if len(toClean) == 0 {
				fmt.Println("No items selected.")
				if vendorCleanup {
					runVendorCleanups(vendorEcos(ecos, toClean), dryRun)
				}
				return nil
			}

			// Show summary
			var totalSize int64
			for _, r := range toClean {
				totalSize += r.Size
			}

			if dryRun {
				fmt.Printf(
					"\n%s %d items (%s)\n\n",
					ui.ProjectStyle.Render("Would clean:"),
					len(toClean), model.HumanSize(totalSize),
				)
				groups := model.GroupByProject(toClean)
				for _, p := range groups {
					fmt.Printf("  %s (%s) %s\n", p.Name, model.HumanSize(p.TotalSize), ui.DimStyle.Render(pathutil.ShortenHome(p.Path)))
					for _, r := range p.Items {
						relPath := filepath.Base(r.Path)
						if p.Path != "" {
							if rel, err := filepath.Rel(p.Path, r.Path); err == nil {
								relPath = rel
							}
						}
						fmt.Printf("    %s %s (%s)\n", ui.DimStyle.Render("•"), relPath, ui.DimStyle.Render(model.HumanSize(r.Size)))
					}
				}
				if vendorCleanup {
					runVendorCleanups(vendorEcos(ecos, toClean), dryRun)
				}
				return nil
			}

			// Choose delete method if not --yes
			if !yes {
				var deleteMethod string
				err := huh.NewSelect[string]().
					Title(fmt.Sprintf("Delete %d items (%s)?", len(toClean), model.HumanSize(totalSize))).
					Options(
						huh.NewOption("Move to Trash (recoverable)", "trash"),
						huh.NewOption("Permanently delete", "force"),
						huh.NewOption("Cancel", "cancel"),
					).
					Value(&deleteMethod).
					Run()
				if err != nil || deleteMethod == "cancel" {
					fmt.Println("Cancelled.")
					return nil
				}
				if deleteMethod == "force" {
					force = true
				}
			}

			// Execute cleanup
			c := cleaner.New(cleaner.Options{Force: force})

			var cleaned int
			var freedSize int64
			var failed int

			groups := model.GroupByProject(toClean)
			for _, p := range groups {
				fmt.Printf("\n  %s %s\n", ui.ProjectStyle.Render(p.Name), ui.DimStyle.Render(pathutil.ShortenHome(p.Path)))
				for _, r := range p.Items {
					relPath := r.Path
					if p.Path != "" {
						if rel, err := filepath.Rel(p.Path, r.Path); err == nil {
							relPath = rel
						}
					}
					err := c.Clean(r)
					if err != nil {
						failed++
						fmt.Printf("    %s %s — %v\n", ui.ErrStyle.Render("✗"), relPath, err)
					} else {
						cleaned++
						freedSize += r.Size
						fmt.Printf("    %s %s (%s)\n", ui.SafeStyle.Render("✔"), relPath, model.HumanSize(r.Size))
					}
				}
			}

			fmt.Printf(
				"\n%s\n",
				ui.SafeStyle.Render(fmt.Sprintf("Cleaned %d items (%s freed)", cleaned, model.HumanSize(freedSize))),
			)
			if failed > 0 {
				fmt.Printf("%s\n", ui.ErrStyle.Render(fmt.Sprintf("%d items failed", failed)))
			}

			if vendorCleanup {
				runVendorCleanups(vendorEcos(ecos, toClean), dryRun)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&scanPath, "path", "", "path to scan (default: ~)")
	cmd.Flags().StringSliceVar(&ecos, "eco", nil, "ecosystems to clean, e.g. node,python")
	cmd.Flags().StringVar(&status, "status", "", "filter by status: active, recent, stale, dormant")
	cmd.Flags().StringVar(&minSizeStr, "min-size", "", "skip artifacts smaller than this (e.g. 1MB, 500KB)")
	cmd.Flags().BoolVar(&safeOnly, "safe", false, "clean only safe items (skip caution/protected)")
	cmd.Flags().BoolVar(&force, "force", false, "permanent delete (skip Trash)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without deleting")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation and interactive selection (deletes only safe items unless --include-caution)")
	cmd.Flags().BoolVar(&includeCaution, "include-caution", false, "with --yes, also delete caution items (shared impact or hard to regenerate); protected is still never deleted")
	cmd.Flags().BoolVar(&vendorCleanup, "vendor-cleanup", false, "also run ecosystem-native cleanup commands (e.g. 'xcrun simctl delete unavailable' for xcode)")

	return cmd
}

// selectForYes chooses which results a non-interactive (--yes) clean deletes.
// It deletes only `safe` (auto-regenerated) items by default; `caution` items
// carry shared impact or hold state that is slow or impossible to regenerate,
// so removing them without a human looking is opt-in via includeCaution.
// `protected` items (git-tracked dirty, or protected safety) are never deleted.
// This keeps a single mis-classified catalog entry from becoming silent data
// loss under --yes. It returns the items to clean and how many caution items
// were skipped (for the user-facing notice).
func selectForYes(results []model.ScanResult, includeCaution bool) (toClean []model.ScanResult, skippedCaution int) {
	for _, r := range results {
		if r.Protected {
			continue
		}
		switch r.Safety {
		case model.SafetySafe:
			toClean = append(toClean, r)
		case model.SafetyCaution:
			if includeCaution {
				toClean = append(toClean, r)
			} else {
				skippedCaution++
			}
		}
	}
	return toClean, skippedCaution
}

// vendorEcos resolves the vendor-cleanup scope: the explicit --eco selection,
// or else the ecosystems of the artifacts actually being cleaned. Without
// --eco we must not fall back to the whole registry — that would run vendor
// commands (e.g. simctl delete) for ecosystems the user never targeted.
func vendorEcos(ecos []string, toClean []model.ScanResult) []model.Ecosystem {
	if len(ecos) > 0 {
		return toEcosystems(ecos)
	}
	seen := make(map[model.Ecosystem]bool)
	var out []model.Ecosystem
	for _, r := range toClean {
		if !seen[r.Ecosystem] {
			seen[r.Ecosystem] = true
			out = append(out, r.Ecosystem)
		}
	}
	return out
}

// runVendorCleanups executes vendor-native cleanup actions for the given
// ecosystems (none = nothing to run). In dry-run mode it prints what would
// run without executing.
func runVendorCleanups(ecosystems []model.Ecosystem, dryRun bool) {
	if len(ecosystems) == 0 {
		return
	}
	scanners := scanner.DefaultRegistry().ForEcosystems(ecosystems)

	type pending struct {
		eco    string
		action scanner.VendorCleanup
	}
	var actions []pending
	for _, s := range scanners {
		vc, ok := s.(scanner.VendorCleaner)
		if !ok {
			continue
		}
		for _, a := range vc.VendorCleanups() {
			actions = append(actions, pending{eco: s.Name(), action: a})
		}
	}

	if len(actions) == 0 {
		return
	}

	fmt.Printf("\n%s\n", ui.ProjectStyle.Render("Vendor cleanup:"))
	for _, p := range actions {
		header := fmt.Sprintf("  [%s] %s", p.eco, p.action.Description)
		fmt.Println(ui.DimStyle.Render("    " + p.action.Command))
		if dryRun {
			fmt.Printf("%s %s\n", header, ui.DimStyle.Render("(dry-run)"))
			continue
		}
		if err := p.action.Run(context.Background()); err != nil {
			fmt.Printf("%s — %s\n", header, ui.ErrStyle.Render(err.Error()))
			continue
		}
		fmt.Printf("%s %s\n", header, ui.SafeStyle.Render("✔"))
	}
}

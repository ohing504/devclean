package cli

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/ohing504/devclean/internal/cleaner"
	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/pathutil"
	"github.com/ohing504/devclean/internal/ui"
	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	var (
		scanPath string
		ecos     []string
		status   string
		safeOnly bool
		force    bool
		dryRun   bool
		yes      bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean reclaimable disk space",
		Example: `  devclean clean --eco node
  devclean clean --eco node --status dormant --yes
  devclean clean --eco node --safe --dry-run
  devclean clean --eco node --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := runScanPipeline(ScanPipelineOptions{
				Path:     scanPath,
				Ecos:     ecos,
				Status:   status,
				SafeOnly: safeOnly,
			})
			if err != nil {
				return err
			}

			if len(results) == 0 {
				fmt.Println("No items to clean.")
				return nil
			}

			var toClean []model.ScanResult

			if yes {
				// Non-interactive: clean all non-protected
				for _, r := range results {
					if !r.Protected {
						toClean = append(toClean, r)
					}
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
				return nil
			}

			// Show summary
			var totalSize int64
			for _, r := range toClean {
				totalSize += r.Size
			}

			if dryRun {
				fmt.Printf("\n%s %d items (%s)\n\n",
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

			fmt.Printf("\n%s\n",
				ui.SafeStyle.Render(fmt.Sprintf("Cleaned %d items (%s freed)", cleaned, model.HumanSize(freedSize))),
			)
			if failed > 0 {
				fmt.Printf("%s\n", ui.ErrStyle.Render(fmt.Sprintf("%d items failed", failed)))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&scanPath, "path", "", "path to scan (default: ~)")
	cmd.Flags().StringSliceVar(&ecos, "eco", nil, "ecosystems to clean, e.g. node,python")
	cmd.Flags().StringVar(&status, "status", "", "filter by status: active, recent, stale, dormant")
	cmd.Flags().BoolVar(&safeOnly, "safe", false, "clean only safe items (skip caution/protected)")
	cmd.Flags().BoolVar(&force, "force", false, "permanent delete (skip Trash)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without deleting")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation and interactive selection")

	return cmd
}

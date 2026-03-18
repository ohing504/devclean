package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/ohing504/devclean/internal/cleaner"
	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/output"
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

			// Group by project
			projects := model.GroupByProject(results)

			// Select projects to clean
			var selected []model.ProjectGroup
			if yes {
				// Non-interactive: clean all non-protected
				for _, p := range projects {
					if !p.Protected {
						selected = append(selected, p)
					}
				}
			} else {
				// Show scan results first so user can see the full picture
				fmt.Println()
				output.WriteTableWithOptions(os.Stdout, results, output.TableOptions{})

				// Interactive selection
				var err error
				selected, err = interactiveSelect(projects)
				if err != nil {
					return err
				}
			}

			if len(selected) == 0 {
				fmt.Println("No projects selected. (Protected projects cannot be cleaned)")
				return nil
			}

			// Show what was selected with artifact details
			fmt.Printf("\nSelected %d projects:\n", len(selected))
			for _, p := range selected {
				fmt.Printf("  %s (%s) %s\n", ui.ProjectStyle.Render(p.Name), model.HumanSize(p.TotalSize), ui.DimStyle.Render(pathutil.ShortenHome(p.Path)))
				for _, r := range p.Items {
					relPath := filepath.Base(r.Path)
					if p.Path != "" {
						if rel, err := filepath.Rel(p.Path, r.Path); err == nil {
							relPath = rel
						}
					}
					fmt.Printf("    %s %s (%s)\n",
						ui.DimStyle.Render("•"),
						relPath,
						ui.DimStyle.Render(model.HumanSize(r.Size)),
					)
				}
			}

			// Collect items from selected projects
			var toClean []model.ScanResult
			for _, p := range selected {
				toClean = append(toClean, p.Items...)
			}

			// Show summary
			var totalSize int64
			for _, r := range toClean {
				totalSize += r.Size
			}

			action := "Move to Trash"
			if force {
				action = "Permanently delete"
			}
			if dryRun {
				action = "Would clean"
			}

			bold := lipgloss.NewStyle().Bold(true)
			fmt.Printf("\n%s %d projects (%s)\n\n",
				bold.Render(action+":"),
				len(selected),
				model.HumanSize(totalSize),
			)

			if dryRun {
				for _, p := range selected {
					fmt.Printf("  %s (%s)\n", p.Name, model.HumanSize(p.TotalSize))
					fmt.Printf("  %s\n", ui.DimStyle.Render(pathutil.ShortenHome(p.Path)))
				}
				fmt.Println()
				return nil
			}

			// Confirm and choose delete method if not --yes
			if !yes {
				var deleteMethod string
				err := huh.NewSelect[string]().
					Title(fmt.Sprintf("Delete %d projects (%s)?", len(selected), model.HumanSize(totalSize))).
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

			// Group toClean by project for display
			projectItems := make(map[string][]model.ScanResult)
			for _, r := range toClean {
				key := r.ProjectRoot
				if key == "" {
					key = filepath.Dir(r.Path)
				}
				projectItems[key] = append(projectItems[key], r)
			}

			for _, p := range selected {
				items := projectItems[p.Path]
				fmt.Printf("\n  %s %s\n", ui.ProjectStyle.Render(p.Name), ui.DimStyle.Render(pathutil.ShortenHome(p.Path)))
				for _, r := range items {
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

func interactiveSelect(projects []model.ProjectGroup) ([]model.ProjectGroup, error) {
	// Separate cleanable and protected
	var cleanable []model.ProjectGroup
	var protectedCount int
	for _, p := range projects {
		if p.Protected {
			protectedCount++
		} else {
			cleanable = append(cleanable, p)
		}
	}

	if len(cleanable) == 0 {
		if protectedCount > 0 {
			fmt.Printf("\nAll %d projects are protected (uncommitted changes).\n", protectedCount)
		}
		return nil, nil
	}

	if protectedCount > 0 {
		fmt.Printf("\n%s\n", ui.DimStyle.Render(fmt.Sprintf("(%d protected projects hidden — commit or stash changes first)", protectedCount)))
	}

	var options []huh.Option[int]
	for i, p := range cleanable {
		shortPath := pathutil.ShortenHome(p.Path)
		label := fmt.Sprintf("%-25s %-10s %10s  %s",
			p.Name,
			string(p.Activity),
			model.HumanSize(p.TotalSize),
			shortPath,
		)
		options = append(options, huh.NewOption(label, i))
	}

	var selectedIndices []int

	err := huh.NewMultiSelect[int]().
		Title("Select projects to clean (space to toggle, enter to confirm)").
		Options(options...).
		Value(&selectedIndices).
		Run()
	if err != nil {
		return nil, err
	}

	var selected []model.ProjectGroup
	for _, idx := range selectedIndices {
		selected = append(selected, cleanable[idx])
	}
	return selected, nil
}

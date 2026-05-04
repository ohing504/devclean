package cli

import (
	"os"
	"sort"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/output"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var (
		scanPath   string
		ecos       []string
		category   string
		status     string
		minSizeStr string
		sortBy     string
		reverse    bool
		top        int
		verbose    bool
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan for reclaimable disk space",
		Example: `  devclean scan --path ~/workspace --eco node
  devclean scan --status dormant -n 10
  devclean scan --sort time --asc
  devclean scan --eco node --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			minSize, err := parseMinSize(minSizeStr)
			if err != nil {
				return err
			}
			results, err := runScanPipeline(ScanPipelineOptions{
				Path:     scanPath,
				Ecos:     ecos,
				Status:   status,
				Category: category,
				MinSize:  minSize,
				Quiet:    jsonOutput,
			})
			if err != nil {
				return err
			}

			// Sort
			sortResults(results, sortBy, reverse)

			if jsonOutput {
				return output.WriteJSON(os.Stdout, results)
			}

			output.WriteTableWithOptions(os.Stdout, results, output.TableOptions{
				TopN:    top,
				Verbose: verbose,
			})
			return nil
		},
	}

	cmd.Flags().StringVar(&scanPath, "path", "", "path to scan (default: ~)")
	cmd.Flags().StringSliceVar(&ecos, "eco", nil, "ecosystems to scan, e.g. node,python,xcode")
	cmd.Flags().StringVar(&category, "category", "", "filter by category: cache, build, runtime, deps")
	cmd.Flags().StringVar(&status, "status", "", "filter by status: active, recent, stale, dormant")
	cmd.Flags().StringVar(&minSizeStr, "min-size", "", "skip artifacts smaller than this (e.g. 1MB, 500KB)")
	cmd.Flags().StringVar(&sortBy, "sort", "size", "sort by: size, time, name")
	cmd.Flags().BoolVar(&reverse, "asc", false, "sort ascending instead of descending")
	cmd.Flags().IntVarP(&top, "top", "n", 0, "show only top N projects (0 = all)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show all artifacts including small ones")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "JSON output for scripting and AI agents")

	return cmd
}

func sortResults(results []model.ScanResult, sortBy string, ascending bool) {
	sort.Slice(results, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "time":
			less = results[i].LastMod.After(results[j].LastMod)
		case "name":
			less = results[i].Path < results[j].Path
		default: // "size"
			less = results[i].Size > results[j].Size
		}
		if ascending {
			return !less
		}
		return less
	})
}

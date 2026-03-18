package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root devclean command.
func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devclean",
		Short: "Developer disk cleanup CLI",
		Long:  "Scan and clean developer environment artifacts, caches, and dependencies to reclaim disk space.",
	}

	cmd.AddCommand(
		newScanCmd(),
		newCleanCmd(),
		newListCmd(),
	)

	return cmd
}

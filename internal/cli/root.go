package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// BuildInfo carries the build-time identity injected by goreleaser via
// -ldflags. Local `go build` invocations leave the defaults from main.go.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRootCmd creates the root devclean command.
func NewRootCmd(info BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "devclean",
		Short:   "Developer disk cleanup CLI",
		Long:    "Scan and clean developer environment artifacts, caches, and dependencies to reclaim disk space.",
		Version: fmt.Sprintf("%s (commit %s, built %s)", info.Version, info.Commit, info.Date),
	}

	cmd.AddCommand(
		newScanCmd(),
		newCleanCmd(),
		newListCmd(),
	)

	return cmd
}

package main

import (
	"os"

	"github.com/ohing504/devclean/internal/cli"
)

// These are populated at build time via -ldflags by goreleaser.
// Local builds (`go build`) leave them at the defaults.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd := cli.NewRootCmd(cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

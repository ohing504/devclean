package main

import (
	"os"

	"github.com/ohing504/devclean/internal/cli"
)

var version = "dev"

func main() {
	cmd := cli.NewRootCmd(version)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

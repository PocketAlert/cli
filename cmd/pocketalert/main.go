package main

import (
	"os"

	"github.com/PocketAlert/cli/internal/cli"
)

// Set by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(cli.Execute(cli.BuildInfo{Version: version, Commit: commit, Date: date}))
}

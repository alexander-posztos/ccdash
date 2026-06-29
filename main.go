// Command ccdash is a "where did I leave off?" dashboard over your Claude Code
// sessions. Single static binary, read-only over ~/.claude.
package main

import "github.com/alexander-posztos/ccdash/cmd"

// Set by goreleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetBuildInfo(version, commit, date)
	cmd.Execute()
}

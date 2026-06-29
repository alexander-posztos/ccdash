// Command ccdash is a "where did I leave off?" dashboard over your Claude Code
// sessions. Single static binary, read-only over ~/.claude.
package main

import (
	"runtime/debug"

	"github.com/alexander-posztos/ccdash/cmd"
)

// Set by goreleaser via -ldflags on release builds; otherwise filled in from the
// embedded Go build info (see resolveBuildInfo).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	v, c, d := resolveBuildInfo(version, commit, date)
	cmd.SetBuildInfo(v, c, d)
	cmd.Execute()
}

// resolveBuildInfo backfills version/commit/date from runtime/debug build info
// when they were not injected via -ldflags - notably `go install
// github.com/alexander-posztos/ccdash@latest`, where the module version is
// embedded but the ldflags are not, so version is still the "dev" default. A
// release build (ldflags applied, version != "dev") is returned unchanged.
func resolveBuildInfo(version, commit, date string) (string, string, string) {
	if version != "dev" {
		return version, commit, date
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version, commit, date
	}
	// go install pkg@version embeds the module version here; a plain `go build`
	// in the source tree reports "(devel)", which we leave as "dev".
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		version = v
	}
	// vcs.* are present for `go build` inside the repo (not for module installs).
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if s.Value != "" {
				commit = s.Value
			}
		case "vcs.time":
			if s.Value != "" {
				date = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" && commit != "none" {
				commit += "+dirty"
			}
		}
	}
	return version, commit, date
}

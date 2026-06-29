package engine

import (
	"fmt"
	"strings"
)

// OfflineRecap derives a recap summary from git state and the latest session
// title, with no LLM call (it does not read project notes - those feed only the
// live recap). It backs both the --offline / CCDASH_OFFLINE mode and the
// automatic fallback when claude is unavailable or errors.
func OfflineRecap(p *Project) string {
	return offlineLast(p, GitStateCached(p.Dir))
}

func offlineLast(p *Project, git *GitState) string {
	var parts []string
	if s := latestSession(p); s != nil {
		parts = append(parts, `was on "`+clip(oneLine(s.DisplayTitle()), 90)+`"`)
	}
	if git != nil {
		switch {
		case git.Dirty > 0:
			parts = append(parts, fmt.Sprintf("%d uncommitted change(s) on %s", git.Dirty, git.Branch))
		case git.Commit != nil:
			parts = append(parts, fmt.Sprintf(`last commit "%s" (%s)`, oneLine(git.Commit.Subject), git.Commit.When))
		}
	}
	if len(parts) == 0 {
		return "no recent activity recorded"
	}
	return strings.Join(parts, "; ")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

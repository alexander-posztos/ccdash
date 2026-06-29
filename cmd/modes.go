package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexander-posztos/ccdash/internal/config"
	"github.com/alexander-posztos/ccdash/internal/engine"
)

type projectJSON struct {
	Name     string           `json:"name"`
	Dir      string           `json:"dir"`
	Last     string           `json:"last"`
	Sessions int              `json:"sessions"`
	Active   bool             `json:"active"`
	Missing  bool             `json:"missing"`
	Hidden   bool             `json:"hidden"`
	Recap    string           `json:"recap,omitempty"`
	Git      *engine.GitState `json:"git,omitempty"`
}

func runJSON() error {
	cfg := loadCfg()
	projects := engine.GroupProjects(cfg)
	out := make([]projectJSON, 0, len(projects))
	for _, p := range projects {
		last := ""
		if !p.Last.IsZero() {
			last = p.Last.Format(time.RFC3339)
		}
		out = append(out, projectJSON{
			Name:     p.Name,
			Dir:      p.Dir,
			Last:     last,
			Sessions: len(p.RealSessions),
			Active:   engine.IsActive(cfg, p),
			Missing:  p.Missing,
			Hidden:   p.Hidden,
			Recap:    recapText(cfg, p),
			Git:      engine.GitStateCached(p.Dir),
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func runList(all bool) error {
	cfg := loadCfg()
	projects := engine.GroupProjects(cfg)
	// Hidden projects never appear in the human list; moved projects (logged dir
	// gone) are dropped from the default list too, matching the dashboard's active
	// view. --all widens both cutoffs and brings the moved ones back. Use
	// `ccdash --json` (which carries the hidden + missing flags) to see everything.
	var shown []*engine.Project
	for _, p := range projects {
		if p.Hidden {
			continue
		}
		if !all && (!engine.IsActive(cfg, p) || p.Missing) {
			continue
		}
		shown = append(shown, p)
	}
	for _, p := range shown {
		printProject(cfg, p)
	}
	fmt.Fprintf(os.Stderr, "\n%d shown (%d total).\n", len(shown), len(projects))
	return nil
}

func printProject(cfg config.Config, p *engine.Project) {
	gtag := ""
	if g := engine.GitStateCached(p.Dir); g != nil {
		gtag = "  [" + g.Branch
		if g.Dirty > 0 {
			gtag += fmt.Sprintf("*%d", g.Dirty)
		}
		gtag += "]"
	}
	missing := ""
	if p.Missing {
		missing = "  [MOVED/MISSING]"
	}
	fmt.Printf("\n* %-22s %8s%s  (%d sessions)%s\n",
		p.Name, engine.RelTime(p.Last), gtag, len(p.RealSessions), missing)
	for _, line := range splitLines(recapText(cfg, p)) {
		fmt.Println("    " + line)
	}
}

// recapText returns the cached recap if present, otherwise an offline one.
func recapText(cfg config.Config, p *engine.Project) string {
	if r := engine.LoadRecap(cfg, p); r != nil && r.Text != "" {
		return r.Text
	}
	return engine.OfflineRecap(p)
}

// realpath collapses symlinks like Python's os.path.realpath, but tolerates a
// non-existent path (EvalSymlinks fails there) by falling back to an absolute,
// cleaned form so path matching still has something stable to compare.
func realpath(p string) string {
	if p == "" {
		return ""
	}
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		return rp
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// expandHome expands a leading ~ or ~/ to the user's home dir (the shell would
// have done this for an interactive caller, but a CLI argument arrives raw).
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

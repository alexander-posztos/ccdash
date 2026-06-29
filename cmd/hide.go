// hide.go adds the headless preference commands: hide/unhide a project so it
// drops out of the dashboard. They write the same prefs file the TUI mutates, so
// a change made here shows up in the next dashboard launch (and vice versa).
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexander-posztos/ccdash/internal/engine"
	"github.com/alexander-posztos/ccdash/internal/prefs"
)

var hideCmd = &cobra.Command{
	Use:   "hide <dir>",
	Short: "Hide a project so it drops out of the dashboard",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return setHidden(args[0], true) },
}

var unhideCmd = &cobra.Command{
	Use:   "unhide <dir>",
	Short: "Unhide a previously hidden project",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return setHidden(args[0], false) },
}

func setHidden(arg string, hide bool) error {
	cfg := loadCfg()
	key, name, ok := resolveKeyIn(engine.GroupProjects(cfg), arg)
	if !ok {
		// Allow hiding even a project that no longer groups (e.g. a gone dir): fall
		// back to the cleaned, resolved arg as the key.
		key = prefs.Key(realpath(expandHome(arg)))
		name = filepath.Base(key)
	}
	var err error
	if hide {
		err = prefs.Hide(cfg.PrefsFile, key, time.Now())
	} else {
		err = prefs.Unhide(cfg.PrefsFile, key)
	}
	if err != nil {
		return err
	}
	verb := "hid"
	if !hide {
		verb = "unhid"
	}
	fmt.Fprintf(os.Stderr, "ccdash: %s %s\n", verb, name)
	return nil
}

// resolveKeyIn finds the project in projects whose live dir or grouping key
// matches arg (symlink-collapsed, ~ expanded) and returns its grouping key (Raw)
// and name.
func resolveKeyIn(projects []*engine.Project, arg string) (key, name string, ok bool) {
	target := realpath(expandHome(strings.TrimSpace(arg)))
	for _, p := range projects {
		if (p.Dir != "" && realpath(p.Dir) == target) || realpath(p.Raw) == target {
			return p.Raw, p.Name, true
		}
	}
	return "", "", false
}

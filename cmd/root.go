// Package cmd is ccdash's command-line surface.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"

	"github.com/alexander-posztos/ccdash/internal/config"
	"github.com/alexander-posztos/ccdash/internal/engine"
	"github.com/alexander-posztos/ccdash/internal/tui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func SetBuildInfo(v, c, d string) {
	version, commit, date = v, c, d
	rootCmd.Version = v
}

var (
	flagList      bool
	flagJSON      bool
	flagAll       bool
	flagSingleton bool
	flagOffline   bool
)

// loadCfg loads config and folds in the --offline flag (CCDASH_OFFLINE is already
// read by config.Load; the flag can only turn offline mode on, never off).
func loadCfg() config.Config {
	cfg := config.Load()
	if flagOffline {
		cfg.Offline = true
	}
	return cfg
}

// Plain modes are flags, not subcommands, kept minimal: the dashboard refreshes
// its own stale recaps in the background on open, so there is no batch or hook
// refresh entry point to wire up.
var rootCmd = &cobra.Command{
	Use:           "ccdash",
	Short:         "Where did I leave off? A dashboard across your Claude Code projects",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch {
		case flagJSON:
			return runJSON()
		case flagList:
			return runList(flagAll)
		default:
			if !isTTY() {
				return runList(false)
			}
			return runTUI()
		}
	},
}

// runTUI runs the dashboard, then performs any resume/editor handoff AFTER the
// program exits, so claude/the editor inherit a fully restored terminal.
func runTUI() error {
	cfg := loadCfg()
	if flagSingleton {
		lock, held, err := acquireSingleton(cfg)
		switch {
		case err != nil:
			// Lock dir unwritable etc: degrade to a normal launch rather than refuse.
			fmt.Fprintln(os.Stderr, "ccdash: singleton lock unavailable:", err)
		case !held:
			// Another ccdash already owns the lock; exit cleanly and let the window
			// manager raise the existing window (the launcher recipes do the focus).
			return nil
		default:
			// Held for the lifetime of the dashboard. On a resume/editor handoff the
			// lock fd is close-on-exec (Go default), so syscall.Exec releases it.
			defer func() { _ = lock.Unlock() }()
		}
	}
	res, err := tui.Run(cfg)
	if err != nil {
		return err
	}
	if res == nil {
		return nil
	}
	switch {
	case res.Resume != nil:
		return engine.DoResume(cfg, res.Resume)
	case res.New != nil:
		return engine.DoNew(cfg, res.New)
	}
	return nil
}

func init() {
	f := rootCmd.Flags()
	f.BoolVarP(&flagList, "list", "l", false, "plain-text dump of projects + recaps, then exit")
	f.BoolVar(&flagJSON, "json", false, "machine-readable project index as JSON, then exit")
	f.BoolVar(&flagAll, "all", false, "with --list: include inactive projects too")
	f.BoolVar(&flagSingleton, "singleton", false, "if another ccdash is open, exit 0 (let the WM focus it) instead of launching")
	f.BoolVar(&flagOffline, "offline", false, "derive recaps locally; never call claude")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(hideCmd, unhideCmd)
}

// acquireSingleton tries to take a process-wide advisory lock. It returns
// (lock, true, nil) when this process now owns it, (nil, false, nil) when another
// ccdash already holds it, or a non-nil error if the lock could not be attempted.
func acquireSingleton(cfg config.Config) (*flock.Flock, bool, error) {
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return nil, false, err
	}
	lock := flock.New(filepath.Join(cfg.CacheDir, "singleton.lock"))
	held, err := lock.TryLock()
	if err != nil {
		return nil, false, err
	}
	return lock, held, nil
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ccdash:", err)
		os.Exit(1)
	}
}

package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexander-posztos/ccdash/internal/config"
)

const (
	tailMaxMsgs    = 22
	tailMaxChars   = 6000
	defaultTimeout = 90 * time.Second
)

// ClaudeBin resolves an absolute path to the claude CLI. A hotkey-launched window
// runs under a non-interactive login shell where ~/.local/bin can be off PATH,
// so a bare "claude" can fail (exit 127); resolve it up front. An explicit
// cfg.ClaudeBin (CCDASH_CLAUDE_BIN) always wins, which also lets tests point at
// a fake.
func ClaudeBin(cfg config.Config) string {
	if cfg.ClaudeBin != "" {
		return cfg.ClaudeBin
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, cand := range []string{
		filepath.Join(home, ".local", "bin", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
	} {
		if isExecutable(cand) {
			return cand
		}
	}
	return "claude" // last resort: let the shell try and report not-found
}

// GenerateRecap runs claude to produce a fresh recap summary and caches it. It
// always returns a usable *Recap: on success the live recap (cached); on any
// failure (claude missing, error, timeout, empty output) the offline recap,
// uncached, so the project stays stale and gets retried on the next open instead
// of being pinned to a degraded recap. The returned error is non-nil only in that
// degraded case and explains why; the TUI (the sole caller) ignores it and shows
// the offline card, but it is kept so any future caller can surface why a recap
// degraded.
func GenerateRecap(cfg config.Config, p *Project) (*Recap, error) {
	if cfg.Offline {
		// Offline mode (--offline / CCDASH_OFFLINE): never exec claude. Return the
		// locally-derived recap uncached, so a later online run still regenerates it.
		return &Recap{Signature: Signature(p), Text: OfflineRecap(p), Mode: "offline", Generated: time.Now()}, nil
	}
	out, err := runClaude(cfg, recapPrompt(cfg, p))
	if err == nil && strings.TrimSpace(out) == "" {
		err = fmt.Errorf("claude returned empty output")
	}
	if err != nil {
		return &Recap{Signature: Signature(p), Text: OfflineRecap(p), Mode: "offline", Generated: time.Now()}, err
	}
	r := &Recap{
		Signature: Signature(p),
		Text:      strings.TrimSpace(out),
		Git:       GitStateCached(p.Dir),
		Mode:      "live",
		Generated: time.Now(),
	}
	_ = storeRecap(cfg, p, r)
	return r, nil
}

func runClaude(cfg config.Config, prompt string) (string, error) {
	timeout := cfg.RecapTimeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ClaudeBin(cfg), "-p", prompt, "--model", cfg.Model)
	cmd.WaitDelay = 5 * time.Second // do not hang on a child holding stdout after the deadline
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// cmd.Output discards stderr, which turned a real claude failure (auth
		// expired, rate limit, bad model, timeout) into a silent offline
		// fallback. Fold a trimmed tail of stderr into the error so a caller
		// can report why a recap degraded.
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, clip(msg, 500))
		}
		return "", err
	}
	return string(out), nil
}

// recapPrompt is adapted from the Python generate_recap prompt (tuned to 2-3
// sentences for the Go build). Keep it emdash-free (the CI gate rejects U+2014).
func recapPrompt(cfg config.Config, p *Project) string {
	latest := latestSession(p)
	git := GitStateCached(p.Dir)

	tail := ""
	if latest != nil && latest.File != "" {
		tail = sessionTail(latest.File, tailMaxMsgs, tailMaxChars)
	}
	if tail == "" {
		tail = "(no readable transcript)"
	}
	notes := readNotes(p.Dir)
	if notes == "" {
		notes = "(none)"
	}

	gitline := "not a git repo"
	if git != nil {
		gitline = fmt.Sprintf("branch=%s, uncommitted_changes=%d", git.Branch, git.Dirty)
		if git.Commit != nil {
			gitline += fmt.Sprintf(", last_commit=%q (%s)", git.Commit.Subject, git.Commit.When)
		}
	}

	lastTitle := "n/a"
	if latest != nil {
		switch {
		case latest.Title != "":
			lastTitle = latest.Title
		case latest.FirstPrompt != "":
			lastTitle = latest.FirstPrompt
		}
	}

	// The transcript tail goes last, in the highest-weight slot, with the notes
	// above it: the notes are developer-written and may be stale, so they must
	// not outrank the session that actually just happened.
	context := "PROJECT: " + p.Name + "\n" +
		"GIT: " + gitline + "\n" +
		"LAST SESSION TITLE: " + lastTitle + "\n\n" +
		"=== PROJECT NOTES (optional, may be out of date) ===\n" + notes + "\n\n" +
		"=== TAIL OF LAST CLAUDE CODE SESSION ===\n" + tail + "\n"

	return "You write terse status recaps for a developer reopening a project after a break. " +
		"From the context below, output a single 2-3 sentence summary of what was just " +
		"being worked on and the current state, and nothing else. " +
		"The tail of the last session is the authoritative record of what just happened; base the recap on it. " +
		"The project notes are developer-written and may be stale, so treat them only as supporting context: " +
		"prefer the transcript and git when they conflict, and ignore any note items (especially 'NEXT' or 'TODO' " +
		"lines) that the transcript does not back up. " +
		"Be specific and concrete. No preamble, no labels, no markdown headers, no fluff. " +
		"Never use emdashes - use a plain hyphen. " +
		"If git shows uncommitted changes, factor that in.\n\n" + context
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

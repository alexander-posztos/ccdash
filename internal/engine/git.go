package engine

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	When    string `json:"when"`
}

type GitState struct {
	Branch string  `json:"branch"`
	Dirty  int     `json:"dirty"`
	Commit *Commit `json:"commit,omitempty"`
}

var (
	gitMu    sync.Mutex
	gitCache = map[string]*GitState{}
)

// GitStateCached returns git info for a dir, memoized for the process lifetime.
func GitStateCached(dir string) *GitState {
	if dir == "" {
		return nil
	}
	gitMu.Lock()
	v, ok := gitCache[dir]
	gitMu.Unlock()
	if ok {
		return v
	}
	v = gitState(dir)
	gitMu.Lock()
	gitCache[dir] = v
	gitMu.Unlock()
	return v
}

func gitState(dir string) *GitState {
	if !isDir(filepath.Join(dir, ".git")) {
		return nil
	}
	gs := &GitState{Branch: "?"}
	if out, ok := git(dir, "rev-parse", "--abbrev-ref", "HEAD"); ok {
		gs.Branch = strings.TrimSpace(out)
	}
	if out, ok := git(dir, "status", "--porcelain"); ok {
		for _, l := range strings.Split(out, "\n") {
			if strings.TrimSpace(l) != "" {
				gs.Dirty++
			}
		}
	}
	if out, ok := git(dir, "log", "-1", "--format=%h\x1f%s\x1f%cr"); ok {
		if parts := strings.Split(strings.TrimSpace(out), "\x1f"); len(parts) == 3 {
			gs.Commit = &Commit{Hash: parts[0], Subject: parts[1], When: parts[2]}
		}
	}
	return gs
}

func git(dir string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, gitBin(), append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

// gitBin resolves an absolute path to git for the same reason ClaudeBin does: a
// hotkey-launched window runs under a non-interactive login shell where PATH can
// be thin, so a bare "git" may fail to resolve (exit 127). Resolve it up front,
// falling back to the common install locations, then to bare "git" as a last
// resort so a normal PATH still works.
func gitBin() string {
	if p, err := exec.LookPath("git"); err == nil {
		return p
	}
	for _, cand := range []string{
		"/usr/bin/git",
		"/opt/homebrew/bin/git",
		"/usr/local/bin/git",
	} {
		if isExecutable(cand) {
			return cand
		}
	}
	return "git" // last resort: let the shell try and report not-found
}

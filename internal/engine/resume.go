package engine

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/alexander-posztos/ccdash/internal/config"
)

// resumable reports whether a session can be handed to `claude --resume`, which
// needs the session transcript on disk (s.File). claude prunes old transcripts
// while history.jsonl - ccdash's session index - outlives them, so a listed
// session can lack a transcript; resuming it would fail "No conversation found".
func resumable(s *Session) bool { return s.File != "" }

// DoResume replaces the current process with `claude --resume <sid>` in the
// session's project dir, wrapped in `caffeinate -dims` and passed
// --dangerously-skip-permissions (resume is always into a project you own). If
// the session's transcript has been pruned it cannot be resumed, so ccdash skips
// the doomed call and opens a shell in the project dir with a clear note instead.
// The shell mechanics (input flush, survive-exit, $EDITOR backfill) live in
// handoffShell. Returns only if exec fails.
func DoResume(cfg config.Config, s *Session) error {
	dir := ResolveDir(firstNonEmpty(s.CWDVerified, s.CWD))
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}

	if !resumable(s) {
		fmt.Fprintf(os.Stderr, "\n-> cd %s  (cannot resume %s: transcript no longer on disk)\n", dir, s.SID)
		note := fmt.Sprintf("[ccdash] cannot resume %s: its transcript is no longer on disk "+
			"(claude prunes old session transcripts). Start a new session to continue here.", s.SID)
		return handoffShell(dir, "printf '%s\\n\\n' "+shellQuote(note))
	}

	var parts []string
	if caf := caffeinatePath(); caf != "" {
		parts = append(parts, caf, "-dims")
	}
	parts = append(parts, ClaudeBin(cfg), "--resume", s.SID, "--dangerously-skip-permissions")
	resume := shellJoin(parts)

	if dir != "" {
		fmt.Fprintf(os.Stderr, "\n-> cd %s && %s\n", dir, resume)
	} else {
		fmt.Fprintf(os.Stderr, "\n-> %s\n", resume)
	}
	return handoffShell(dir, resume+"; rc=$?; "+
		"[ $rc -ne 0 ] && printf '\\n[ccdash] claude --resume exited %s (see above); dropping into a shell.\\n' \"$rc\"")
}

// DoNew replaces the current process with a fresh `claude` in the project dir.
// Mirrors DoResume's exec hardening (caffeinate, --dangerously-skip-permissions,
// a shell that survives claude exiting, the pre-handoff input flush, the $EDITOR
// backfill, and ClaudeBin's absolute path so it works from the PATH-less,
// hotkey-launched window) but drops --resume: a bare claude starts a NEW session
// in p.Dir.
//
// Returns only if exec fails.
func DoNew(cfg config.Config, p *Project) error {
	if p.Dir == "" {
		return fmt.Errorf("no directory for %s", p.Name)
	}

	var parts []string
	if caf := caffeinatePath(); caf != "" {
		parts = append(parts, caf, "-dims")
	}
	parts = append(parts, ClaudeBin(cfg), "--dangerously-skip-permissions")
	cmd := shellJoin(parts)

	fmt.Fprintf(os.Stderr, "\n-> cd %s && %s\n", p.Dir, cmd)
	return handoffShell(p.Dir, cmd+"; rc=$?; "+
		"[ $rc -ne 0 ] && printf '\\n[ccdash] claude exited %s (see above); dropping into a shell.\\n' \"$rc\"")
}

// handoffShell replaces the ccdash process with an interactive shell in dir (cwd
// unchanged when dir is ""), after running pre - a shell snippet such as the
// resume/new command or a notice (empty for none). It flushes stray terminal
// input first (a buffered Enter/mouse-release from the TUI handoff would otherwise
// be eaten by claude's resume gate), and the shell is interactive (-i) so the
// window survives the command exiting and any error stays visible. withEditor
// backfills $EDITOR into the handed-off environment. Returns only if exec fails.
func handoffShell(dir, pre string) error {
	if dir != "" {
		_ = os.Chdir(dir)
	}
	flushInput(os.Stdin.Fd())
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	inner := "exec " + shellQuote(shell) + " -i"
	if pre != "" {
		inner = pre + "; " + inner
	}
	return syscall.Exec(shell, []string{shell, "-c", inner}, withEditor(os.Environ()))
}

// withEditor returns env augmented so the resumed claude always sees an $EDITOR.
// A hotkey-launched ccdash runs under a non-interactive login shell (zsh -lc)
// where ~/.zshrc, which exports EDITOR, is never sourced; without this the
// resumed `claude --resume` inherits no $EDITOR and its Ctrl+G external-editor
// picker falls through to IDE auto-detection (it finds `code` on PATH) and opens
// VS Code instead of the user's editor. Same fix as ClaudeBin: fill the gap left
// by the non-interactive shell, but never override an editor the user did set.
func withEditor(env []string) []string {
	if envHasNonEmpty(env, "EDITOR") || envHasNonEmpty(env, "VISUAL") {
		return env
	}
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, "EDITOR=") {
			continue // drop an empty EDITOR= so we never leave a duplicate
		}
		out = append(out, kv)
	}
	return append(out, "EDITOR=nvim")
}

// envHasNonEmpty reports whether KEY=<non-empty> is present in a KEY=VALUE list.
func envHasNonEmpty(env []string, key string) bool {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) && len(kv) > len(prefix) {
			return true
		}
	}
	return false
}

func caffeinatePath() string {
	if p, err := exec.LookPath("caffeinate"); err == nil {
		return p
	}
	if isExecutable("/usr/bin/caffeinate") {
		return "/usr/bin/caffeinate"
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func shellJoin(parts []string) string {
	q := make([]string, len(parts))
	for i, p := range parts {
		q[i] = shellQuote(p)
	}
	return strings.Join(q, " ")
}

// shellQuote single-quotes s for POSIX shells when it contains anything outside
// a conservative safe set (mirrors Python shlex.quote closely enough for paths).
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		safe := r == '_' || r == '-' || r == '.' || r == '/' || r == ':' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !safe {
			return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
		}
	}
	return s
}

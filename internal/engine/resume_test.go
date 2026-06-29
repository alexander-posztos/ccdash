package engine

import (
	"strings"
	"testing"
)

func editorOf(env []string) (string, bool) {
	val, found := "", false
	for _, kv := range env {
		if strings.HasPrefix(kv, "EDITOR=") {
			val, found = strings.TrimPrefix(kv, "EDITOR="), true
		}
	}
	return val, found
}

func countKey(env []string, prefix string) (n int) {
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			n++
		}
	}
	return
}

// The bug: a hotkey-launched ccdash (zsh -lc, .zshrc unsourced) has neither
// EDITOR nor VISUAL, so the resumed claude's Ctrl+G picker fell back to VS Code.
// A session ccdash lists from history.jsonl can outlive its transcript (claude
// prunes old ones); without a transcript file it must not be offered to
// `claude --resume`, which would fail "No conversation found".
func TestResumable(t *testing.T) {
	if !resumable(&Session{File: "/c/projects/p/s.jsonl"}) {
		t.Error("session with a transcript file should be resumable")
	}
	if resumable(&Session{File: ""}) {
		t.Error("session whose transcript was pruned (no File) must not be resumable")
	}
}

func TestWithEditor_InjectsDefaultWhenEnvHasNone(t *testing.T) {
	got, ok := editorOf(withEditor([]string{"HOME=/home/u", "PATH=/usr/bin"}))
	if !ok || got != "nvim" {
		t.Fatalf("non-interactive env: want EDITOR=nvim injected, got %q present=%v", got, ok)
	}
}

func TestWithEditor_RespectsExplicitEditor(t *testing.T) {
	out := withEditor([]string{"HOME=/x", "EDITOR=emacs"})
	if got, _ := editorOf(out); got != "emacs" {
		t.Fatalf("explicit EDITOR must win, got %q", got)
	}
	if n := countKey(out, "EDITOR="); n != 1 {
		t.Fatalf("must not duplicate EDITOR, found %d entries", n)
	}
}

func TestWithEditor_RespectsVisualWithoutInjecting(t *testing.T) {
	if _, ok := editorOf(withEditor([]string{"VISUAL=code"})); ok {
		t.Fatalf("VISUAL set: must not inject EDITOR")
	}
}

func TestWithEditor_TreatsEmptyEditorAsUnset(t *testing.T) {
	out := withEditor([]string{"EDITOR="})
	got, ok := editorOf(out)
	if !ok || got != "nvim" {
		t.Fatalf("empty EDITOR must fall back to nvim, got %q present=%v", got, ok)
	}
	if n := countKey(out, "EDITOR="); n != 1 {
		t.Fatalf("must not leave a duplicate EDITOR, found %d entries", n)
	}
}

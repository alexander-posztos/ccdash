package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alexander-posztos/ccdash/internal/config"
)

func TestGitState(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("checkout", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "a.txt")
	git("commit", "-q", "-m", "first commit")

	gs := GitStateCached(dir)
	if gs == nil {
		t.Fatal("nil git state for a real repo")
	}
	if gs.Branch != "main" {
		t.Errorf("branch: got %q, want main", gs.Branch)
	}
	if gs.Dirty != 0 {
		t.Errorf("dirty: got %d, want 0", gs.Dirty)
	}
	if gs.Commit == nil || gs.Commit.Subject != "first commit" {
		t.Errorf("commit: got %+v", gs.Commit)
	}
}

func TestRecapCacheRoundTrip(t *testing.T) {
	cfg := config.Config{CacheDir: t.TempDir()}
	p := &Project{
		Dir: "/x/proj", Raw: "/x/proj", Name: "proj", Last: time.Now(),
		Sessions: []*Session{{SID: "s1", Last: time.Now()}},
	}
	if !RecapIsStale(cfg, p) {
		t.Error("should be stale with no cache file")
	}
	r := &Recap{Signature: Signature(p), Text: "wired up the parser", Generated: time.Now()}
	if err := storeRecap(cfg, p, r); err != nil {
		t.Fatal(err)
	}
	if RecapIsStale(cfg, p) {
		t.Error("should be fresh right after store")
	}
	if got := LoadRecap(cfg, p); got == nil || got.Text != r.Text {
		t.Errorf("loaded recap: got %+v", got)
	}
	p.Sessions[0].SID = "s2" // newest session changed -> stale
	if !RecapIsStale(cfg, p) {
		t.Error("should be stale after the newest session changes")
	}
}

// fakeClaude writes an executable stub that runs `body` and returns its path,
// so recap tests never shell out to the real claude (which costs money + time).
func fakeClaude(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func recapTestProject() *Project {
	now := time.Now()
	s := &Session{SID: "s1", Last: now, FirstPrompt: "wire up the parser"}
	return &Project{Dir: "", Raw: "/w/proj", Name: "proj", Last: now,
		Sessions: []*Session{s}, RealSessions: []*Session{s}}
}

func TestClaudeBinOverride(t *testing.T) {
	if got := ClaudeBin(config.Config{ClaudeBin: "/fake/claude"}); got != "/fake/claude" {
		t.Errorf("override ignored: got %q", got)
	}
}

func TestGenerateRecapLive(t *testing.T) {
	cfg := config.Config{
		CacheDir:     t.TempDir(),
		Model:        "haiku",
		ClaudeBin:    fakeClaude(t, "printf 'parser landed, tests green\\n'"),
		RecapTimeout: 10 * time.Second,
	}
	p := recapTestProject()
	r, err := GenerateRecap(cfg, p)
	if err != nil {
		t.Fatalf("live recap errored: %v", err)
	}
	if r == nil || !strings.Contains(r.Text, "parser landed") {
		t.Fatalf("live recap not returned: %+v", r)
	}
	if r.Mode != "live" {
		t.Errorf("mode: got %q, want live", r.Mode)
	}
	if RecapIsStale(cfg, p) {
		t.Error("should be fresh right after a successful live regen")
	}
	if got := LoadRecap(cfg, p); got == nil || got.Text != r.Text {
		t.Errorf("live recap not cached: %+v", got)
	}
}

func TestGenerateRecapFallbackNotCached(t *testing.T) {
	cfg := config.Config{
		CacheDir:     t.TempDir(),
		Model:        "haiku",
		ClaudeBin:    fakeClaude(t, "exit 1"), // claude fails
		RecapTimeout: 10 * time.Second,
	}
	p := recapTestProject()
	r, err := GenerateRecap(cfg, p)
	if err == nil {
		t.Error("a failed regen must return an error explaining the offline degrade")
	}
	if r == nil || r.Text == "" || strings.Contains(r.Text, "NEXT") {
		t.Fatalf("fallback should be a plain offline summary with no NEXT: %+v", r)
	}
	if r.Mode != "offline" {
		t.Errorf("mode: got %q, want offline", r.Mode)
	}
	if !RecapIsStale(cfg, p) {
		t.Error("a failed regen must stay stale so it gets retried")
	}
	if LoadRecap(cfg, p) != nil {
		t.Error("a failed regen must not write the cache")
	}
}

func TestReadNotesRuneCap(t *testing.T) {
	dir := t.TempDir()
	// 2490 ASCII + 20 euro signs (3 bytes each): 2550 bytes but only 2510 runes.
	content := strings.Repeat("a", 2490) + strings.Repeat("€", 20)
	if err := os.WriteFile(filepath.Join(dir, "STATUS.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	body := strings.TrimPrefix(readNotes(dir), "--- STATUS.md ---\n")
	if !utf8.ValidString(body) {
		t.Error("readNotes produced invalid UTF-8 (split a rune)")
	}
	if n := utf8.RuneCountInString(body); n != 2500 {
		t.Errorf("readNotes cap: got %d runes, want 2500", n)
	}
}

func TestSessionTailRuneCap(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("€", 7000) // 7000 runes, 21000 bytes
	line := `{"type":"assistant","message":{"content":"` + big + `"}}` + "\n"
	f := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(f, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	out := sessionTail(f, 22, 6000)
	if !utf8.ValidString(out) {
		t.Error("sessionTail produced invalid UTF-8 (split a rune)")
	}
	if n := utf8.RuneCountInString(out); n != 6000 {
		t.Errorf("sessionTail cap: got %d runes, want 6000", n)
	}
}

func TestOfflineRecap(t *testing.T) {
	p := &Project{Name: "proj", Sessions: []*Session{{SID: "s1", FirstPrompt: "build the thing", Last: time.Now()}}}
	r := OfflineRecap(p)
	if strings.Contains(r, "NEXT") {
		t.Errorf("offline recap should have no NEXT line: %q", r)
	}
	if !strings.Contains(r, "build the thing") {
		t.Errorf("expected the session title in the recap: %q", r)
	}
}

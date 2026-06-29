package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexander-posztos/ccdash/internal/config"
)

// writeClaudeFixture builds a synthetic ~/.claude tree plus a work dir holding
// the real project folders. alpha and beta exist on disk; ghost's folder is
// intentionally absent so it groups as "missing". Timestamps are relative to now
// so the active/inactive assertions never go stale. Returns (claudeDir, workDir).
func writeClaudeFixture(t *testing.T) (string, string) {
	t.Helper()
	claude := t.TempDir()
	work := t.TempDir()
	alpha := filepath.Join(work, "alpha")
	beta := filepath.Join(work, "beta")
	ghost := filepath.Join(work, "ghost") // never created on disk
	for _, d := range []string{alpha, beta} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	iso := func(d time.Duration) string { return time.Now().Add(-d).UTC().Format(time.RFC3339) }
	day := 24 * time.Hour

	history := "" +
		`{"sessionId":"s-alpha","timestamp":"` + iso(day) + `","project":"` + alpha + `","display":"add login form"}` + "\n" +
		`{"sessionId":"s-alpha","timestamp":"` + iso(day-5*time.Minute) + `","project":"` + alpha + `","display":"/clear"}` + "\n" +
		`{"sessionId":"s-beta","timestamp":"` + iso(2*day) + `","project":"` + beta + `","display":"fix flaky test"}` + "\n" +
		`{"sessionId":"s-ghost","timestamp":"` + iso(50*day) + `","project":"` + ghost + `","display":"old idea"}` + "\n"
	mustWrite(t, filepath.Join(claude, "history.jsonl"), history)

	sess := `{"type":"ai-title","aiTitle":"Login form"}` + "\n" +
		`{"type":"user","cwd":"` + alpha + `","gitBranch":"main","message":{"role":"user","content":"add login form"}}` + "\n"
	mustWrite(t, filepath.Join(claude, "projects", "p1", "s-alpha.jsonl"), sess)
	return claude, work
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGroupProjects(t *testing.T) {
	claude, work := writeClaudeFixture(t)

	t.Setenv("CCDASH_CLAUDE_DIR", claude)
	t.Setenv("CCDASH_CACHE_DIR", t.TempDir()) // keep the scan cache out of the real cache dir
	cfg := config.Load()

	ps := GroupProjects(cfg)
	if len(ps) != 3 {
		t.Fatalf("got %d projects, want 3", len(ps))
	}

	wantOrder := []string{"alpha", "beta", "ghost"}
	for i, name := range wantOrder {
		if ps[i].Name != name {
			t.Errorf("project %d: got %q, want %q", i, ps[i].Name, name)
		}
	}

	alpha := ps[0]
	if alpha.Dir != filepath.Join(work, "alpha") {
		t.Errorf("alpha dir: got %q, want %q", alpha.Dir, filepath.Join(work, "alpha"))
	}
	if alpha.Missing {
		t.Error("alpha should not be missing")
	}
	if len(alpha.RealSessions) != 1 {
		t.Fatalf("alpha real sessions: got %d, want 1", len(alpha.RealSessions))
	}
	if got := alpha.RealSessions[0].Title; got != "Login form" {
		t.Errorf("alpha title: got %q, want %q", got, "Login form")
	}
	if got := alpha.RealSessions[0].Branch; got != "main" {
		t.Errorf("alpha branch: got %q, want %q", got, "main")
	}
	if !IsActive(cfg, alpha) {
		t.Error("alpha should be active")
	}

	ghost := ps[2]
	if !ghost.Missing || ghost.Dir != "" {
		t.Errorf("ghost should be missing with empty dir, got missing=%v dir=%q", ghost.Missing, ghost.Dir)
	}
	if IsActive(cfg, ghost) {
		t.Error("ghost should be inactive (50 days old)")
	}
}

// TestGroupProjectsHidden: a project whose grouping key is in the prefs hidden
// set carries Hidden=true (which the views filter on); others stay false.
func TestGroupProjectsHidden(t *testing.T) {
	claude, work := writeClaudeFixture(t)
	// alpha's grouping key (Raw) is its on-disk dir work/alpha - hide on that.
	prefsPath := filepath.Join(t.TempDir(), "prefs.json")
	mustWrite(t, prefsPath, `{"version":1,"hidden":{"`+filepath.Join(work, "alpha")+`":{"at":"2026-01-01T00:00:00Z"}}}`)

	t.Setenv("CCDASH_CLAUDE_DIR", claude)
	t.Setenv("CCDASH_CACHE_DIR", t.TempDir())
	t.Setenv("CCDASH_PREFS_FILE", prefsPath)
	cfg := config.Load()

	ps := GroupProjects(cfg)
	for _, p := range ps {
		switch p.Name {
		case "alpha":
			if !p.Hidden {
				t.Error("alpha should be hidden")
			}
		case "beta", "ghost":
			if p.Hidden {
				t.Errorf("%s should not be hidden", p.Name)
			}
		}
	}
}

func TestIsRealSession(t *testing.T) {
	cases := []struct {
		s    *Session
		want bool
	}{
		{&Session{Title: "Some title"}, true},
		{&Session{FirstPrompt: "do the thing"}, true},
		{&Session{FirstPrompt: "/clear", N: 1}, false},
		{&Session{FirstPrompt: "/clear", N: 3}, true},
		{&Session{FirstPrompt: ":wq", N: 2}, false},
		{&Session{FirstPrompt: "", N: 1}, false},
	}
	for i, c := range cases {
		if got := IsRealSession(c.s); got != c.want {
			t.Errorf("case %d: got %v, want %v", i, got, c.want)
		}
	}
}

func TestParseTime(t *testing.T) {
	for _, s := range []string{
		"2026-06-21T10:00:00Z",
		"2026-06-21T10:00:00.123Z",
		"2026-06-21T10:00:00+02:00",
	} {
		if _, ok := parseTime(s); !ok {
			t.Errorf("failed to parse %q", s)
		}
	}
	if _, ok := parseTime("not a time"); ok {
		t.Error("expected failure on garbage input")
	}
}

func titleOf(sessions []*Session, sid string) string {
	for _, s := range sessions {
		if s.SID == sid {
			return s.Title
		}
	}
	return ""
}

// TestScanCache verifies the transcript metadata cache: an unchanged file (same
// mtime+size) is served from cache without re-reading, and a changed mtime
// forces a re-parse.
func TestScanCache(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CCDASH_CLAUDE_DIR", claude)
	t.Setenv("CCDASH_CACHE_DIR", t.TempDir())
	cfg := config.Load()

	now := time.Now().UTC().Format(time.RFC3339)
	mustWrite(t, filepath.Join(claude, "history.jsonl"),
		`{"sessionId":"s1","timestamp":"`+now+`","project":"/w/p","display":"hi"}`+"\n")

	sf := filepath.Join(claude, "projects", "p", "s1.jsonl")
	meta := "\n" + `{"type":"user","cwd":"/w/p","gitBranch":"main"}` + "\n"
	v1 := `{"type":"ai-title","aiTitle":"alpha-one"}` + meta
	v2 := `{"type":"ai-title","aiTitle":"alpha-two"}` + meta // same byte length as v1
	if len(v1) != len(v2) {
		t.Fatalf("test bug: v1/v2 lengths differ (%d vs %d)", len(v1), len(v2))
	}
	mustWrite(t, sf, v1)

	if got := titleOf(BuildSessions(cfg), "s1"); got != "alpha-one" {
		t.Fatalf("first build: got title %q, want alpha-one", got)
	}

	// Rewrite with identical mtime+size but different content: a cache HIT must
	// return the old title, proving the file was not re-read.
	fi, err := os.Stat(sf)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, sf, v2)
	if err := os.Chtimes(sf, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if got := titleOf(BuildSessions(cfg), "s1"); got != "alpha-one" {
		t.Errorf("cache hit: got title %q, want stale alpha-one", got)
	}

	// Bump mtime: a cache MISS must re-parse and pick up the new title.
	future := fi.ModTime().Add(time.Hour)
	if err := os.Chtimes(sf, future, future); err != nil {
		t.Fatal(err)
	}
	if got := titleOf(BuildSessions(cfg), "s1"); got != "alpha-two" {
		t.Errorf("cache miss after mtime change: got title %q, want alpha-two", got)
	}
}

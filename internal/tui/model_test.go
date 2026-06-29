package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexander-posztos/ccdash/internal/config"
	"github.com/alexander-posztos/ccdash/internal/engine"
)

// projectRows counts the selectable project rows in the list, ignoring the
// Today/This week/Older divider rows the list now interleaves.
func projectRows(m Model) int {
	n := 0
	for _, it := range m.list.Items() {
		if _, ok := it.(projectItem); ok {
			n++
		}
	}
	return n
}

func TestModelFlow(t *testing.T) {
	m := New(config.Config{})

	step := func(msg tea.Msg) {
		nm, _ := m.Update(msg)
		m = nm.(Model)
	}
	step(tea.WindowSizeMsg{Width: 120, Height: 40})

	now := time.Now()
	ps := []*engine.Project{{
		Name: "alpha", Raw: "/x/alpha", Last: now,
		Sessions:     []*engine.Session{{SID: "s1", FirstPrompt: "do alpha", Last: now}},
		RealSessions: []*engine.Session{{SID: "s1", FirstPrompt: "do alpha", Last: now}},
	}}
	step(projectsLoadedMsg{ps})

	if !m.ready {
		t.Fatal("model not ready after projectsLoadedMsg")
	}
	v := m.View()
	if !strings.Contains(v, "alpha") {
		t.Errorf("view missing project name:\n%s", v)
	}
	if !strings.Contains(v, "RECAP") {
		t.Errorf("view missing the recap section:\n%s", v)
	}

	step(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != paneDetail {
		t.Errorf("tab did not switch focus to detail")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q produced no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q did not produce a QuitMsg")
	}
}

func TestScopeFilterResume(t *testing.T) {
	m := New(config.Config{ActiveDays: 30})
	step := func(msg tea.Msg) {
		nm, _ := m.Update(msg)
		m = nm.(Model)
	}
	typ := func(r rune) { step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	step(tea.WindowSizeMsg{Width: 120, Height: 40})

	now := time.Now()
	old := now.Add(-100 * 24 * time.Hour)
	mk := func(name, sid string, last time.Time) *engine.Project {
		s := &engine.Session{SID: sid, FirstPrompt: name, Last: last}
		return &engine.Project{Name: name, Raw: "/x/" + name, Last: last,
			Sessions: []*engine.Session{s}, RealSessions: []*engine.Session{s}}
	}
	step(projectsLoadedMsg{[]*engine.Project{mk("alpha", "s1", now), mk("oldproj", "s2", old)}})

	if got := projectRows(m); got != 1 {
		t.Fatalf("active scope: got %d projects, want 1 (only alpha)", got)
	}
	typ('a') // active -> all
	if got := projectRows(m); got != 2 {
		t.Fatalf("all scope: got %d projects, want 2", got)
	}

	typ('/')
	if !m.filtering {
		t.Fatal("/ did not enter filter mode")
	}
	for _, r := range "old" {
		typ(r)
	}
	if got := projectRows(m); got != 1 {
		t.Fatalf("filtered: got %d projects, want 1 (oldproj)", got)
	}
	step(tea.KeyMsg{Type: tea.KeyEnter}) // close filter, keep value
	if m.filtering {
		t.Error("enter should close the filter")
	}

	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = nm.(Model)
	if m.result == nil || m.result.Resume == nil || m.result.Resume.SID != "s2" {
		t.Fatalf("resume latest: got %+v", m.result)
	}
	if cmd == nil {
		t.Error("resume should quit the program")
	}
}

// TestMovedSuppressedFromActive covers the auto-hide of moved projects: a
// recent-but-moved project (its dir is gone) is kept out of the active view and
// counted as "moved" in the topbar, but reappears under `a` -> all.
func TestMovedSuppressedFromActive(t *testing.T) {
	m := New(config.Config{ActiveDays: 30})
	step := func(msg tea.Msg) {
		nm, _ := m.Update(msg)
		m = nm.(Model)
	}
	typ := func(r rune) { step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	step(tea.WindowSizeMsg{Width: 120, Height: 40})

	now := time.Now()
	mk := func(name string, missing bool) *engine.Project {
		s := &engine.Session{SID: name, FirstPrompt: name, Last: now}
		p := &engine.Project{Name: name, Raw: "/x/" + name, Last: now, Missing: missing,
			Sessions: []*engine.Session{s}, RealSessions: []*engine.Session{s}}
		if !missing {
			p.Dir = "/x/" + name
		}
		return p
	}
	// Both are recent; only the live one belongs in the active view.
	step(projectsLoadedMsg{[]*engine.Project{mk("alpha", false), mk("ghost", true)}})

	if got := projectRows(m); got != 1 {
		t.Fatalf("active scope: got %d rows, want 1 (moved ghost suppressed)", got)
	}
	if bar := m.topbar(); !strings.Contains(bar, "1 moved") {
		t.Fatalf("topbar should show the moved count, got %q", bar)
	}
	typ('a') // active -> all reveals the moved project
	if got := projectRows(m); got != 2 {
		t.Fatalf("all scope: got %d rows, want 2 (ghost revealed)", got)
	}
}

func TestNewSession(t *testing.T) {
	m := New(config.Config{})
	step := func(msg tea.Msg) {
		nm, _ := m.Update(msg)
		m = nm.(Model)
	}
	step(tea.WindowSizeMsg{Width: 120, Height: 40})

	now := time.Now()
	ps := []*engine.Project{{
		Name: "alpha", Raw: "/x/alpha", Dir: "/x/alpha", Last: now,
		Sessions:     []*engine.Session{{SID: "s1", FirstPrompt: "do alpha", Last: now}},
		RealSessions: []*engine.Session{{SID: "s1", FirstPrompt: "do alpha", Last: now}},
	}}
	step(projectsLoadedMsg{ps})

	// n starts a fresh session in the selected project's dir (no --resume).
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = nm.(Model)
	if m.result == nil || m.result.New == nil || m.result.New.Dir != "/x/alpha" {
		t.Fatalf("new session: got %+v", m.result)
	}
	if cmd == nil {
		t.Fatal("new should quit the program")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("new did not produce a QuitMsg")
	}
}

func TestDateDividers(t *testing.T) {
	m := New(config.Config{ActiveDays: 30})
	step := func(msg tea.Msg) {
		nm, _ := m.Update(msg)
		m = nm.(Model)
	}
	press := func(r rune) { step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	sel := func() string {
		if p := m.selectedProject(); p != nil {
			return p.Name
		}
		return "<nil>"
	}
	step(tea.WindowSizeMsg{Width: 120, Height: 40})

	now := time.Now()
	mk := func(name string, last time.Time) *engine.Project {
		s := &engine.Session{SID: name, FirstPrompt: name, Last: last}
		return &engine.Project{Name: name, Raw: "/x/" + name, Dir: "/x/" + name, Last: last,
			Sessions: []*engine.Session{s}, RealSessions: []*engine.Session{s}}
	}
	// Newest-first, one project per band: Today / This week / Older.
	step(projectsLoadedMsg{[]*engine.Project{
		mk("alpha", now),
		mk("beta", now.Add(-2*24*time.Hour)),
		mk("gamma", now.Add(-10*24*time.Hour)),
	}})

	// 3 dividers interleaved with 3 projects.
	if got := projectRows(m); got != 3 {
		t.Fatalf("got %d project rows, want 3", got)
	}
	if got := len(m.list.Items()); got != 6 {
		t.Fatalf("got %d total rows, want 6 (3 dividers + 3 projects)", got)
	}

	// Initial selection is the first project, not the leading Today divider.
	if got := sel(); got != "alpha" {
		t.Fatalf("initial selection: got %q, want alpha", got)
	}

	// j steps down, skipping each divider, and clamps at the last project.
	press('j')
	if got := sel(); got != "beta" {
		t.Fatalf("after j: got %q, want beta", got)
	}
	press('j')
	if got := sel(); got != "gamma" {
		t.Fatalf("after jj: got %q, want gamma", got)
	}
	press('j')
	if got := sel(); got != "gamma" {
		t.Fatalf("j at end: got %q, want gamma (clamped)", got)
	}

	// k steps back up, skipping dividers, and clamps at the first project.
	press('k')
	if got := sel(); got != "beta" {
		t.Fatalf("after k: got %q, want beta", got)
	}
	press('k')
	if got := sel(); got != "alpha" {
		t.Fatalf("after kk: got %q, want alpha", got)
	}
	press('k')
	if got := sel(); got != "alpha" {
		t.Fatalf("k at top: got %q, want alpha (clamped)", got)
	}
}

// TestAutoRefreshOnOpen covers the stale-while-revalidate behavior: opening the
// dashboard kicks off a background regen of active, stale recaps (online), and
// never does so in offline mode (where recaps are uncached and would spin
// forever). It inspects the in-flight regen state and the returned command
// WITHOUT executing the command, so claude is never shelled out to.
func TestAutoRefreshOnOpen(t *testing.T) {
	now := time.Now()
	mk := func() *engine.Project {
		s := &engine.Session{SID: "s1", FirstPrompt: "alpha", Last: now}
		return &engine.Project{Name: "alpha", Raw: "/x/alpha", Dir: "/x/alpha", Last: now,
			Sessions: []*engine.Session{s}, RealSessions: []*engine.Session{s}}
	}

	// Online: a stale, active project is auto-regenerated on open.
	m := New(config.Config{ActiveDays: 30})
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(Model)
	nm, cmd := m.Update(projectsLoadedMsg{[]*engine.Project{mk()}})
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("expected a background regen command on open for a stale active project")
	}
	if len(m.st.regen) != 1 {
		t.Fatalf("expected 1 in-flight regen after open, got %d", len(m.st.regen))
	}

	// Offline: never auto-regen, since offline recaps are uncached and would stay
	// stale (the spinner would never resolve).
	mo := New(config.Config{ActiveDays: 30, Offline: true})
	nm, _ = mo.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mo = nm.(Model)
	nm, cmd = mo.Update(projectsLoadedMsg{[]*engine.Project{mk()}})
	mo = nm.(Model)
	if cmd != nil {
		t.Fatal("offline mode must not auto-regen on open")
	}
	if len(mo.st.regen) != 0 {
		t.Fatalf("offline mode should have 0 in-flight regens, got %d", len(mo.st.regen))
	}
}

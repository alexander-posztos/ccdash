package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexander-posztos/ccdash/internal/config"
	"github.com/alexander-posztos/ccdash/internal/engine"
	"github.com/alexander-posztos/ccdash/internal/prefs"
)

type pane int

const (
	paneList pane = iota
	paneDetail
)

// scopeMode is the list's visibility filter, cycled by the `a` key:
// active (recent only) -> all (include inactive) -> hidden (only hidden ones).
type scopeMode int

const (
	scopeActive scopeMode = iota
	scopeAll
	scopeHidden
)

// recapSem bounds how many `claude` recap processes run at once during the
// stale-while-revalidate fan-out that refreshes stale recaps on open. Each regen
// Cmd acquires a slot before shelling out.
var recapSem = make(chan struct{}, 4)

type projectsLoadedMsg struct{ projects []*engine.Project }
type recapDoneMsg struct{ key string }
type regenGlowDoneMsg struct{}
type flashClearMsg struct{ id int }

// Result is what the TUI hands back to main() to act on AFTER the terminal is
// restored (resume / editor handoff replaces the process via exec).
type Result struct {
	Resume *engine.Session
	New    *engine.Project
}

type Model struct {
	cfg       config.Config
	projects  []*engine.Project
	byKey     map[string]*engine.Project
	list      list.Model
	detail    viewport.Model
	spin      spinner.Model
	filterIn  textinput.Model
	st        *listState
	focus     pane
	scope     scopeMode
	filtering bool
	showHelp  bool
	ready     bool
	width     int
	height    int
	dcursor   int    // highlighted session index within the detail pane
	nShown    int    // sessions currently shown in the detail pane
	regenGlow bool   // brief "done" afterglow after a regen batch finishes
	flashMsg  string // transient status line (hide feedback)
	flashID   int    // generation counter so a stale clear-timer never wipes a newer flash
	result    *Result
}

func New(cfg config.Config) Model {
	useTheme(resolveTheme(cfg.Theme))
	st := &listState{
		stale: map[string]bool{},
		regen: map[string]bool{},
		frame: "·",
	}
	l := list.New(nil, projectDelegate{st: st}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))

	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "filter projects"

	return Model{
		cfg:      cfg,
		byKey:    map[string]*engine.Project{},
		list:     l,
		detail:   viewport.New(0, 0),
		spin:     sp,
		filterIn: ti,
		st:       st,
		focus:    paneList,
	}
}

func (m Model) Init() tea.Cmd { return loadProjectsCmd(m.cfg) }

func loadProjectsCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		ps := engine.GroupProjects(cfg)
		for _, p := range ps {
			if p.Dir != "" && engine.IsActive(cfg, p) {
				engine.GitStateCached(p.Dir) // warm the cache off the UI goroutine
			}
		}
		return projectsLoadedMsg{ps}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case projectsLoadedMsg:
		m.projects = msg.projects
		m.byKey = make(map[string]*engine.Project, len(msg.projects))
		for _, p := range msg.projects {
			k := projectKey(p)
			m.byKey[k] = p
			m.st.stale[k] = engine.RecapIsStale(m.cfg, p)
		}
		m.ready = true
		m.rebuildList()
		m.layout()
		// Stale-while-revalidate: the list now shows cached recaps instantly, so
		// kick off a bounded background regen of every active, stale recap and let
		// each card swap to a fresh summary in place - no keypress, no hook. Offline
		// recaps are never cached (they would stay "stale" and spin forever), so skip
		// the auto-refresh entirely in offline mode.
		if m.cfg.Offline {
			return m, nil
		}
		return m, m.startRegen(m.staleActiveTargets())

	case spinner.TickMsg:
		if len(m.st.regen) == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		m.st.frame = m.spin.View()
		// The list rows read m.st.frame live on the next View, so they animate for
		// free. The detail pane caches its content, so re-render it only while the
		// selected project is the one regenerating, to keep its recap spinner moving.
		if m.st.regen[m.selectedKey()] {
			m.syncDetail()
		}
		return m, cmd

	case recapDoneMsg:
		delete(m.st.regen, msg.key)
		if p := m.byKey[msg.key]; p != nil {
			m.st.stale[msg.key] = engine.RecapIsStale(m.cfg, p)
		}
		var cmd tea.Cmd
		if len(m.st.regen) == 0 {
			m.regenGlow = true
			cmd = tea.Tick(2*time.Second, func(time.Time) tea.Msg { return regenGlowDoneMsg{} })
		}
		if msg.key == m.selectedKey() {
			m.syncDetail()
		}
		return m, cmd

	case regenGlowDoneMsg:
		m.regenGlow = false
		return m, nil

	case flashClearMsg:
		if msg.id == m.flashID {
			m.flashMsg = ""
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	if m.filtering {
		m.filterIn, cmd = m.filterIn.Update(msg)
		return m, cmd
	}
	if m.focus == paneList {
		m.list, cmd = m.list.Update(msg)
	} else {
		m.detail, cmd = m.detail.Update(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if m.filtering {
		return m.handleFilterKey(msg)
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.focus == paneDetail {
			m.focus = paneList
			m.syncDetail()
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "/":
		m.filtering = true
		m.filterIn.Focus()
		return m, textinput.Blink
	case "a":
		m.scope = (m.scope + 1) % 3 // active -> all -> hidden -> active
		m.focus = paneList
		m.rebuildList()
		return m, nil
	case "tab", "shift+tab":
		m.toggleFocus()
		return m, nil
	}
	if m.focus == paneDetail {
		return m.handleDetailKey(msg)
	}
	return m.handleListKey(msg)
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "right", "enter":
		if m.hasSessions() {
			m.focus = paneDetail
			m.dcursor = 0
			m.syncDetail()
		}
		return m, nil
	case "l":
		return m.resume(0)
	case "n":
		return m.startNew()
	case "r":
		return m, m.startRegen(m.selectedRegenTargets())
	case "x":
		return m, m.toggleHide()
	}
	prev := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.skipHeaderRow(msg.String())
	if m.list.Index() != prev {
		m.dcursor = 0
		m.syncDetail()
	}
	return m, cmd
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left":
		m.focus = paneList
		m.syncDetail()
	case "j", "down":
		if m.dcursor < m.nShown-1 {
			m.dcursor++
			m.syncDetail()
		}
	case "k", "up":
		if m.dcursor > 0 {
			m.dcursor--
			m.syncDetail()
		}
	case "g":
		m.dcursor = 0
		m.syncDetail()
	case "G":
		if m.nShown > 0 {
			m.dcursor = m.nShown - 1
			m.syncDetail()
		}
	case "enter":
		return m.resume(m.dcursor)
	case "l":
		return m.resume(0)
	case "n":
		return m.startNew()
	case "r":
		return m, m.startRegen(m.selectedRegenTargets())
	case "x":
		return m, m.toggleHide()
	case "pgdown":
		m.detail.ScrollDown(m.detail.Height / 2)
	case "pgup":
		m.detail.ScrollUp(m.detail.Height / 2)
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filterIn.Blur()
		m.filterIn.SetValue("")
		m.rebuildList()
		return m, nil
	case "enter":
		m.filtering = false
		m.filterIn.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.filterIn, cmd = m.filterIn.Update(msg)
	m.rebuildList()
	return m, cmd
}

// toggleHide hides or unhides the selected project, persists the change, and
// rebuilds the list so it drops out of (or, in the hidden scope, stays in) view.
// Hiding only flips a derived flag; the next GroupProjects re-derives it from the
// same prefs file, so there is no need for a full reload here.
func (m *Model) toggleHide() tea.Cmd {
	p := m.selectedProject()
	if p == nil {
		return nil
	}
	hide := !p.Hidden
	var err error
	if hide {
		err = prefs.Hide(m.cfg.PrefsFile, p.Raw, time.Now())
	} else {
		err = prefs.Unhide(m.cfg.PrefsFile, p.Raw)
	}
	if err != nil {
		return m.flash("could not save prefs: " + err.Error())
	}
	p.Hidden = hide
	m.rebuildList()
	if hide {
		return m.flash("hid " + p.Name + "  ·  a cycles to the hidden view")
	}
	return m.flash("unhid " + p.Name)
}

// flash sets a transient status line and schedules its own clear. The id guard
// means a later flash's timer is the only one that can clear it.
func (m *Model) flash(s string) tea.Cmd {
	m.flashID++
	m.flashMsg = s
	id := m.flashID
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return flashClearMsg{id} })
}

// resume targets the i-th real session of the selected project (0 = latest) and
// quits so main() can exec into claude with a restored terminal.
func (m Model) resume(i int) (tea.Model, tea.Cmd) {
	p := m.selectedProject()
	if p == nil || i < 0 || i >= len(p.RealSessions) {
		return m, nil
	}
	m.result = &Result{Resume: p.RealSessions[i]}
	return m, tea.Quit
}

// startNew starts a fresh claude in the selected project's dir (no --resume) and
// quits so main() can exec into it. No-ops if the project has no resolvable dir.
func (m Model) startNew() (tea.Model, tea.Cmd) {
	if p := m.selectedProject(); p != nil && p.Dir != "" {
		m.result = &Result{New: p}
		return m, tea.Quit
	}
	return m, nil
}

// selectedRegenTargets is the manual `r` target: just the highlighted project.
func (m Model) selectedRegenTargets() []*engine.Project {
	if p := m.selectedProject(); p != nil {
		return []*engine.Project{p}
	}
	return nil
}

// staleActiveTargets is the set auto-refreshed when the dashboard opens: every
// active, non-hidden, non-moved project whose cached recap is stale. Hidden and
// moved projects are skipped (both are absent from the active view) so we never
// spend a claude call keeping a recap warm for a card you are not looking at. A
// manual `r` still regenerates a moved project's recap from its transcripts.
func (m Model) staleActiveTargets() []*engine.Project {
	var out []*engine.Project
	for _, p := range m.projects {
		if p.Hidden || p.Missing || !engine.IsActive(m.cfg, p) {
			continue
		}
		if m.st.stale[projectKey(p)] {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) startRegen(targets []*engine.Project) tea.Cmd {
	wasIdle := len(m.st.regen) == 0
	cmds := make([]tea.Cmd, 0, len(targets)+1)
	for _, p := range targets {
		key := projectKey(p)
		if m.st.regen[key] {
			continue // already in flight; do not double-spend a claude call
		}
		m.st.regen[key] = true
		cmds = append(cmds, regenRecapCmd(m.cfg, p))
	}
	if len(cmds) == 0 {
		return nil
	}
	if wasIdle {
		cmds = append(cmds, m.spin.Tick)
	}
	return tea.Batch(cmds...)
}

func regenRecapCmd(cfg config.Config, p *engine.Project) tea.Cmd {
	return func() tea.Msg {
		recapSem <- struct{}{}
		defer func() { <-recapSem }()
		// A degraded recap surfaces as the offline card; we cannot write to
		// stderr here without punching through the alt-screen.
		_, _ = engine.GenerateRecap(cfg, p)
		return recapDoneMsg{key: projectKey(p)}
	}
}

func (m *Model) toggleFocus() {
	if m.focus == paneList {
		if m.hasSessions() {
			m.focus = paneDetail
			m.dcursor = 0
		}
	} else {
		m.focus = paneList
	}
	m.syncDetail()
}

func (m Model) visibleProjects() []*engine.Project {
	f := strings.ToLower(strings.TrimSpace(m.filterIn.Value()))
	var out []*engine.Project
	for _, p := range m.projects {
		switch m.scope {
		case scopeHidden:
			if !p.Hidden {
				continue // the hidden view shows ONLY hidden projects
			}
		default: // scopeActive, scopeAll: hidden projects are suppressed
			if p.Hidden {
				continue
			}
			if m.scope == scopeActive {
				// The active view is "live work I can resume in place", so it skips a
				// project that is stale OR moved (its dir is gone). Moved ones stay one
				// keypress away under `a` -> all; this suppression is derived, not a
				// persisted hide, so restoring the dir (or adding its new home to a
				// project root) brings the project back on the next load.
				if !engine.IsActive(m.cfg, p) || p.Missing {
					continue
				}
			}
		}
		if f != "" && !strings.Contains(strings.ToLower(p.Name), f) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (m *Model) rebuildList() {
	prevKey := m.selectedKey()
	items := groupedRows(m.visibleProjects())
	m.list.SetItems(items)
	m.list.Select(firstSelectable(items, prevKey))
	m.dcursor = 0
	m.syncDetail()
}

// firstSelectable returns the index of the projectItem whose key matches want,
// or the first projectItem when there's no match (divider rows are never
// selected). Returns 0 for an empty list.
func firstSelectable(items []list.Item, want string) int {
	first, found := 0, false
	for i, it := range items {
		pi, ok := it.(projectItem)
		if !ok {
			continue
		}
		if !found {
			first, found = i, true
		}
		if want != "" && projectKey(pi.p) == want {
			return i
		}
	}
	return first
}

// skipHeaderRow keeps the list cursor off divider rows: if the just-processed
// nav key landed the selection on a headerItem, it steps to the nearest
// projectItem in the direction of travel, falling back the other way at the
// list ends. Runs inside the same Update, so a divider never shows as selected.
func (m *Model) skipHeaderRow(key string) {
	items := m.list.Items()
	n := len(items)
	if n == 0 {
		return
	}
	idx := m.list.Index()
	if _, isHeader := items[idx].(headerItem); !isHeader {
		return
	}
	dir := 1
	switch key {
	case "k", "up", "pgup", "g", "home", "ctrl+p":
		dir = -1
	}
	for i := idx + dir; i >= 0 && i < n; i += dir {
		if _, h := items[i].(headerItem); !h {
			m.list.Select(i)
			return
		}
	}
	for i := idx - dir; i >= 0 && i < n; i -= dir {
		if _, h := items[i].(headerItem); !h {
			m.list.Select(i)
			return
		}
	}
}

func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	bodyH := m.height - 2 // topbar + footer
	if bodyH < 3 {
		bodyH = 3
	}
	leftW := m.width * 2 / 5
	if leftW < 37 {
		leftW = 37
	}
	rightW := m.width - leftW - 1
	innerH := bodyH - 2 // pane border top/bottom
	m.list.SetSize(leftW-4, innerH)
	m.detail.Width = rightW - 4
	m.detail.Height = innerH
	if m.ready {
		m.syncDetail()
	}
}

func (m *Model) syncDetail() {
	p := m.selectedProject()
	if p == nil {
		m.detail.SetContent(m.emptyDetail())
		m.nShown = 0
		return
	}
	m.nShown = min(len(p.RealSessions), maxSessions)
	if m.dcursor >= m.nShown {
		m.dcursor = max(0, m.nShown-1)
	}
	cursor := -1
	if m.focus == paneDetail && m.nShown > 0 {
		cursor = m.dcursor
	}
	content, sessStart := renderDetail(m.cfg, p, m.detail.Width, cursor, m.st.regen[projectKey(p)], m.st.frame)
	m.detail.SetContent(content)
	if cursor >= 0 {
		m.ensureVisible(sessStart + m.dcursor)
	} else {
		m.detail.GotoTop()
	}
}

func (m *Model) ensureVisible(line int) {
	h := m.detail.Height
	if h <= 0 {
		return
	}
	switch top := m.detail.YOffset; {
	case line < top:
		m.detail.SetYOffset(line)
	case line >= top+h:
		m.detail.SetYOffset(line - h + 1)
	}
}

func (m Model) emptyDetail() string {
	if f := strings.TrimSpace(m.filterIn.Value()); f != "" {
		return stMuted.Render(fmt.Sprintf("no projects match %q", f))
	}
	switch m.scope {
	case scopeHidden:
		return stMuted.Render("no hidden projects\npress  a  to cycle back")
	case scopeActive:
		return stMuted.Render(fmt.Sprintf("no active projects in the last %d days\npress  a  to show all", m.cfg.ActiveDays))
	}
	return stMuted.Render("no projects")
}

func (m Model) selectedProject() *engine.Project {
	if it, ok := m.list.SelectedItem().(projectItem); ok {
		return it.p
	}
	return nil
}

func (m Model) selectedKey() string {
	if p := m.selectedProject(); p != nil {
		return projectKey(p)
	}
	return ""
}

func (m Model) hasSessions() bool {
	p := m.selectedProject()
	return p != nil && len(p.RealSessions) > 0
}

func projectKey(p *engine.Project) string {
	if p.Dir != "" {
		return p.Dir
	}
	return p.Raw
}

func (m Model) View() string {
	if !m.ready {
		return "\n  reading your sessions..."
	}
	if m.showHelp {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, helpBox())
	}
	left := paneStyle(m.focus == paneList, m.list.Width()+2, m.list.Height()).Render(m.list.View())
	right := paneStyle(m.focus == paneDetail, m.detail.Width+2, m.detail.Height).Render(m.detail.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	bottom := m.footer()
	if m.filtering {
		bottom = m.filterIn.View()
	}
	return m.topbar() + "\n" + body + "\n" + bottom
}

func (m Model) topbar() string {
	active, moved, hidden := 0, 0, 0
	for _, p := range m.projects {
		if p.Hidden {
			hidden++
			continue
		}
		if p.Missing {
			moved++ // suppressed from active; surfaced here so they are not a surprise
			continue
		}
		if engine.IsActive(m.cfg, p) {
			active++
		}
	}
	scope := fmt.Sprintf("active %dd", m.cfg.ActiveDays)
	switch m.scope {
	case scopeAll:
		scope = "all"
	case scopeHidden:
		scope = "hidden"
	}
	counts := fmt.Sprintf("   %d active   %d total", active, len(m.projects))
	if moved > 0 {
		counts += fmt.Sprintf("   %d moved", moved)
	}
	if hidden > 0 {
		counts += fmt.Sprintf("   %d hidden", hidden)
	}
	b := stAmber.Render("● ") + stName.Render("ccdash") +
		stMuted.Render(counts+"   "+scope)
	if n := len(m.st.regen); n > 0 {
		b += stMuted.Render("   ") + stAmber.Render(fmt.Sprintf("%s regenerating %d", m.st.frame, n))
	} else if m.regenGlow {
		b += stMuted.Render("   ") + stSage.Render("✓ done")
	}
	if m.flashMsg != "" {
		b += stMuted.Render("   ") + stAmber.Render(m.flashMsg)
	}
	if f := strings.TrimSpace(m.filterIn.Value()); f != "" {
		b += stMuted.Render("   ") + stAmber.Render("/"+f)
	}
	if m.width > 0 {
		return lipgloss.NewStyle().MaxWidth(m.width).Render(b)
	}
	return b
}

func (m Model) footer() string {
	var pairs [][2]string
	if m.focus == paneDetail {
		pairs = [][2]string{{"j/k", "move"}, {"↵", "resume"}, {"←", "back"}, {"l", "latest"}, {"n", "new"}, {"r", "recap"}, {"x", "hide"}, {"?", "help"}, {"q", "quit"}}
	} else {
		pairs = [][2]string{{"j/k", "move"}, {"→", "sessions"}, {"l", "resume"}, {"n", "new"}, {"r", "recap"}, {"x", "hide"}, {"a", "scope"}, {"/", "filter"}, {"?", "help"}, {"q", "quit"}}
	}
	out := ""
	for i, p := range pairs {
		cand := out
		if i > 0 {
			cand += stMuted.Render("   ")
		}
		cand += stFg.Render(p[0]) + stMuted.Render(" "+p[1])
		if m.width > 0 && lipgloss.Width(cand) > m.width {
			break
		}
		out = cand
	}
	return out
}

func helpBox() string {
	rows := [][2]string{
		{"j / k", "move highlight"},
		{"tab / →", "switch pane / sessions"},
		{"g / G", "jump to first / last session"},
		{"pgup / pgdn", "scroll the detail pane"},
		{"enter", "drill in  ·  resume session"},
		{"l", "resume latest session"},
		{"n", "new session in project"},
		{"r", "regen the selected recap"},
		{"x", "hide / unhide project"},
		{"a", "scope: active / all / hidden"},
		{"/", "filter by name"},
		{"? esc q", "help  ·  back  ·  quit"},
	}
	body := stAmberB.Render("ccdash") + stMuted.Render("  where did I leave off?") + "\n\n"
	for _, r := range rows {
		body += stAmber.Render(fmt.Sprintf("  %-10s", r[0])) + stFg.Render(r[1]) + "\n"
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cAmber).
		Padding(1, 3).
		Render(body)
}

func Run(cfg config.Config) (*Result, error) {
	fm, err := tea.NewProgram(New(cfg), tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}
	if m, ok := fm.(Model); ok {
		return m.result, nil
	}
	return nil, nil
}

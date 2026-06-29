package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexander-posztos/ccdash/internal/engine"
)

// listState holds the Model's recap-status bookkeeping: which projects have a
// stale recap (drives R = regen all stale) and which are regenerating now
// (drives the topbar count plus the per-row spinner the delegate draws). The
// delegate holds a pointer to this state so a regenerating row can spin in place.
type listState struct {
	stale map[string]bool // project key -> recap is stale
	regen map[string]bool // project key -> recap regen in flight
	frame string          // current spinner frame, shared by the topbar and rows
}

type projectItem struct{ p *engine.Project }

func (i projectItem) FilterValue() string { return i.p.Name }

// headerItem is a non-selectable divider row marking an age band (Today / This
// week / Older). The list cursor is steered around these by skipHeaderRow; they
// exist only so the delegate can draw a band rule between project groups.
type headerItem struct{ label string }

func (i headerItem) FilterValue() string { return "" }

// projectDelegate is otherwise stateless, but carries a pointer to the live
// listState so renderProject can spin the leading dot on the one row whose recap
// is regenerating. The pointer means rows read the current frame on every View.
type projectDelegate struct{ st *listState }

func (d projectDelegate) Height() int                         { return 1 }
func (d projectDelegate) Spacing() int                        { return 0 }
func (d projectDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d projectDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	switch it := item.(type) {
	case headerItem:
		renderGroupHeader(w, m.Width(), it.label)
	case projectItem:
		d.renderProject(w, m, index, it.p)
	}
}

func (d projectDelegate) renderProject(w io.Writer, m list.Model, index int, p *engine.Project) {
	width := m.Width()
	if width < 24 {
		width = 24
	}
	whenW, branchW := 7, 14
	// A trailing cell (gap + 1-wide glyph) is reserved on every row so the regen
	// spinner has a home at the right edge without shifting the columns when it
	// blinks in or out; idle rows leave it blank.
	nameW := width - whenW - branchW - 5 // leading space + 2 column gaps + trailing gap + spinner
	if nameW < 6 {
		nameW = 6
	}

	name := clip(p.Name, nameW)
	when := whenLabel(p.Last)
	branch := branchLabel(p, branchW)
	// A moved/missing project has no git state, so reuse the branch column to flag
	// it, so the moved status is visible from the list, not just the detail pane.
	if p.Missing {
		branch = "△ moved"
	}

	// While this project's recap regenerates, the reserved trailing cell carries a
	// spinner; it is gone the moment regen finishes. On the selection bar the dot
	// inherits the bar's ink; off it, the accent matches the topbar spinner.
	regen := d.st != nil && d.st.regen[projectKey(p)]

	if index == m.Index() {
		spin := " "
		if regen {
			spin = d.st.frame
		}
		line := fmt.Sprintf(" %-*s %*s %-*s %s", nameW, name, whenW, when, branchW, branch, spin)
		_, _ = fmt.Fprint(w, stSelected.Width(width).Render(line))
		return
	}

	nameStyle := stName
	if p.Hidden {
		nameStyle = stMuted // hidden projects (shown only in the hidden scope) read as set-aside
	}
	branchStyle := stMuted
	if p.Missing {
		branchStyle = stCoral
	}
	nameCell := nameStyle.Width(nameW).Render(name)
	whenCell := stMuted.Width(whenW).Align(lipgloss.Right).Render(when)
	branchCell := branchStyle.Width(branchW).Render(branch)
	spin := " "
	if regen {
		spin = stAmber.Render(d.st.frame)
	}
	_, _ = fmt.Fprint(w, " "+nameCell+" "+whenCell+" "+branchCell+" "+spin)
}

// renderGroupHeader draws a dim band divider, e.g. "── Today ─────────────".
func renderGroupHeader(w io.Writer, width int, label string) {
	if width < 24 {
		width = 24
	}
	lead := "── " + label + " "
	if pad := width - lipgloss.Width(lead); pad > 0 {
		lead += strings.Repeat("─", pad)
	}
	_, _ = fmt.Fprint(w, stMuted.Render(clip(lead, width)))
}

// groupedRows turns recency-sorted projects into list rows, inserting a band
// divider before each new age band. Assumes ps is already sorted newest-first
// (GroupProjects does this), so each band is a contiguous run.
func groupedRows(ps []*engine.Project) []list.Item {
	items := make([]list.Item, 0, len(ps)+3)
	lastBucket := -1
	for _, p := range ps {
		if b := ageBucket(p.Last); b != lastBucket {
			items = append(items, headerItem{bucketLabel(b)})
			lastBucket = b
		}
		items = append(items, projectItem{p})
	}
	return items
}

// ageBucket maps a last-activity time to a coarse recency band, lower = newer:
// 0 = Today (<24h), 1 = This week (<7d), 2 = Older (>=7d or unknown). Thresholds
// match the old recency styling the bands replaced.
func ageBucket(t time.Time) int {
	if t.IsZero() {
		return 2
	}
	switch d := time.Since(t).Hours() / 24; {
	case d < 1:
		return 0
	case d < 7:
		return 1
	default:
		return 2
	}
}

func bucketLabel(b int) string {
	switch b {
	case 0:
		return "Today"
	case 1:
		return "This week"
	default:
		return "Older"
	}
}

func branchLabel(p *engine.Project, width int) string {
	g := engine.GitStateCached(p.Dir)
	if g == nil {
		return "-"
	}
	if g.Dirty > 0 {
		return clip(fmt.Sprintf("%s ±%d", clip(g.Branch, width-4), g.Dirty), width)
	}
	return clip(g.Branch, width)
}

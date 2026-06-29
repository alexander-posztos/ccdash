package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/alexander-posztos/ccdash/internal/config"
	"github.com/alexander-posztos/ccdash/internal/engine"
)

const maxSessions = 12

// renderDetail builds the right pane for a project. When cursor >= 0 the session
// at that index is highlighted (the detail pane is focused). While regen is true
// the RECAP header carries a spinner (frame is the current glyph), matching the
// per-row indicator in the list. It returns the rendered content and the line
// index where the session rows begin, so the model can keep the highlighted
// session scrolled into view.
func renderDetail(cfg config.Config, p *engine.Project, width, cursor int, regen bool, frame string) (string, int) {
	var b strings.Builder

	b.WriteString(stAmberB.Render(p.Name) + "\n")
	if p.Hidden {
		b.WriteString(stMuted.Render("✕ hidden  "))
	}
	if p.Missing {
		b.WriteString(stCoral.Render("△ moved  "))
	}
	dir := p.Dir
	if dir == "" {
		dir = p.Raw
	}
	b.WriteString(stMuted.Render(shortHome(dir)) + "\n\n")

	if g := engine.GitStateCached(p.Dir); g != nil {
		b.WriteString(stName.Render(g.Branch) + "   ")
		if g.Dirty > 0 {
			b.WriteString(stCoral.Render(fmt.Sprintf("± %d uncommitted", g.Dirty)))
		} else {
			b.WriteString(stSage.Render("○ ") + stMuted.Render("clean"))
		}
		if g.Commit != nil {
			b.WriteString(stMuted.Render("   ·   " + clip(g.Commit.Subject, 42) + "  " + g.Commit.When))
		}
		b.WriteString("\n\n")
	}

	b.WriteString(recapEyebrow(width, regen, frame) + "\n")
	b.WriteString(stFg.Render(wrap(recapText(cfg, p), width)) + "\n")

	b.WriteString("\n" + eyebrow("SESSIONS", width) + "\n")
	sessStart := strings.Count(b.String(), "\n")

	sess := p.RealSessions
	shown := min(len(sess), maxSessions)
	for i := 0; i < shown; i++ {
		s := sess[i]
		titleW := width - 12
		if titleW < 20 {
			titleW = 20
		}
		when := whenLabel(s.Last)
		title := clip(oneLine(s.DisplayTitle()), titleW)
		if i == cursor {
			b.WriteString(stSelected.Render(fmt.Sprintf(" %-7s  %s ", when, title)) + "\n")
		} else {
			b.WriteString(stMuted.Render(fmt.Sprintf("%-7s", when)) + "  " + stFg.Render(title) + "\n")
		}
	}
	if len(sess) > maxSessions {
		b.WriteString(stMuted.Render(fmt.Sprintf("+%d more", len(sess)-maxSessions)) + "\n")
	}
	if len(sess) == 0 {
		b.WriteString(stMuted.Render("no sessions") + "\n")
	}
	return b.String(), sessStart
}

// recapEyebrow is the RECAP section header. When the project's recap is
// regenerating it tucks an accent spinner between the label and the rule, so the
// detail pane shows the same in-flight signal as the spinning list row, then
// drops back to the plain eyebrow the moment regen finishes.
func recapEyebrow(width int, regen bool, frame string) string {
	if !regen {
		return eyebrow("RECAP", width)
	}
	head := stMuted.Bold(true).Render("RECAP") + " " + stAmber.Render(frame)
	n := width - lipgloss.Width(head) - 1
	if n < 0 {
		n = 0
	}
	return head + " " + lipgloss.NewStyle().Foreground(cPanel).Render(strings.Repeat("─", n))
}

func recapText(cfg config.Config, p *engine.Project) string {
	if r := engine.LoadRecap(cfg, p); r != nil && r.Text != "" {
		return r.Text
	}
	return engine.OfflineRecap(p)
}

func shortHome(path string) string {
	if path == "" {
		return "?"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// wrap word-wraps prose to width without padding, so recaps longer than the
// pane do not overflow the viewport (which would corrupt the bordered layout).
func wrap(s string, width int) string {
	if width <= 4 {
		return s
	}
	var out strings.Builder
	for li, line := range strings.Split(s, "\n") {
		if li > 0 {
			out.WriteByte('\n')
		}
		col := 0
		for wi, word := range strings.Fields(line) {
			wl := lipgloss.Width(word)
			if wi > 0 {
				if col+1+wl > width {
					out.WriteByte('\n')
					col = 0
				} else {
					out.WriteByte(' ')
					col++
				}
			}
			out.WriteString(word)
			col += wl
		}
	}
	return out.String()
}

// whenLabel is RelTime, but compacts old absolute dates (YYYY-MM-DD) to a short
// "jan'25" form so they fit the narrow when column.
func whenLabel(t time.Time) string {
	rel := engine.RelTime(t)
	if len(rel) == 10 && rel[4] == '-' && rel[7] == '-' {
		if d, err := time.Parse("2006-01-02", rel); err == nil {
			return strings.ToLower(d.Format("Jan'06"))
		}
	}
	return rel
}

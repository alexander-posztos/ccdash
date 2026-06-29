package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is a palette of named SEMANTIC color slots. Fields are lipgloss's
// TerminalColor interface, so a slot can hold a plain Color, an AdaptiveColor
// (light/dark), or a CompleteColor without changing this type. Every color in
// the TUI resolves through the active theme; no call site hardcodes a value.
type Theme struct {
	Name   string
	Bg     lipgloss.TerminalColor // ink: text shown on the selection bar
	Panel  lipgloss.TerminalColor // idle pane border + eyebrow rule
	Fg     lipgloss.TerminalColor // primary text
	Muted  lipgloss.TerminalColor // dim / secondary text
	Accent lipgloss.TerminalColor // focus / now / selection bg / spinner
	Danger lipgloss.TerminalColor // dirty git / moved / attention
	Good   lipgloss.TerminalColor // clean git
}

// slateTheme is ccdash's default: a cool, neutral blue-gray palette tuned for
// legibility on a dark terminal (desaturated sky-blue accent, steel-gray text).
// Every slot clears WCAG AA on a dark background; the muted slot is deliberately
// lifted so timestamps and the branch column stay readable.
var slateTheme = Theme{
	Name:   "slate",
	Bg:     lipgloss.Color("#0E1116"), // dark ink shown ON the accent selection bar
	Panel:  lipgloss.Color("#272D3A"),
	Fg:     lipgloss.Color("#D5DAE3"),
	Muted:  lipgloss.Color("#8A93A6"),
	Accent: lipgloss.Color("#5C9CE6"),
	Danger: lipgloss.Color("#E8736B"),
	Good:   lipgloss.Color("#5FB98E"),
}

// emberTheme is ccdash's original warm dark palette (amber is focus/now, slate
// gray is secondary text, coral is attention). Once the default; still selectable
// via CCDASH_THEME=ember.
var emberTheme = Theme{
	Name:   "ember",
	Bg:     lipgloss.Color("#13141B"),
	Panel:  lipgloss.Color("#262A39"),
	Fg:     lipgloss.Color("#D2D4DE"),
	Muted:  lipgloss.Color("#6A7088"),
	Accent: lipgloss.Color("#F2A65A"),
	Danger: lipgloss.Color("#E96E6E"),
	Good:   lipgloss.Color("#8FB573"),
}

// tokyoNightTheme maps the canonical Tokyo Night ("night" variant, from
// folke/tokyonight.nvim) onto the semantic slots: blue is the accent, orange
// flags uncommitted work, green is a clean tree.
var tokyoNightTheme = Theme{
	Name:   "tokyonight",
	Bg:     lipgloss.Color("#1a1b26"), // bg
	Panel:  lipgloss.Color("#3b4261"), // fg_gutter
	Fg:     lipgloss.Color("#c0caf5"), // fg
	Muted:  lipgloss.Color("#565f89"), // comment
	Accent: lipgloss.Color("#7aa2f7"), // blue
	Danger: lipgloss.Color("#ff9e64"), // orange
	Good:   lipgloss.Color("#9ece6a"), // green
}

// themes is the selectable registry: add a theme here and CCDASH_THEME reaches it.
var themes = map[string]Theme{
	slateTheme.Name:      slateTheme,
	emberTheme.Name:      emberTheme,
	tokyoNightTheme.Name: tokyoNightTheme,
}

// resolveTheme turns a CCDASH_THEME value into a Theme, falling back to the
// default (slate) when it cannot. See resolveThemeErr for the resolution order.
func resolveTheme(name string) Theme {
	t, _ := resolveThemeErr(name)
	return t
}

// resolveThemeErr resolves a CCDASH_THEME value in order: (1) a built-in name,
// matched case/separator-insensitively so "Tokyo Night", "tokyo-night", and
// "tokyonight" all hit; (2) a direct path to a JSON theme file (the value holds a
// separator or ends in .json); (3) a bare name resolved to <themeDir>/<name>.json.
// An unresolved name falls back to slate with a nil error; a file that exists but
// fails to parse falls back to slate WITH the error (the TUI path ignores it; the
// error is what the theme tests assert on).
func resolveThemeErr(name string) (Theme, error) {
	if name == "" {
		return slateTheme, nil
	}
	if t, ok := themes[normalizeName(name)]; ok {
		return t, nil
	}
	if looksLikePath(name) {
		return loadThemeFile(expandHome(name))
	}
	path := filepath.Join(themeDir(), name+".json")
	if _, err := os.Stat(path); err == nil {
		return loadThemeFile(path)
	}
	return slateTheme, nil
}

func normalizeName(name string) string {
	return strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(name))
}

// looksLikePath is true when a CCDASH_THEME value should be read as a file path
// rather than a theme name: it holds a path separator or ends in .json.
func looksLikePath(s string) bool {
	return strings.ContainsAny(s, "/\\") || strings.HasSuffix(strings.ToLower(s), ".json")
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// themeDir is where bare-name theme files live (CCDASH_THEME=gruvbox ->
// <themeDir>/gruvbox.json). Defaults to ~/.config/ccdash/themes, overridable
// with CCDASH_THEME_DIR.
func themeDir() string {
	if d := os.Getenv("CCDASH_THEME_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ccdash", "themes")
}

// themeFile is the on-disk JSON shape: the 7 semantic slots as #RGB/#RRGGBB hex
// plus an optional display name (defaults to the file's base name).
type themeFile struct {
	Name   string `json:"name"`
	Bg     string `json:"bg"`
	Panel  string `json:"panel"`
	Fg     string `json:"fg"`
	Muted  string `json:"muted"`
	Accent string `json:"accent"`
	Danger string `json:"danger"`
	Good   string `json:"good"`
}

// loadThemeFile reads and validates a JSON theme. On any failure it returns the
// default theme plus the error, never a half-populated palette.
func loadThemeFile(path string) (Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return slateTheme, err
	}
	var tf themeFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return slateTheme, fmt.Errorf("theme %s: %w", path, err)
	}
	// Fixed slot order so a missing/invalid slot reports deterministically.
	slots := []struct{ name, val string }{
		{"bg", tf.Bg}, {"panel", tf.Panel}, {"fg", tf.Fg}, {"muted", tf.Muted},
		{"accent", tf.Accent}, {"danger", tf.Danger}, {"good", tf.Good},
	}
	for _, s := range slots {
		if !isHexColor(s.val) {
			return slateTheme, fmt.Errorf("theme %s: slot %q is %q, want a #RGB or #RRGGBB hex color", path, s.name, s.val)
		}
	}
	nm := tf.Name
	if nm == "" {
		nm = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	return Theme{
		Name:   nm,
		Bg:     lipgloss.Color(tf.Bg),
		Panel:  lipgloss.Color(tf.Panel),
		Fg:     lipgloss.Color(tf.Fg),
		Muted:  lipgloss.Color(tf.Muted),
		Accent: lipgloss.Color(tf.Accent),
		Danger: lipgloss.Color(tf.Danger),
		Good:   lipgloss.Color(tf.Good),
	}, nil
}

// isHexColor reports whether s is a #RGB or #RRGGBB hex color.
func isHexColor(s string) bool {
	if (len(s) != 4 && len(s) != 7) || s[0] != '#' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return false
		}
	}
	return true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// Active palette + styles, populated by useTheme from the selected Theme. They
// stay package-level (matching the existing TUI) because the theme is chosen
// once at startup and never switched at runtime. The cAmber/stAmber names are
// historical (ember); under another theme the "accent" slot may be any hue.
var (
	cInk, cPanel, cFg, cMuted, cAmber, cCoral, cSage lipgloss.TerminalColor

	stName, stFg, stMuted, stAmber, stAmberB, stCoral, stSage, stSelected lipgloss.Style
)

func init() { useTheme(slateTheme) } // sane defaults before New() selects a theme

// useTheme makes t the active theme: it repopulates the palette vars and
// rebuilds every derived style. Called once from New before render.
func useTheme(t Theme) {
	cInk, cPanel, cFg, cMuted = t.Bg, t.Panel, t.Fg, t.Muted
	cAmber, cCoral, cSage = t.Accent, t.Danger, t.Good

	stName = lipgloss.NewStyle().Bold(true).Foreground(cFg)
	stFg = lipgloss.NewStyle().Foreground(cFg)
	stMuted = lipgloss.NewStyle().Foreground(cMuted)
	stAmber = lipgloss.NewStyle().Foreground(cAmber)
	stAmberB = lipgloss.NewStyle().Bold(true).Foreground(cAmber)
	stCoral = lipgloss.NewStyle().Foreground(cCoral)
	stSage = lipgloss.NewStyle().Foreground(cSage)
	stSelected = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAmber)
}

func paneStyle(focused bool, w, h int) lipgloss.Style {
	border := cPanel
	if focused {
		border = cAmber
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(w).
		Height(h)
}

// eyebrow is a left-aligned label followed by a faint rule out to width.
func eyebrow(label string, width int) string {
	lab := stMuted.Bold(true).Render(label)
	n := width - lipgloss.Width(lab) - 1
	if n < 0 {
		n = 0
	}
	return lab + " " + lipgloss.NewStyle().Foreground(cPanel).Render(strings.Repeat("─", n))
}

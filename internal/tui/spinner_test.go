package tui

// Covers the per-card regen indicator: a spinner on the one row whose recap is
// regenerating (list delegate) and the matching spinner in the detail-pane RECAP
// header. A sentinel frame glyph stands in for the live spinner so the assertions
// do not depend on which braille frame is current.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"

	"github.com/alexander-posztos/ccdash/internal/engine"
)

// TestRegenRowSpinner checks that a row carries the spinner glyph only while its
// recap is in flight, on both the selected and unselected row, and that it is
// gone the moment regen clears.
func TestRegenRowSpinner(t *testing.T) {
	st := &listState{stale: map[string]bool{}, regen: map[string]bool{}, frame: "Z"}
	p := &engine.Project{Name: "alpha", Raw: "/x/alpha", Dir: "/x/alpha"}
	d := projectDelegate{st: st}
	l := list.New([]list.Item{projectItem{p}}, d, 48, 5) // Index() defaults to 0

	render := func(index int) string {
		var b strings.Builder
		d.Render(&b, l, index, projectItem{p})
		return b.String()
	}

	// Idle: no row carries the glyph.
	if got := render(0); strings.Contains(got, "Z") {
		t.Fatalf("idle selected row should not carry the spinner glyph: %q", got)
	}
	if got := render(1); strings.Contains(got, "Z") {
		t.Fatalf("idle row should not carry the spinner glyph: %q", got)
	}

	// In flight: the glyph shows on the selected row (index == Index()) and on an
	// unselected row alike.
	st.regen[projectKey(p)] = true
	if got := render(0); !strings.Contains(got, "Z") {
		t.Fatalf("regenerating selected row should carry the spinner glyph: %q", got)
	}
	if got := render(1); !strings.Contains(got, "Z") {
		t.Fatalf("regenerating row should carry the spinner glyph: %q", got)
	}

	// Cleared: gone again.
	delete(st.regen, projectKey(p))
	if got := render(1); strings.Contains(got, "Z") {
		t.Fatalf("after regen clears the glyph must be gone: %q", got)
	}
}

// TestRecapEyebrowSpinner checks the detail-pane RECAP header shows the spinner
// only while regenerating, and still reads as RECAP either way.
func TestRecapEyebrowSpinner(t *testing.T) {
	if got := recapEyebrow(60, false, "Z"); strings.Contains(got, "Z") {
		t.Fatalf("idle recap eyebrow must not show the spinner: %q", got)
	}
	got := recapEyebrow(60, true, "Z")
	if !strings.Contains(got, "RECAP") || !strings.Contains(got, "Z") {
		t.Fatalf("regenerating recap eyebrow should show RECAP and the spinner: %q", got)
	}
}

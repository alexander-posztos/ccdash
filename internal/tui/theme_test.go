package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestResolveBuiltinThemes(t *testing.T) {
	// Point the theme dir at an empty temp dir so unknown names cannot
	// accidentally resolve to a file on the dev's real machine.
	t.Setenv("CCDASH_THEME_DIR", t.TempDir())
	cases := map[string]string{
		"":            "slate", // empty -> default
		"slate":       "slate",
		"Ember":       "ember",
		"tokyo night": "tokyonight",
		"tokyo-night": "tokyonight",
		"nope":        "slate", // unknown name -> default
	}
	for in, want := range cases {
		if got := resolveTheme(in).Name; got != want {
			t.Errorf("resolveTheme(%q).Name = %q, want %q", in, got, want)
		}
	}
}

func TestBuiltinBeatsThemeFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCDASH_THEME_DIR", dir)
	// A file named like a built-in must NOT shadow the built-in.
	writeTheme(t, filepath.Join(dir, "slate.json"), `{"bg":"#000000","panel":"#000000","fg":"#000000","muted":"#000000","accent":"#000000","danger":"#000000","good":"#000000"}`)
	if got := resolveTheme("slate"); got.Accent != slateTheme.Accent {
		t.Errorf("built-in slate should win over slate.json; got accent %v", got.Accent)
	}
}

func TestThemeFileByName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCDASH_THEME_DIR", dir)
	writeTheme(t, filepath.Join(dir, "gruvbox.json"), `{"name":"gruvbox","bg":"#1d2021","panel":"#3c3836","fg":"#ebdbb2","muted":"#a89984","accent":"#fabd2f","danger":"#fb4934","good":"#b8bb26"}`)
	got, err := resolveThemeErr("gruvbox")
	if err != nil {
		t.Fatalf("resolveThemeErr: %v", err)
	}
	if got.Name != "gruvbox" || got.Accent != lipgloss.Color("#fabd2f") || got.Fg != lipgloss.Color("#ebdbb2") {
		t.Errorf("loaded theme mismatch: %+v", got)
	}
}

func TestThemeFileByPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.json")
	writeTheme(t, path, `{"bg":"#101010","panel":"#202020","fg":"#f0f0f0","muted":"#909090","accent":"#5c9ce6","danger":"#e8736b","good":"#5fb98e"}`)
	got, err := resolveThemeErr(path)
	if err != nil {
		t.Fatalf("resolveThemeErr: %v", err)
	}
	// Name falls back to the file's base name when omitted.
	if got.Name != "custom" || got.Accent != lipgloss.Color("#5c9ce6") {
		t.Errorf("path-loaded theme mismatch: %+v", got)
	}
}

func TestThemeFileInvalid(t *testing.T) {
	dir := t.TempDir()

	bad := filepath.Join(dir, "bad.json")
	writeTheme(t, bad, `{"bg":"not-a-color","panel":"#202020","fg":"#f0f0f0","muted":"#909090","accent":"#5c9ce6","danger":"#e8736b","good":"#5fb98e"}`)
	got, err := resolveThemeErr(bad)
	if err == nil {
		t.Fatal("expected error for invalid hex, got nil")
	}
	if got.Name != "slate" {
		t.Errorf("invalid theme should fall back to slate, got %q", got.Name)
	}

	missing := filepath.Join(dir, "missing.json")
	writeTheme(t, missing, `{"bg":"#101010"}`)
	if _, err := resolveThemeErr(missing); err == nil {
		t.Fatal("expected error for missing slots, got nil")
	}

	malformed := filepath.Join(dir, "malformed.json")
	writeTheme(t, malformed, `{not json`)
	if _, err := resolveThemeErr(malformed); err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func writeTheme(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

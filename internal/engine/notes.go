package engine

import (
	"os"
	"path/filepath"
)

var noteFiles = []string{"STATUS.md", "HANDOFF.md", "TODO.md", "NEXT.md", "NOTES.md"}

// noteFileName returns the first project note file that exists, or "".
func noteFileName(dir string) string {
	if dir == "" {
		return ""
	}
	for _, nf := range noteFiles {
		if fi, err := os.Stat(filepath.Join(dir, nf)); err == nil && !fi.IsDir() {
			return nf
		}
	}
	return ""
}

// readNotes returns the first existing note file's contents (capped), labeled,
// for use as recap context.
func readNotes(dir string) string {
	nf := noteFileName(dir)
	if nf == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(dir, nf))
	if err != nil {
		return ""
	}
	s := string(b)
	if r := []rune(s); len(r) > 2500 {
		s = string(r[:2500]) // cap by characters, not bytes, so we never split a rune
	}
	return "--- " + nf + " ---\n" + s
}

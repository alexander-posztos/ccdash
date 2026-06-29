package engine

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGitBinAbsolute: when git is resolvable, gitBin must return an absolute path
// rather than the bare command "git". A hotkey-launched window runs under a
// non-interactive shell with a thin PATH, so a bare "git" can fail to resolve;
// resolving it up front (as ClaudeBin already does) avoids that.
func TestGitBinAbsolute(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; nothing to resolve")
	}
	got := gitBin()
	if !filepath.IsAbs(got) {
		t.Errorf("gitBin() = %q, want an absolute path", got)
	}
}

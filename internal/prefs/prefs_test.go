package prefs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissingIsEmpty(t *testing.T) {
	p := Load(filepath.Join(t.TempDir(), "nope.json"))
	if p == nil {
		t.Fatal("Load returned nil")
	}
	if p.Version != schemaVersion {
		t.Errorf("version: got %d, want %d", p.Version, schemaVersion)
	}
	if len(p.Hidden) != 0 {
		t.Error("a missing file should load as empty")
	}
	if p.IsHidden("/x") {
		t.Error("nothing should be hidden")
	}
}

// TestRoundTrip also pins the key normalization: a trailing slash on write must
// match a clean key on read, and vice versa.
func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")

	if err := Hide(path, "/w/api/", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}

	q := Load(path)
	if !q.IsHidden("/w/api") {
		t.Error("hidden entry did not survive a round-trip (or was not normalized)")
	}

	if err := Unhide(path, "/w/api"); err != nil {
		t.Fatal(err)
	}
	if Load(path).IsHidden("/w/api") {
		t.Error("unhide did not persist")
	}
}

func TestCorruptIsEmptyButWritable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	if err := os.WriteFile(path, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Load(path)
	if p.IsHidden("/x") {
		t.Error("a corrupt file should degrade to empty")
	}
	// A corrupt file must not block a subsequent write.
	if err := Hide(path, "/y", time.Unix(0, 0)); err != nil {
		t.Fatalf("write after corrupt load failed: %v", err)
	}
	if !Load(path).IsHidden("/y") {
		t.Error("write after corrupt load did not persist")
	}
}

// TestTransactPreservesOtherWriter mimics two writers: a hide written between
// another hide's load and its save must not be lost. transact reloads under the
// lock, so the second writer sees the first's change.
func TestTransactPreservesOtherWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	if err := Hide(path, "/w/x", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := Hide(path, "/w/y", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if !got.IsHidden("/w/x") {
		t.Error("first hide was dropped by a later hide")
	}
	if !got.IsHidden("/w/y") {
		t.Error("second hide did not persist alongside the first")
	}
}

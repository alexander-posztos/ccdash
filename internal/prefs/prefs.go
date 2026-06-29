// Package prefs is ccdash's writable user-preference store: the hidden-project
// set that overlays the read-only, log-derived project list. It deliberately
// lives in the OS config dir (not the recap cache), so clearing the cache never
// discards user intent. The on-disk format mirrors the engine's scan/recap
// caches - a single JSON document and atomic temp-file + rename writes - but,
// because a hide can be issued from both the TUI and the CLI at once, every
// write is a lock-guarded load-modify-save so concurrent writers cannot drop
// each other's changes.
package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// schemaVersion is stamped into every written file so a future format change can
// migrate old data instead of discarding it.
const schemaVersion = 1

// HideEntry records when a project was hidden. The timestamp is informational
// today (it powers nothing) but future-proofs niceties like "hidden 3d ago".
type HideEntry struct {
	At string `json:"at"` // RFC3339
}

// Prefs is the on-disk preference document. Keys in Hidden are the cleaned
// grouping key of a project (engine.Project.Raw); see Key. A leftover "remaps"
// key from an older format is silently ignored on load (no struct field).
type Prefs struct {
	Version int                  `json:"version"`
	Hidden  map[string]HideEntry `json:"hidden,omitempty"`
}

// Key normalizes a project grouping key so a lookup and a stored key always
// match regardless of trailing slashes etc. Callers pass Project.Raw.
func Key(raw string) string { return filepath.Clean(raw) }

// Load reads the prefs file at path. A missing or corrupt file yields an empty
// (but usable) Prefs - it is never fatal, mirroring engine.loadScanIndex. This
// is the unlocked read path used by readers (the engine, plain modes); writers
// go through Hide/Unhide, which load again under a lock.
func Load(path string) *Prefs {
	p := &Prefs{Version: schemaVersion, Hidden: map[string]HideEntry{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	_ = json.Unmarshal(b, p) // a partial/garbage file degrades to what parsed; never fatal
	if p.Hidden == nil {
		p.Hidden = map[string]HideEntry{}
	}
	p.Version = schemaVersion
	return p
}

// IsHidden reports whether raw's project is hidden.
func (p *Prefs) IsHidden(raw string) bool {
	_, ok := p.Hidden[Key(raw)]
	return ok
}

// Hide hides raw's project and persists. now stamps the hide time.
func Hide(path, raw string, now time.Time) error {
	return transact(path, func(p *Prefs) {
		p.Hidden[Key(raw)] = HideEntry{At: now.UTC().Format(time.RFC3339)}
	})
}

// Unhide removes raw's project from the hidden set and persists.
func Unhide(path, raw string) error {
	return transact(path, func(p *Prefs) { delete(p.Hidden, Key(raw)) })
}

// transact runs a single load-modify-save under an advisory file lock, so two
// writers (e.g. a `ccdash hide` racing an open TUI's `x`) serialize instead of
// clobbering each other's whole-file write. The lock is best effort: if it
// cannot be taken (unwritable dir on first run, unsupported FS) we proceed
// unlocked rather than refuse a preference change.
func transact(path string, fn func(*Prefs)) error {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	lock := flock.New(path + ".lock")
	if err := lock.Lock(); err == nil {
		defer func() { _ = lock.Unlock() }()
	}
	p := Load(path) // fresh read inside the lock: never modify a stale snapshot
	fn(p)
	return p.save(path)
}

// save atomically writes the document via a unique temp file + rename, so a
// concurrent reader (e.g. another ccdash process) never observes a half-written
// file.
func (p *Prefs) save(path string) error {
	p.Version = schemaVersion
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".prefs-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

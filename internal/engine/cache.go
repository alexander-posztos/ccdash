package engine

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/alexander-posztos/ccdash/internal/config"
)

type Recap struct {
	Signature string    `json:"signature"`
	Text      string    `json:"recap"`
	Git       *GitState `json:"git,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	Generated time.Time `json:"generated"`
}

func cachePath(cfg config.Config, p *Project) string {
	key := p.Dir
	if key == "" {
		key = p.Raw
	}
	sum := sha1.Sum([]byte(key))
	return filepath.Join(cfg.CacheDir, hex.EncodeToString(sum[:])[:16]+".json")
}

// Signature changes whenever a project's newest session changes, which is what
// makes a cached recap stale.
func Signature(p *Project) string {
	sid := "none"
	if s := latestSession(p); s != nil {
		sid = s.SID
	}
	last := "na"
	if !p.Last.IsZero() {
		last = p.Last.Format(time.RFC3339)
	}
	return sid + ":" + last
}

func latestSession(p *Project) *Session {
	if len(p.RealSessions) > 0 {
		return p.RealSessions[0]
	}
	if len(p.Sessions) > 0 {
		return p.Sessions[0]
	}
	return nil
}

func LoadRecap(cfg config.Config, p *Project) *Recap {
	b, err := os.ReadFile(cachePath(cfg, p))
	if err != nil {
		return nil
	}
	var r Recap
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

func RecapIsStale(cfg config.Config, p *Project) bool {
	r := LoadRecap(cfg, p)
	return r == nil || r.Signature != Signature(p)
}

func storeRecap(cfg config.Config, p *Project, r *Recap) error {
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	// Unique temp name so concurrent writers (two regens of one project, or the
	// second ccdash instance) never clobber a shared <hash>.tmp.
	tmp, err := os.CreateTemp(cfg.CacheDir, ".recap-*.tmp")
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
	return os.Rename(tmpName, cachePath(cfg, p))
}

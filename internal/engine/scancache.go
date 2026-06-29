package engine

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/alexander-posztos/ccdash/internal/config"
)

// scanEntry is the transcript-derived metadata for one session file, cached so
// an unchanged transcript is never re-read on the next launch. (Mtime,Size) is
// the freshness key: if either changes, the file is re-parsed.
type scanEntry struct {
	Mtime  int64  `json:"mtime"` // ModTime, unix nanoseconds
	Size   int64  `json:"size"`
	Title  string `json:"title"`
	Branch string `json:"branch"`
	CWD    string `json:"cwd"` // CWDVerified
}

// scanIndex maps a transcript file path to its cached metadata.
type scanIndex map[string]scanEntry

func scanIndexPath(cfg config.Config) string {
	return filepath.Join(cfg.CacheDir, "scan-index.json")
}

// loadScanIndex reads the persisted transcript-metadata cache. A missing or
// corrupt file yields an empty index, so every transcript is then a cache miss.
func loadScanIndex(cfg config.Config) scanIndex {
	idx := scanIndex{}
	b, err := os.ReadFile(scanIndexPath(cfg))
	if err != nil {
		return idx
	}
	_ = json.Unmarshal(b, &idx) // a partial/garbage cache just re-parses; never fatal
	return idx
}

// storeScanIndex atomically writes the index via a unique temp file + rename, so
// a concurrent ccdash (e.g. a second instance) never sees a half-written cache.
// Best effort: a write failure must not break startup.
func storeScanIndex(cfg config.Config, idx scanIndex) {
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return
	}
	b, err := json.Marshal(idx)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(cfg.CacheDir, ".scan-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	_ = os.Rename(tmpName, scanIndexPath(cfg))
}

package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ClaudeDir    string
	HistoryFile  string
	ProjectsDir  string
	SettingsFile string
	CacheDir     string
	PrefsFile    string // CCDASH_PREFS_FILE; writable hidden-set prefs (config dir, survives a cache wipe)
	ActiveDays   int
	Model        string
	Theme        string // CCDASH_THEME: palette name ("slate" default; also "ember", "tokyonight") or a JSON theme file
	ClaudeBin    string // explicit override (CCDASH_CLAUDE_BIN); "" -> auto-resolve
	RecapTimeout time.Duration
	Offline      bool // CCDASH_OFFLINE / --offline: derive recaps locally, never exec claude
}

func Load() Config {
	home, _ := os.UserHomeDir()
	claudeDir := env("CCDASH_CLAUDE_DIR", filepath.Join(home, ".claude"))
	return Config{
		ClaudeDir:    claudeDir,
		HistoryFile:  filepath.Join(claudeDir, "history.jsonl"),
		ProjectsDir:  filepath.Join(claudeDir, "projects"),
		SettingsFile: filepath.Join(claudeDir, "settings.json"),
		CacheDir:     env("CCDASH_CACHE_DIR", defaultCacheDir(home)),
		PrefsFile:    env("CCDASH_PREFS_FILE", defaultPrefsFile(home)),
		ActiveDays:   envInt("CCDASH_ACTIVE_DAYS", 30),
		Model:        env("CCDASH_MODEL", "haiku"),
		Theme:        env("CCDASH_THEME", "slate"),
		ClaudeBin:    os.Getenv("CCDASH_CLAUDE_BIN"),
		RecapTimeout: time.Duration(envInt("CCDASH_RECAP_TIMEOUT", 90)) * time.Second,
		Offline:      envBool("CCDASH_OFFLINE", false),
	}
}

func defaultCacheDir(home string) string {
	if uc, err := os.UserCacheDir(); err == nil {
		return filepath.Join(uc, "ccdash")
	}
	return filepath.Join(home, ".cache", "ccdash")
}

// defaultPrefsFile resolves the writable prefs file under the OS config dir
func defaultPrefsFile(home string) string {
	if uc, err := os.UserConfigDir(); err == nil {
		return filepath.Join(uc, "ccdash", "prefs.json")
	}
	return filepath.Join(home, ".config", "ccdash", "prefs.json")
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

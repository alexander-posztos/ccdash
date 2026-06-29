package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexander-posztos/ccdash/internal/config"
	"github.com/alexander-posztos/ccdash/internal/prefs"
)

// BuildSessions reads history.jsonl for the session index, then enriches each
// session with title/branch/cwd from its transcript file.
func BuildSessions(cfg config.Config) []*Session {
	sessions := map[string]*Session{}

	eachLine(cfg.HistoryFile, func(b []byte) {
		var d historyLine
		if json.Unmarshal(b, &d) != nil || d.SessionID == "" {
			return
		}
		s := sessions[d.SessionID]
		if s == nil {
			s = &Session{SID: d.SessionID, CWD: d.Project, FirstPrompt: strings.TrimSpace(d.Display)}
			sessions[d.SessionID] = s
		}
		s.N++
		if dt := d.Timestamp.t; !dt.IsZero() {
			if s.First.IsZero() || dt.Before(s.First) {
				s.First = dt
			}
			if s.Last.IsZero() || dt.After(s.Last) {
				s.Last = dt
			}
		}
		disp := strings.TrimSpace(d.Display)
		if disp != "" && !strings.HasPrefix(disp, "/") &&
			(s.FirstPrompt == "" || strings.HasPrefix(s.FirstPrompt, "/")) {
			s.FirstPrompt = disp
		}
		if d.Project != "" {
			s.CWD = d.Project
		}
	})

	// Enrich each session with title/branch/cwd from its transcript. Transcripts
	// are large and mostly unchanged between launches, so results are cached by
	// (mtime,size): an unchanged file is never re-read. See scancache.go.
	matches, _ := filepath.Glob(filepath.Join(cfg.ProjectsDir, "*", "*.jsonl"))
	idx := loadScanIndex(cfg)
	next := make(scanIndex, len(matches))
	changed := false
	for _, sf := range matches {
		sid := strings.TrimSuffix(filepath.Base(sf), ".jsonl")
		s := sessions[sid]
		if s == nil {
			continue
		}
		s.File = sf

		fi, err := os.Stat(sf)
		if err == nil {
			if e, ok := idx[sf]; ok && e.Mtime == fi.ModTime().UnixNano() && e.Size == fi.Size() {
				s.Title, s.Branch, s.CWDVerified = e.Title, e.Branch, e.CWD
				next[sf] = e
				continue
			}
		}

		scanTranscript(sf, s)

		if err == nil {
			next[sf] = scanEntry{
				Mtime:  fi.ModTime().UnixNano(),
				Size:   fi.Size(),
				Title:  s.Title,
				Branch: s.Branch,
				CWD:    s.CWDVerified,
			}
			changed = true
		}
	}
	if changed || len(next) != len(idx) {
		storeScanIndex(cfg, next)
	}

	out := make([]*Session, 0, len(sessions))
	for _, s := range sessions {
		if !s.Last.IsZero() {
			out = append(out, s)
		}
	}
	return out
}

// scanTranscript reads a session transcript and fills the fields that are not in
// history.jsonl (Title, Branch, CWDVerified). It is the cold path behind the
// scan cache: the parse logic is unchanged from the original inline loop, so a
// cache miss yields exactly the same result as before.
func scanTranscript(path string, s *Session) {
	eachLine(path, func(b []byte) {
		if !bytes.Contains(b, []byte(`"ai-title"`)) &&
			!bytes.Contains(b, []byte(`"cwd"`)) &&
			!bytes.Contains(b, []byte(`"gitBranch"`)) {
			return
		}
		var d sessionMeta
		if json.Unmarshal(b, &d) != nil {
			return
		}
		if d.Type == "ai-title" && d.AITitle != "" {
			s.Title = d.AITitle
		}
		if s.Branch == "" && d.GitBranch != "" {
			s.Branch = d.GitBranch
		}
		if d.CWD != "" {
			s.CWDVerified = d.CWD
		}
	})
}

// ResolveDir returns the session's working dir if it still exists, else "". A
// project whose directory is gone stays "missing" until the directory returns;
// ccdash does not hunt for a renamed or relocated copy.
func ResolveDir(cwd string) string {
	if cwd != "" && isDir(cwd) {
		return cwd
	}
	return ""
}

func GroupProjects(cfg config.Config) []*Project {
	sessions := BuildSessions(cfg)
	pf := prefs.Load(cfg.PrefsFile)

	projects := map[string]*Project{}
	for _, s := range sessions {
		cwd := firstNonEmpty(s.CWDVerified, s.CWD)
		rd := ResolveDir(cwd)
		key := rd
		if key == "" {
			if key = cwd; key == "" {
				key = "?"
			}
		}
		p := projects[key]
		if p == nil {
			p = &Project{Dir: rd, Raw: key, Name: baseName(key), Missing: rd == "", Hidden: pf.IsHidden(key)}
			projects[key] = p
		}
		p.Sessions = append(p.Sessions, s)
		if p.Last.IsZero() || s.Last.After(p.Last) {
			p.Last = s.Last
		}
	}

	out := make([]*Project, 0, len(projects))
	for _, p := range projects {
		sort.SliceStable(p.Sessions, func(i, j int) bool { return p.Sessions[i].Last.After(p.Sessions[j].Last) })
		for _, s := range p.Sessions {
			if IsRealSession(s) {
				p.RealSessions = append(p.RealSessions, s)
			}
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Last.After(out[j].Last) })
	return out
}

// IsRealSession filters out throwaway sessions (slash-command-only or trivial)
// unless they ran more than a couple of turns.
func IsRealSession(s *Session) bool {
	if strings.TrimSpace(s.Title) != "" {
		return true
	}
	fp := strings.TrimSpace(s.FirstPrompt)
	if fp == "" || strings.HasPrefix(fp, "/") || fp == ":wq" || fp == ":Wq" || fp == ":q" {
		return s.N > 2
	}
	return true
}

func IsActive(cfg config.Config, p *Project) bool {
	if p.Last.IsZero() {
		return false
	}
	return int(time.Since(p.Last).Hours()/24) <= cfg.ActiveDays
}

func baseName(key string) string {
	return filepath.Base(strings.TrimRight(key, "/"))
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

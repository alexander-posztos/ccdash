package engine

import "time"

type Session struct {
	SID         string
	CWD         string // working dir from history.jsonl
	CWDVerified string // cwd recorded inside the session transcript
	First       time.Time
	Last        time.Time
	N           int
	FirstPrompt string
	Title       string
	Branch      string
	File        string // path to the session transcript jsonl
}

type Project struct {
	Dir          string // resolved absolute dir, "" if it could not be found
	Raw          string // grouping key
	Name         string
	Last         time.Time
	Missing      bool
	Hidden       bool // user hid this project via prefs; filtered out of the default views
	Sessions     []*Session
	RealSessions []*Session
}

func (s *Session) DisplayTitle() string {
	if s.Title != "" {
		return s.Title
	}
	if s.FirstPrompt != "" {
		return s.FirstPrompt
	}
	return "(untitled)"
}

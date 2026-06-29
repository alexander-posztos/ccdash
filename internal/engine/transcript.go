package engine

import (
	"bufio"
	"bytes"
	"os"
)

// eachLine streams a jsonl file, calling fn with each non-empty trimmed line.
// Missing files and read errors are silently skipped; ReadBytes handles
// arbitrarily long lines so a giant pasted message will not truncate.
func eachLine(path string, fn func([]byte)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if t := bytes.TrimSpace(line); len(t) > 0 {
			fn(t)
		}
		if err != nil {
			return
		}
	}
}

type historyLine struct {
	SessionID string  `json:"sessionId"`
	Timestamp rawTime `json:"timestamp"`
	Project   string  `json:"project"`
	Display   string  `json:"display"`
}

type sessionMeta struct {
	Type      string `json:"type"`
	AITitle   string `json:"aiTitle"`
	CWD       string `json:"cwd"`
	GitBranch string `json:"gitBranch"`
}

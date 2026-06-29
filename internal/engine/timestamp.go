package engine

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

var tsLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, l := range tsLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// rawTime accepts either an ISO string or epoch-milliseconds number, and
// tolerates anything else by staying zero.
type rawTime struct{ t time.Time }

func (r *rawTime) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if json.Unmarshal(b, &s) == nil {
			if t, ok := parseTime(s); ok {
				r.t = t
			}
		}
		return nil
	}
	var f float64
	if json.Unmarshal(b, &f) == nil && f > 0 {
		r.t = time.UnixMilli(int64(f))
	}
	return nil
}

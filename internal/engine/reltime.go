package engine

import (
	"fmt"
	"time"
)

// RelTime renders a compact relative-time label ("3h ago", "2d ago", or an
// absolute date once it is old enough).
func RelTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	secs := time.Since(t).Seconds()
	if secs < 0 {
		secs = 0
	}
	switch {
	case secs < 90:
		return fmt.Sprintf("%ds ago", int(secs))
	case secs < 5400:
		return fmt.Sprintf("%dm ago", int(secs/60))
	case secs < 36*3600:
		return fmt.Sprintf("%dh ago", int(secs/3600))
	}
	d := int(secs / 86400)
	switch {
	case d < 14:
		return fmt.Sprintf("%dd ago", d)
	case d < 60:
		return fmt.Sprintf("%dw ago", d/7)
	}
	return t.Format("2006-01-02")
}

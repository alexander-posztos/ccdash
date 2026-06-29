package engine

import (
	"bytes"
	"encoding/json"
	"strings"
)

// sessionTail returns the last maxMsgs human-readable user/assistant turns of a
// transcript, capped at maxChars, as recap context.
func sessionTail(path string, maxMsgs, maxChars int) string {
	type turn struct{ role, text string }
	var turns []turn
	eachLine(path, func(b []byte) {
		if !bytes.Contains(b, []byte(`"user"`)) && !bytes.Contains(b, []byte(`"assistant"`)) {
			return
		}
		var d struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(b, &d) != nil || (d.Type != "user" && d.Type != "assistant") {
			return
		}
		text := strings.TrimSpace(extractText(d.Message.Content))
		if text == "" || strings.HasPrefix(text, "<") || len(text) < 2 {
			return
		}
		turns = append(turns, turn{d.Type, text})
	})

	if len(turns) > maxMsgs {
		turns = turns[len(turns)-maxMsgs:]
	}
	var sb strings.Builder
	for i, t := range turns {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("[" + t.role + "] " + t.text)
	}
	out := sb.String()
	if r := []rune(out); len(r) > maxChars {
		out = string(r[len(r)-maxChars:]) // cap by characters, not bytes (matches Python)
	}
	return out
}

func extractText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) == nil {
		var sb strings.Builder
		for _, bl := range blocks {
			if bl.Type == "text" {
				sb.WriteString(bl.Text)
			}
		}
		return sb.String()
	}
	return ""
}

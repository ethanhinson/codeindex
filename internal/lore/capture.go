package lore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// capturePayload is the best-effort shape of a host Stop-hook payload.
type capturePayload struct {
	SessionID            string `json:"session_id"`
	Cwd                  string `json:"cwd"`
	LastAssistantMessage string `json:"last_assistant_message"`
	Summary              string `json:"summary"`
}

// CaptureSession appends a metadata-only session note to the overlay's
// sessions layer. Fail-open by design: unusable input returns ("", nil).
// No LLM calls — this is the cheap ambient channel; curation happens later
// via promote.
func CaptureSession(l Layout, raw []byte, now time.Time) (string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", nil
	}
	var p capturePayload
	body := ""
	if err := json.Unmarshal(raw, &p); err != nil {
		body = text // freeform: keep the raw text as the observation
	} else {
		msg := p.LastAssistantMessage
		if msg == "" {
			msg = p.Summary
		}
		if msg == "" && p.Cwd == "" {
			return "", nil // JSON with nothing usable
		}
		if len(msg) > 500 {
			msg = msg[:500]
		}
		if p.Cwd != "" {
			body = "cwd: " + p.Cwd + "\n"
		}
		if msg != "" {
			body += "\n## Last activity\n" + msg + "\n"
		}
	}
	sid := p.SessionID
	if sid == "" {
		sid = "unknown"
	}
	if len(sid) > 8 {
		sid = sid[:8]
	}
	dir := l.SessionsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	date := now.Format("2006-01-02")
	path := filepath.Join(dir, date+"-"+sid+".md")
	if _, err := os.Stat(path); err == nil {
		entry := fmt.Sprintf("\n---\n## %s UTC\n%s\n", now.Format("15:04"), body)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := f.WriteString(entry); err != nil {
			return "", err
		}
		return path, nil
	}
	rec := Record{
		ID: NewID(TypeNote), Type: TypeNote,
		Title: "Session " + sid + " " + date, Date: date,
		Body: body + "\n",
	}
	b, err := rec.Marshal()
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, b, 0o644)
}

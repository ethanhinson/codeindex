package lore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func captureLayout(t *testing.T) Layout {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	l, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestCaptureSessionWritesNote(t *testing.T) {
	l := captureLayout(t)
	now := time.Date(2026, 7, 29, 15, 4, 0, 0, time.UTC)
	raw := []byte(`{"session_id":"abcd1234efgh","cwd":"/w","last_assistant_message":"Fixed the resolver bug."}`)
	path, err := CaptureSession(l, raw, now)
	if err != nil || path == "" {
		t.Fatalf("capture: %q %v", path, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "id: note-") || !strings.Contains(s, "Session abcd1234") ||
		!strings.Contains(s, "Fixed the resolver bug.") {
		t.Fatalf("note content:\n%s", s)
	}
	if filepath.Base(path) != "2026-07-29-abcd1234.md" {
		t.Fatalf("filename %q", filepath.Base(path))
	}
	// Round-trip: the written file must parse as a valid note.
	if _, err := Parse(b, TypeNote); err != nil {
		t.Fatalf("captured note does not parse: %v", err)
	}
}

func TestCaptureSessionAppendsSameDay(t *testing.T) {
	l := captureLayout(t)
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)
	raw := []byte(`{"session_id":"abcd1234","summary":"first"}`)
	p1, _ := CaptureSession(l, raw, now)
	raw2 := []byte(`{"session_id":"abcd1234","summary":"second"}`)
	p2, err := CaptureSession(l, raw2, now.Add(time.Hour))
	if err != nil || p1 != p2 {
		t.Fatalf("append: %q vs %q, %v", p1, p2, err)
	}
	b, _ := os.ReadFile(p2)
	if !strings.Contains(string(b), "first") || !strings.Contains(string(b), "second") ||
		strings.Count(string(b), "id: note-") != 1 {
		t.Fatalf("appended file:\n%s", b)
	}
}

func TestCaptureSessionFreeformAndEmpty(t *testing.T) {
	l := captureLayout(t)
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)
	if p, err := CaptureSession(l, []byte("plain text observation"), now); err != nil || p == "" {
		t.Fatalf("freeform: %q %v", p, err)
	}
	if p, err := CaptureSession(l, []byte("   \n"), now); err != nil || p != "" {
		t.Fatalf("empty input must skip silently: %q %v", p, err)
	}
}

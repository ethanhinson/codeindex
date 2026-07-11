package progress

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONLEventsWellFormedAndVersioned(t *testing.T) {
	var buf bytes.Buffer
	j := NewJSONL(&buf)
	j.Report(Event{Phase: "parse", Done: 1, Total: 10})
	j.Report(Event{Phase: "parse", Done: 10, Total: 10}) // final: always emitted
	j.Report(Event{Phase: "resolve", Done: 1, Total: 5}) // phase change: always emitted
	j.Finish("indexed 10 files")

	sc := bufio.NewScanner(&buf)
	var lines []map[string]any
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("non-JSON line %q: %v", sc.Text(), err)
		}
		if m["v"] != float64(1) {
			t.Fatalf("event missing v:1: %v", m)
		}
		lines = append(lines, m)
	}
	if len(lines) != 4 {
		t.Fatalf("want 4 events (2 parse + 1 resolve + done), got %d", len(lines))
	}
	last := lines[len(lines)-1]
	if last["phase"] != "done" || last["summary"] != "indexed 10 files" {
		t.Fatalf("missing terminal done event: %v", last)
	}
}

func TestSidecarLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")
	sc := NewSidecar(path, "building")

	read := func() map[string]any {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if st := read()["state"]; st != "building" {
		t.Fatalf("initial state = %v", st)
	}
	sc.Report(Event{Phase: "parse", Done: 3, Total: 10})
	m := read()
	if m["phase"] != "parse" || m["done"] != float64(3) || m["total"] != float64(10) {
		t.Fatalf("progress not recorded: %v", m)
	}
	sc.FinishCounts(10, 42)
	m = read()
	if m["state"] != "fresh" || m["files"] != float64(10) || m["symbols"] != float64(42) {
		t.Fatalf("terminal state wrong: %v", m)
	}
	if m["indexed_at"] == nil || m["duration_ms"] == nil {
		t.Fatalf("terminal state missing timestamps: %v", m)
	}
}

func TestTTYRendersBarAndSummary(t *testing.T) {
	var buf bytes.Buffer
	r := NewTTY(&buf, "index demo")
	r.Report(Event{Phase: "parse", Done: 5, Total: 10})
	r.Finish("indexed 10 files (42 symbols)")
	out := buf.String()
	if !strings.Contains(out, "█") || !strings.Contains(out, "░") {
		t.Fatalf("no progress bar in output: %q", out)
	}
	if !strings.Contains(out, "5/10 (50%)") {
		t.Fatalf("no counts in output: %q", out)
	}
	if !strings.Contains(out, "✓ indexed 10 files (42 symbols) in ") {
		t.Fatalf("no final summary: %q", out)
	}
}

func TestMultiToleratesNil(t *testing.T) {
	var buf bytes.Buffer
	m := Multi(nil, NewJSONL(&buf))
	m.Report(Event{Phase: "parse", Done: 1, Total: 1})
	m.Finish("ok")
	if buf.Len() == 0 {
		t.Fatal("wrapped reporter received nothing")
	}
}

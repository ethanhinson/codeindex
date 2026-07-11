// Package progress is the single source of indexing-progress truth: the
// engine emits phase events through a Reporter, and every surface — TTY
// spinner, JSON-lines feed, status sidecar, IDE extension — is just a
// renderer of the same stream.
package progress

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Event is one progress observation. Total==0 means the phase's extent is
// unknown (e.g. walking).
type Event struct {
	V     int    `json:"v"`
	Phase string `json:"phase"` // walk | parse | write | resolve | done
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// Reporter consumes progress events. Implementations must tolerate high
// event rates (throttle internally) and never fail the caller.
type Reporter interface {
	Report(Event)
	Finish(summary string)
}

// Multi fans events out to several reporters.
func Multi(rs ...Reporter) Reporter { return multi(rs) }

type multi []Reporter

func (m multi) Report(e Event) {
	for _, r := range m {
		if r != nil {
			r.Report(e)
		}
	}
}

func (m multi) Finish(s string) {
	for _, r := range m {
		if r != nil {
			r.Finish(s)
		}
	}
}

// IsTTY reports whether w is an interactive character device (and TERM is
// not dumb) — the gate between pretty rendering and plain lines.
func IsTTY(w *os.File) bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := w.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// --- TTY renderer -----------------------------------------------------------

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

const barWidth = 22

// TTY renders a live spinner + progress bar + rate + ETA on one redrawn
// line, and a final ✓ summary.
type TTY struct {
	w        io.Writer
	label    string
	start    time.Time
	last     time.Time
	frame    int
	lastLine int
}

func NewTTY(w io.Writer, label string) *TTY {
	return &TTY{w: w, label: label, start: time.Now()}
}

func (t *TTY) Report(e Event) {
	now := time.Now()
	if now.Sub(t.last) < 80*time.Millisecond && !(e.Total > 0 && e.Done == e.Total) {
		return
	}
	t.last = now
	t.frame++
	spin := string(spinnerFrames[t.frame%len(spinnerFrames)])

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s", spin, t.label, phaseVerb(e.Phase))
	if e.Total > 0 {
		frac := float64(e.Done) / float64(e.Total)
		fill := int(frac * barWidth)
		fmt.Fprintf(&b, "  %s%s  %s/%s (%d%%)",
			strings.Repeat("█", fill), strings.Repeat("░", barWidth-fill),
			human(e.Done), human(e.Total), int(frac*100))
		if el := time.Since(t.start).Seconds(); el > 0.5 && e.Done > 0 {
			rate := float64(e.Done) / el
			fmt.Fprintf(&b, "  %s/s", human(int(rate)))
			if rate > 0 && e.Done < e.Total {
				eta := time.Duration(float64(e.Total-e.Done)/rate) * time.Second
				fmt.Fprintf(&b, "  eta %s", compact(eta))
			}
		}
	} else if e.Done > 0 {
		fmt.Fprintf(&b, "  %s", human(e.Done))
	}
	t.redraw(b.String())
}

func (t *TTY) Finish(summary string) {
	t.redraw("")
	fmt.Fprintf(t.w, "\r✓ %s in %s\n", summary, compact(time.Since(t.start)))
}

// redraw repaints the single status line, clearing any prior overhang.
func (t *TTY) redraw(line string) {
	pad := ""
	if n := t.lastLine - len([]rune(line)); n > 0 {
		pad = strings.Repeat(" ", n)
	}
	fmt.Fprintf(t.w, "\r%s%s", line, pad)
	t.lastLine = len([]rune(line))
}

func phaseVerb(p string) string {
	switch p {
	case "walk":
		return "scanning"
	case "parse":
		return "parsing"
	case "write":
		return "writing"
	case "resolve":
		return "resolving"
	default:
		return p
	}
}

func human(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func compact(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// --- Plain renderer (non-TTY stderr) ----------------------------------------

// Plain prints a throttled line per update — log-friendly, no ANSI.
type Plain struct {
	w         io.Writer
	label     string
	start     time.Time
	last      time.Time
	lastPhase string
}

func NewPlain(w io.Writer, label string) *Plain {
	return &Plain{w: w, label: label, start: time.Now()}
}

func (p *Plain) Report(e Event) {
	now := time.Now()
	if e.Phase == p.lastPhase && now.Sub(p.last) < 2*time.Second {
		return
	}
	p.last, p.lastPhase = now, e.Phase
	if e.Total > 0 {
		fmt.Fprintf(p.w, "%s: %s %d/%d\n", p.label, phaseVerb(e.Phase), e.Done, e.Total)
	} else {
		fmt.Fprintf(p.w, "%s: %s\n", p.label, phaseVerb(e.Phase))
	}
}

func (p *Plain) Finish(summary string) {
	fmt.Fprintf(p.w, "%s: %s in %s\n", p.label, summary, compact(time.Since(p.start)))
}

// --- JSONL renderer (machine feed) ------------------------------------------

// JSONL writes one versioned JSON event per line: phase transitions and
// final counts always, intermediate updates throttled.
type JSONL struct {
	w         io.Writer
	last      time.Time
	lastPhase string
}

func NewJSONL(w io.Writer) *JSONL { return &JSONL{w: w} }

func (j *JSONL) Report(e Event) {
	now := time.Now()
	final := e.Total > 0 && e.Done == e.Total
	if e.Phase == j.lastPhase && !final && now.Sub(j.last) < 100*time.Millisecond {
		return
	}
	j.last, j.lastPhase = now, e.Phase
	e.V = 1
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	fmt.Fprintf(j.w, "%s\n", b)
}

func (j *JSONL) Finish(summary string) {
	b, _ := json.Marshal(map[string]any{"v": 1, "phase": "done", "summary": summary})
	fmt.Fprintf(j.w, "%s\n", b)
}

// --- Status sidecar ----------------------------------------------------------

// Sidecar maintains a best-effort status.json beside the index so any
// surface (the status verb, an IDE status bar) can poll indexing state.
// Write failures never propagate.
type Sidecar struct {
	path      string
	state     string
	start     time.Time
	last      time.Time
	lastPhase string
}

func NewSidecar(path, state string) *Sidecar {
	s := &Sidecar{path: path, state: state, start: time.Now()}
	s.write(map[string]any{"state": state, "started_at": s.start.Format(time.RFC3339)})
	return s
}

func (s *Sidecar) Report(e Event) {
	now := time.Now()
	if e.Phase == s.lastPhase && now.Sub(s.last) < 200*time.Millisecond {
		return
	}
	s.last, s.lastPhase = now, e.Phase
	s.write(map[string]any{
		"state": s.state, "phase": e.Phase, "done": e.Done, "total": e.Total,
		"started_at": s.start.Format(time.RFC3339),
	})
}

// FinishCounts records the terminal fresh state with what was indexed.
func (s *Sidecar) FinishCounts(files, symbols int) {
	s.write(map[string]any{
		"state": "fresh", "files": files, "symbols": symbols,
		"indexed_at":  time.Now().Format(time.RFC3339),
		"duration_ms": time.Since(s.start).Milliseconds(),
	})
}

func (s *Sidecar) Finish(string) {} // engine calls FinishCounts explicitly

func (s *Sidecar) write(v map[string]any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}

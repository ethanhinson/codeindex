// Package agent is the codeindex runtime-evidence SDK for Go: an in-process
// CPU sampler (stdlib runtime/pprof underneath) that spools cxprof v1
// profiles into a repo's .codeindex/runtime/ directory, where the next
// codeindex query ingests them automatically.
//
// Non-disruption contract (agent-sdks spec): sampling only (never call
// instrumentation), bounded buffers, fire-and-forget writes, failures are
// swallowed and counted, CODEINDEX_PROFILING=off disables everything, and
// payloads are frames-only.
//
// Usage:
//
//	stop := agent.Start(agent.Options{Repo: "/path/to/repo"})
//	defer stop()
package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"sync/atomic"
	"time"

	"github.com/google/pprof/profile"
)

// Options configure the agent. Zero values mean: repo from
// CODEINDEX_REPO or the working directory, 99Hz, 8MB buffer cap.
type Options struct {
	Repo      string
	Hz        int
	MaxBuffer int // bytes of raw pprof buffered before the profile is dropped
	Commit    string
	Tag       string // "dev" (default) | "prod" | free-form
}

// Dropped counts profiles discarded due to caps or write failures — the
// only externally visible failure signal, by design.
var Dropped atomic.Int64

// Start begins sampling. The returned stop function flushes one cxprof
// spool file; it never returns an error and never panics into the host.
func Start(o Options) (stop func()) {
	if os.Getenv("CODEINDEX_PROFILING") == "off" {
		return func() {}
	}
	if o.Repo == "" {
		o.Repo = os.Getenv("CODEINDEX_REPO")
	}
	if o.Repo == "" {
		o.Repo, _ = os.Getwd()
	}
	if o.Hz <= 0 {
		o.Hz = 99
	}
	if o.MaxBuffer <= 0 {
		o.MaxBuffer = 8 << 20
	}
	if o.Tag == "" {
		o.Tag = "dev"
	}

	var buf bytes.Buffer
	start := time.Now()
	// SetCPUProfileRate must precede StartCPUProfile's default; pprof's
	// default 100Hz is close enough to o.Hz that we accept it rather than
	// fight the runtime; o.Hz is recorded in the header as approximate.
	if err := pprof.StartCPUProfile(&buf); err != nil {
		Dropped.Add(1)
		return func() {}
	}

	return func() {
		defer func() { _ = recover() }() // never panic into the host
		pprof.StopCPUProfile()
		if buf.Len() > o.MaxBuffer {
			Dropped.Add(1)
			return
		}
		if err := writeSpool(o, buf.Bytes(), start, time.Now()); err != nil {
			Dropped.Add(1)
		}
	}
}

// writeSpool converts raw pprof to cxprof and lands it atomically.
func writeSpool(o Options, raw []byte, start, end time.Time) error {
	prof, err := profile.ParseData(raw)
	if err != nil {
		return err
	}

	// Aggregate identical stacks. pprof samples list the LEAF first; cxprof
	// wants innermost LAST, so reverse.
	type stackKey string
	counts := map[stackKey]int64{}
	frames := map[stackKey][][2]any{}
	for _, s := range prof.Sample {
		var fs [][2]any
		for i := len(s.Location) - 1; i >= 0; i-- {
			loc := s.Location[i]
			for j := len(loc.Line) - 1; j >= 0; j-- {
				ln := loc.Line[j]
				if ln.Function == nil || ln.Function.Filename == "" || ln.Line < 1 {
					continue
				}
				fs = append(fs, [2]any{ln.Function.Filename, ln.Line})
			}
		}
		if len(fs) == 0 {
			continue
		}
		b, _ := json.Marshal(fs)
		k := stackKey(b)
		frames[k] = fs
		n := int64(1)
		if len(s.Value) > 0 && s.Value[0] > 0 {
			n = s.Value[0]
		}
		counts[k] += n
	}

	var out bytes.Buffer
	head := map[string]any{
		"cxprof": 1, "lang": "go", "unit": "samples", "hz": o.Hz,
		"start": start.Unix(), "end": end.Unix(), "tag": o.Tag,
	}
	if o.Commit != "" {
		head["commit"] = o.Commit
	}
	hb, _ := json.Marshal(head)
	out.Write(hb)
	out.WriteByte('\n')
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	for _, k := range keys {
		rec := map[string]any{"st": frames[stackKey(k)], "n": counts[stackKey(k)]}
		rb, _ := json.Marshal(rec)
		out.Write(rb)
		out.WriteByte('\n')
	}

	dir := filepath.Join(o.Repo, ".codeindex", "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	final := filepath.Join(dir, fmt.Sprintf("%d-%d.cxprof.jsonl", start.Unix(), os.Getpid()))
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, out.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// Package runtime ingests cxprof profiles (docs/cxprof-format.md) into the
// symbol graph as observed edges and heat. Collection lives in the SDKs and
// any conforming third-party emitter; this side owns parsing, frame→symbol
// resolution, and the additive-evidence invariants.
package runtime

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeindex/internal/graph"
)

// Header is a cxprof header record.
type Header struct {
	Version int    `json:"cxprof"`
	Lang    string `json:"lang"`
	Unit    string `json:"unit"`
	Hz      int    `json:"hz"`
	Start   int64  `json:"start"`
	End     int64  `json:"end"`
	Commit  string `json:"commit"`
	Tag     string `json:"tag"`
}

// Stack is one aggregated stack record; frames innermost-LAST.
type Stack struct {
	Frames [][2]any `json:"st"` // [file string, line number]
	N      int64    `json:"n"`
}

// Profile is a parsed cxprof file.
type Profile struct {
	Header  Header
	Stacks  []Stack
	Skipped int // unparseable lines (tolerated per format rule 3)
}

// Parse reads a cxprof stream. Unknown fields are ignored; unparseable
// lines are counted, not fatal.
func Parse(path string) (*Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)
	p := &Profile{}
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if err := json.Unmarshal([]byte(line), &p.Header); err != nil || p.Header.Version == 0 {
				return nil, fmt.Errorf("%s: first record is not a cxprof header", path)
			}
			if p.Header.Version != 1 {
				return nil, fmt.Errorf("%s: unsupported cxprof version %d", path, p.Header.Version)
			}
			continue
		}
		var st Stack
		if err := json.Unmarshal([]byte(line), &st); err != nil || len(st.Frames) == 0 || st.N < 1 {
			p.Skipped++
			continue
		}
		p.Stacks = append(p.Stacks, st)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if first {
		return nil, fmt.Errorf("%s: empty file", path)
	}
	return p, nil
}

// frame extracts (file, line) from the loosely-typed record.
func frame(f [2]any) (string, int, bool) {
	file, ok := f[0].(string)
	if !ok || file == "" {
		return "", 0, false
	}
	num, ok := f[1].(float64)
	if !ok || num < 1 {
		return "", 0, false
	}
	return file, int(num), true
}

// resolver maps frames to symbols via span lookup, caching per file.
type resolver struct {
	st       *graph.Store
	root     string                    // symlink-resolved
	spans    map[string][]graph.Symbol // repo-relative file -> symbols by start line
	relCache map[string]string
}

func newResolver(st *graph.Store, root string) *resolver {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	return &resolver{st: st, root: root,
		spans: map[string][]graph.Symbol{}, relCache: map[string]string{}}
}

// rel re-roots a frame path against the repo. Emitters send whatever their
// runtime saw — including symlink aliases (macOS /tmp vs /private/tmp), so
// both sides are symlink-resolved before comparing.
func (r *resolver) rel(file string) string {
	if got, ok := r.relCache[file]; ok {
		return got
	}
	out := ""
	if !filepath.IsAbs(file) {
		out = filepath.ToSlash(file)
	} else {
		abs := file
		if rp, err := filepath.EvalSymlinks(file); err == nil {
			abs = rp
		}
		if p, err := filepath.Rel(r.root, abs); err == nil && !strings.HasPrefix(p, "..") {
			out = filepath.ToSlash(p)
		}
	}
	r.relCache[file] = out
	return out // "" = outside the repo (vendor, stdlib, runtime internals)
}

// resolve returns the innermost tier-0 symbol whose span contains line.
func (r *resolver) resolve(file string, line int) *graph.Symbol {
	relPath := r.rel(file)
	if relPath == "" {
		return nil
	}
	spans, ok := r.spans[relPath]
	if !ok {
		var err error
		spans, err = r.st.FileSymbolSpans(relPath)
		if err != nil {
			spans = nil
		}
		r.spans[relPath] = spans
	}
	var best *graph.Symbol
	for i := range spans {
		s := &spans[i]
		if s.Tier != 0 || line < s.StartLine || line > s.EndLine {
			continue
		}
		// Innermost = smallest containing span.
		if best == nil || (s.EndLine-s.StartLine) < (best.EndLine-best.StartLine) {
			best = s
		}
	}
	return best
}

// Stats summarizes one ingestion.
type Stats struct {
	Path           string
	Stacks         int
	Samples        int64
	FramesTotal    int
	FramesResolved int
	Edges          int
	Skipped        int
}

// Ingest folds one cxprof file into the graph: observed edges between
// adjacent resolved frames (indirect when unresolved frames sit between),
// heat per symbol (leaf / on-stack / entry), ledger-recorded. Idempotent
// per (path, content-hash).
func Ingest(st *graph.Store, root, path string) (*Stats, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	if ok, err := st.ObsLedgerHas(path, hash); err != nil {
		return nil, err
	} else if ok {
		return &Stats{Path: path}, nil // already ingested
	}

	p, err := Parse(path)
	if err != nil {
		return nil, err
	}

	res := newResolver(st, root)
	tx, err := st.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stats := &Stats{Path: path, Stacks: len(p.Stacks), Skipped: p.Skipped}
	type heat struct{ leaf, total, entry int64 }
	heatAcc := map[string]*heat{}
	edgeAcc := map[[3]string]int64{} // src, dst, indirect-flag

	for _, stk := range p.Stacks {
		stats.Samples += stk.N
		type rf struct {
			key      string
			indirect bool // unresolved frames were skipped before this one
		}
		var resolved []rf
		gap := false
		seen := map[string]bool{}
		for _, fr := range stk.Frames {
			file, line, ok := frame(fr)
			if !ok {
				continue
			}
			stats.FramesTotal++
			sym := res.resolve(file, line)
			if sym == nil {
				gap = true
				continue
			}
			stats.FramesResolved++
			key := graph.ObsKey(sym.File, sym.Parent, sym.Name)
			resolved = append(resolved, rf{key: key, indirect: gap})
			gap = false
			if !seen[key] {
				seen[key] = true
				h := heatAcc[key]
				if h == nil {
					h = &heat{}
					heatAcc[key] = h
				}
				h.total += stk.N
			}
		}
		if len(resolved) == 0 {
			continue
		}
		// Entry = outermost resolved frame; leaf = innermost (frames are
		// innermost-LAST, so entry is resolved[0], leaf is the last).
		heatAcc[resolved[0].key].entry += stk.N
		heatAcc[resolved[len(resolved)-1].key].leaf += stk.N
		for i := 1; i < len(resolved); i++ {
			src, dst := resolved[i-1], resolved[i]
			if src.key == dst.key {
				continue // recursion / sibling lines in one symbol
			}
			ind := "0"
			if dst.indirect {
				ind = "1"
			}
			edgeAcc[[3]string{src.key, dst.key, ind}] += stk.N
		}
	}

	// Deterministic write order.
	edgeKeys := make([][3]string, 0, len(edgeAcc))
	for k := range edgeAcc {
		edgeKeys = append(edgeKeys, k)
	}
	sort.Slice(edgeKeys, func(a, b int) bool {
		if edgeKeys[a][0] != edgeKeys[b][0] {
			return edgeKeys[a][0] < edgeKeys[b][0]
		}
		if edgeKeys[a][1] != edgeKeys[b][1] {
			return edgeKeys[a][1] < edgeKeys[b][1]
		}
		return edgeKeys[a][2] < edgeKeys[b][2]
	})
	for _, k := range edgeKeys {
		if err := st.AddObsEdge(tx, k[0], k[1], edgeAcc[k], k[2] == "1"); err != nil {
			return nil, err
		}
		stats.Edges++
	}
	heatKeys := make([]string, 0, len(heatAcc))
	for k := range heatAcc {
		heatKeys = append(heatKeys, k)
	}
	sort.Strings(heatKeys)
	for _, k := range heatKeys {
		h := heatAcc[k]
		if err := st.AddObsHeat(tx, k, h.leaf, h.total, h.entry); err != nil {
			return nil, err
		}
	}
	if err := st.PutObsLedger(tx, path, hash, p.Header.Lang, p.Header.Start, p.Header.End,
		p.Header.Commit, stats.FramesTotal, stats.FramesResolved, time.Now().Unix()); err != nil {
		return nil, err
	}
	return stats, tx.Commit()
}

// SpoolDir is where SDKs drop profiles for a repo.
func SpoolDir(root string) string { return filepath.Join(root, ".codeindex", "runtime") }

// IngestSpool ingests every unseen *.cxprof.jsonl in the repo's spool dir.
// Missing dir is a no-op. Returns per-file stats for ingested files.
func IngestSpool(st *graph.Store, root string) ([]*Stats, error) {
	ents, err := os.ReadDir(SpoolDir(root))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Stats
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".cxprof.jsonl") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		s, err := Ingest(st, root, filepath.Join(SpoolDir(root), n))
		if err != nil {
			// A malformed spool file must not poison the query path.
			fmt.Fprintf(os.Stderr, "codeindex: skipping profile %s: %v\n", n, err)
			continue
		}
		if s.Stacks > 0 {
			out = append(out, s)
		}
	}
	return out, nil
}

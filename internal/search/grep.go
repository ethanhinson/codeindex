package search

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"codeindex/internal/graph"
	"codeindex/internal/merkle"
)

// GrepHit is one raw content match.
type GrepHit struct {
	File string
	Line int
	Text string
}

// GrepGroup is the enriched form: hits attributed to their enclosing symbol,
// deduped with counts. Symbol is nil for hits outside any symbol.
type GrepGroup struct {
	Sym    *graph.Symbol
	File   string
	Hits   int
	First  GrepHit
	IsDef  bool // a hit sits on the symbol's definition line
	IsTest bool
}

// Grep searches file contents (ripgrep when available, internal scan
// otherwise) and attributes/dedups hits by enclosing symbol. word restricts
// matches to word boundaries (rg -w semantics).
func Grep(st *graph.Store, root, pattern string, limit int, word bool) (groups []GrepGroup, rawHits int, backend string, err error) {
	hits, backend, err := rawGrep(root, pattern, word)
	if err != nil {
		return nil, 0, backend, err
	}
	rawHits = len(hits)

	// Attribute per file: load spans once, binary-search lines.
	type key struct {
		file string
		id   int64 // 0 = outside any symbol
	}
	spansCache := map[string][]graph.Symbol{}
	agg := map[key]*GrepGroup{}
	for _, h := range hits {
		spans, ok := spansCache[h.File]
		if !ok {
			spans, err = st.FileSymbolSpans(h.File)
			if err != nil {
				return nil, 0, backend, err
			}
			spansCache[h.File] = spans
		}
		sym := enclosingByLine(spans, h.Line)
		k := key{file: h.File}
		if sym != nil {
			k.id = sym.ID
		}
		g, ok := agg[k]
		if !ok {
			g = &GrepGroup{Sym: sym, File: h.File, First: h,
				IsTest: strings.Contains(h.File, "_test.") || strings.Contains(h.File, "tests/")}
			agg[k] = g
		}
		g.Hits++
		if sym != nil && h.Line == sym.StartLine {
			g.IsDef = true
		}
	}

	for _, g := range agg {
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(a, b int) bool {
		ga, gb := groups[a], groups[b]
		if ga.IsDef != gb.IsDef {
			return ga.IsDef
		}
		if ga.IsTest != gb.IsTest {
			return !ga.IsTest
		}
		if ga.Hits != gb.Hits {
			return ga.Hits > gb.Hits
		}
		if ga.File != gb.File {
			return ga.File < gb.File
		}
		return ga.First.Line < gb.First.Line
	})
	if limit > 0 && len(groups) > limit {
		groups = groups[:limit]
	}
	return groups, rawHits, backend, nil
}

// enclosingByLine finds the innermost symbol whose line span contains line.
func enclosingByLine(spans []graph.Symbol, line int) *graph.Symbol {
	var best *graph.Symbol
	bestSize := 1 << 30
	for i := range spans {
		s := &spans[i]
		if s.StartLine <= line && line <= s.EndLine {
			if size := s.EndLine - s.StartLine; size < bestSize {
				best, bestSize = s, size
			}
		}
	}
	return best
}

// rawGrep runs ripgrep when available, else an internal Go-regexp scan over
// the indexed file set (correct; slower — surfaced via backend).
func rawGrep(root, pattern string, word bool) ([]GrepHit, string, error) {
	if rg, err := exec.LookPath("rg"); err == nil {
		args := []string{"--no-heading", "-n"}
		if word {
			args = append(args, "-w")
		}
		args = append(args, "-e", pattern, ".")
		cmd := exec.Command(rg, args...)
		cmd.Dir = root
		out, err := cmd.Output()
		if err == nil || len(out) > 0 { // rg exits 1 on no matches
			return parseRgOutput(out), "ripgrep", nil
		}
	}
	hits, err := internalScan(root, pattern, word)
	return hits, "internal", err
}

func parseRgOutput(out []byte) []GrepHit {
	var hits []GrepHit
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), ":", 3)
		if len(parts) < 3 {
			continue
		}
		line, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		hits = append(hits, GrepHit{
			File: graph.CleanRel(strings.TrimPrefix(parts[0], "./")),
			Line: line, Text: strings.TrimSpace(parts[2]),
		})
	}
	return hits
}

func internalScan(root, pattern string, word bool) ([]GrepHit, error) {
	if word {
		pattern = `\b(?:` + pattern + `)\b`
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("bad pattern: %w", err)
	}
	paths, err := merkle.Walk(root)
	if err != nil {
		return nil, err
	}
	var hits []GrepHit
	for _, rel := range paths {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(b), "\n") {
			if re.MatchString(line) {
				hits = append(hits, GrepHit{File: rel, Line: i + 1, Text: strings.TrimSpace(line)})
			}
		}
	}
	return hits, nil
}

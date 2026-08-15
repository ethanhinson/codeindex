package search

import (
	"math"
	"sort"
	"strings"

	"codeindex/internal/graph"
)

// Result is one ranked symbol match with its explanation inputs.
type Result struct {
	Sym     graph.Symbol
	Callers int
	Score   float64
	Match   string // which ladder rung matched (for explainability)
}

// Opts filter and bound a search.
type Opts struct {
	Kind  string // optional symbol kind filter
	Path  string // optional file-path substring filter
	Limit int
}

// Find scores every symbol against the query (D1: in-memory scan — measured
// tens of ms at kubernetes scale) and returns the top matches.
func Find(st *graph.Store, query string, opts Opts) ([]Result, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	syms, callers, err := st.AllSymbolsWithCallers()
	if err != nil {
		return nil, err
	}

	qLower := strings.ToLower(strings.TrimSpace(query))
	qTokens := Tokenize(query)
	qStems := make([]string, len(qTokens))
	for i, t := range qTokens {
		qStems[i] = Stem(t)
	}

	var results []Result
	for i := range syms {
		s := &syms[i]
		if opts.Kind != "" && string(s.Kind) != opts.Kind {
			continue
		}
		if opts.Path != "" && !strings.Contains(s.File, opts.Path) {
			continue
		}
		quality, rung := matchQuality(s.Name, qLower, qStems)
		if quality == 0 {
			continue
		}
		score := quality * boosts(s, callers[s.ID])
		results = append(results, Result{
			Sym: *s, Callers: callers[s.ID], Score: score, Match: rung,
		})
	}

	sort.Slice(results, func(a, b int) bool {
		if results[a].Score != results[b].Score {
			return results[a].Score > results[b].Score
		}
		if results[a].Sym.Name != results[b].Sym.Name {
			return results[a].Sym.Name < results[b].Sym.Name
		}
		if results[a].Sym.File != results[b].Sym.File {
			return results[a].Sym.File < results[b].Sym.File
		}
		return results[a].Sym.StartLine < results[b].Sym.StartLine
	})
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results, nil
}

// matchQuality implements the deterministic ladder (D3): exact > token-set >
// prefix > all-tokens-present > subsequence; synonym-matched tokens weigh 0.8.
func matchQuality(name, qLower string, qStems []string) (float64, string) {
	nLower := strings.ToLower(name)
	if nLower == qLower {
		return 100, "exact"
	}

	nTokens := Tokenize(name)
	nStems := make(map[string]bool, len(nTokens))
	for _, t := range nTokens {
		nStems[Stem(t)] = true
	}

	if len(qStems) > 0 {
		// exact token set (order-independent)
		if len(qStems) == len(nTokens) {
			all := true
			for _, qs := range qStems {
				if !nStems[qs] {
					all = false
					break
				}
			}
			if all {
				return 90, "token-set"
			}
		}
		if strings.HasPrefix(nLower, qLower) {
			return 80, "prefix"
		}
		// all query tokens present (direct or synonym; synonyms weigh 0.8)
		matched, direct := 0, 0
		for _, qs := range qStems {
			if nStems[qs] {
				matched++
				direct++
				continue
			}
			for _, syn := range Expand(qs) {
				if nStems[Stem(syn)] {
					matched++
					break
				}
			}
		}
		if matched == len(qStems) {
			w := (float64(direct) + 0.8*float64(matched-direct)) / float64(matched)
			return 70 * w, "all-tokens"
		}
	} else if strings.HasPrefix(nLower, qLower) {
		return 80, "prefix"
	}

	if isSubsequence(qLower, nLower) {
		return 50, "subsequence"
	}
	return 0, ""
}

// boosts multiplies graph signals into the score (D3). Composition of the
// compressed-envelope graph boost and the categorical file-class penalty —
// find's ladder applies both at full strength.
func boosts(s *graph.Symbol, callers int) float64 {
	return graphBoost(s, callers) * filePenalty(s.File)
}

// graphBoost is the usage/tier/kind signal. Semantic search compresses THIS
// with boostGamma (its raw range is ~0.4–3.9×); the file-class penalty must
// stay outside that envelope — compressed, a 0.7 test penalty becomes a
// meaningless 0.88 (measured: the compressed sample-dir penalty moved
// fixture noise by ~1 rank and flipped nothing).
func graphBoost(s *graph.Symbol, callers int) float64 {
	b := 1 + math.Log10(1+float64(callers))
	if s.Tier != 0 {
		b *= 0.6 // dep tier below project
	}
	switch s.Kind {
	case graph.KindMethod:
		b *= 0.9
	}
	return b
}

// filePenalty is the categorical file-class signal: test files and
// demo/fixture corpora (residuals bucket 3 — sample apps and e2e fixtures
// answer no "where is X implemented" question).
func filePenalty(file string) float64 {
	if strings.Contains(file, "_test.") || strings.Contains(file, "/test") ||
		strings.Contains(file, "tests/") || strings.Contains(file, ".spec.") ||
		inSampleDir(file) {
		return 0.7
	}
	return 1
}

// sampleDirs are matched by exact path segment so core files merely
// containing these words are untouched.
var sampleDirs = map[string]bool{
	"sample": true, "samples": true, "example": true, "examples": true,
	"demo": true, "demos": true, "benchmark": true, "benchmarks": true,
	"fixture": true, "fixtures": true, "integration": true,
}

func inSampleDir(file string) bool {
	for _, seg := range strings.Split(file, "/") {
		if sampleDirs[seg] {
			return true
		}
	}
	return false
}

func isSubsequence(needle, hay string) bool {
	if needle == "" {
		return false
	}
	j := 0
	for i := 0; i < len(hay) && j < len(needle); i++ {
		if hay[i] == needle[j] {
			j++
		}
	}
	return j == len(needle)
}

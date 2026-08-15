package search

import (
	"math"
	"os"
	"regexp"
	"sort"
	"strings"

	"codeindex/internal/graph"
)

// The literal lane (literal-lane-retrieval): symptom language lives verbatim
// in code strings — error messages, warnings, config keys — which names and
// cards never carry. Measured: grep attribution beat semantic search on
// real-issue queries (bench/results/issues-v2-controls-*.json), so the
// winning control runs INSIDE fusion as a third lane. Always on; influence
// self-weighted from its own result statistics (design D3, constants
// frozen before measurement).

const (
	litK        = 20    // steep RRF constant: grep order is already quality-sorted
	litWordGrep = 15    // grep group limit per distinctive word
	litWords    = 2     // distinctive words taken from the query
	phrasePin   = 900.0 // exactness rung 2: below exact-name (+1000), above fusion
	phraseCap   = 3     // max symbols pinned per phrase

	// Relative-threshold gate (FINDINGS-literal-lane registered follow-up):
	// distinctiveness is repo-relative. A fixed cap (=100, iteration 2) was
	// right at gin's size and wrong at laravel's 10× size. Frozen BEFORE the
	// one-shot rerun: cap = max(minWordHits, symbols/wordHitsPerSym).
	minWordHits    = 50
	wordHitsPerSym = 10
)

// maxWordHits scales the word-noise cap by repo size.
func maxWordHits(symbols int) int {
	if cap := symbols / wordHitsPerSym; cap > minWordHits {
		return cap
	}
	return minWordHits
}

// litStopwords mirrors the bench measurability-guard list (small, closed).
var litStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "for": true, "and": true,
	"or": true, "to": true, "in": true, "on": true, "at": true, "by": true,
	"with": true, "from": true, "as": true, "is": true, "are": true,
	"was": true, "be": true, "been": true, "this": true, "that": true,
	"these": true, "those": true, "it": true, "its": true, "if": true,
	"not": true, "no": true, "all": true, "any": true, "can": true,
	"cant": true, "will": true, "should": true, "may": true, "when": true,
	"which": true, "who": true, "what": true, "how": true, "into": true,
	"out": true, "up": true, "down": true, "over": true, "under": true,
	"than": true, "then": true, "them": true, "they": true, "there": true,
	"here": true, "also": true, "such": true, "some": true, "same": true,
	"other": true, "using": true, "used": true, "use": true, "does": true,
	"doesnt": true, "why": true, "after": true, "before": true, "while": true,
}

var wordRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*`)

// distinctiveWords picks the query's most distinctive content words
// (design D1): stopwords and short tokens out, longest first, top N.
func distinctiveWords(q string, n int) []string {
	seen := map[string]bool{}
	var words []string
	for _, w := range wordRe.FindAllString(q, -1) {
		lw := strings.ToLower(w)
		if len(lw) < 3 || litStopwords[lw] || seen[lw] {
			continue
		}
		seen[lw] = true
		words = append(words, lw)
	}
	sort.SliceStable(words, func(a, b int) bool { return len(words[a]) > len(words[b]) })
	if len(words) > n {
		words = words[:n]
	}
	return words
}

// literalLane holds the lane's ranked symbol ids and its self-computed
// confidence plus any verbatim-phrase pins.
type literalLane struct {
	rank map[int64]int // symbol id -> lane rank (0 = best)
	conf float64
	pins []int64 // exactness rung 2: phrase-matched symbol ids
}

// quotedPhrase extracts a double- or single-quoted span from the query.
func quotedPhrase(q string) string {
	for _, quote := range []string{`"`, "'", "`"} {
		if i := strings.Index(q, quote); i >= 0 {
			if j := strings.Index(q[i+1:], quote); j > 0 {
				return q[i+1 : i+1+j]
			}
		}
	}
	return ""
}

// buildLiteralLane runs the lane (design D2/D3/D4). root == "" disables it
// (grep needs the working tree). Failures degrade to an inactive lane —
// literal evidence is additive, never load-bearing.
//
// GATE VERDICT (bench/engine/FINDINGS-literal-lane.md): the ALWAYS-ON lane
// failed its pre-registered conjunction after both iterations (fixed
// per-word hit caps mean different things at different repo sizes), so per
// registration it is WITHHELD from defaults: the lane runs only with
// explicit symptom evidence (error_text) or the experimental flag. The
// registered follow-up gate is a repo-size-relative word threshold.
func buildLiteralLane(st *graph.Store, root, query, errorText string, symbols int) literalLane {
	lane := literalLane{rank: map[int64]int{}, conf: 1.0}
	if root == "" {
		return lane
	}
	if errorText == "" && os.Getenv("CODEINDEX_LITERAL_LANE") != "1" {
		return lane // withheld by default (gate verdict); explicit evidence only
	}
	sel := query
	if errorText != "" {
		sel += " " + errorText
	}
	words := distinctiveWords(sel, litWords)
	if len(words) == 0 {
		return lane
	}

	type hit struct {
		id    int64
		hits  int
		words int
		order int // first-seen grep order (deterministic tiebreak)
	}
	acc := map[int64]*hit{}
	rawTotal := 0
	order := 0
	for _, w := range words {
		groups, raw, _, err := Grep(st, root, regexp.QuoteMeta(w), litWordGrep)
		if err != nil {
			continue
		}
		if raw > maxWordHits(symbols) {
			continue // repo-common word: no discriminative power
		}
		rawTotal += raw
		for _, g := range groups {
			// Test files are literal-dense noise for this lane (registered
			// iteration 1: gin curated regressions were test symbols riding
			// grep order into the top ranks).
			if g.Sym == nil || g.Sym.Tier != 0 || g.IsTest {
				continue
			}
			h := acc[g.Sym.ID]
			if h == nil {
				h = &hit{id: g.Sym.ID, order: order}
				order++
				acc[g.Sym.ID] = h
			}
			h.hits += g.Hits
			h.words++
		}
	}
	if len(acc) == 0 {
		return lane
	}

	// Rank: co-occurring symbols first (by hits desc), then grep order.
	hits := make([]*hit, 0, len(acc))
	coocc := false
	for _, h := range acc {
		hits = append(hits, h)
		if h.words >= 2 {
			coocc = true
		}
	}
	sort.Slice(hits, func(a, b int) bool {
		if (hits[a].words >= 2) != (hits[b].words >= 2) {
			return hits[a].words >= 2
		}
		if hits[a].words >= 2 && hits[a].hits != hits[b].hits {
			return hits[a].hits > hits[b].hits
		}
		return hits[a].order < hits[b].order
	})
	for i, h := range hits {
		lane.rank[h.id] = i
	}

	// Self-weighting (D3): coocc up, dispersion down, quote-shape up.
	conf := 1.0
	if coocc {
		conf *= 1.5
	}
	disp := 2.0 / math.Log10(10+float64(rawTotal)) // ≤30 hits ≈ 1.24; 3000 ≈ 0.57
	if disp > 1.5 {
		disp = 1.5
	}
	if disp < 0.3 {
		disp = 0.3
	}
	conf *= disp
	phrase := quotedPhrase(query)
	if errorText != "" {
		if p := quotedPhrase(errorText); p != "" {
			phrase = p
		} else {
			phrase = strings.TrimSpace(errorText)
		}
	}
	if phrase == "" {
		// Full query counts as a phrase when it has >=3 content words (D4).
		if len(distinctiveWords(query, 3)) >= 3 {
			phrase = strings.TrimSpace(query)
		}
	} else {
		conf *= 1.5 // quote-shaped input (D3)
	}

	// Exactness rung 2 (D4): verbatim phrase inside a symbol span.
	if phrase != "" && len(phrase) >= 8 {
		if groups, _, _, err := Grep(st, root, "(?i)"+regexp.QuoteMeta(phrase), phraseCap+2); err == nil {
			for _, g := range groups {
				if g.Sym != nil && g.Sym.Tier == 0 && !g.IsTest && len(lane.pins) < phraseCap {
					lane.pins = append(lane.pins, g.Sym.ID)
				}
			}
		}
	}
	lane.conf = conf
	return lane
}

package index

import (
	"math"
	"sort"
	"strings"
	"time"

	"codeindex/internal/search"
)

type Hit struct {
	Rec     StoredRecord
	Score   float64
	Snippet string
}

// Search ranks records against a query. The corpus is small (hundreds of
// records), so this is a full in-memory scan — the same D1 pattern
// internal/search validated at far larger scale.
func Search(recs []StoredRecord, query string, now time.Time, limit int) []Hit {
	if limit <= 0 {
		limit = 10
	}
	qStems := stems(query)
	if len(qStems) == 0 {
		return nil
	}
	var hits []Hit
	for _, r := range recs {
		score, snippet := scoreRecord(r, qStems)
		if score == 0 {
			continue
		}
		score *= layerFactor(r, now) * statusFactor(r.Status)
		if r.Stale {
			score *= 0.7
		}
		hits = append(hits, Hit{Rec: r, Score: score, Snippet: snippet})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Rec.ID < hits[j].Rec.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func stems(s string) []string {
	toks := search.Tokenize(s)
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = search.Stem(t)
	}
	return out
}

// scoreRecord returns the best chunk score (title weighted 2x) and that
// chunk's snippet.
func scoreRecord(r StoredRecord, qStems []string) (float64, string) {
	best, snippet := 0.0, ""
	consider := func(text string, weight float64) {
		s := chunkScore(text, qStems) * weight
		if s > best {
			best, snippet = s, snip(text)
		}
	}
	consider(r.Title, 2.0)
	for _, chunk := range strings.Split(r.Body, "\n## ") {
		consider(chunk, 1.0)
	}
	return best, snippet
}

func chunkScore(text string, qStems []string) float64 {
	cs := stems(text)
	tf := map[string]int{}
	for _, s := range cs {
		tf[s]++
	}
	matched, freq := 0, 0
	for _, q := range qStems {
		if tf[q] > 0 {
			matched++
			freq += tf[q]
		}
	}
	if matched == 0 {
		return 0
	}
	return float64(matched) / float64(len(qStems)) * (1 + math.Log(1+float64(freq)))
}

func layerFactor(r StoredRecord, now time.Time) float64 {
	if r.Layer != "session" {
		return 1.0
	}
	d, err := time.Parse("2006-01-02", r.Date)
	if err != nil {
		return 1.0
	}
	ageDays := now.Sub(d).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	return math.Exp(-math.Ln2 * ageDays / 7) // 7-day half-life
}

func statusFactor(status string) float64 {
	switch status {
	case "superseded", "rejected", "done", "dropped":
		return 0.5
	}
	return 1.0
}

func snip(text string) string {
	line := strings.Join(strings.Fields(text), " ")
	if len(line) > 120 {
		line = line[:120]
	}
	return line
}

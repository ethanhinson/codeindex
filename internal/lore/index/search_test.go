package index

import (
	"math"
	"strings"
	"testing"
	"time"

	"codeindex/internal/lore"
)

func rec(id, title, body, status, layer, date string) StoredRecord {
	return StoredRecord{
		Record: lore.Record{ID: id, Type: lore.TypeDecision, Title: title,
			Body: body, Status: status, Date: date},
		Layer: layer,
	}
}

func TestSearchRanksTitleAndStatus(t *testing.T) {
	now := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	recs := []StoredRecord{
		rec("dec-1", "Use Go for the engine", "why go\n", "active", "repo", "2026-07-01"),
		rec("dec-2", "Old runtime choice", "we chose go once\n", "superseded", "repo", "2026-06-01"),
		rec("dec-3", "Unrelated", "nothing here\n", "active", "repo", "2026-07-01"),
	}
	hits := Search(recs, "go engine", now, 10)
	if len(hits) != 2 {
		t.Fatalf("hits=%d want 2 (unrelated omitted)", len(hits))
	}
	if hits[0].Rec.ID != "dec-1" {
		t.Fatalf("active title match should outrank superseded body: %s", hits[0].Rec.ID)
	}
}

func TestSearchSessionDecay(t *testing.T) {
	now := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	fresh := rec("note-f", "postgres tuning", "b\n", "", "session", "2026-07-28")
	old := rec("note-o", "postgres tuning", "b\n", "", "session", "2026-06-01")
	hits := Search([]StoredRecord{old, fresh}, "postgres tuning", now, 10)
	if len(hits) != 2 || hits[0].Rec.ID != "note-f" {
		t.Fatalf("fresh session note should outrank old: %+v", hits)
	}
	if hits[1].Score >= hits[0].Score/2 {
		t.Fatalf("8-week-old note barely decayed: %v vs %v", hits[1].Score, hits[0].Score)
	}
}

func TestSearchChunkSnippet(t *testing.T) {
	r := rec("dec-c", "T", "intro\n\n## Alternatives considered\nRust was rejected here\n",
		"active", "repo", "2026-07-01")
	hits := Search([]StoredRecord{r}, "rust rejected", time.Now().UTC(), 10)
	if len(hits) != 1 || hits[0].Snippet == "" ||
		!containsFold(hits[0].Snippet, "rust") {
		t.Fatalf("snippet from matching chunk: %+v", hits)
	}
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// TestSearchConfidenceMultiplierAffectsRanking: two otherwise identical records
// where one has Confidence=1.0 and the other Confidence=0. The high-confidence
// record must rank first and its score must be ~1.5× the low-confidence score
// (1.2/0.8 = 1.5).
func TestSearchConfidenceMultiplierAffectsRanking(t *testing.T) {
	now := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	highConf := StoredRecord{
		Record: lore.Record{
			ID: "dec-HIGH", Type: lore.TypeDecision, Title: "engine optimization",
			Body: "b\n", Status: "active", Date: "2026-07-30",
		},
		Layer:      "repo",
		Confidence: 1.0,
	}
	lowConf := StoredRecord{
		Record: lore.Record{
			ID: "dec-LOW", Type: lore.TypeDecision, Title: "engine optimization",
			Body: "b\n", Status: "active", Date: "2026-07-30",
		},
		Layer:      "repo",
		Confidence: 0.0,
	}

	hits := Search([]StoredRecord{lowConf, highConf}, "engine optimization", now, 10)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].Rec.ID != "dec-HIGH" {
		t.Fatalf("high-confidence record should rank first; got %s first", hits[0].Rec.ID)
	}
	// Score ratio should be 1.2/0.8 = 1.5
	ratio := hits[0].Score / hits[1].Score
	if math.Abs(ratio-1.5) > 1e-9 {
		t.Fatalf("score ratio: want 1.5 (1.2/0.8), got %.6f (scores: %.6f / %.6f)",
			ratio, hits[0].Score, hits[1].Score)
	}
}

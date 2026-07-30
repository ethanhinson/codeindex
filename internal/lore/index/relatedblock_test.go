package index

import (
	"strings"
	"testing"

	"codeindex/internal/lore"
)

func TestRelatedLoreBlock(t *testing.T) {
	recs := []StoredRecord{
		{Record: lore.Record{ID: "dec-1", Type: lore.TypeDecision, Title: "Anchored",
			Status: "active", Anchors: []lore.Anchor{{Symbol: "Foo"}}, Related: []string{"note-2"}}},
		{Record: lore.Record{ID: "note-2", Type: lore.TypeNote, Title: "Linked note"}},
	}
	block := RelatedLoreBlock(recs, "Foo", -1)
	if !strings.Contains(block, "dec-1") {
		t.Fatalf("expected anchored record in block:\n%s", block)
	}
	if !strings.Contains(block, "note-2") {
		t.Fatalf("expected one-hop related record via full trace:\n%s", block)
	}
	if RelatedLoreBlock(recs, "Unanchored", -1) != "" {
		t.Fatalf("no anchor match must yield empty block")
	}
}

// TestRelatedLoreBlock_DeterministicOrder verifies that records at the same
// distance with the same status rank are sorted by ID, not by map-iteration
// order. Running the same input multiple times must always produce identical
// output.
func TestRelatedLoreBlock_DeterministicOrder(t *testing.T) {
	// Three records all anchored to the same symbol (distance 0), same status —
	// only the ID distinguishes them. The expected order is alphabetical by ID.
	recs := []StoredRecord{
		{Record: lore.Record{ID: "zzz-3", Type: lore.TypeNote, Title: "C",
			Status: "active", Anchors: []lore.Anchor{{Symbol: "Bar"}}}},
		{Record: lore.Record{ID: "aaa-1", Type: lore.TypeNote, Title: "A",
			Status: "active", Anchors: []lore.Anchor{{Symbol: "Bar"}}}},
		{Record: lore.Record{ID: "mmm-2", Type: lore.TypeNote, Title: "B",
			Status: "active", Anchors: []lore.Anchor{{Symbol: "Bar"}}}},
	}
	first := RelatedLoreBlock(recs, "Bar", -1)
	for i := 0; i < 20; i++ {
		if got := RelatedLoreBlock(recs, "Bar", -1); got != first {
			t.Fatalf("non-deterministic output on iteration %d:\nfirst:\n%s\ngot:\n%s", i+1, first, got)
		}
	}
	// Also assert the concrete expected order: aaa-1 before mmm-2 before zzz-3.
	posA := strings.Index(first, "aaa-1")
	posM := strings.Index(first, "mmm-2")
	posZ := strings.Index(first, "zzz-3")
	if posA < 0 || posM < 0 || posZ < 0 {
		t.Fatalf("one or more IDs missing from block:\n%s", first)
	}
	if !(posA < posM && posM < posZ) {
		t.Fatalf("unexpected order: aaa-1@%d mmm-2@%d zzz-3@%d\n%s", posA, posM, posZ, first)
	}
}

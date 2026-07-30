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

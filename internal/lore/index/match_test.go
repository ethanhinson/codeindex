package index

import (
	"testing"

	"codeindex/internal/lore"
)

func TestAnchorMatches(t *testing.T) {
	cases := []struct {
		a    lore.Anchor
		q    string
		want bool
	}{
		{lore.Anchor{Symbol: "ResolveImports"}, "ResolveImports", true},
		{lore.Anchor{Symbol: "ResolveImports"}, "Resolve", false},
		{lore.Anchor{Path: "internal/engine/"}, "internal/engine/resolver.go", true},
		{lore.Anchor{Path: "internal/engine/resolver.go"}, "internal/engine/", true},
		{lore.Anchor{Path: ""}, "anything", false},
		{lore.Anchor{Path: "docs/"}, "internal/", false},
	}
	for _, c := range cases {
		if got := AnchorMatches(c.a, c.q); got != c.want {
			t.Fatalf("AnchorMatches(%+v, %q) = %v, want %v", c.a, c.q, got, c.want)
		}
	}
}

func TestRecordsForAnchor(t *testing.T) {
	recs := []StoredRecord{
		{Record: lore.Record{ID: "dec-A", Anchors: []lore.Anchor{{Path: "internal/engine/"}}}},
		{Record: lore.Record{ID: "dec-B", Anchors: []lore.Anchor{{Symbol: "Foo"}}}},
		{Record: lore.Record{ID: "dec-C"}},
	}
	got := RecordsForAnchor(recs, "internal/engine/x.go")
	if len(got) != 1 || got[0].ID != "dec-A" {
		t.Fatalf("got %+v", got)
	}
}

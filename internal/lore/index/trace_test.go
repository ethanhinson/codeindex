package index

import (
	"testing"

	"codeindex/internal/lore"
)

func recWith(id, title string, related, blockedBy []string, supersedes string) StoredRecord {
	return StoredRecord{Record: lore.Record{
		ID: id, Title: title, Related: related, BlockedBy: blockedBy, Supersedes: supersedes,
	}}
}

func TestResolveID(t *testing.T) {
	recs := []StoredRecord{recWith("dec-1", "No Graph Coupling", nil, nil, "")}
	if id, ok := ResolveID(recs, "dec-1"); !ok || id != "dec-1" {
		t.Fatalf("by id: %q %v", id, ok)
	}
	if id, ok := ResolveID(recs, lore.Slug("No Graph Coupling")); !ok || id != "dec-1" {
		t.Fatalf("by slug: %q %v", id, ok)
	}
	if _, ok := ResolveID(recs, "nope"); ok {
		t.Fatalf("missing should not resolve")
	}
}

func TestTraceFullAndDepth(t *testing.T) {
	// A -> B -> C, and a cycle A <-> B via B.related back to A.
	recs := []StoredRecord{
		recWith("A", "a", []string{"B"}, nil, ""),
		recWith("B", "b", []string{"C", "A"}, nil, ""),
		recWith("C", "c", nil, nil, ""),
	}
	full := Trace(recs, "A", TraceOpts{Depth: -1})
	dist := map[string]int{}
	for _, r := range full {
		dist[r.ID] = r.Distance
	}
	if dist["B"] != 1 || dist["C"] != 2 {
		t.Fatalf("distances = %v", dist)
	}
	if _, ok := dist["A"]; ok {
		t.Fatalf("start node must be excluded")
	}
	// Depth 1 reaches only B.
	d1 := Trace(recs, "A", TraceOpts{Depth: 1})
	if len(d1) != 1 || d1[0].ID != "B" {
		t.Fatalf("depth-1 = %+v", d1)
	}
}

func TestTraceCap(t *testing.T) {
	var recs []StoredRecord
	// chain of 10 nodes n0->n1->...->n9
	for i := 0; i < 10; i++ {
		var rel []string
		if i < 9 {
			rel = []string{itoa(i + 1)}
		}
		recs = append(recs, recWith(itoa(i), itoa(i), rel, nil, ""))
	}
	got := Trace(recs, itoa(0), TraceOpts{Depth: -1, Cap: 3})
	if len(got) != 3 {
		t.Fatalf("cap should bound reached to 3, got %d", len(got))
	}
}

func TestBacklinks(t *testing.T) {
	recs := []StoredRecord{
		recWith("A", "a", []string{"B"}, nil, ""),
		recWith("C", "c", nil, []string{"B"}, ""),   // blocked_by B
		recWith("D", "d", nil, nil, "B"),             // supersedes B
		recWith("B", "b", nil, nil, ""),
	}
	bl := Backlinks(recs, "B")
	ids := map[string]bool{}
	for _, r := range bl {
		ids[r.ID] = true
	}
	if !ids["A"] || !ids["C"] || !ids["D"] {
		t.Fatalf("backlinks = %v", ids)
	}
}

func itoa(i int) string { return string(rune('n')) + string(rune('0'+i)) }

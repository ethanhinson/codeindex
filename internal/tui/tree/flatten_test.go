package tree

import "testing"

func rowLabels(rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Node.Label)
	}
	return out
}

func TestVisibleRespectsExpansion(t *testing.T) {
	root := BuildTree(fixtureSymbols())

	// All collapsed: only top level shows.
	got := rowLabels(Visible(root))
	want := []string{"internal", "main.go"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("collapsed rows = %v, want %v", got, want)
	}

	// Expand internal/ → its children appear, at depth 1.
	child(t, root, "internal").Expanded = true
	rows := Visible(root)
	got = rowLabels(rows)
	want = []string{"internal", "graph", "query", "main.go"}
	if len(got) != 4 || got[1] != "graph" || got[3] != "main.go" {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if rows[0].Depth != 0 || rows[1].Depth != 1 {
		t.Fatalf("depths = %d,%d want 0,1", rows[0].Depth, rows[1].Depth)
	}
}

func TestVisibleDeepExpansion(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	internal := child(t, root, "internal")
	internal.Expanded = true
	graphDir := child(t, internal, "graph")
	graphDir.Expanded = true
	storeGo := child(t, graphDir, "store.go")
	storeGo.Expanded = true

	got := rowLabels(Visible(root))
	// internal, graph, store.go, Store, open, query, main.go
	if len(got) != 7 || got[3] != "Store" || got[4] != "open" {
		t.Fatalf("rows = %v", got)
	}
}

func TestParentIndex(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	internal := child(t, root, "internal")
	internal.Expanded = true
	child(t, internal, "graph").Expanded = true
	rows := Visible(root)
	// rows: internal(0) graph(1) store.go(2) query(3) main.go(4)
	if p := ParentIndex(rows, 2); p != 1 {
		t.Errorf("parent of store.go = %d, want 1", p)
	}
	if p := ParentIndex(rows, 3); p != 0 {
		t.Errorf("parent of query = %d, want 0", p)
	}
	if p := ParentIndex(rows, 0); p != -1 {
		t.Errorf("parent of internal = %d, want -1", p)
	}
}

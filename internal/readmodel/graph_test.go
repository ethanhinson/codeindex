// internal/readmodel/graph_test.go
package readmodel

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/engine"
	"codeindex/internal/graph"
	"codeindex/internal/lore"
	loreindex "codeindex/internal/lore/index"
)

// writeTree writes files under a fresh temp dir and returns the dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func buildStore(t *testing.T, files map[string]string) *graph.Store {
	t.Helper()
	dir := writeTree(t, files)
	db := filepath.Join(dir, "g.db")
	if _, err := engine.Build(dir, db); err != nil {
		t.Fatal(err)
	}
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestSymbolNeighborhood(t *testing.T) {
	st := buildStore(t, map[string]string{
		"a.go": "package p\nfunc Helper(x int) int { return x + 1 }\nfunc A() int { return Helper(1) }\n",
		"b.go": "package p\nfunc B() int { return A() }\n",
	})
	g, err := SymbolNeighborhood(st, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	if g.Focus != "sym:A" {
		t.Fatalf("focus = %q, want sym:A", g.Focus)
	}
	// Expect nodes: A (focus), Helper (callee), B (caller).
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	for _, want := range []string{"sym:A", "sym:Helper", "sym:B"} {
		if !ids[want] {
			t.Errorf("missing node %q; got %v", want, ids)
		}
	}
	// Expect edges: B->A (calls), A->Helper (calls).
	var hasCallerEdge, hasCalleeEdge bool
	for _, e := range g.Edges {
		if e.Source == "sym:B" && e.Target == "sym:A" && e.Kind == EdgeCalls {
			hasCallerEdge = true
		}
		if e.Source == "sym:A" && e.Target == "sym:Helper" && e.Kind == EdgeCalls {
			hasCalleeEdge = true
		}
	}
	if !hasCallerEdge || !hasCalleeEdge {
		t.Errorf("edges wrong: %+v", g.Edges)
	}
}

func openLoreStore(t *testing.T) *loreindex.Store {
	t.Helper()
	s, err := loreindex.Open(filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAttachAnchoredLore(t *testing.T) {
	ls := openLoreStore(t)
	rec := lore.Record{
		ID: "dec-A", Type: lore.TypeDecision, Title: "Keep Helper pure",
		Status: "active", Date: "2026-07-29",
		Anchors: []lore.Anchor{{Symbol: "Helper"}},
	}
	if err := ls.Upsert(rec, "repo", "/repo/.lore/decisions/a.md"); err != nil {
		t.Fatal(err)
	}
	recs, err := ls.All()
	if err != nil {
		t.Fatal(err)
	}

	g := Graph{
		Focus: "sym:Helper",
		Nodes: []Node{{ID: "sym:Helper", Kind: NodeSymbol, Label: "Helper"}},
	}
	AttachAnchoredLore(&g, recs)

	var hasNode, hasEdge bool
	for _, n := range g.Nodes {
		if n.ID == "dec-A" && n.Kind == NodeDecision && n.Label == "Keep Helper pure" {
			hasNode = true
		}
	}
	for _, e := range g.Edges {
		if e.Source == "dec-A" && e.Target == "sym:Helper" && e.Kind == EdgeAnchors {
			hasEdge = true
		}
	}
	if !hasNode || !hasEdge {
		t.Fatalf("anchored lore not attached: nodes=%+v edges=%+v", g.Nodes, g.Edges)
	}
}

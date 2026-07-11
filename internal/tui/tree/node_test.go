package tree

import (
	"testing"

	"codeindex/internal/graph"
)

func fixtureSymbols() []graph.Symbol {
	return []graph.Symbol{
		{File: "internal/graph/store.go", Name: "Store", Kind: graph.KindType,
			Signature: "type Store struct", StartLine: 5, EndLine: 90},
		{File: "internal/graph/store.go", Name: "Close", Parent: "Store",
			Kind: graph.KindMethod, Signature: "func (s *Store) Close() error",
			StartLine: 40, EndLine: 42},
		{File: "internal/graph/store.go", Name: "open", Kind: graph.KindFunc,
			Signature: "func open()", StartLine: 100, EndLine: 110},
		{File: "internal/query/query.go", Name: "Fresh", Kind: graph.KindFunc,
			Signature: "func Fresh()", StartLine: 10, EndLine: 20},
		// Method whose parent type lives in another file: falls back to the file.
		{File: "internal/query/query.go", Name: "Orphan", Parent: "Ghost",
			Kind: graph.KindMethod, Signature: "func (g Ghost) Orphan()",
			StartLine: 30, EndLine: 31},
		{File: "main.go", Name: "main", Kind: graph.KindFunc,
			Signature: "func main()", StartLine: 1, EndLine: 3},
	}
}

// child finds a direct child by label or fails the test.
func child(t *testing.T, n *Node, label string) *Node {
	t.Helper()
	for _, c := range n.Children {
		if c.Label == label {
			return c
		}
	}
	t.Fatalf("node %q has no child %q (children: %v)", n.Label, label, labels(n))
	return nil
}

func labels(n *Node) []string {
	var out []string
	for _, c := range n.Children {
		out = append(out, c.Label)
	}
	return out
}

func TestBuildTreeStructure(t *testing.T) {
	root := BuildTree(fixtureSymbols())

	// Dirs sort before files: internal/ before main.go.
	if got := labels(root); len(got) != 2 || got[0] != "internal" || got[1] != "main.go" {
		t.Fatalf("root children = %v, want [internal main.go]", got)
	}

	internal := child(t, root, "internal")
	if internal.Kind != KindDir || internal.Expanded {
		t.Fatalf("internal: kind=%v expanded=%v, want dir, collapsed", internal.Kind, internal.Expanded)
	}

	storeGo := child(t, child(t, internal, "graph"), "store.go")
	if storeGo.Kind != KindFile || storeGo.File != "internal/graph/store.go" {
		t.Fatalf("store.go node = %+v", storeGo)
	}

	// Symbols ordered by line: Store (5) before open (100).
	if got := labels(storeGo); len(got) != 2 || got[0] != "Store" || got[1] != "open" {
		t.Fatalf("store.go children = %v, want [Store open]", got)
	}

	// Method nests under its type node.
	storeType := child(t, storeGo, "Store")
	closeM := child(t, storeType, "Close")
	if closeM.SymKind != "method" || closeM.SymParent != "Store" || closeM.Line != 40 {
		t.Fatalf("Close node = %+v", closeM)
	}

	// Orphan method (no type node in file) hangs off the file, qualified.
	queryGo := child(t, child(t, internal, "query"), "query.go")
	orphan := child(t, queryGo, "Ghost.Orphan")
	if orphan.SymName != "Orphan" || orphan.SymParent != "Ghost" {
		t.Fatalf("orphan node = %+v", orphan)
	}
}

func TestBuildTreeEmpty(t *testing.T) {
	root := BuildTree(nil)
	if root == nil || len(root.Children) != 0 || !root.Expanded {
		t.Fatalf("empty root = %+v", root)
	}
}

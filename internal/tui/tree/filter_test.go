package tree

import "testing"

func TestFilterTreeExpandsPathsToMatches(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	f := FilterTree(root, "fresh")
	if f == nil {
		t.Fatal("expected a match for 'fresh'")
	}
	rows := Visible(f)
	got := rowLabels(rows)
	// Ancestor chain auto-expanded: internal → query → query.go → Fresh.
	want := []string{"internal", "query", "query.go", "Fresh"}
	if len(got) != 4 {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
}

func TestFilterTreeCaseInsensitive(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	if FilterTree(root, "STORE") == nil {
		t.Fatal("filter should be case-insensitive")
	}
}

func TestFilterTreeMatchKeepsSubtreeCollapsed(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	f := FilterTree(root, "store.go")
	rows := Visible(f)
	got := rowLabels(rows)
	// store.go matches; its symbols stay present but collapsed.
	if got[len(got)-1] != "store.go" {
		t.Fatalf("rows = %v, want to end at store.go", got)
	}
	last := rows[len(rows)-1].Node
	if len(last.Children) != 2 || last.Expanded {
		t.Fatalf("matched node should keep children collapsed: %+v", last)
	}
}

func TestFilterTreeNoMatch(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	if f := FilterTree(root, "zzzznothing"); f != nil {
		t.Fatalf("expected nil, got %v", labels(f))
	}
}

func TestFilterTreeDoesNotMutateOriginal(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	FilterTree(root, "fresh")
	if child(t, root, "internal").Expanded {
		t.Fatal("filter mutated the original tree")
	}

	// Verify slice isolation: filter a tree, mutate the filtered copy's
	// children, and verify the original is untouched.
	root2 := BuildTree(fixtureSymbols())
	filtered := FilterTree(root2, "store.go")
	if filtered == nil {
		t.Fatal("expected filter to match store.go")
	}

	// Navigate to store.go in the filtered tree
	storeGoFiltered := child(t, child(t, child(t, filtered, "internal"), "graph"), "store.go")
	origChildCount := len(storeGoFiltered.Children)
	if origChildCount == 0 {
		t.Fatal("store.go should have children in filtered tree")
	}

	// Append a dummy node to the filtered copy's children
	storeGoFiltered.Children = append(storeGoFiltered.Children, &Node{Label: "DummyNode"})

	// Verify the original tree's store.go still has the original child count
	storeGoOriginal := child(t, child(t, child(t, root2, "internal"), "graph"), "store.go")
	if len(storeGoOriginal.Children) != origChildCount {
		t.Fatalf("original tree was mutated: %d children, want %d", len(storeGoOriginal.Children), origChildCount)
	}
}

func TestFilterTreeEmptyQueryReturnsRoot(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	if FilterTree(root, "") != root {
		t.Fatal("empty query should return the original root")
	}
}

func TestMatchesQualifiedName(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	storeGo := child(t, child(t, child(t, root, "internal"), "graph"), "store.go")
	closeM := child(t, child(t, storeGo, "Store"), "Close")
	if !Matches(closeM, "store.close") {
		t.Fatal("qualified name should match")
	}
	if Matches(closeM, "fresh") {
		t.Fatal("unrelated query should not match")
	}
}

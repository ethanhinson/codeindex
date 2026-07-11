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

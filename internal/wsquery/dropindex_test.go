package wsquery

import (
	"strings"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/query"
)

// The suppression filter's own-half join (§3.6).
//
// FilterSuppressedEdges says WHICH tier-1 edges are double-counted; these
// indexes say which ROWS of a per-repo answer those edges correspond to. The
// join is the risky half: over-drop and a still-correct edge disappears with
// nothing in its place, which is exactly the failure the filter's narrowing
// exists to prevent in the other direction.

func tierOne(srcFile, srcName, srcParent, dstName, kind string, line int) graph.TierOneEdge {
	return graph.TierOneEdge{
		UnresolvedEdge: graph.UnresolvedEdge{
			SrcFile: srcFile, SrcName: srcName, SrcParent: srcParent,
			DstName: dstName, Kind: kind, Line: line,
		},
		DstNamespace: "Vendor\\Ns",
	}
}

func TestCallerDropsMatchOnlyTheDroppedRow(t *testing.T) {
	dropped := []graph.TierOneEdge{
		tierOne("c.go", "Consumer", "", "Target", "calls", 12),
	}
	d := callerDrops(dropped, "Target")

	// The row the filter removed.
	if !d.take(inKey{File: "c.go", QName: "Consumer", Line: 12}) {
		t.Error("the dropped row was not matched")
	}
	// Its neighbours must survive: a different line, a different caller, a
	// different file.
	survivors := []inKey{
		{File: "c.go", QName: "Consumer", Line: 13},
		{File: "c.go", QName: "Other", Line: 12},
		{File: "d.go", QName: "Consumer", Line: 12},
	}
	for _, k := range survivors {
		if d.take(k) {
			t.Errorf("over-dropped a row the filter did not remove: %+v", k)
		}
	}
}

func TestCallerDropsIgnoreEdgesTargetingAnotherName(t *testing.T) {
	dropped := []graph.TierOneEdge{
		tierOne("c.go", "Consumer", "", "SomethingElse", "calls", 12),
	}
	d := callerDrops(dropped, "Target")
	if d.take(inKey{File: "c.go", QName: "Consumer", Line: 12}) {
		t.Error("a drop for another target name filtered this anchor's caller row")
	}
}

// TestDropSetIsAMultisetNotASet: two edges can share a key, differing only in a
// column the answer row does not carry. Set semantics would drop BOTH rows when
// the filter removed one.
func TestDropSetIsAMultisetNotASet(t *testing.T) {
	dropped := []graph.TierOneEdge{
		tierOne("c.go", "Consumer", "", "Target", "calls", 12),
	}
	d := callerDrops(dropped, "Target")
	k := inKey{File: "c.go", QName: "Consumer", Line: 12}
	if !d.take(k) {
		t.Fatal("first row was not dropped")
	}
	if d.take(k) {
		t.Error("a single dropped edge removed TWO answer rows")
	}

	two := callerDrops([]graph.TierOneEdge{
		tierOne("c.go", "Consumer", "", "Target", "calls", 12),
		tierOne("c.go", "Consumer", "", "Target", "calls", 12),
	}, "Target")
	if !two.take(k) || !two.take(k) {
		t.Error("two dropped edges must remove two answer rows")
	}
	if two.take(k) {
		t.Error("two dropped edges removed a third answer row")
	}
}

func TestCallerDropsKeyCarriesTheParent(t *testing.T) {
	d := callerDrops([]graph.TierOneEdge{
		tierOne("c.go", "handle", "Controller", "Target", "calls", 7),
	}, "Target")
	if d.take(inKey{File: "c.go", QName: "handle", Line: 7}) {
		t.Error("the parent was dropped from the key; a bare name matched a method row")
	}
	if !d.take(inKey{File: "c.go", QName: "Controller.handle", Line: 7}) {
		t.Error("the qualified caller row was not matched")
	}
}

func TestDependentDropsAreKindedAndPathSuffixed(t *testing.T) {
	dropped := []graph.TierOneEdge{
		// (*graph.Store).Dependents matches dst_name = ? OR dst_name LIKE '%/' || ?
		tierOne("c.go", "Consumer", "", "vendor/pkg/Target", "imports", 3),
		// A call edge is not a dependents row.
		tierOne("c.go", "Consumer", "", "Target", "calls", 4),
	}
	d := dependentDrops(dropped, "Target")
	if !d.take(inKey{Kind: "imports", File: "c.go", QName: "Consumer", Line: 3}) {
		t.Error("the suffix-matched import row was not dropped")
	}
	if d.take(inKey{Kind: "calls", File: "c.go", QName: "Consumer", Line: 4}) {
		t.Error("a call edge leaked into the dependents drop index")
	}
}

func TestOutDropsMatchTheAnchorAsSource(t *testing.T) {
	dropped := []graph.TierOneEdge{
		tierOne("a.go", "Anchor", "", "Vendored", "calls", 9),
		tierOne("a.go", "Neighbour", "", "Vendored", "calls", 9),
	}
	d := outDrops(dropped, "Anchor", "", callKinds)
	if !d.take(outKey{Kind: "calls", DstName: "Vendored", Line: 9}) {
		t.Error("the anchor's own dropped callee row was not matched")
	}
	if d.take(outKey{Kind: "calls", DstName: "Vendored", Line: 9}) {
		t.Error("a neighbouring symbol's dropped edge was counted as the anchor's")
	}

	// With a parent, the source match narrows exactly as the per-repo reader's
	// `sc.name = ? AND sc.parent = ?` does.
	withParent := outDrops([]graph.TierOneEdge{
		tierOne("a.go", "run", "Other", "Vendored", "calls", 9),
	}, "run", "Anchor", callKinds)
	if withParent.take(outKey{Kind: "calls", DstName: "Vendored", Line: 9}) {
		t.Error("a same-named method on ANOTHER parent was treated as the anchor")
	}
}

func TestOutDropsRespectTheVerbsKindSet(t *testing.T) {
	dropped := []graph.TierOneEdge{
		tierOne("a.go", "Anchor", "", "Base", "extends", 3),
	}
	if outDrops(dropped, "Anchor", "", callKinds).take(outKey{Kind: "extends", DstName: "Base", Line: 3}) {
		t.Error("an extends edge leaked into the callees drop index")
	}
	if !outDrops(dropped, "Anchor", "", symbolDepKinds).take(outKey{Kind: "extends", DstName: "Base", Line: 3}) {
		t.Error("the deps drop index missed an extends edge")
	}
}

// TestImpactCoverageSentenceMatchesRepoMode guards the one string this package
// duplicates from internal/query. impactCoverage is unexported there and this
// slice may not widen that package's API for a display string, so the constant
// is copied — and a copy with no guard is a drift site. This is the guard: it
// reads the sentence off a real repo-mode answer.
func TestImpactCoverageSentenceMatchesRepoMode(t *testing.T) {
	root := fixtureRepo(t)
	a, err := query.Impact(root, "Target", 10)
	if err != nil {
		t.Fatal(err)
	}
	if a.Coverage != impactCoverage {
		t.Errorf("wsquery's impactCoverage has drifted from internal/query's\n"+
			" wsquery: %q\n   query: %q", impactCoverage, a.Coverage)
	}
	if !strings.Contains(a.Coverage, "type-usage") {
		t.Errorf("repo-mode coverage sentence is unexpectedly %q", a.Coverage)
	}
}

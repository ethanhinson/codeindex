package wsquery

import (
	"path/filepath"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/overlay"
	"codeindex/internal/wsresolve"
)

// --- fixtures --------------------------------------------------------------

// vendoringMember builds a member index holding one project file whose
// function Caller calls Zeta, plus a VENDORED tier-1 copy of namespace
// "acme/lib" that Zeta resolves into. It is the fixture the whole
// member-over-dep story runs on, and it mirrors internal/wsresolve's own
// buildConsumer so the two packages test the same shape.
//
// graph.Open is the BUILD path and is legitimate in test code.
func vendoringMember(t *testing.T, id string) wsresolve.Member {
	t.Helper()
	dir := t.TempDir()

	st, err := graph.Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	pf := &graph.ParsedFile{
		Path: "c.go",
		Symbols: []graph.Symbol{
			{File: "c.go", Name: "Caller", Kind: graph.KindFunc, StartLine: 1, EndLine: 2},
		},
		Calls: []graph.RawCall{{EnclosingIdx: 0, Callee: "Zeta", Line: 10}},
	}
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: "c.go", Hash: "h", Size: 1, Mtime: 1}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	mapPath := filepath.Join(dir, "dep.map.db")
	m, err := graph.Open(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WriteDepMeta("acme/lib", "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	syms := []graph.Symbol{{Name: "Zeta", Kind: graph.KindFunc, StartLine: 1, EndLine: 2}}
	if err := m.PutDepSymbols("acme/lib", "v1.2.3", "z.go", "maphash-z", 1, 1, syms); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	ns, _, names, err := st.AttachMap(mapPath, "vendor/acme/lib")
	if err != nil {
		t.Fatal(err)
	}
	if ns != "acme/lib" {
		t.Fatalf("attached namespace = %q, want acme/lib", ns)
	}
	if err := st.ReResolve(names); err != nil {
		t.Fatal(err)
	}
	return wsresolve.Member{ID: id, Namespaces: []string{"example.com/app"}, Store: st}
}

// vendoredStore is vendoringMember's index alone: an index whose ONLY Zeta is
// a vendored tier-1 snapshot.
func vendoredStore(t *testing.T) *graph.Store {
	t.Helper()
	return vendoringMember(t, "app").Store
}

// ownerMember is a member that owns "acme/lib" and defines defName in it.
func ownerMember(t *testing.T, id, defName string) wsresolve.Member {
	t.Helper()
	st := defsStore(t, symdef{"lib/z.go", defName, "", 4})
	return wsresolve.Member{ID: id, Namespaces: []string{"acme/lib"}, Store: st}
}

// realRecords drives the REAL resolver over the fixture, so the suppression
// records, the cross-edges and the tier-1 edges the filter joins all come from
// the sites that write them in production. Hand-building any of the three keys
// here would make the filter's alignment claim untestable — the join is exactly
// the thing that can silently drift.
func realRecords(t *testing.T, consumer wsresolve.Member, others ...wsresolve.Member) ([]overlay.Suppression, []overlay.CrossEdge, []graph.TierOneEdge) {
	t.Helper()
	members := append([]wsresolve.Member{consumer}, others...)

	sup, err := wsresolve.Suppress(members)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := wsresolve.NewPass().Ladder(consumer, members, sup.Repoint[consumer.ID])
	if err != nil {
		t.Fatal(err)
	}
	tierOne, err := consumer.Store.TierOneEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(tierOne) == 0 {
		t.Fatal("fixture is vacuous: the consumer holds no tier-1 edge to filter")
	}
	return sup.Records, recs.CrossEdges, tierOne
}

// --- the positive case -----------------------------------------------------

// The double-count the recorded obligation exists to prevent: the consumer's
// intra-repo edge into its vendored copy SURVIVES in its own graph.db, and the
// overlay additionally carries a cross-edge from the same call site into the
// owning member. Unioning both would count one call twice, so the intra-repo
// edge is dropped.
//
// Every key in this test is produced by the real resolver, so it also pins the
// alignment claim: TierOneEdges' (src_file, src_name, src_parent, kind, line)
// and the overlay's src_* columns name the same call site.
func TestSuppressedEdgeWithACrossEdgeAtTheSameCallSiteIsDropped(t *testing.T) {
	app := vendoringMember(t, "app")
	lib := ownerMember(t, "lib", "Zeta")

	sup, cross, tierOne := realRecords(t, app, lib)
	if len(sup) != 1 {
		t.Fatalf("fixture: suppressions = %+v, want exactly one", sup)
	}
	if len(cross) != 1 {
		t.Fatalf("fixture: cross edges = %+v, want exactly one", cross)
	}

	kept, dropped := FilterSuppressedEdges("app", sup, tierOne, cross)
	if len(dropped) != 1 {
		t.Fatalf("dropped = %+v, want the double-counted tier-1 edge", dropped)
	}
	if dropped[0].DstNamespace != "acme/lib" {
		t.Fatalf("dropped = %+v, want the acme/lib edge", dropped[0])
	}
	if len(kept) != 0 {
		t.Fatalf("kept = %+v, want nothing left", kept)
	}
}

// --- the characterization: a suppression is NOT the condition --------------

// TestBYDESIGNSuppressionWithoutACrossEdgeKeepsTheIntraRepoEdge pins behaviour
// that is CORRECT, not a limitation awaiting repair. A reader who finds this
// test must not "fix" the survival.
//
// A suppression is recorded on OWNER-UNIQUENESS ALONE. The re-pointed edge only
// becomes a cross-edge when the name ALSO resolves in the owner; when it does
// not, the edge falls through rungs 2-4 and the call site's ONLY edge is the
// surviving intra-repo one. internal/wsresolve's
// TestRepointedEdgeFallsThroughWhenOwnerLacksTheName proves that state is real
// on the merged tree; this test proves the filter does not delete the edge
// there. Absent a cross-edge there is nothing to double-count, so filtering
// would delete a still-correct tier-1 edge and put NOTHING in its place.
//
// The condition is a cross-edge at the same call site, NEVER the suppression
// record alone. Widening the filter to key on the suppression is the failure
// this test exists to catch.
func TestBYDESIGNSuppressionWithoutACrossEdgeKeepsTheIntraRepoEdge(t *testing.T) {
	app := vendoringMember(t, "app")
	// The owner claims acme/lib but does NOT define Zeta, so the suppression
	// stands while the re-pointed edge resolves to nothing.
	lib := ownerMember(t, "lib", "Other")

	sup, cross, tierOne := realRecords(t, app, lib)
	if len(sup) != 1 {
		t.Fatalf("fixture: suppressions = %+v, want the suppression to stand", sup)
	}
	if len(cross) != 0 {
		t.Fatalf("fixture: cross edges = %+v, want none — the owner lacks the name", cross)
	}

	kept, dropped := FilterSuppressedEdges("app", sup, tierOne, cross)
	if len(dropped) != 0 {
		t.Fatalf("dropped = %+v: the intra-repo edge is this call site's ONLY edge", dropped)
	}
	if len(kept) != len(tierOne) {
		t.Fatalf("kept = %+v, want all %d tier-1 edges", kept, len(tierOne))
	}
}

// --- no widening: the join is on all four key parts ------------------------

// Each variant perturbs exactly ONE part of the call-site key, so a filter that
// silently ignores that part reddens here and nowhere else.
func TestCrossEdgeMustMatchEveryPartOfTheCallSiteKey(t *testing.T) {
	app := vendoringMember(t, "app")
	lib := ownerMember(t, "lib", "Zeta")
	sup, cross, tierOne := realRecords(t, app, lib)
	if len(cross) != 1 {
		t.Fatalf("fixture: cross edges = %+v, want exactly one", cross)
	}

	perturb := map[string]func(overlay.CrossEdge) overlay.CrossEdge{
		"different line": func(e overlay.CrossEdge) overlay.CrossEdge {
			e.Line++
			return e
		},
		"different src file": func(e overlay.CrossEdge) overlay.CrossEdge {
			e.Src.File = "other.go"
			return e
		},
		"different src qname": func(e overlay.CrossEdge) overlay.CrossEdge {
			e.Src.QName = "OtherCaller"
			return e
		},
		"different kind": func(e overlay.CrossEdge) overlay.CrossEdge {
			e.Kind = "reference"
			return e
		},
		"different src member": func(e overlay.CrossEdge) overlay.CrossEdge {
			e.Src.Member = "someone-else"
			return e
		},
	}
	for name, mutate := range perturb {
		t.Run(name, func(t *testing.T) {
			_, dropped := FilterSuppressedEdges("app", sup, tierOne, []overlay.CrossEdge{mutate(cross[0])})
			if len(dropped) != 0 {
				t.Fatalf("dropped = %+v: a cross-edge with a %s is a DIFFERENT call site", dropped, name)
			}
		})
	}

	// Control arm: unperturbed, the same inputs DO drop. Without this the five
	// subtests above would pass on a filter that drops nothing at all.
	if _, dropped := FilterSuppressedEdges("app", sup, tierOne, cross); len(dropped) != 1 {
		t.Fatalf("control arm did not drop: %+v", dropped)
	}
}

// The namespace half of the condition. A cross-edge at the same call site is
// not enough on its own: the edge's resolved target must be in a namespace this
// consumer had suppressed.
func TestUnsuppressedNamespaceSurvivesEvenWithACrossEdge(t *testing.T) {
	app := vendoringMember(t, "app")
	lib := ownerMember(t, "lib", "Zeta")
	sup, cross, tierOne := realRecords(t, app, lib)

	other := []overlay.Suppression{{
		ConsumerMember:    "app",
		Namespace:         "example.com/unrelated",
		OwnerMember:       "lib",
		SuppressedVersion: "v1",
	}}
	if _, dropped := FilterSuppressedEdges("app", other, tierOne, cross); len(dropped) != 0 {
		t.Fatalf("dropped = %+v, want none: acme/lib was never suppressed", dropped)
	}

	// A suppression recorded against a DIFFERENT consumer is not this
	// consumer's; the records are consumer-scoped and reading them unscoped
	// would drop another member's still-correct edges.
	foreign := sup
	for i := range foreign {
		foreign[i].ConsumerMember = "somebody-else"
	}
	if _, dropped := FilterSuppressedEdges("app", foreign, tierOne, cross); len(dropped) != 0 {
		t.Fatalf("dropped = %+v, want none: the suppression belongs to another consumer", dropped)
	}
}

// --- the two key builders name the same call site --------------------------

// The join is only sound if both builders agree, so they are asserted against
// each other rather than each against a literal.
func TestCallSiteBuildersAgreeOnTheSameCallSite(t *testing.T) {
	app := vendoringMember(t, "app")
	lib := ownerMember(t, "lib", "Zeta")
	_, cross, tierOne := realRecords(t, app, lib)
	if len(cross) != 1 {
		t.Fatalf("fixture: cross edges = %+v, want exactly one", cross)
	}

	from := SourceCallSite(tierOne[0].UnresolvedEdge)
	to := CrossEdgeCallSite(cross[0])
	if from != to {
		t.Fatalf("call sites disagree:\n  tier-1 edge: %+v\n  cross-edge:  %+v", from, to)
	}
	if from.QName != "Caller" || from.File != "c.go" || from.Kind == "" || from.Line == 0 {
		t.Fatalf("call site = %+v, want the calling symbol's five-part tuple", from)
	}
}

// The key's QName is Parent + "." + Name on both sides, so a parented source
// symbol keys the same way from either shape. Building the tier-1 side from
// SrcName alone would silently un-parent it.
func TestCallSiteQNameCarriesTheParent(t *testing.T) {
	e := graph.UnresolvedEdge{
		SrcFile: "s.ts", SrcName: "method", SrcParent: "outer.Inner",
		Kind: "call", Line: 7,
	}
	got := SourceCallSite(e)
	want := CallSite{File: "s.ts", QName: "outer.Inner.method", Kind: "call", Line: 7}
	if got != want {
		t.Fatalf("call site = %+v, want %+v", got, want)
	}
	if got != CrossEdgeCallSite(overlay.CrossEdge{
		Src:  overlay.SymKey{Member: "m", File: "s.ts", QName: "outer.Inner.method"},
		Kind: "call", Line: 7,
	}) {
		t.Fatal("the two builders disagree on a parented source symbol")
	}
}

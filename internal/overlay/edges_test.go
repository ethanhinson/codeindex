package overlay

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func sym(member, file, qname string) SymKey {
	return SymKey{Member: member, File: file, QName: qname}
}

// edgeSet is a small deterministic fixture spanning three members, written out
// of stable-key order so a read that forgot its ORDER BY would show it.
func edgeSet() []CrossEdge {
	return []CrossEdge{
		{
			Src: sym("web", "a.php", "A::x"), Dst: sym("lib", "b.php", "B::y"),
			Kind: "call", Provenance: "cross_repo_import", Confidence: "exact", Line: 7,
		},
		{
			Src: sym("app", "c.php", "C::z"), Dst: sym("lib", "b.php", "B::y"),
			Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 2,
		},
		{
			Src: sym("app", "c.php", "C::z"), Dst: sym("lib", "b.php", "B::w"),
			Kind: "extends", Provenance: "cross_repo_import", Confidence: "exact", Line: 1,
		},
	}
}

func TestPutCrossEdgesIdempotentAndOrdered(t *testing.T) {
	s := newStore(t)
	edges := edgeSet()
	if err := s.PutCrossEdges(edges); err != nil {
		t.Fatalf("PutCrossEdges: %v", err)
	}
	first := countQ(t, s, `SELECT COUNT(*) FROM cross_edges`)
	if first != len(edges) {
		t.Fatalf("cross_edges = %d, want %d", first, len(edges))
	}
	if err := s.PutCrossEdges(edges); err != nil {
		t.Fatalf("PutCrossEdges (again): %v", err)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_edges`); got != first {
		t.Fatalf("re-put changed row count: %d -> %d", first, got)
	}

	// OutEdges: both app-sourced edges, ordered by dst key then kind then line.
	out, err := s.OutEdges(sym("app", "c.php", "C::z"))
	if err != nil {
		t.Fatalf("OutEdges: %v", err)
	}
	wantOut := []CrossEdge{edges[2], edges[1]} // B::w before B::y
	if !reflect.DeepEqual(out, wantOut) {
		t.Fatalf("OutEdges =\n %+v\nwant\n %+v", out, wantOut)
	}

	// InEdges on lib/B::y: app and web both point at it; ordered by src key.
	in, err := s.InEdges(sym("lib", "b.php", "B::y"))
	if err != nil {
		t.Fatalf("InEdges: %v", err)
	}
	wantIn := []CrossEdge{edges[1], edges[0]} // app before web
	if !reflect.DeepEqual(in, wantIn) {
		t.Fatalf("InEdges =\n %+v\nwant\n %+v", in, wantIn)
	}

	// MemberEdges is either end, and lib is only ever a destination here.
	mem, err := s.MemberEdges("lib")
	if err != nil {
		t.Fatalf("MemberEdges: %v", err)
	}
	wantMem := []CrossEdge{edges[2], edges[1], edges[0]}
	if !reflect.DeepEqual(mem, wantMem) {
		t.Fatalf("MemberEdges(lib) =\n %+v\nwant\n %+v", mem, wantMem)
	}

	// The order must not depend on insertion order: re-putting the fixture
	// reversed leaves every read identical.
	rev := []CrossEdge{edges[2], edges[1], edges[0]}
	if err := s.PutCrossEdges(rev); err != nil {
		t.Fatalf("PutCrossEdges (reversed): %v", err)
	}
	again, err := s.MemberEdges("lib")
	if err != nil {
		t.Fatalf("MemberEdges: %v", err)
	}
	if !reflect.DeepEqual(again, wantMem) {
		t.Fatalf("MemberEdges after reversed write =\n %+v\nwant\n %+v", again, wantMem)
	}
}

func TestOutEdgesEmptyIsNonNil(t *testing.T) {
	s := newStore(t)
	got, err := s.OutEdges(sym("nobody", "x.php", "X"))
	if err != nil {
		t.Fatalf("OutEdges: %v", err)
	}
	if got == nil {
		t.Fatal("OutEdges returned nil slice, want non-nil empty")
	}
	if len(got) != 0 {
		t.Fatalf("OutEdges = %+v, want empty", got)
	}
}

// TestPutCrossEdgesRungPrecedence is decision 17: D3's ladder puts rung 1
// (import-mediated, exact) above rung 2 (name-matched, inferred), so an exact
// upgrades an inferred and an inferred never demotes an exact — whichever rung
// happens to emit first.
func TestPutCrossEdgesRungPrecedence(t *testing.T) {
	inferred := CrossEdge{
		Src: sym("app", "c.php", "C::z"), Dst: sym("lib", "b.php", "B::y"),
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 4,
	}
	exact := inferred
	exact.Provenance = "cross_repo_import"
	exact.Confidence = "exact"

	for _, tc := range []struct {
		name  string
		order []CrossEdge
	}{
		{"inferred then exact", []CrossEdge{inferred, exact}},
		{"exact then inferred", []CrossEdge{exact, inferred}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t)
			for _, e := range tc.order {
				if err := s.PutCrossEdges([]CrossEdge{e}); err != nil {
					t.Fatalf("PutCrossEdges: %v", err)
				}
			}
			if got := countQ(t, s, `SELECT COUNT(*) FROM cross_edges`); got != 1 {
				t.Fatalf("cross_edges = %d, want 1", got)
			}
			got, err := s.OutEdges(inferred.Src)
			if err != nil {
				t.Fatalf("OutEdges: %v", err)
			}
			if !reflect.DeepEqual(got, []CrossEdge{exact}) {
				t.Fatalf("stored edge = %+v, want %+v", got, exact)
			}
		})
	}
}

// TestPutCrossEdgesRungPrecedenceWithinOneCall covers both rungs arriving in
// the same batch, which is what a single ladder pass actually emits.
func TestPutCrossEdgesRungPrecedenceWithinOneCall(t *testing.T) {
	inferred := CrossEdge{
		Src: sym("app", "c.php", "C::z"), Dst: sym("lib", "b.php", "B::y"),
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 4,
	}
	exact := inferred
	exact.Provenance = "cross_repo_import"
	exact.Confidence = "exact"

	s := newStore(t)
	if err := s.PutCrossEdges([]CrossEdge{exact, inferred}); err != nil {
		t.Fatalf("PutCrossEdges: %v", err)
	}
	got, err := s.OutEdges(inferred.Src)
	if err != nil {
		t.Fatalf("OutEdges: %v", err)
	}
	if !reflect.DeepEqual(got, []CrossEdge{exact}) {
		t.Fatalf("stored edge = %+v, want %+v", got, exact)
	}
}

func ambiguitySet() []Ambiguity {
	return []Ambiguity{
		{
			Src: sym("app", "c.php", "C::z"), RefName: "Thing", RefNS: "Acme",
			Kind: "call", Line: 9,
			Candidates: []SymKey{
				sym("lib", "b.php", "B::y"),
				sym("web", "a.php", "A::x"),
			},
			Count: 2,
		},
		{
			Src: sym("app", "c.php", "C::z"), RefName: "Other", RefNS: "",
			Kind: "call", Line: 3,
			Candidates: []SymKey{sym("lib", "b.php", "B::w")},
			// Count above len(Candidates): upstream truncated the list.
			Count: 5,
		},
	}
}

func TestPutAmbiguitiesRoundTripAndCandidateIdempotence(t *testing.T) {
	s := newStore(t)
	amb := ambiguitySet()
	if err := s.PutAmbiguities(amb); err != nil {
		t.Fatalf("PutAmbiguities: %v", err)
	}
	// Direct row counts, not the read API: decision 18's orphan growth is
	// invisible to every read.
	firstAmbig := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities`)
	firstCands := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates`)
	if firstAmbig != 2 || firstCands != 3 {
		t.Fatalf("after first put: ambiguities = %d (want 2), candidates = %d (want 3)",
			firstAmbig, firstCands)
	}

	got, err := s.AmbiguitiesFor("app")
	if err != nil {
		t.Fatalf("AmbiguitiesFor: %v", err)
	}
	want := []Ambiguity{amb[1], amb[0]} // "Other" sorts before "Thing"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AmbiguitiesFor =\n %+v\nwant\n %+v", got, want)
	}

	// Re-putting the same input must not mint new candidate rows. INSERT OR
	// REPLACE would give the ambiguity a fresh rowid and strand the old
	// candidates forever, with no foreign key to collect them.
	if err := s.PutAmbiguities(amb); err != nil {
		t.Fatalf("PutAmbiguities (again): %v", err)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities`); got != firstAmbig {
		t.Fatalf("cross_ambiguities grew: %d -> %d", firstAmbig, got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates`); got != firstCands {
		t.Fatalf("cross_ambiguity_candidates grew (orphaned rows): %d -> %d", firstCands, got)
	}
	// Every surviving candidate must still belong to a live ambiguity.
	if n := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates c
	  WHERE NOT EXISTS (SELECT 1 FROM cross_ambiguities a WHERE a.id = c.ambiguity_id)`); n != 0 {
		t.Fatalf("%d orphaned candidate rows", n)
	}
	after, err := s.AmbiguitiesFor("app")
	if err != nil {
		t.Fatalf("AmbiguitiesFor: %v", err)
	}
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("AmbiguitiesFor after re-put =\n %+v\nwant\n %+v", after, want)
	}
}

// TestPutAmbiguitiesShrinkingCandidateList is the same orphan hazard from the
// other side: a re-put with fewer candidates must leave exactly the new ones.
func TestPutAmbiguitiesShrinkingCandidateList(t *testing.T) {
	s := newStore(t)
	amb := ambiguitySet()[:1]
	if err := s.PutAmbiguities(amb); err != nil {
		t.Fatalf("PutAmbiguities: %v", err)
	}
	shrunk := amb[0]
	shrunk.Candidates = []SymKey{sym("lib", "b.php", "B::y")}
	shrunk.Count = 1
	if err := s.PutAmbiguities([]Ambiguity{shrunk}); err != nil {
		t.Fatalf("PutAmbiguities (shrunk): %v", err)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates`); got != 1 {
		t.Fatalf("cross_ambiguity_candidates = %d, want 1", got)
	}
	got, err := s.AmbiguitiesFor("app")
	if err != nil {
		t.Fatalf("AmbiguitiesFor: %v", err)
	}
	if !reflect.DeepEqual(got, []Ambiguity{shrunk}) {
		t.Fatalf("AmbiguitiesFor =\n %+v\nwant\n %+v", got, shrunk)
	}
}

func TestPutAmbiguitiesRejectsCountBelowCandidates(t *testing.T) {
	s := newStore(t)
	bad := Ambiguity{
		Src: sym("app", "c.php", "C::z"), RefName: "Thing", Kind: "call", Line: 1,
		Candidates: []SymKey{sym("lib", "b.php", "B::y"), sym("web", "a.php", "A::x")},
		Count:      1,
	}
	// A good record ahead of the bad one: validation runs before any write, so
	// nothing at all lands.
	good := ambiguitySet()[0]
	err := s.PutAmbiguities([]Ambiguity{good, bad})
	if err == nil {
		t.Fatal("PutAmbiguities accepted Count < len(Candidates)")
	}
	for _, n := range []string{"1", "2"} {
		if !strings.Contains(err.Error(), n) {
			t.Fatalf("error %q does not name both numbers", err)
		}
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities`); got != 0 {
		t.Fatalf("a rejected batch half-applied: %d ambiguity rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates`); got != 0 {
		t.Fatalf("a rejected batch half-applied: %d candidate rows", got)
	}
}

func suppressionSet() []Suppression {
	return []Suppression{
		{ConsumerMember: "app", Namespace: "Acme\\Lib", OwnerMember: "lib", SuppressedVersion: "1.0"},
		{ConsumerMember: "app", Namespace: "Acme\\Web", OwnerMember: "web", SuppressedVersion: ""},
		{ConsumerMember: "web", Namespace: "Acme\\Lib", OwnerMember: "lib", SuppressedVersion: "2.0"},
	}
}

func TestPutSuppressionsUpsertsAndOrders(t *testing.T) {
	s := newStore(t)
	sup := suppressionSet()
	if err := s.PutSuppressions(sup); err != nil {
		t.Fatalf("PutSuppressions: %v", err)
	}
	if err := s.PutSuppressions(sup); err != nil {
		t.Fatalf("PutSuppressions (again): %v", err)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM dep_suppressions`); got != 3 {
		t.Fatalf("dep_suppressions = %d, want 3", got)
	}
	got, err := s.Suppressions()
	if err != nil {
		t.Fatalf("Suppressions: %v", err)
	}
	if !reflect.DeepEqual(got, sup) {
		t.Fatalf("Suppressions =\n %+v\nwant\n %+v", got, sup)
	}

	// Same primary key, new payload: an update, not a second row.
	moved := Suppression{ConsumerMember: "app", Namespace: "Acme\\Lib", OwnerMember: "lib2", SuppressedVersion: "9.9"}
	if err := s.PutSuppressions([]Suppression{moved}); err != nil {
		t.Fatalf("PutSuppressions (moved): %v", err)
	}
	got, err = s.Suppressions()
	if err != nil {
		t.Fatalf("Suppressions: %v", err)
	}
	want := []Suppression{moved, sup[1], sup[2]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Suppressions =\n %+v\nwant\n %+v", got, want)
	}
}

// seedForReplace fills the overlay with edges, ambiguities and suppressions
// touching "web" as src, as dst, as consumer and as owner.
func seedForReplace(t *testing.T, s *Store) {
	t.Helper()
	if err := s.PutCrossEdges(edgeSet()); err != nil {
		t.Fatalf("seed edges: %v", err)
	}
	if err := s.PutCrossEdges([]CrossEdge{{
		Src: sym("lib", "b.php", "B::y"), Dst: sym("web", "a.php", "A::x"),
		Kind: "call", Provenance: "cross_repo_import", Confidence: "exact", Line: 5,
	}}); err != nil {
		t.Fatalf("seed inbound edge: %v", err)
	}
	if err := s.PutAmbiguities([]Ambiguity{
		{
			Src: sym("web", "a.php", "A::x"), RefName: "Thing", RefNS: "Acme",
			Kind: "call", Line: 3,
			Candidates: []SymKey{sym("lib", "b.php", "B::y")}, Count: 1,
		},
		{
			Src: sym("app", "c.php", "C::z"), RefName: "Thing", RefNS: "Acme",
			Kind: "call", Line: 4,
			Candidates: []SymKey{sym("web", "a.php", "A::x"), sym("lib", "b.php", "B::y")}, Count: 2,
		},
	}); err != nil {
		t.Fatalf("seed ambiguities: %v", err)
	}
	if err := s.PutSuppressions(suppressionSet()); err != nil {
		t.Fatalf("seed suppressions: %v", err)
	}
}

// assertNoOrphanCandidates checks the raw candidates table for rows whose
// parent ambiguity is gone — decision 18's hazard, invisible to every read.
func assertNoOrphanCandidates(t *testing.T, s *Store) {
	t.Helper()
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates c
	  WHERE NOT EXISTS (SELECT 1 FROM cross_ambiguities a WHERE a.id = c.ambiguity_id)`); got != 0 {
		t.Fatalf("%d orphaned candidate rows", got)
	}
}

// assertCountsMatchCandidates checks the other half of the same invariant: no
// surviving ambiguity may store a candidate_count below its own candidate rows.
// A count *above* is legal (upstream truncation); a count below means a
// candidate list was thinned underneath a row that still claims the old size.
func assertCountsMatchCandidates(t *testing.T, s *Store) {
	t.Helper()
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities a
	  WHERE a.candidate_count <
	        (SELECT COUNT(*) FROM cross_ambiguity_candidates c WHERE c.ambiguity_id = a.id)`); got != 0 {
		t.Fatalf("%d ambiguities whose candidate_count is below their own candidate rows", got)
	}
}

// TestReplaceMemberEdgesDeletesCandidateSideAmbiguities is the either-end rule
// for cross_ambiguities: re-resolving M may rename or delete any symbol M owns,
// so an ambiguity sourced from another member N that names M among its
// candidates is stale the moment M is replaced — and nothing else would ever
// clear it, since ReplaceMemberEdges(N) only fires when N re-resolves and
// registry pruning only fires when M leaves the manifest. The whole parent goes:
// thinning just M's candidate row would leave N's stored candidate_count
// contradicting its surviving rows.
func TestReplaceMemberEdgesDeletesCandidateSideAmbiguities(t *testing.T) {
	s := newStore(t)

	namesWeb := Ambiguity{
		Src: sym("app", "c.php", "C::z"), RefName: "Thing", RefNS: "Acme",
		Kind: "call", Line: 4,
		Candidates: []SymKey{sym("lib", "b.php", "B::y"), sym("web", "a.php", "A::x")},
		Count:      2,
	}
	// Same source member, no mention of web: the delete must be scoped to
	// incidence, not swept across N's whole contribution.
	untouched := Ambiguity{
		Src: sym("app", "c.php", "C::q"), RefName: "Other", RefNS: "",
		Kind: "call", Line: 8,
		Candidates: []SymKey{sym("lib", "b.php", "B::w")},
		Count:      3, // above len(Candidates): upstream truncated
	}
	if err := s.PutAmbiguities([]Ambiguity{namesWeb, untouched}); err != nil {
		t.Fatalf("PutAmbiguities: %v", err)
	}

	if err := s.ReplaceMemberEdges("web", nil, nil, nil); err != nil {
		t.Fatalf("ReplaceMemberEdges: %v", err)
	}

	got, err := s.AmbiguitiesFor("app")
	if err != nil {
		t.Fatalf("AmbiguitiesFor(app): %v", err)
	}
	if !reflect.DeepEqual(got, []Ambiguity{untouched}) {
		t.Fatalf("AmbiguitiesFor(app) =\n %+v\nwant only\n %+v", got, untouched)
	}

	// Raw rows: the parent ambiguity and BOTH of its candidate rows are gone —
	// the list was deleted with its parent, never thinned in place.
	if n := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities`); n != 1 {
		t.Fatalf("cross_ambiguities = %d, want 1", n)
	}
	if n := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates`); n != 1 {
		t.Fatalf("cross_ambiguity_candidates = %d, want 1", n)
	}
	if n := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates WHERE member_id='web'`); n != 0 {
		t.Fatalf("%d candidate rows still name the replaced member", n)
	}
	assertNoOrphanCandidates(t, s)
	assertCountsMatchCandidates(t, s)
}

// TestReplaceMemberEdgesManyIncidentAmbiguities crosses the id-chunking
// boundary of the either-end delete: more incident ambiguities than one DELETE
// binds at a time, so a chunk loop that dropped its tail — or a single bound
// statement that blew the host-parameter limit — shows up here.
func TestReplaceMemberEdgesManyIncidentAmbiguities(t *testing.T) {
	s := newStore(t)
	const n = ambiguityDeleteChunk*2 + 3
	amb := make([]Ambiguity, 0, n+1)
	for i := range n {
		amb = append(amb, Ambiguity{
			Src: sym("app", "c.php", "C::z"), RefName: fmt.Sprintf("Ref%d", i),
			Kind: "call", Line: i,
			Candidates: []SymKey{sym("web", "a.php", "A::x")}, Count: 1,
		})
	}
	keep := Ambiguity{
		Src: sym("app", "c.php", "C::z"), RefName: "Keep", Kind: "call", Line: n,
		Candidates: []SymKey{sym("lib", "b.php", "B::y")}, Count: 1,
	}
	amb = append(amb, keep)
	if err := s.PutAmbiguities(amb); err != nil {
		t.Fatalf("PutAmbiguities: %v", err)
	}

	if err := s.ReplaceMemberEdges("web", nil, nil, nil); err != nil {
		t.Fatalf("ReplaceMemberEdges: %v", err)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities`); got != 1 {
		t.Fatalf("cross_ambiguities = %d, want 1 (only the ambiguity not naming web)", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates`); got != 1 {
		t.Fatalf("cross_ambiguity_candidates = %d, want 1", got)
	}
	assertNoOrphanCandidates(t, s)
	assertCountsMatchCandidates(t, s)
}

func TestReplaceMemberEdges(t *testing.T) {
	s := newStore(t)
	seedForReplace(t, s)

	newEdge := CrossEdge{
		Src: sym("web", "a.php", "A::x"), Dst: sym("app", "c.php", "C::z"),
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 11,
	}
	newAmbig := Ambiguity{
		Src: sym("web", "a.php", "A::x"), RefName: "Fresh", RefNS: "Acme",
		Kind: "call", Line: 12,
		Candidates: []SymKey{sym("app", "c.php", "C::z")}, Count: 1,
	}
	newSup := Suppression{ConsumerMember: "web", Namespace: "Acme\\App", OwnerMember: "app", SuppressedVersion: "3.0"}

	if err := s.ReplaceMemberEdges("web", []CrossEdge{newEdge}, []Ambiguity{newAmbig},
		[]Suppression{newSup}); err != nil {
		t.Fatalf("ReplaceMemberEdges: %v", err)
	}

	// Cross-edges: both prior web-incident edges (src side and dst side) are
	// gone; the app->lib edges that never named web survive.
	edges, err := s.MemberEdges("web")
	if err != nil {
		t.Fatalf("MemberEdges: %v", err)
	}
	if !reflect.DeepEqual(edges, []CrossEdge{newEdge}) {
		t.Fatalf("MemberEdges(web) =\n %+v\nwant\n %+v", edges, []CrossEdge{newEdge})
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_edges`); got != 3 {
		t.Fatalf("cross_edges = %d, want 3 (2 app-sourced survivors + 1 new)", got)
	}

	// Ambiguities are replaced on either end: the web-sourced one is rewritten,
	// and app's — whose candidate list names web — is deleted outright, because
	// web's rebuild may have renamed or deleted the symbol that candidate points
	// at and no other code path would ever remove it.
	ambs, err := s.AmbiguitiesFor("web")
	if err != nil {
		t.Fatalf("AmbiguitiesFor(web): %v", err)
	}
	if !reflect.DeepEqual(ambs, []Ambiguity{newAmbig}) {
		t.Fatalf("AmbiguitiesFor(web) =\n %+v\nwant\n %+v", ambs, []Ambiguity{newAmbig})
	}
	appAmbs, err := s.AmbiguitiesFor("app")
	if err != nil {
		t.Fatalf("AmbiguitiesFor(app): %v", err)
	}
	if len(appAmbs) != 0 {
		t.Fatalf("app's ambiguity names web among its candidates and must be deleted whole: %+v", appAmbs)
	}
	// Raw rows, not the read API: only the new ambiguity and its one candidate
	// remain, with nothing stranded behind them.
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities`); got != 1 {
		t.Fatalf("cross_ambiguities = %d, want 1 (only the new web-sourced row)", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates`); got != 1 {
		t.Fatalf("cross_ambiguity_candidates = %d, want 1", got)
	}
	assertNoOrphanCandidates(t, s)
	assertCountsMatchCandidates(t, s)

	// Suppressions: consumer_member = web is cleared and rewritten; the row
	// where web is only the owner_member is deliberately left alone.
	sups, err := s.Suppressions()
	if err != nil {
		t.Fatalf("Suppressions: %v", err)
	}
	all := suppressionSet()
	want := []Suppression{all[0], all[1], newSup}
	if !reflect.DeepEqual(sups, want) {
		t.Fatalf("Suppressions =\n %+v\nwant\n %+v", sups, want)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM dep_suppressions WHERE owner_member='web'`); got != 1 {
		t.Fatalf("owner-side suppression for web = %d rows, want 1 (left alone)", got)
	}
}

func TestReplaceMemberEdgesRejectsForeignSuppression(t *testing.T) {
	s := newStore(t)
	seedForReplace(t, s)
	before := countQ(t, s, `SELECT COUNT(*) FROM cross_edges`)

	foreign := Suppression{ConsumerMember: "app", Namespace: "Acme\\X", OwnerMember: "web"}
	err := s.ReplaceMemberEdges("web", nil, nil, []Suppression{foreign})
	if err == nil {
		t.Fatal("ReplaceMemberEdges accepted a suppression for another consumer")
	}
	if !strings.Contains(err.Error(), "app") || !strings.Contains(err.Error(), "web") {
		t.Fatalf("error %q does not name both members", err)
	}
	// Rejected before any write: the seeded state is intact.
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_edges`); got != before {
		t.Fatalf("cross_edges = %d, want %d — the rejected call still deleted", got, before)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM dep_suppressions WHERE namespace='Acme\X'`); got != 0 {
		t.Fatalf("the rejected suppression was written")
	}
}

func TestReplaceMemberEdgesRejectsShortAmbiguityCount(t *testing.T) {
	s := newStore(t)
	seedForReplace(t, s)
	before := countQ(t, s, `SELECT COUNT(*) FROM cross_edges`)

	bad := Ambiguity{
		Src: sym("web", "a.php", "A::x"), RefName: "Bad", Kind: "call", Line: 1,
		Candidates: []SymKey{sym("lib", "b.php", "B::y")}, Count: 0,
	}
	if err := s.ReplaceMemberEdges("web", nil, []Ambiguity{bad}, nil); err == nil {
		t.Fatal("ReplaceMemberEdges accepted Count < len(Candidates)")
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_edges`); got != before {
		t.Fatalf("cross_edges = %d, want %d — the rejected call still deleted", got, before)
	}
}

// TestReplaceMemberEdgesEmptyClears is the freshen case where a member turns
// out to contribute nothing: its prior rows must still go.
func TestReplaceMemberEdgesEmptyClears(t *testing.T) {
	s := newStore(t)
	seedForReplace(t, s)
	if err := s.ReplaceMemberEdges("web", nil, nil, nil); err != nil {
		t.Fatalf("ReplaceMemberEdges: %v", err)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_edges WHERE src_member='web' OR dst_member='web'`); got != 0 {
		t.Fatalf("web-incident cross-edges survived: %d", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities WHERE src_member='web'`); got != 0 {
		t.Fatalf("web-sourced ambiguities survived: %d", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates WHERE member_id='web'`); got != 0 {
		t.Fatalf("candidate rows naming web survived: %d", got)
	}
	assertNoOrphanCandidates(t, s)
	assertCountsMatchCandidates(t, s)
	if got := countQ(t, s, `SELECT COUNT(*) FROM dep_suppressions WHERE consumer_member='web'`); got != 0 {
		t.Fatalf("web consumer suppressions survived: %d", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM dep_suppressions WHERE owner_member='web'`); got != 1 {
		t.Fatalf("owner-side suppression for web = %d rows, want 1 (left alone)", got)
	}
}

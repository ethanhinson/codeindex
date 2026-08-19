package wsresolve

import (
	"path/filepath"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/overlay"
)

// def is one tier-0 definition to plant in a fixture member index.
type def struct {
	file   string
	name   string
	parent string
	line   int
}

// newMember builds a small hand-made member index in a temp dir. graph.Open is
// legitimate here: it is the BUILD path, banned only from non-test code.
func newMember(t *testing.T, id string, namespaces, deps []string, defs ...def) Member {
	t.Helper()
	st, err := graph.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	byFile := map[string][]graph.Symbol{}
	var order []string
	for _, d := range defs {
		line := d.line
		if line == 0 {
			line = 1
		}
		if _, seen := byFile[d.file]; !seen {
			order = append(order, d.file)
		}
		byFile[d.file] = append(byFile[d.file], graph.Symbol{
			File: d.file, Name: d.name, Parent: d.parent,
			Kind: graph.KindFunc, StartLine: line, EndLine: line + 1,
		})
	}
	for _, f := range order {
		tx, err := st.Begin()
		if err != nil {
			t.Fatal(err)
		}
		pf := &graph.ParsedFile{Path: f, Symbols: byFile[f]}
		if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: f, Hash: "h", Size: 1, Mtime: 1}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	return Member{ID: id, Namespaces: namespaces, Deps: deps, Store: st}
}

func edge(name, qualifier, ns string) graph.UnresolvedEdge {
	return graph.UnresolvedEdge{
		SrcFile: "s.go", SrcName: "Caller", DstName: name,
		DstQualifier: qualifier, DstNS: ns, Kind: "call", Line: 7,
	}
}

func srcKey() overlay.SymKey {
	return overlay.SymKey{Member: "s", File: "s.go", QName: "Caller"}
}

func run(t *testing.T, src Member, members []Member, edges ...graph.UnresolvedEdge) Records {
	t.Helper()
	got, err := NewPass().Ladder(src, members, edges)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func wantOneCross(t *testing.T, got Records, want overlay.CrossEdge) {
	t.Helper()
	if len(got.Ambiguities) != 0 {
		t.Fatalf("ambiguities = %+v, want none", got.Ambiguities)
	}
	if len(got.CrossEdges) != 1 {
		t.Fatalf("cross edges = %+v, want exactly 1", got.CrossEdges)
	}
	if got.CrossEdges[0] != want {
		t.Fatalf("cross edge = %+v, want %+v", got.CrossEdges[0], want)
	}
}

// --- rung 1 ---------------------------------------------------------------

func TestRungOneImportMediatedExact(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Boot", "", 3})
	b := newMember(t, "b", []string{"example.com/b"}, nil)

	got := run(t, src, []Member{src, a, b}, edge("Boot", "", "example.com/a/deep"))
	wantOneCross(t, got, overlay.CrossEdge{
		Src:  srcKey(),
		Dst:  overlay.SymKey{Member: "a", File: "a/a.go", QName: "Boot"},
		Kind: "call", Provenance: "cross_repo_import", Confidence: "exact", Line: 7,
	})
}

// The hint matches TWO members, so rung 1 cannot select one: the ladder falls
// through to rungs 2/3. Both members define Boot once, so rung 2 has two hits
// and rung 3 records the ambiguity.
func TestRungOneMissHintMatchesTwoMembers(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"shared/ns"}, nil, def{"a/a.go", "Boot", "", 1})
	b := newMember(t, "b", []string{"shared/ns"}, nil, def{"b/b.go", "Boot", "", 1})

	got := run(t, src, []Member{src, a, b}, edge("Boot", "", "shared/ns/x"))
	if len(got.CrossEdges) != 0 {
		t.Fatalf("cross edges = %+v, want none", got.CrossEdges)
	}
	if len(got.Ambiguities) != 1 || got.Ambiguities[0].Count != 2 {
		t.Fatalf("ambiguities = %+v", got.Ambiguities)
	}
	// A rung-3 record may carry a non-empty RefNS — see Ladder's doc comment.
	if got.Ambiguities[0].RefNS != "shared/ns/x" {
		t.Fatalf("RefNS = %q, want the hint preserved", got.Ambiguities[0].RefNS)
	}
}

// The hint selects exactly one member, but N is ambiguous INSIDE it, so rung 1
// misses. Rung 2 then finds no member defining N exactly once (the hinted one
// has two), and rung 3 records both.
func TestRungOneMissAmbiguousInsideHintedMember(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"example.com/a"}, nil,
		def{"a/one.go", "Boot", "", 1}, def{"a/two.go", "Boot", "", 1})
	b := newMember(t, "b", []string{"example.com/b"}, nil)

	got := run(t, src, []Member{src, a, b}, edge("Boot", "", "example.com/a"))
	if len(got.CrossEdges) != 0 {
		t.Fatalf("cross edges = %+v, want none", got.CrossEdges)
	}
	if len(got.Ambiguities) != 1 || got.Ambiguities[0].Count != 2 {
		t.Fatalf("ambiguities = %+v", got.Ambiguities)
	}
	want := []overlay.SymKey{
		{Member: "a", File: "a/one.go", QName: "Boot"},
		{Member: "a", File: "a/two.go", QName: "Boot"},
	}
	assertCands(t, got.Ambiguities[0].Candidates, want)
}

// Rung 1 beats rung 2 when both could fire: only member a is hinted, but a is
// also the only member defining N once, so rung 2 would produce the same edge
// at a weaker class. The recorded class must be exact.
func TestRungOneBeatsRungTwo(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Boot", "", 1})

	got := run(t, src, []Member{src, a}, edge("Boot", "", "example.com/a"))
	wantOneCross(t, got, overlay.CrossEdge{
		Src:  srcKey(),
		Dst:  overlay.SymKey{Member: "a", File: "a/a.go", QName: "Boot"},
		Kind: "call", Provenance: "cross_repo_import", Confidence: "exact", Line: 7,
	})

	// Same workspace, hint dropped: rung 2 fires and the class drops.
	got = run(t, src, []Member{src, a}, edge("Boot", "", ""))
	wantOneCross(t, got, overlay.CrossEdge{
		Src:  srcKey(),
		Dst:  overlay.SymKey{Member: "a", File: "a/a.go", QName: "Boot"},
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 7,
	})
}

// Rung 1's namespace matching is asserted against the SHARED nsPrefixCases
// table, so this call site and nsPrefix itself can never drift apart. A
// matching hint yields an exact edge into the hinted member; a non-matching one
// leaves both members merely name-hits and falls to rung 3.
func TestRungOneUsesSharedNsPrefixTable(t *testing.T) {
	for _, c := range nsPrefixCases() {
		t.Run(c.name, func(t *testing.T) {
			src := newMember(t, "s", []string{"src.only"}, nil)
			hinted := newMember(t, "hinted", []string{c.ns}, nil, def{"h.go", "Boot", "", 1})
			// A namespace no table hint can prefix-match.
			other := newMember(t, "other", []string{"zzz-unmatchable"}, nil, def{"o.go", "Boot", "", 1})

			got := run(t, src, []Member{src, hinted, other}, edge("Boot", "", c.h))
			if c.want {
				wantOneCross(t, got, overlay.CrossEdge{
					Src:  srcKey(),
					Dst:  overlay.SymKey{Member: "hinted", File: "h.go", QName: "Boot"},
					Kind: "call", Provenance: "cross_repo_import", Confidence: "exact", Line: 7,
				})
				return
			}
			if len(got.CrossEdges) != 0 {
				t.Fatalf("nsPrefix(%q,%q) is false but rung 1 fired: %+v", c.ns, c.h, got.CrossEdges)
			}
			if len(got.Ambiguities) != 1 || got.Ambiguities[0].Count != 2 {
				t.Fatalf("ambiguities = %+v", got.Ambiguities)
			}
		})
	}
}

// --- rung 2 ---------------------------------------------------------------

func TestRungTwoUniqueBareName(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Serve", "Server", 4})
	b := newMember(t, "b", []string{"example.com/b"}, nil, def{"b/b.go", "Other", "", 1})

	got := run(t, src, []Member{src, a, b}, edge("Serve", "", ""))
	wantOneCross(t, got, overlay.CrossEdge{
		Src:  srcKey(),
		Dst:  overlay.SymKey{Member: "a", File: "a/a.go", QName: "Server.Serve"},
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 7,
	})
}

// Rung 2 fires on a rung-1 MISS, not on "H is literally empty": the hint here
// names nobody, and the unique bare name still wins.
func TestRungTwoFiresAfterRungOneMissWithNonEmptyHint(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Boot", "", 1})

	got := run(t, src, []Member{src, a}, edge("Boot", "", "third.party/unowned"))
	wantOneCross(t, got, overlay.CrossEdge{
		Src:  srcKey(),
		Dst:  overlay.SymKey{Member: "a", File: "a/a.go", QName: "Boot"},
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 7,
	})
}

// --- rung 3 ---------------------------------------------------------------

func assertCands(t *testing.T, got, want []overlay.SymKey) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("candidates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %+v, want %+v (full: %+v)", i, got[i], want[i], got)
		}
	}
}

// ambigWorkspace: three candidate members, b and c each with one definition,
// a with two, so ordering within a member is observable too.
func ambigWorkspace(t *testing.T, srcDeps []string) (Member, []Member) {
	t.Helper()
	src := newMember(t, "s", []string{"example.com/s"}, srcDeps)
	a := newMember(t, "a", []string{"example.com/a"}, nil,
		def{"a/two.go", "Boot", "", 1}, def{"a/one.go", "Boot", "", 1})
	b := newMember(t, "b", []string{"example.com/b"}, nil, def{"b/b.go", "Boot", "", 1})
	c := newMember(t, "c", []string{"example.com/c"}, nil, def{"c/c.go", "Boot", "", 1})
	return src, []Member{src, a, b, c}
}

func TestRungThreeNoDepsTiebreakerIsManifestOrder(t *testing.T) {
	src, members := ambigWorkspace(t, nil)
	got := run(t, src, members, edge("Boot", "", ""))
	if len(got.CrossEdges) != 0 {
		t.Fatalf("cross edges = %+v, want none", got.CrossEdges)
	}
	if len(got.Ambiguities) != 1 {
		t.Fatalf("ambiguities = %+v", got.Ambiguities)
	}
	am := got.Ambiguities[0]
	assertCands(t, am.Candidates, []overlay.SymKey{
		{Member: "a", File: "a/one.go", QName: "Boot"},
		{Member: "a", File: "a/two.go", QName: "Boot"},
		{Member: "b", File: "b/b.go", QName: "Boot"},
		{Member: "c", File: "c/c.go", QName: "Boot"},
	})
	if am.Count != len(am.Candidates) {
		t.Fatalf("Count = %d, want %d", am.Count, len(am.Candidates))
	}
	if am.Src != srcKey() || am.RefName != "Boot" || am.RefNS != "" || am.Kind != "call" || am.Line != 7 {
		t.Fatalf("ambiguity header = %+v", am)
	}
}

func TestRungThreeDepsTiebreakerOrdersNamedMemberFirst(t *testing.T) {
	src, members := ambigWorkspace(t, []string{"c"})
	got := run(t, src, members, edge("Boot", "", ""))
	assertCands(t, got.Ambiguities[0].Candidates, []overlay.SymKey{
		{Member: "c", File: "c/c.go", QName: "Boot"},
		{Member: "a", File: "a/one.go", QName: "Boot"},
		{Member: "a", File: "a/two.go", QName: "Boot"},
		{Member: "b", File: "b/b.go", QName: "Boot"},
	})
}

// deps naming TWO candidate members is not a tiebreaker at all: no reorder.
func TestRungThreeDepsNamingTwoCandidatesDoesNotReorder(t *testing.T) {
	src, members := ambigWorkspace(t, []string{"b", "c"})
	got := run(t, src, members, edge("Boot", "", ""))
	assertCands(t, got.Ambiguities[0].Candidates, []overlay.SymKey{
		{Member: "a", File: "a/one.go", QName: "Boot"},
		{Member: "a", File: "a/two.go", QName: "Boot"},
		{Member: "b", File: "b/b.go", QName: "Boot"},
		{Member: "c", File: "c/c.go", QName: "Boot"},
	})
}

// A dep naming a member that contributes no candidate is not "the single
// candidate member named in deps", so it cannot reorder anything.
func TestRungThreeDepsNamingNonCandidateDoesNotReorder(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, []string{"empty"})
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Boot", "", 1})
	b := newMember(t, "b", []string{"example.com/b"}, nil, def{"b/b.go", "Boot", "", 1})
	empty := newMember(t, "empty", []string{"example.com/e"}, nil)

	got := run(t, src, []Member{src, empty, a, b}, edge("Boot", "", ""))
	assertCands(t, got.Ambiguities[0].Candidates, []overlay.SymKey{
		{Member: "a", File: "a/a.go", QName: "Boot"},
		{Member: "b", File: "b/b.go", QName: "Boot"},
	})
}

// --- rung 4 ---------------------------------------------------------------

func TestRungFourRecordsNothing(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil, def{"s.go", "Boot", "", 1})
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Other", "", 1})

	// Nobody else defines Boot — and src's own definition must not count.
	got := run(t, src, []Member{src, a}, edge("Boot", "", "example.com/s"))
	if len(got.CrossEdges) != 0 || len(got.Ambiguities) != 0 {
		t.Fatalf("want no records, got %+v", got)
	}
}

// --- workspace-level invariants -------------------------------------------

// The degenerate one-member workspace: every rung requires a member other than
// src, so nothing is ever recorded, not even for a name src defines itself.
func TestSingleMemberWorkspaceRecordsNothing(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil,
		def{"s.go", "Boot", "", 1}, def{"s2.go", "Boot", "", 1})
	got := run(t, src, []Member{src},
		edge("Boot", "", "example.com/s"), edge("Boot", "", ""))
	if len(got.CrossEdges) != 0 || len(got.Ambiguities) != 0 {
		t.Fatalf("want no records for a single-member workspace, got %+v", got)
	}
}

// Manifest order decides candidate ORDER, so the same members supplied in the
// other order must still produce the same cross-edges — and the ambiguity
// candidate list reorders with the manifest, by design.
func TestReorderedMembersProduceSameCrossEdges(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Boot", "", 1})
	b := newMember(t, "b", []string{"example.com/b"}, nil, def{"b/b.go", "Serve", "", 1})

	edges := []graph.UnresolvedEdge{
		edge("Boot", "", "example.com/a"),
		edge("Serve", "", ""),
	}
	fwd, err := NewPass().Ladder(src, []Member{src, a, b}, edges)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := NewPass().Ladder(src, []Member{b, a, src}, edges)
	if err != nil {
		t.Fatal(err)
	}
	if len(fwd.CrossEdges) != 2 {
		t.Fatalf("cross edges = %+v, want 2", fwd.CrossEdges)
	}
	if len(rev.CrossEdges) != len(fwd.CrossEdges) {
		t.Fatalf("reordered = %+v, want %+v", rev.CrossEdges, fwd.CrossEdges)
	}
	for i := range fwd.CrossEdges {
		if fwd.CrossEdges[i] != rev.CrossEdges[i] {
			t.Fatalf("edge %d: %+v vs %+v", i, fwd.CrossEdges[i], rev.CrossEdges[i])
		}
	}
}

// The Q retry: a qualified lookup that finds nothing falls back to the bare
// one, because the qualifier is a narrowing hint and the receiver type may well
// live in a third member.
func TestQualifierRetryFallsBackToBareLookup(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Serve", "Server", 1})

	// Qualifier names a receiver type that is not a's parent for Serve.
	got := run(t, src, []Member{src, a}, edge("Serve", "Handler", ""))
	wantOneCross(t, got, overlay.CrossEdge{
		Src:  srcKey(),
		Dst:  overlay.SymKey{Member: "a", File: "a/a.go", QName: "Server.Serve"},
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 7,
	})
}

// The qualified form is still preferred when it DOES match: the retry is a
// fall-through, not a replacement.
func TestQualifierNarrowsWhenItMatches(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"example.com/a"}, nil,
		def{"a/a.go", "Serve", "Server", 1}, def{"a/b.go", "Serve", "Handler", 1})

	got := run(t, src, []Member{src, a}, edge("Serve", "Handler", ""))
	wantOneCross(t, got, overlay.CrossEdge{
		Src:  srcKey(),
		Dst:  overlay.SymKey{Member: "a", File: "a/b.go", QName: "Handler.Serve"},
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 7,
	})
}

// --- pass-scoped cache -----------------------------------------------------

// The defs cache is pass-scoped, so its key MUST carry the member alongside
// (name, qualifier). Drop the member from defsKey and the first member's
// answer is served for every later member's lookup: here `b` defines nothing,
// but a member-blind cache would report a's Boot for b too, turning rung 2's
// single hit into two and demoting the cross edge to a rung-3 ambiguity.
//
// The second source reuses the SAME Pass, which is the whole point of hoisting
// it: a shared cache must not leak one source's view into another's.
func TestPassCacheIsScopedByMember(t *testing.T) {
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Boot", "", 3})
	b := newMember(t, "b", []string{"example.com/b"}, nil)
	s1 := newMember(t, "s", []string{"example.com/s"}, nil)
	s2 := newMember(t, "s2", []string{"example.com/s2"}, nil)

	p := NewPass()
	want := overlay.CrossEdge{
		Src:  srcKey(),
		Dst:  overlay.SymKey{Member: "a", File: "a/a.go", QName: "Boot"},
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 7,
	}

	got, err := p.Ladder(s1, []Member{s1, a, b}, []graph.UnresolvedEdge{edge("Boot", "", "")})
	if err != nil {
		t.Fatal(err)
	}
	wantOneCross(t, got, want)

	// Same Pass, different source: b is still empty and a still uniquely wins.
	got, err = p.Ladder(s2, []Member{s2, a, b}, []graph.UnresolvedEdge{edge("Boot", "", "")})
	if err != nil {
		t.Fatal(err)
	}
	want.Src = overlay.SymKey{Member: "s2", File: "s.go", QName: "Caller"}
	wantOneCross(t, got, want)
}

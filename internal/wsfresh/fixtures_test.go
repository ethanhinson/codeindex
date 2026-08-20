package wsfresh

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/graph"
	"codeindex/internal/overlay"

	_ "github.com/mattn/go-sqlite3"
)

// This file carries the fixture machinery Freshen's tests are built on, on top
// of the minimal set in freshen_test.go:
//
//   - real two-member source trees that produce a REAL cross-edge, because a
//     single-member fixture cannot reproduce this area's bugs (0012's either-end
//     incidence, 0013's self-clobbering per-member loop are both invisible with
//     one member);
//   - the stateBadVersion affordance — an index that exists but fails
//     graph.OpenExisting;
//   - a source-mutation affordance that forces a CLEAN member's edge into a
//     DIRTY member to change;
//   - overlayContent, the content-equality witness.
//
// Every index here is built by the real engine over real source. wsresolve's
// writeIndex (synthetic rows through graph.Open) is unusable in this package:
// Freshen runs query.Fresh -> engine.Patch over the member's working tree and
// MemberMerkleRoot folds real per-file content hashes, so a synthetic index has
// no source behind it and an "edit the source" test could never move the fold.

// --- real two-member source with a real cross-edge ------------------------

// Namespaces the import-mediated rung matches on: app's source imports nsLib
// verbatim, and lib declares it, so app -> lib resolves at rung 1
// (cross_repo_import / exact) rather than by bare name.
const (
	nsApp = "example.com/app"
	nsLib = "example.com/lib"
)

// targetFile is where lib defines Target in the unmutated fixture.
const targetFile = "target.go"

// libSrc is the lib member's tree: Target lives in its own file so it can be
// MOVED without touching any other definition. LibOne is a second definition
// that stays put, so a moved Target does not look like "the whole member
// changed".
func libSrc(target string) map[string]string {
	return map[string]string{
		target:   "package lib\n\nfunc Target() int { return 1 }\n",
		"lib.go": "package lib\n\nfunc LibOne() int { return 2 }\n",
	}
}

// appSrc is the app member's tree: it imports lib by nsLib and calls
// lib.Target(), which the Go adapter records as an unresolved reference
// carrying that import path as its namespace hint. Target is defined nowhere in
// app, so in-repo resolution loses and the reference reaches the cross-repo
// ladder — which is what makes this a REAL cross-edge rather than a planted row.
func appSrc() map[string]string {
	return map[string]string{
		"app.go": "package app\n\nimport \"" + nsLib + "\"\n\n" +
			"func AppOne() int { return lib.Target() }\n",
	}
}

// crossEdgeWS is the standard two-member fixture: app (source of the edge) and
// lib (its destination), declared in that manifest order, both indexed.
// Nothing has been freshened yet — the overlay does not exist.
func crossEdgeWS(t *testing.T, extra ...wsMember) string {
	t.Helper()
	members := []wsMember{
		{id: "app", namespaces: []string{nsApp}, deps: []string{"lib"}, src: appSrc()},
		{id: "lib", namespaces: []string{nsLib}, src: libSrc(targetFile)},
	}
	return buildWS(t, append(members, extra...)...)
}

// appToLibEdge is the cross-edge crossEdgeWS is built to produce, with Target
// defined in dstFile. Src is app's caller; Dst is lib's definition.
func appToLibEdge(dstFile string) (src, dst overlay.SymKey) {
	return overlay.SymKey{Member: "app", File: "app.go", QName: "AppOne"},
		overlay.SymKey{Member: "lib", File: dstFile, QName: "Target"}
}

// assertCrossEdge fails unless the overlay records the app -> lib edge with
// Target in dstFile. Every test whose subject is cross-member behaviour should
// call this to guard its own premise: if the fixture ever stopped producing a
// cross-edge, the tests built on it would pass by asserting nothing.
func assertCrossEdge(t *testing.T, wsRoot, dstFile string) overlay.CrossEdge {
	t.Helper()
	src, dst := appToLibEdge(dstFile)
	ov := openOverlay(t, wsRoot)
	edges, err := ov.MemberEdges("app")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edges {
		if e.Src == src && e.Dst == dst {
			return e
		}
	}
	t.Fatalf("no app -> lib cross-edge %+v -> %+v; MemberEdges(app) = %+v", src, dst, edges)
	return overlay.CrossEdge{}
}

func openOverlay(t *testing.T, wsRoot string) *overlay.Store {
	t.Helper()
	ov, err := overlay.Open(overlay.Path(wsRoot))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ov.Close() })
	return ov
}

// --- source mutation affordances ------------------------------------------

func memberDir(wsRoot, memberID string) string { return filepath.Join(wsRoot, memberID) }

// writeMemberFile writes (or overwrites) one source file in a member's tree.
// The index is NOT rebuilt: noticing the edit is Freshen's own job, via
// query.Fresh.
func writeMemberFile(t *testing.T, wsRoot, memberID, rel, body string) {
	t.Helper()
	p := filepath.Join(memberDir(wsRoot, memberID), rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// moveMemberFile relocates one source file inside a member's tree, byte for
// byte. This is the cross-member mutation affordance: moving lib's target.go
// changes lib's merkle root (so lib is dirty) AND moves the definition app's
// edge points at (so the CLEAN member app's row into lib must change, from
// Dst.File=target.go to the new file). Nothing in app's own tree changes, which
// is precisely why a re-resolution scoped to the dirty member gets it wrong.
func moveMemberFile(t *testing.T, wsRoot, memberID, from, to string) {
	t.Helper()
	dir := memberDir(wsRoot, memberID)
	body, err := os.ReadFile(filepath.Join(dir, from))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, to), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, from)); err != nil {
		t.Fatal(err)
	}
}

// --- unavailability and crash affordances ---------------------------------

// memberDB is a member's index path, composed through Freshen's OWN
// memberIndexPath rather than re-joining ".codeindex/graph.db" a seventh time.
func memberDB(wsRoot, memberID string) string {
	return memberIndexPath(memberDir(wsRoot, memberID))
}

// corruptIndexVersion forges a version mismatch on an already-built index:
// the file exists and is a valid database, but graph.OpenExisting rejects it.
// OpenRaw plus a PRAGMA write is the only way to do this — graph.Open would
// delete and rebuild the file. Mirrors wsresolve_test.go's writeIndex.
func corruptIndexVersion(t *testing.T, wsRoot, memberID string) {
	t.Helper()
	path := memberDB(wsRoot, memberID)
	db, err := graph.OpenRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 999999`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// deleteStamp removes one member's stamp row from the overlay directly,
// simulating a pass that died after writing content but before 0013's
// stamps-last step. overlay exposes no stamp deletion — deliberately, it is not
// a product operation — so this reaches the table through the driver.
func deleteStamp(t *testing.T, wsRoot, memberID string) {
	t.Helper()
	deleteOneRow(t, wsRoot, `DELETE FROM member_stamps WHERE member_id = ?`, memberID,
		"stamp")
}

// deleteRegistryRow removes one member from the stored registry and NOTHING
// else. ReplaceRegistry cannot express this: it prunes the departing member's
// cross-member rows as well, which is correct for the product and useless for
// proving that the witness keeps looking at members the registry has forgotten.
func deleteRegistryRow(t *testing.T, wsRoot, memberID string) {
	t.Helper()
	deleteOneRow(t, wsRoot, `DELETE FROM members WHERE id = ?`, memberID, "registry row")
}

// deleteOneRow reaches the overlay's tables through the driver, for the two
// deletions overlay deliberately exposes no API for. It insists the row was
// there: a no-op DELETE would leave the caller asserting against a state it
// never reached.
func deleteOneRow(t *testing.T, wsRoot, stmt, memberID, what string) {
	t.Helper()
	db, err := sql.Open("sqlite3", overlay.Path(wsRoot))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	res, err := db.Exec(stmt, memberID)
	if err != nil {
		t.Fatal(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleting %s of member %q: %d rows affected, want 1 — the row this simulates losing was never there",
			what, memberID, n)
	}
}

// --- the content-equality witness -----------------------------------------

// fieldSep separates fields inside one rendered record. It is a byte no source
// path, qname or namespace can contain, so a rendered record is an unambiguous
// encoding of its fields — which is what makes SORTING the rendered records a
// sort on a key TOTAL over every field. A key built from a subset of the fields
// passes vacuously when two rows agree on that subset and differ elsewhere:
// SQLite returns such rows in arbitrary order, and comparing two reads of one
// store in one process hides it (learning
// determinism-tests-need-a-total-sort-key).
const fieldSep = "\x1f"

func join(parts ...string) string { return strings.Join(parts, fieldSep) }

// edgeRecord renders a cross-edge as seen under member view. Every field the
// row can carry is present: the full Src and Dst triples, Kind, Provenance,
// Confidence and Line. view is part of the record because MemberEdges is
// incidence-scoped on EITHER endpoint — one edge is legitimately visible under
// two members, and losing one of those views is a real change.
func edgeRecord(view string, e overlay.CrossEdge) string {
	return join("edge", view,
		e.Src.Member, e.Src.File, e.Src.QName,
		e.Dst.Member, e.Dst.File, e.Dst.QName,
		e.Kind, e.Provenance, e.Confidence, strconv.Itoa(e.Line))
}

// ambRecord renders an ambiguity as seen under member view: the Src triple,
// RefName, RefNS, Kind, Line, Count and the ORDERED candidate list. Candidate
// order is load-bearing (the deps-named candidate leads), so the candidates are
// rendered in order, not sorted.
func ambRecord(view string, a overlay.Ambiguity) string {
	parts := []string{"ambiguity", view,
		a.Src.Member, a.Src.File, a.Src.QName,
		a.RefName, a.RefNS, a.Kind,
		strconv.Itoa(a.Line), strconv.Itoa(a.Count), strconv.Itoa(len(a.Candidates))}
	for _, c := range a.Candidates {
		parts = append(parts, c.Member, c.File, c.QName)
	}
	return join(parts...)
}

// registryRecord renders one stored registry member with its POSITION, so that
// a registry rewritten in a different order does not compare equal after the
// sort.
func registryRecord(pos int, m config.Member) string {
	parts := []string{"registry", strconv.Itoa(pos), m.ID, m.Root,
		strconv.Itoa(len(m.Namespaces))}
	parts = append(parts, m.Namespaces...)
	parts = append(parts, strconv.Itoa(len(m.Deps)))
	parts = append(parts, m.Deps...)
	return join(parts...)
}

// stampRecord renders a stamp WITHOUT ResolvedAt. ResolvedAt is a 1-second
// granular wall clock: including it would make the witness both flaky and, in a
// fast test, silently blind — two passes inside the same second produce the
// same value whether or not the stamp was rewritten. MerkleRoot is the part
// that carries meaning.
func stampRecord(s overlay.Stamp) string {
	return join("stamp", s.MemberID, s.MerkleRoot)
}

func suppressionRecord(s overlay.Suppression) string {
	return join("suppression", s.ConsumerMember, s.Namespace, s.OwnerMember, s.SuppressedVersion)
}

// overlayContent is the content-equality witness: a canonical, comparable
// snapshot of everything the overlay holds for wsRoot.
//
// It compares CONTENT, never file bytes or a file hash — overlay.Open
// re-executes the schema and PRAGMA user_version on every open, so the file is
// not a stable witness of "nothing was written".
//
// MemberEdges and AmbiguitiesFor are scoped by member, so the per-member rows
// are gathered by iterating members; Registry, Stamps and Suppressions are
// whole-store and are read once. The iteration runs over the MANIFEST's members
// in manifest order — NOT the registry's — for two reasons: a registry write
// that DROPS a member must not silently drop that member's rows from the
// snapshot too (which would let it compare equal), and members that are absent
// from disk or unindexed must still be visited, since rows about them can
// survive a pass.
func overlayContent(t *testing.T, wsRoot string) string {
	t.Helper()
	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	ov := openOverlay(t, wsRoot)

	var recs []string
	for _, m := range ws.Members {
		edges, err := ov.MemberEdges(m.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range edges {
			recs = append(recs, edgeRecord(m.ID, e))
		}
		ambs, err := ov.AmbiguitiesFor(m.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range ambs {
			recs = append(recs, ambRecord(m.ID, a))
		}
	}
	reg, err := ov.Registry()
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range reg {
		recs = append(recs, registryRecord(i, m))
	}
	stamps, err := ov.Stamps()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stamps {
		recs = append(recs, stampRecord(s))
	}
	sups, err := ov.Suppressions()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sups {
		recs = append(recs, suppressionRecord(s))
	}

	sort.Strings(recs)
	return strings.Join(recs, "\n")
}

// --- fixture self-tests: prove the machinery actually bites ---------------

// TestCrossEdgeFixtureProducesARealCrossEdge is the premise guard for every
// fixture in this file. The edge must be produced by the engine and the ladder
// from real source — rung 1, import-mediated, exact — not planted.
func TestCrossEdgeFixtureProducesARealCrossEdge(t *testing.T) {
	wsRoot := crossEdgeWS(t)
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if !rep.Resolved {
		t.Fatal("first Freshen did not resolve; no cross-edge could have been written")
	}
	e := assertCrossEdge(t, wsRoot, targetFile)
	if e.Provenance != "cross_repo_import" || e.Confidence != "exact" {
		t.Errorf("edge %+v: want cross_repo_import/exact — the import hint must be doing the work", e)
	}
	if rep.Stats.CrossEdges != 1 {
		t.Errorf("Stats.CrossEdges = %d, want 1", rep.Stats.CrossEdges)
	}
}

// TestMoveMemberFileMovesTheCrossEdgeDestination proves the cross-member
// mutation affordance expresses what it claims: after moving Target inside lib,
// lib is dirty, app's SOURCE is untouched — and yet app's edge into lib must
// now name the new file. It is the affordance, not the assertion, that this
// test exists for.
func TestMoveMemberFileMovesTheCrossEdgeDestination(t *testing.T) {
	wsRoot := crossEdgeWS(t)
	if _, err := Freshen(wsRoot); err != nil {
		t.Fatalf("priming Freshen: %v", err)
	}
	assertCrossEdge(t, wsRoot, targetFile)

	moveMemberFile(t, wsRoot, "lib", targetFile, "moved.go")

	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if len(rep.Dirty) != 1 || rep.Dirty[0] != "lib" {
		t.Fatalf("Dirty = %v, want [lib] — only lib's tree changed", rep.Dirty)
	}
	assertCrossEdge(t, wsRoot, "moved.go")
	// And the stale destination is gone, not merely joined by the new one.
	src, staleDst := appToLibEdge(targetFile)
	ov := openOverlay(t, wsRoot)
	edges, err := ov.MemberEdges("app")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edges {
		if e.Src == src && e.Dst == staleDst {
			t.Fatalf("stale edge into %s survived: %+v", targetFile, e)
		}
	}
}

// TestBadVersionFixtureExistsButFailsOpenExisting: the stateBadVersion member's
// index is on disk and is not otherwise damaged — it fails the ONE availability
// predicate, graph.OpenExisting, and nothing else. A fixture that merely
// deleted the file would exercise stateNoIndex instead and test 7 would prove
// nothing about the version-mismatch path.
func TestBadVersionFixtureExistsButFailsOpenExisting(t *testing.T) {
	wsRoot := crossEdgeWS(t, wsMember{
		id: "old", namespaces: []string{"example.com/old"},
		src: goSrc("old", "OldOne"), state: stateBadVersion,
	})
	path := memberDB(wsRoot, "old")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("bad-version member has no index file: %v", err)
	}
	if st, err := graph.OpenExisting(path); err == nil {
		st.Close()
		t.Fatal("graph.OpenExisting succeeded on the bad-version index; the fixture forges nothing")
	}
	// It is still a readable database — only the version is wrong.
	if _, err := graph.OpenRaw(path); err != nil {
		t.Fatalf("bad-version index is not a readable database: %v", err)
	}

	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if rep.MembersUnindexed != 1 {
		t.Errorf("MembersUnindexed = %d, want 1", rep.MembersUnindexed)
	}
	for _, id := range rep.Dirty {
		if id == "old" {
			t.Error("unavailable member 'old' appears in Dirty")
		}
	}
}

// TestDeleteStampRemovesExactlyOneStamp proves the crash affordance: the row is
// gone and no other member's stamp is disturbed.
func TestDeleteStampRemovesExactlyOneStamp(t *testing.T) {
	wsRoot := crossEdgeWS(t)
	if _, err := Freshen(wsRoot); err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	deleteStamp(t, wsRoot, "lib")

	ov := openOverlay(t, wsRoot)
	if _, ok, err := ov.Stamp("lib"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("lib's stamp survived deleteStamp")
	}
	if _, ok, err := ov.Stamp("app"); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("app's stamp was collateral damage")
	}
}

// --- the witness must discriminate ----------------------------------------

// mutateEdges rewrites member's incident edges through mutate. ReplaceMemberEdges
// clears on EITHER endpoint and re-puts, so this is a genuine overlay content
// write, not a simulated one.
func mutateEdges(t *testing.T, wsRoot, member string, mutate func([]overlay.CrossEdge) []overlay.CrossEdge) {
	t.Helper()
	ov := openOverlay(t, wsRoot)
	edges, err := ov.MemberEdges(member)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) == 0 {
		t.Fatalf("member %q has no edges to mutate; the premise of this test is gone", member)
	}
	if err := ov.ReplaceMemberEdges(member, mutate(edges), nil, nil); err != nil {
		t.Fatal(err)
	}
}

// freshenedCrossEdgeWS is crossEdgeWS taken through a settled Freshen, with the
// cross-edge asserted present — so a snapshot of it is a snapshot of something.
func freshenedCrossEdgeWS(t *testing.T) string {
	t.Helper()
	wsRoot := crossEdgeWS(t)
	if _, err := Freshen(wsRoot); err != nil {
		t.Fatalf("priming Freshen: %v", err)
	}
	assertCrossEdge(t, wsRoot, targetFile)
	return wsRoot
}

// TestOverlayContentIsNotEmpty: an all-empty witness compares equal to itself
// forever. Every discrimination test below rests on this.
func TestOverlayContentIsNotEmpty(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	snap := overlayContent(t, wsRoot)
	for _, want := range []string{"edge", "registry", "stamp"} {
		if !strings.Contains(snap, want+fieldSep) {
			t.Errorf("snapshot carries no %s record:\n%s", want, snap)
		}
	}
}

// TestOverlayContentDetectsADroppedEdge — the single most important property:
// remove ONE row and the witness must change.
func TestOverlayContentDetectsADroppedEdge(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	before := overlayContent(t, wsRoot)
	mutateEdges(t, wsRoot, "app", func(e []overlay.CrossEdge) []overlay.CrossEdge { return nil })
	if after := overlayContent(t, wsRoot); after == before {
		t.Fatalf("witness did not notice a dropped cross-edge; it compares nothing:\n%s", before)
	}
}

// TestOverlayContentDetectsAPerturbedLine: same row set, one integer field
// moved by one.
func TestOverlayContentDetectsAPerturbedLine(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	before := overlayContent(t, wsRoot)
	mutateEdges(t, wsRoot, "app", func(edges []overlay.CrossEdge) []overlay.CrossEdge {
		out := append([]overlay.CrossEdge(nil), edges...)
		out[0].Line++
		return out
	})
	if after := overlayContent(t, wsRoot); after == before {
		t.Fatalf("witness did not notice a perturbed Line:\n%s", before)
	}
}

// TestOverlayContentDetectsAPerturbedConfidence: Provenance and Confidence are
// absent from MemberEdges' ORDER BY, so a witness whose key stopped at the
// ordered columns would miss this.
func TestOverlayContentDetectsAPerturbedConfidence(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	before := overlayContent(t, wsRoot)
	mutateEdges(t, wsRoot, "app", func(edges []overlay.CrossEdge) []overlay.CrossEdge {
		out := append([]overlay.CrossEdge(nil), edges...)
		out[0].Provenance = "cross_repo_name"
		out[0].Confidence = "inferred"
		return out
	})
	if after := overlayContent(t, wsRoot); after == before {
		t.Fatalf("witness did not notice a demoted provenance/confidence:\n%s", before)
	}
}

// TestOverlayContentDetectsADeletedStamp: the stamp table is part of the
// content, so 0013's stamps-last crash window is observable.
func TestOverlayContentDetectsADeletedStamp(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	before := overlayContent(t, wsRoot)
	deleteStamp(t, wsRoot, "lib")
	if after := overlayContent(t, wsRoot); after == before {
		t.Fatalf("witness did not notice a deleted stamp:\n%s", before)
	}
}

// countEdgeViews is how many of the snapshot's edge records were observed under
// member — the per-member half of the witness, isolated.
func countEdgeViews(snapshot, member string) int {
	n := 0
	for _, line := range strings.Split(snapshot, "\n") {
		if strings.HasPrefix(line, join("edge", member)+fieldSep) {
			n++
		}
	}
	return n
}

// TestOverlayContentDetectsARegistryWrite: Registry() is in the witness because
// registry drift is a gate INPUT — a spurious ReplaceRegistry on the clean path
// is a content write and must not pass unseen.
//
// The two cases are not interchangeable. Dropping a member also prunes its
// cross-member rows (ReplaceRegistry's pruneOrphans), so that case would still
// be caught by a witness that carried only edges — it proves the drop is
// visible, not that the registry is in the tuple. The namespaces edit touches
// member_namespaces and NOTHING else, so it is the case that goes red the
// moment Registry() leaves the witness.
func TestOverlayContentDetectsARegistryWrite(t *testing.T) {
	manifest := func(t *testing.T, wsRoot string) *config.Workspace {
		t.Helper()
		ws, err := config.LoadWorkspace(wsRoot)
		if err != nil {
			t.Fatal(err)
		}
		return ws
	}
	for name, rewrite := range map[string]func(*testing.T, string) *config.Workspace{
		"namespaces edited": func(t *testing.T, wsRoot string) *config.Workspace {
			ws := manifest(t, wsRoot)
			ws.Members[0].Namespaces = append(ws.Members[0].Namespaces, "example.com/extra")
			return ws
		},
		"member dropped": func(t *testing.T, wsRoot string) *config.Workspace {
			ws := manifest(t, wsRoot)
			ws.Members = ws.Members[:1]
			return ws
		},
	} {
		t.Run(name, func(t *testing.T) {
			wsRoot := freshenedCrossEdgeWS(t)
			before := overlayContent(t, wsRoot)
			ov := openOverlay(t, wsRoot)
			if err := ov.ReplaceRegistry(rewrite(t, wsRoot)); err != nil {
				t.Fatal(err)
			}
			if after := overlayContent(t, wsRoot); after == before {
				t.Fatalf("witness did not notice a registry write (%s):\n%s", name, before)
			}
		})
	}
}

// TestOverlayContentIteratesTheManifestNotTheRegistry is why the iteration set
// comes from config.LoadWorkspace rather than ov.Registry(). Here the registry
// row alone is deleted — ReplaceRegistry cannot express this, because its
// pruneOrphans step also removes the departing member's rows — leaving lib's
// cross-edge rows in place with no registry row behind them. A witness that
// iterated the registry would stop LOOKING at lib and lose those rows from the
// snapshot, so a store that still holds them and a store that never did would
// render identically. Iterating the manifest keeps looking.
func TestOverlayContentIteratesTheManifestNotTheRegistry(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	before := overlayContent(t, wsRoot)
	libView := countEdgeViews(before, "lib")
	if libView == 0 {
		t.Fatal("fixture: no edge is visible under lib, so this test proves nothing")
	}

	deleteRegistryRow(t, wsRoot, "lib")

	after := overlayContent(t, wsRoot)
	if after == before {
		t.Fatal("witness did not notice a registry row deleted out from under it")
	}
	if got := countEdgeViews(after, "lib"); got != libView {
		t.Fatalf("edges visible under lib = %d, want %d: the witness iterates the REGISTRY, "+
			"so a member missing from it takes its surviving rows out of the snapshot too",
			got, libView)
	}
	// And the registry section did lose the member — the drop itself is visible.
	if strings.Contains(after, join("registry", "1", "lib")) {
		t.Error("lib's registry record survived the delete; the witness is reading something else")
	}
}

// TestOverlayContentOmitsResolvedAt: the wall clock must not be a witness. It
// is 1-second granular, so including it makes the comparison flaky across a
// second boundary and blind within one.
func TestOverlayContentOmitsResolvedAt(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	ov := openOverlay(t, wsRoot)
	stamps, err := ov.Stamps()
	if err != nil {
		t.Fatal(err)
	}
	if len(stamps) == 0 {
		t.Fatal("fixture: no stamps")
	}
	snap := overlayContent(t, wsRoot)
	for _, s := range stamps {
		if s.ResolvedAt == 0 {
			t.Fatalf("stamp %+v has no ResolvedAt; this test's premise is gone", s)
		}
		if strings.Contains(snap, strconv.FormatInt(s.ResolvedAt, 10)) {
			t.Errorf("snapshot contains resolved_at %d — the wall clock is not a witness:\n%s",
				s.ResolvedAt, snap)
		}
	}
}

// --- the sort key is TOTAL over every field -------------------------------

// A record's rendering is what the sort orders, so "the key is total over every
// field" is exactly "changing any one field changes the rendering". These two
// tests walk every field of every row type one at a time. A key built from a
// PREFIX of the fields is the bug this repo already shipped once: two rows
// agreeing on every ORDER BY term come back in arbitrary order, and comparing
// two reads of the same store inside one process never shows it.

func TestEdgeRecordKeyIsTotal(t *testing.T) {
	base := overlay.CrossEdge{
		Src:  overlay.SymKey{Member: "app", File: "a.go", QName: "AppOne"},
		Dst:  overlay.SymKey{Member: "lib", File: "target.go", QName: "Target"},
		Kind: "call", Provenance: "cross_repo_import", Confidence: "exact", Line: 5,
	}
	cases := map[string]func(*overlay.CrossEdge){
		"Src.Member": func(e *overlay.CrossEdge) { e.Src.Member = "other" },
		"Src.File":   func(e *overlay.CrossEdge) { e.Src.File = "other.go" },
		"Src.QName":  func(e *overlay.CrossEdge) { e.Src.QName = "Other" },
		"Dst.Member": func(e *overlay.CrossEdge) { e.Dst.Member = "other" },
		"Dst.File":   func(e *overlay.CrossEdge) { e.Dst.File = "other.go" },
		"Dst.QName":  func(e *overlay.CrossEdge) { e.Dst.QName = "Other" },
		"Kind":       func(e *overlay.CrossEdge) { e.Kind = "ref" },
		"Provenance": func(e *overlay.CrossEdge) { e.Provenance = "cross_repo_name" },
		"Confidence": func(e *overlay.CrossEdge) { e.Confidence = "inferred" },
		"Line":       func(e *overlay.CrossEdge) { e.Line = 6 },
	}
	want := edgeRecord("app", base)
	for name, mutate := range cases {
		got := base
		mutate(&got)
		if edgeRecord("app", got) == want {
			t.Errorf("CrossEdge.%s is absent from the sort key: two rows differing only there collide", name)
		}
	}
	if edgeRecord("lib", base) == want {
		t.Error("the viewing member is absent from the sort key: losing one endpoint's view would pass unseen")
	}
}

func TestAmbiguityRecordKeyIsTotal(t *testing.T) {
	base := overlay.Ambiguity{
		Src:     overlay.SymKey{Member: "app", File: "a.go", QName: "AppOne"},
		RefName: "Target", RefNS: nsLib, Kind: "call", Line: 5, Count: 2,
		Candidates: []overlay.SymKey{
			{Member: "lib", File: "target.go", QName: "Target"},
			{Member: "old", File: "old.go", QName: "Target"},
		},
	}
	cases := map[string]func(*overlay.Ambiguity){
		"Src.Member":       func(a *overlay.Ambiguity) { a.Src.Member = "other" },
		"Src.File":         func(a *overlay.Ambiguity) { a.Src.File = "other.go" },
		"Src.QName":        func(a *overlay.Ambiguity) { a.Src.QName = "Other" },
		"RefName":          func(a *overlay.Ambiguity) { a.RefName = "Other" },
		"RefNS":            func(a *overlay.Ambiguity) { a.RefNS = "example.com/other" },
		"Kind":             func(a *overlay.Ambiguity) { a.Kind = "ref" },
		"Line":             func(a *overlay.Ambiguity) { a.Line = 6 },
		"Count":            func(a *overlay.Ambiguity) { a.Count = 3 },
		"Candidates.len":   func(a *overlay.Ambiguity) { a.Candidates = a.Candidates[:1] },
		"Candidates.field": func(a *overlay.Ambiguity) { a.Candidates[1].File = "other.go" },
		"Candidates.order": func(a *overlay.Ambiguity) {
			a.Candidates = []overlay.SymKey{a.Candidates[1], a.Candidates[0]}
		},
	}
	want := ambRecord("app", base)
	for name, mutate := range cases {
		got := base
		got.Candidates = append([]overlay.SymKey(nil), base.Candidates...)
		mutate(&got)
		if ambRecord("app", got) == want {
			t.Errorf("Ambiguity.%s is absent from the sort key: two rows differing only there collide", name)
		}
	}
}

func TestRegistryAndWholeStoreRecordKeysAreTotal(t *testing.T) {
	m := config.Member{ID: "app", Root: "app",
		Namespaces: []string{nsApp}, Deps: []string{"lib"}}
	base := registryRecord(0, m)
	moved := m
	moved.Root = "elsewhere"
	nsEdit := m
	nsEdit.Namespaces = []string{nsApp, "example.com/extra"}
	depEdit := m
	depEdit.Deps = nil
	for name, got := range map[string]string{
		"position":   registryRecord(1, m),
		"Root":       registryRecord(0, moved),
		"Namespaces": registryRecord(0, nsEdit),
		"Deps":       registryRecord(0, depEdit),
	} {
		if got == base {
			t.Errorf("registry %s is absent from the sort key", name)
		}
	}

	st := overlay.Stamp{MemberID: "app", MerkleRoot: "abc", ResolvedAt: 1}
	moot := st
	moot.MerkleRoot = "def"
	if stampRecord(moot) == stampRecord(st) {
		t.Error("Stamp.MerkleRoot is absent from the sort key")
	}
	clockOnly := st
	clockOnly.ResolvedAt = 999
	if stampRecord(clockOnly) != stampRecord(st) {
		t.Error("Stamp.ResolvedAt leaked into the key")
	}

	sup := overlay.Suppression{ConsumerMember: "app", Namespace: nsLib,
		OwnerMember: "lib", SuppressedVersion: "v1"}
	for name, mutate := range map[string]func(*overlay.Suppression){
		"ConsumerMember":    func(s *overlay.Suppression) { s.ConsumerMember = "other" },
		"Namespace":         func(s *overlay.Suppression) { s.Namespace = "example.com/other" },
		"OwnerMember":       func(s *overlay.Suppression) { s.OwnerMember = "other" },
		"SuppressedVersion": func(s *overlay.Suppression) { s.SuppressedVersion = "v2" },
	} {
		got := sup
		mutate(&got)
		if suppressionRecord(got) == suppressionRecord(sup) {
			t.Errorf("Suppression.%s is absent from the sort key", name)
		}
	}
}

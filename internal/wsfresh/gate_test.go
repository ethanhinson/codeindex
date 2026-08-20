package wsfresh

import (
	"os"
	"testing"

	"codeindex/internal/overlay"
)

// The remaining members of the plan's eight-test set, over the real
// cross-edge fixtures. The other four live next to the machinery they extend:
//
//	test 4 (cross-member freshness)   TestMoveMemberFileMovesTheCrossEdgeDestination
//	test 5 (manifest drift + siblings) TestFreshenManifest{Namespaces,Deps}DriftReResolves
//	                                   + TestFreshenConvergesOn{DuplicateNamespace,EmptyDepsArray}
//	test 6 (unindexed / missing)       TestFreshenMissingMember,
//	                                   TestFreshenUnindexedMemberIsNotBuilt

// --- 1. a clean pass writes no overlay CONTENT ----------------------------

// TestFreshenCleanPassWritesNoOverlayContent is the no-write witness the gate's
// clean branch exists for. TestFreshenCleanSecondPassDoesNotResolve already
// asserts the DECISION (Resolved false, Dirty empty); this asserts the
// consequence, which is the part a wrong implementation can still get wrong: a
// pass may decline to call wsresolve.Resolve and yet write content on the way to
// that decision.
//
// Registry() is inside the witness precisely because registry drift is a gate
// INPUT, and the tempting bug is a ReplaceRegistry called "just to be safe"
// before the comparison. But the witness cannot see that write unaided: on the
// clean path stored == NormalizeMembers(ws.Members) BY DEFINITION, so the stray
// call deletes and re-inserts IDENTICAL rows and the snapshot renders
// byte-identically. (TestOverlayContentDetectsARegistryWrite proves only that a
// registry write which CHANGES the registry is visible — a different write.)
// The same blindness covers a redundant PutStamp with an unmoved root, since
// stampRecord deliberately omits ResolvedAt.
//
// So the tripwire is planted rather than hoped for: an orphan member_stamps row
// for an id the manifest does not declare. Stamps() is whole-store, so the row
// is in the snapshot, and pruneOrphans — which ReplaceRegistry runs
// unconditionally, registry change or not — is the only thing in this pass that
// would delete it. A correct clean pass leaves it untouched; the stray write
// erases it and the snapshots differ.
// TestOrphanStampMakesAnUnchangedRegistryWriteVisible is the self-test for
// exactly that tooth.
//
// What this still does NOT prove, plainly: a write that changes no content is
// invisible to any content witness. A redundant PutStamp of the same root would
// pass here, as would a registry write in some future where pruneOrphans has no
// orphan left to take. The assertion is "the clean pass changed no overlay
// content, including content only a redundant write would disturb" — not "the
// clean pass executed no write statement". The latter needs a driver-level
// statement counter, not a snapshot.
//
// overlay.Open itself is not a content write and must not be treated as one —
// it re-executes the schema and PRAGMA user_version on every open, which is why
// the witness compares content and never file bytes.
func TestFreshenCleanPassWritesNoOverlayContent(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	plantOrphanStamp(t, wsRoot, "ghost")
	before := overlayContent(t, wsRoot)

	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("second Freshen: %v", err)
	}
	if len(rep.Dirty) != 0 {
		t.Fatalf("Dirty = %v, want empty on an unchanged workspace", rep.Dirty)
	}
	if rep.Resolved {
		t.Fatal("Resolved = true on a clean pass; the gate must not re-resolve")
	}
	if after := overlayContent(t, wsRoot); after != before {
		t.Fatalf("the clean pass wrote overlay content.\nbefore:\n%s\n\nafter:\n%s", before, after)
	}
}

// --- 2. an edited source re-resolves --------------------------------------

// TestFreshenEditedSourceReResolves is plan test 2. Renaming app's caller is a
// SOURCE edit: it moves app's merkle root, so app's stamp no longer matches and
// app alone is dirty. lib's tree is untouched and lib must stay clean.
//
// "The overlay reflects the edit" is asserted on the edge itself rather than on
// the Report: the new Src.QName is present and the old one is GONE. Asserting
// only the presence of the new row would pass against an overlay that
// accumulated both, which is the failure mode wsresolve's clear-then-put write
// order exists to prevent.
func TestFreshenEditedSourceReResolves(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)

	writeMemberFile(t, wsRoot, "app", "app.go",
		"package app\n\nimport \""+nsLib+"\"\n\nfunc AppTwo() int { return lib.Target() }\n")

	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if len(rep.Dirty) != 1 || rep.Dirty[0] != "app" {
		t.Fatalf("Dirty = %v, want [app] — only app's source changed", rep.Dirty)
	}
	if !rep.Resolved {
		t.Fatal("Resolved = false with app dirty; the gate must take its dirty branch")
	}

	dst := overlay.SymKey{Member: "lib", File: targetFile, QName: "Target"}
	edges := memberEdges(t, wsRoot, "app")
	var sawNew bool
	for _, e := range edges {
		if e.Src.QName == "AppOne" {
			t.Errorf("stale edge from the renamed AppOne survived: %+v", e)
		}
		if e.Src == (overlay.SymKey{Member: "app", File: "app.go", QName: "AppTwo"}) && e.Dst == dst {
			sawNew = true
		}
	}
	if !sawNew {
		t.Fatalf("no AppTwo -> lib.Target edge after the edit; MemberEdges(app) = %+v", edges)
	}
}

// --- 3. crash self-healing -------------------------------------------------

// TestFreshenHealsAMissingStamp is plan test 3. 0013 writes stamps LAST, so a
// pass that dies mid-write leaves content behind with no stamp to vouch for it.
// The absent stamp is therefore the self-healing signal, not an error and not a
// clean member.
//
// The recovery bar is content equality with a from-scratch pass over an
// identical workspace, not merely "Resolved was true": a re-resolution that ran
// but left the store in a state a clean build would never produce has healed
// nothing. Both sides are built from the same relative member roots, so nothing
// tempdir-specific enters the witness.
func TestFreshenHealsAMissingStamp(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	deleteStamp(t, wsRoot, "lib")

	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if len(rep.Dirty) != 1 || rep.Dirty[0] != "lib" {
		t.Fatalf("Dirty = %v, want [lib] — an absent stamp is dirty", rep.Dirty)
	}
	if !rep.Resolved {
		t.Fatal("Resolved = false after a lost stamp; the crash is not being healed")
	}
	assertCrossEdge(t, wsRoot, targetFile)

	healed := overlayContent(t, wsRoot)
	scratch := overlayContent(t, freshenedCrossEdgeWS(t))
	if healed != scratch {
		t.Fatalf("healed content differs from a from-scratch pass.\nhealed:\n%s\n\nscratch:\n%s",
			healed, scratch)
	}
}

// --- 7. convergence with a version-mismatched member ----------------------

// TestFreshenConvergesWithABadVersionMember is plan test 7, and it is a
// CONVERGENCE test: the second consecutive Freshen must report Resolved false.
//
// This is the two-sites invariant with teeth. Freshen's availability predicate
// and wsresolve.Resolve's must be the same one — graph.OpenExisting succeeding.
// If Freshen's were looser (an os.Stat, say), it would call the bad-version
// member available, fold or fail to fold an index the resolver will never stamp,
// find no matching stamp, and mark it dirty on every pass — re-resolving the
// whole workspace forever with no source ever changing.
//
// TestBadVersionFixtureExistsButFailsOpenExisting guards the fixture (the file
// is there and only the version check rejects it) and the single-pass counting;
// this one is the standing-state assertion that fixture makes possible.
func TestFreshenConvergesWithABadVersionMember(t *testing.T) {
	wsRoot := crossEdgeWS(t, wsMember{
		id: "old", namespaces: []string{"example.com/old"},
		src: goSrc("old", "OldOne"), state: stateBadVersion,
	})
	first, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("first Freshen: %v", err)
	}
	if !first.Resolved {
		t.Fatal("first Freshen did not resolve; the fixture never exercises the gate")
	}
	assertCrossEdge(t, wsRoot, targetFile)

	second, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("second Freshen: %v", err)
	}
	if second.MembersUnindexed != 1 {
		t.Errorf("second pass MembersUnindexed = %d, want 1", second.MembersUnindexed)
	}
	if len(second.Dirty) != 0 {
		t.Fatalf("second pass Dirty = %v, want empty — a member the resolver cannot open "+
			"must not be perpetually dirty", second.Dirty)
	}
	if second.Resolved {
		t.Fatal("second pass re-resolved: Freshen's availability predicate is looser than the resolver's")
	}
}

// --- 8. determinism across a genuine rebuild ------------------------------

// TestFreshenIsDeterministicAcrossRebuilds is plan test 8. The trap this repo
// has already hit is comparing two READS of the same store inside one process:
// that proves only that the reads agree, never that two independent resolutions
// of the same content agree. Rows that tie on the query's ORDER BY come back in
// arbitrary order, and one store cannot show it.
//
// So each pass here is a genuine rebuild. Deleting both stamps forces the gate's
// dirty branch, and wsresolve clears and re-puts every derived row — so the two
// snapshots come from two separately constructed row sets over byte-identical
// source, compared on a sort key total over every field.
func TestFreshenIsDeterministicAcrossRebuilds(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)

	rebuild := func(pass int) string {
		deleteStamp(t, wsRoot, "app")
		deleteStamp(t, wsRoot, "lib")
		rep, err := Freshen(wsRoot)
		if err != nil {
			t.Fatalf("rebuild %d: %v", pass, err)
		}
		if !rep.Resolved || len(rep.Dirty) != 2 {
			t.Fatalf("rebuild %d: Resolved=%v Dirty=%v — this pass was not a rebuild, "+
				"so the comparison would prove nothing", pass, rep.Resolved, rep.Dirty)
		}
		return overlayContent(t, wsRoot)
	}

	first := rebuild(1)
	second := rebuild(2)
	if first != second {
		t.Fatalf("two rebuilds over identical content disagree.\nfirst:\n%s\n\nsecond:\n%s",
			first, second)
	}
}

// --- the available -> unavailable transition, closed ----------------------

// TestVanishedMemberIsPrunedThenReportsClean is the INVERSION of the former
// TestKNOWNLIMITATIONVanishedMemberLeavesStaleEdgesReportedClean, which pinned
// the defect this asserts the repair of. The inversion is performed on that
// test's own authority — it licensed exactly this move, conditionally: "If a
// future slice makes this test fail by dropping the stale edge or by reporting
// Resolved true, that is very likely an IMPROVEMENT — but only if
// wsresolve.Resolve now PRUNES the stamp of an unavailable member ... Delete or
// invert this test deliberately, with that check done." Change 0015 landed that
// prune as wsresolve.Resolve's step 9a, so the condition is discharged, and
// TestFreshenConvergesWithABadVersionMember (the non-convergence that condition
// was protecting) is untouched: its member is never stamped in the first place.
//
// The transition is available -> unavailable. lib is resolved and stamped, and
// app carries a cross-edge into it; then lib's own index is deleted. lib now
// fails the availability predicate and is skipped at 5a before any fold, so it
// never enters Dirty — but its SURVIVING STAMP is read by the manifest walk and
// lands in StaleStamped, which trips the gate. The resolution that follows
// re-derives app without lib as a candidate and prunes lib outright.
//
// Dirty is asserted EMPTY on purpose: it proves StaleStamped alone tripped the
// gate, and that the fix did not arrive by quietly widening Dirty to include a
// member that has no re-fold to be dirty against.
func TestVanishedMemberIsPrunedThenReportsClean(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	assertCrossEdge(t, wsRoot, targetFile)

	// lib was available and stamped; now it is not available at all.
	if err := os.Remove(memberDB(wsRoot, "lib")); err != nil {
		t.Fatalf("removing lib's index: %v", err)
	}

	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if rep.MembersUnindexed != 1 {
		t.Fatalf("MembersUnindexed = %d, want 1 — lib was not actually made unavailable, "+
			"so this test is not exercising the transition", rep.MembersUnindexed)
	}
	if len(rep.StaleStamped) != 1 || rep.StaleStamped[0] != "lib" {
		t.Fatalf("StaleStamped = %v, want [lib] — a declared member that is unavailable "+
			"yet still stamped is exactly this field's subject", rep.StaleStamped)
	}
	if len(rep.Dirty) != 0 {
		t.Fatalf("Dirty = %v, want empty — an unavailable member has no re-fold and must "+
			"not be reported dirty; StaleStamped is the trigger here", rep.Dirty)
	}
	if !rep.Resolved {
		t.Fatal("Resolved = false with lib stale-stamped; StaleStamped must trip the gate")
	}
	assertNoCrossEdge(t, wsRoot, targetFile)
	assertStampAbsent(t, wsRoot, "lib")

	// And it settles: the trigger it fired on is the very thing the resolution
	// pruned, so the second pass has nothing to fire on.
	before := overlayContent(t, wsRoot)
	second, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("second Freshen: %v", err)
	}
	if second.Resolved || len(second.StaleStamped) != 0 {
		t.Fatalf("second pass Resolved=%v StaleStamped=%v — the prune must make the "+
			"trigger one-shot, not perpetual", second.Resolved, second.StaleStamped)
	}
	if after := overlayContent(t, wsRoot); after != before {
		t.Fatalf("the settled pass wrote overlay content.\nbefore:\n%s\n\nafter:\n%s",
			before, after)
	}
}

// TestVanishedMemberConvergesInThreePasses is the change's convergence table,
// asserted pass by pass rather than only at its endpoints:
//
//	pass | lib state               | StaleStamped | Resolved | overlay
//	   1 | unavailable, stamped    | [lib]        | true     | pruned
//	   2 | unavailable, unstamped  | []           | false    | unchanged
//	   3 | unavailable, unstamped  | []           | false    | unchanged
//
// Passes 2 and 3 are the point. The primary witness that they wrote nothing is
// STRUCTURAL — Resolved == false means wsresolve.Resolve was never called, so
// no derived write could have happened — and the overlayContent comparison is
// CORROBORATION ONLY. That ordering is deliberate: a content witness is blind
// to a write that stores the same bytes it found, so a redundant re-resolution
// that produced an identical overlay would slip past the snapshot while
// Resolved == false catches it. (The same blindness is spelled out at length on
// TestFreshenCleanPassWritesNoOverlayContent.)
//
// Pass 3 is not redundant with pass 2: pass 2 is the first pass after the state
// change and pass 3 is the first pass after a pass that changed nothing, which
// is where an implementation that re-derived its trigger from the previous
// Report rather than from the overlay would start oscillating.
func TestVanishedMemberConvergesInThreePasses(t *testing.T) {
	wsRoot := freshenedCrossEdgeWS(t)
	if err := os.Remove(memberDB(wsRoot, "lib")); err != nil {
		t.Fatalf("removing lib's index: %v", err)
	}

	// Pass 1: unavailable and still stamped — the trigger fires and prunes.
	first, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("pass 1: %v", err)
	}
	if len(first.StaleStamped) != 1 || first.StaleStamped[0] != "lib" {
		t.Fatalf("pass 1 StaleStamped = %v, want [lib]", first.StaleStamped)
	}
	if !first.Resolved {
		t.Fatal("pass 1 Resolved = false; the table's whole first row is the gate tripping")
	}
	assertNoCrossEdge(t, wsRoot, targetFile)
	assertStampAbsent(t, wsRoot, "lib")

	// Passes 2 and 3: unavailable and unstamped — nothing left to fire on.
	settled := overlayContent(t, wsRoot)
	for pass := 2; pass <= 3; pass++ {
		rep, err := Freshen(wsRoot)
		if err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		if rep.MembersUnindexed != 1 {
			t.Fatalf("pass %d MembersUnindexed = %d, want 1 — lib must still be unavailable, "+
				"or the pass is no longer the one being tested", pass, rep.MembersUnindexed)
		}
		if len(rep.StaleStamped) != 0 {
			t.Fatalf("pass %d StaleStamped = %v, want empty — lib's stamp was pruned on "+
				"pass 1 and nothing restamps an unavailable member", pass, rep.StaleStamped)
		}
		if len(rep.Dirty) != 0 {
			t.Fatalf("pass %d Dirty = %v, want empty", pass, rep.Dirty)
		}
		// PRIMARY: no Resolve ran, so no derived write was possible.
		if rep.Resolved {
			t.Fatalf("pass %d re-resolved: the prune-then-clean cycle is not converging, "+
				"which is the perpetual whole-workspace re-resolution this design forbids", pass)
		}
		// CORROBORATING ONLY — see the doc comment.
		if after := overlayContent(t, wsRoot); after != settled {
			t.Fatalf("pass %d changed overlay content.\nbefore:\n%s\n\nafter:\n%s",
				pass, settled, after)
		}
	}
}

// assertNoCrossEdge is assertCrossEdge's negation: the app -> lib edge with
// Target in dstFile must NOT be in the overlay. Asserting the absence of the
// specific row, rather than "app has no edges", keeps the assertion about the
// pruned member instead of about app being emptied.
func assertNoCrossEdge(t *testing.T, wsRoot, dstFile string) {
	t.Helper()
	src, dst := appToLibEdge(dstFile)
	for _, e := range memberEdges(t, wsRoot, "app") {
		if e.Src == src && e.Dst == dst {
			t.Fatalf("app -> lib cross-edge into the vanished member survived: %+v", e)
		}
	}
}

// assertStampAbsent fails unless memberID carries no overlay stamp.
func assertStampAbsent(t *testing.T, wsRoot, memberID string) {
	t.Helper()
	if _, ok, err := openOverlay(t, wsRoot).Stamp(memberID); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("member %q is still stamped; the prune did not retire it", memberID)
	}
}

// memberEdges reads one member's incident cross-edges.
func memberEdges(t *testing.T, wsRoot, memberID string) []overlay.CrossEdge {
	t.Helper()
	edges, err := openOverlay(t, wsRoot).MemberEdges(memberID)
	if err != nil {
		t.Fatal(err)
	}
	return edges
}

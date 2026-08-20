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

// --- pinned KNOWN LIMITATION, not a guarantee -----------------------------

// TestKNOWNLIMITATIONVanishedMemberLeavesStaleEdgesReportedClean pins CURRENT
// behavior that is WRONG, so that a change to it is noticed rather than
// silently made. Read nothing here as desirable and nothing here as a
// contract: every assertion below describes a defect Freshen's doc comment
// names as a known limitation.
//
// The transition is available -> unavailable. lib is resolved and stamped, and
// app carries a cross-edge into it; then lib's own index is deleted. On the
// next pass lib fails the availability predicate and is skipped BEFORE its
// stamp is read, the manifest is untouched so there is no registry drift, and
// the gate consequently holds. The overlay goes on serving app -> lib edges
// into a member that no longer exists, and Report cannot say so.
//
// A wsresolve.Resolve at that moment would clear app's rows and re-derive them
// without lib as a candidate, dropping the edge. If a future slice makes this
// test fail by dropping the stale edge or by reporting Resolved true, that is
// very likely an IMPROVEMENT — but only if wsresolve.Resolve now PRUNES the
// stamp of an unavailable member. Without that pruning, "fixing" this turns
// the member perpetually dirty and re-resolves the whole workspace on every
// pass forever, which is what TestFreshenConvergesWithABadVersionMember
// forbids. Delete or invert this test deliberately, with that check done.
func TestKNOWNLIMITATIONVanishedMemberLeavesStaleEdgesReportedClean(t *testing.T) {
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
			"so this test is not exercising the limitation", rep.MembersUnindexed)
	}
	if len(rep.Dirty) != 0 || rep.Resolved {
		t.Fatalf("Dirty = %v, Resolved = %v — CURRENT behavior is the gate holding; if this "+
			"changed on purpose, check wsresolve.Resolve prunes unavailable members' stamps "+
			"before accepting it", rep.Dirty, rep.Resolved)
	}
	// The defect itself: the edge into the vanished member survives.
	assertCrossEdge(t, wsRoot, targetFile)
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

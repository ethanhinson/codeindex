package wsfresh

import (
	"fmt"
	"path/filepath"
	"reflect"

	"codeindex/internal/config"
	"codeindex/internal/engine"
	"codeindex/internal/graph"
	"codeindex/internal/overlay"
	"codeindex/internal/query"
	"codeindex/internal/wsresolve"
)

// Freshen runs one freshness pass over the workspace rooted at wsRoot: it
// brings every available member's own index up to date with its working tree,
// re-folds each member's merkle root, and compares that fold against the
// overlay stamp the last resolution left behind.
//
// The pass, in order:
//
//  1. wsRoot must be a workspace root.
//  2. load the manifest, and
//  3. split it into members present on disk and members that are not.
//  4. open the overlay (schema only — no content write on this path).
//  5. for each present member, in manifest order: establish availability,
//     freshen its own index, and re-fold its merkle root.
//  6. mark the member dirty when its stamp is absent or has moved; then, after
//     the loop, walk the manifest once and collect into StaleStamped every
//     DECLARED member that is NOT available this pass yet is still stamped
//     (step 6b).
//  7. compare the stored registry against the manifest's normalized form.
//  8. gate: nothing dirty, nothing stale-stamped and no drift ⇒ return having
//     written no overlay CONTENT at all; otherwise exactly one whole-pass
//     wsresolve.Resolve.
//
// # What a clean pass does and does not entail
//
// Nothing dirty, nothing stale-stamped and no drift means three things: every
// member that is STILL AVAILABLE folds to the root its stamp records, no
// member that has STOPPED being available is still carrying a stamp from when
// it was, and the manifest still matches the stored registry. It does NOT mean
// the overlay's derived set is what a fresh wsresolve.Resolve would produce
// right now — this pass re-derives nothing, so it cannot speak for a row whose
// inputs it never re-read. The available -> unavailable transition used to be
// this claim's one known exception; it is now covered by the second clause,
// and the next section is how.
//
// # A member that goes from available to unavailable
//
// Suppose lib is resolved and stamped, and app carries cross-edges into it.
// Then lib's .codeindex/graph.db is deleted, or lib's root is removed, or a
// repo-wide schema bump makes graph.OpenExisting reject it. lib is counted
// unindexed and skipped at 5a, so it is never freshened and never re-folded,
// and it does NOT enter Dirty: Dirty is a claim about a member's RE-FOLDED
// merkle root — stamp absent, or stamp moved away from that fold — and both
// halves need a fold that an unavailable member does not have. The
// manifest is untouched, so there is no registry drift either. Left there, the
// gate would hold while the overlay went on serving app -> lib cross-edges
// into a member that no longer exists.
//
// What closes it is lib's SURVIVING STAMP, read at step 6b: a declared member
// that is unavailable now but still stamped was resolved by an earlier pass,
// so its rows are in the overlay. Step 6b puts it in StaleStamped, and
// StaleStamped trips the gate exactly as Dirty does.
//
// Acting on that signal converges only because the prune this used to
// forward-reference has LANDED — wsresolve.Resolve retires the whole
// DECLARED ∖ available set across its steps 9a and 11a (change 0015): one
// precomputed set, records deleted at 9a and stamps at 11a. Prune-then-clean,
// pass by pass:
//
//   - Pass 1, the pass on which lib vanishes: StaleStamped is [lib], the gate
//     trips, exactly one whole-pass Resolve runs. It re-derives app without lib
//     as a candidate and prunes lib outright — incident records first, stamp
//     last. Resolved is true.
//   - Pass 2 and onward: lib is still unavailable but no longer stamped, so
//     StaleStamped is empty, Dirty is empty, the gate holds and nothing is
//     written. Resolved is false. The trigger is one-shot per transition, which
//     is what keeps this out of the perpetual whole-workspace re-resolution
//     that TestFreshenConvergesWithABadVersionMember forbids.
//
// Records first and stamp last is what makes that safe rather than merely
// tidy, and it is the reason this function may read the stamp at all. A crash
// between Resolve's two deletes leaves lib unavailable and STILL STAMPED with
// its rows already gone, so 6b fires again on the next pass and the prune
// completes — under-serving, and self-healing. The reverse order would destroy
// the one signal this pass has (an unavailable member cannot reach the
// absent-stamp-is-dirty rule at step 6, which only members that opened ever
// reach) and leave the stale rows served with nothing left to notice them.
// wsresolve.Resolve's doc comment owns that argument in full.
//
// lib's later RETURN to availability needs no machinery of its own: it opens,
// freshens, re-folds, finds no stamp, and absent-stamp-is-dirty at step 6
// re-derives it. Convergence holds in both directions.
//
// # The gate is the whole point
//
// A clean pass must write nothing derived — that is what makes a freshness
// check cheap enough to run unconditionally. A dirty pass re-resolves the
// WHOLE workspace exactly once, never per member: overlay.ReplaceMemberEdges
// deletes on either endpoint, so a per-dirty-member loop clobbers its own
// earlier iterations. Both halves of that are design, not shortcut.
//
// # Availability has exactly one predicate
//
// graph.OpenExisting succeeding, and nothing else. No os.Stat, no
// file-exists check. wsresolve.Resolve uses the same predicate, and the two
// sites must agree: a looser predicate here would call a version-mismatched
// member available, find no usable fold or a fold the resolver never stamps,
// and leave that member permanently dirty — re-resolving the whole workspace
// on every pass, forever.
//
// # No member is ever cold-built
//
// Because availability is established BEFORE query.Fresh is called, Fresh
// always takes its engine.Patch + depmap.VerifyOverlay branch and never its
// cold-build branch. A freshness check must not silently index an arbitrarily
// large repository as a side effect.
//
// # Nothing here is fatal but a broken workspace
//
// A member that is absent, unopenable, or whose own freshen fails is counted
// and skipped: the workspace must keep answering while one member is
// unavailable. Only a bad root, an unloadable manifest, a failed presence
// split, an unopenable overlay, a fold/stamp read that errors on an
// otherwise-available member, or a stamp read that errors during the step-6b
// walk stops the pass. That last one is fatal even though its subject is an
// UNAVAILABLE member, and the distinction is the same one 5d draws: an overlay
// read that ERRORS is a broken overlay, not a member that is merely missing —
// swallowing it would drop the member out of StaleStamped and let the gate
// return a quietly wrong clean verdict.
//
// # Single-threaded, manifest order
//
// query.Fresh holds a package-level mutex for its entire body, so a parallel
// loop would serialize to nothing; determinism of Dirty, MembersMissing and
// StaleStamped is a stated bar, and all three are in manifest order. Dirty and
// MembersMissing get it from iterating members in manifest order; StaleStamped
// gets it from step 6b walking ws.Members once and filtering, rather than from
// appending to the two disjoint subsequences that would be the obvious way to
// build the same SET in the wrong ORDER.
func Freshen(wsRoot string) (Report, error) {
	var rep Report

	// 1. Root kind, checked BEFORE the manifest load — mirroring
	// wsresolve.Resolve — so a plain repo root reports what it actually is
	// instead of a bare fs.ErrNotExist on a path the caller never named.
	kind, err := engine.DetectRootKind(wsRoot)
	if err != nil {
		return rep, fmt.Errorf("wsfresh: %s: %w", wsRoot, err)
	}
	if kind != engine.RootWorkspace {
		return rep, fmt.Errorf("wsfresh: %s is not a workspace root (no %s)",
			wsRoot, config.WorkspaceFile)
	}

	// 2-3. Manifest, then presence.
	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		return rep, fmt.Errorf("wsfresh: %w", err)
	}
	present, missing, err := ws.Resolve(wsRoot)
	if err != nil {
		return rep, fmt.Errorf("wsfresh: %s: %w", wsRoot, err)
	}
	rep.MembersMissing = missing

	// 4. The overlay, opened once for the whole pass. Opening is permitted on
	// the clean path: overlay.Open re-executes the schema and PRAGMA
	// user_version every time, so the file is never a no-write witness. What
	// the clean path must not do is write CONTENT — which is why
	// ReplaceRegistry is NOT called here. Calling it "just to be safe" before
	// the gate would silently defeat the entire change.
	ov, err := overlay.Open(overlay.Path(wsRoot))
	if err != nil {
		return rep, fmt.Errorf("wsfresh: %w", err)
	}
	defer ov.Close()

	// unavailable is the DECLARED ∖ available set: the ids the manifest names
	// that this pass cannot use. It starts as the members absent from disk and
	// collects the present-but-unopenable ones as 5a rejects them, so that step
	// 6b has something to filter ws.Members against. It is a SET, not the
	// ordered answer — the order StaleStamped promises comes from the manifest
	// walk at 6b, never from the order ids were put in here.
	unavailable := make(map[string]bool, len(missing))
	for _, id := range missing {
		unavailable[id] = true
	}

	// 5-6. Per present member, in manifest order.
	for _, rm := range present {
		id := rm.Member.ID

		// 5a. Availability. Every failure class is one answer: unindexed,
		// skipped, no error, no fold and no stamp comparison. The id is
		// RETAINED, because a member this pass could not open may still be
		// carrying a stamp an earlier pass wrote, and that is step 6b's
		// subject.
		st, err := graph.OpenExisting(memberIndexPath(rm.AbsRoot))
		if err != nil {
			rep.MembersUnindexed++
			unavailable[id] = true
			continue
		}
		// 5b. Close before the freshen: query.Fresh runs engine.Patch, which
		// WRITES this same graph.db, and holding a read handle across it
		// invites lock contention.
		st.Close()

		// 5c. The member's own freshen. A failure here is not fatal — and it
		// is NOT the same outcome as 5a. This member opened, so the resolver's
		// one availability predicate accepts it and wsresolve counts it
		// resolved; counting it unindexed here would make Report's stated
		// relation to wsresolve.Stats false in both directions at once.
		if _, err := query.Fresh(rm.AbsRoot); err != nil {
			rep.MembersFreshenFailed++
			continue
		}
		rep.MembersFreshened++

		// 5d. Re-fold, through the canonical fold and no fork of it. The
		// value is OPAQUE: compared for equality only, never parsed, split,
		// or ordered.
		//
		// foldMember re-opens with the SAME graph.OpenExisting predicate 5a
		// used, and unlike 5a a failure here is FATAL. That asymmetry is
		// deliberate: 5a asks "was this member available at all?", and the
		// answer "no" is the ordinary skip. This open asks a strictly later
		// question — the member WAS available moments ago and has just been
		// freshened, so a failure now means the index was rebuilt, removed or
		// version-bumped underneath the pass MID-FLIGHT. That is a different
		// condition, and folding it into the skip path would be unsafe rather
		// than merely lossy: a racing rebuild would drop the member out of
		// Dirty, leave its stale stamp unread, and let the gate return a
		// quietly WRONG clean verdict. A member that was never available
		// cannot produce that error, because it never had a stamp this pass
		// could be wrong about. Failing the pass makes the caller retry
		// against a settled tree instead.
		root, err := foldMember(rm.AbsRoot)
		if err != nil {
			return rep, fmt.Errorf("wsfresh: member %q: %w", id, err)
		}

		// 6. The stamp gate. An ABSENT stamp counts as dirty: that is the
		// crash-self-healing signal 0013's stamps-last write ordering
		// deliberately leaves behind, not an error and not a clean member.
		stamp, ok, err := ov.Stamp(id)
		if err != nil {
			return rep, fmt.Errorf("wsfresh: member %q: %w", id, err)
		}
		if !ok || stamp.MerkleRoot != root {
			rep.Dirty = append(rep.Dirty, id)
		}
	}

	// 6b. The surviving-stamp walk, which is the other half of step 6 and the
	// only place this pass looks at an UNAVAILABLE member's stamp. A declared
	// member that is not available now but is still stamped was resolved by
	// some earlier pass: its cross-member rows are in the overlay and being
	// served, and nothing short of a whole-pass resolution retires them.
	//
	// The walk is over ws.Members, ONCE, filtered by the unavailable set — not
	// `missing` concatenated with the ids 5a rejected. Those two are disjoint
	// manifest-ordered subsequences and their concatenation is not manifest
	// order, which is the order StaleStamped's doc comment promises.
	//
	// DECLARED-scoped, deliberately: the loop cannot see a stamp whose id the
	// manifest does not name, and must not. Orphan stamps are
	// overlay.ReplaceRegistry's prune to take, this pass leaves them alone, and
	// TestFreshenCleanPassWritesNoOverlayContent plants one as a tripwire
	// against exactly this loop being widened to "every stamped member that is
	// not available".
	//
	// A stamp READ is not a content write, so a pass that finds nothing here
	// still reaches the gate having written no overlay content.
	//
	// What comes out is the STAMPED part of the unavailable set, and that is
	// what makes acting on it converge: wsresolve.Resolve prunes precisely
	// DECLARED ∖ available — records first at its step 9a, stamps last at its
	// step 11a — so a member that trips this walk
	// is unstamped by the time the next pass reaches it. Availability is the
	// one graph.OpenExisting predicate at both sites, which is what makes "the
	// set this reads" and "the set Resolve prunes" the same set.
	for _, m := range ws.Members {
		if !unavailable[m.ID] {
			continue
		}
		if _, ok, err := ov.Stamp(m.ID); err != nil {
			return rep, fmt.Errorf("wsfresh: member %q: %w", m.ID, err)
		} else if ok {
			rep.StaleStamped = append(rep.StaleStamped, m.ID)
		}
	}

	// 7. Registry drift. ov.Registry() returns what ReplaceRegistry STORED, so
	// the manifest side is put into that same stored form first, with the very
	// function insertMembers itself uses — the two sides cannot drift apart
	// because there is only one normalizer.
	//
	// The comparison is over WHOLE RECORDS — id, root, ordered namespaces,
	// ordered deps, and member order. Neither weaker comparison is admissible:
	//
	//   - Raw ws.Members, unnormalized: a legal manifest carrying a duplicate
	//     namespace or "deps": [] never equals its own stored form, so every
	//     pass reports drift and re-resolves the whole workspace forever.
	//   - The id set alone: a namespaces: or deps: edit then changes ladder
	//     resolution while every merkle root and every stamp stays put. Zero
	//     dirty members, Resolved false, and the overlay keeps serving edges
	//     derived from the old claims indefinitely. Both fields are ladder
	//     INPUTS — rung 1 resolves on namespaces, Suppress derives precedence
	//     from deps — so this is precisely the silent staleness the gate is
	//     here to prevent.
	stored, err := ov.Registry()
	if err != nil {
		return rep, fmt.Errorf("wsfresh: %w", err)
	}
	drift := !reflect.DeepEqual(stored, overlay.NormalizeMembers(ws.Members))

	// 8. The gate. Nothing dirty, nothing stale-stamped and no drift ⇒ every
	// member that is still AVAILABLE folds to its stamped root, no member that
	// has STOPPED being available is still carrying a stamp, and the manifest
	// still matches the stored registry — so the pass ends having written no
	// CONTENT at all: no registry row, no edge, no ambiguity, no suppression,
	// no stamp. (The overlay was opened, which re-executes schema and PRAGMA
	// user_version; the file is therefore never a no-write witness, and content
	// is. The stamp reads at 6 and 6b are reads, not writes.)
	//
	// StaleStamped is the available -> unavailable transition, and it belongs
	// in the trip condition rather than being reported and ignored: the
	// vanished member's rows are still in the overlay, and a whole-pass
	// wsresolve.Resolve is the only thing that retires them. It converges
	// because that same Resolve prunes the stamp this fired on — see
	// "A member that goes from available to unavailable" in the doc comment
	// above for the pass-by-pass table.
	//
	// It is a SEPARATE condition from Dirty and must stay one. Dirty is a claim
	// about a member's re-folded merkle root — stamp absent, or stamp moved
	// away from that fold — and an unavailable member never got a re-fold, so
	// neither half of Dirty can be said about it. Three tests assert it never
	// appears there.
	//
	// This gate is still strictly weaker than "the derived set is entailed by
	// what is on disk". A clean pass says every available member folds to what
	// it was stamped at and no vanished member is still stamped; it does not
	// re-derive anything, so it cannot speak for rows whose inputs never moved.
	//
	// An unindexed, freshen-failed or missing member does not by itself make
	// the pass dirty: there is nothing new to derive from a member with no
	// usable index, and treating unavailability alone as dirty would re-resolve
	// the workspace on every pass for as long as it stays unavailable. The
	// narrower condition that DOES re-resolve is such a member still being
	// STAMPED, and that is one-shot, because the resolution clears it.
	if len(rep.Dirty) == 0 && len(rep.StaleStamped) == 0 && !drift {
		return rep, nil
	}

	// Otherwise exactly ONE whole-pass resolution. Not a per-member or
	// incident-scoped one: ReplaceMemberEdges deletes rows incident to a member
	// on EITHER endpoint (WHERE src_member = ? OR dst_member = ?), so the
	// obvious loop over dirty members deletes what the previous iteration just
	// wrote — this repo has shipped that bug once already. Scoped
	// re-resolution is deferred by the spec; the whole pass is the decision.
	//
	// Resolve re-derives root kind, manifest, presence and the member opens
	// this function just did. That duplication is accepted: Resolve(wsRoot
	// string) is a frozen signature and widening it would re-litigate change
	// 0013. The cost falls on the dirty path only.
	stats, err := wsresolve.Resolve(wsRoot)
	if err != nil {
		return rep, fmt.Errorf("wsfresh: %w", err)
	}
	rep.Resolved = true
	rep.Stats = stats
	return rep, nil
}

// foldMember re-opens absRoot's index and folds it to its canonical merkle
// root, closing the handle before returning. It exists so the open/fold/close
// triple is not open-coded next to the freshen it must follow.
//
// It uses the same graph.OpenExisting availability predicate as step 5a, but
// reports an open failure as an error rather than swallowing it: by the time
// this runs the member has already opened once this pass, so a failure here
// is a mid-flight mutation, not an unavailable member. Step 5d's call site
// carries the full argument for why that outcome is fatal there.
func foldMember(absRoot string) (string, error) {
	st, err := graph.OpenExisting(memberIndexPath(absRoot))
	if err != nil {
		return "", err
	}
	defer st.Close()
	return st.MemberMerkleRoot()
}

// memberIndexPath is a member's own index inside its repo root.
//
// TODO: this is the sixth open-coded copy of the ".codeindex/graph.db" join
// (see the identical note on wsresolve.memberIndexPath, which lists the other
// five). Consolidating them into one exported constant is deliberately out of
// scope here — a future change should do it across all sites at once.
func memberIndexPath(absRoot string) string {
	return filepath.Join(absRoot, ".codeindex", "graph.db")
}

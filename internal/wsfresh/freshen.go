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
//  6. mark the member dirty when its stamp is absent or has moved.
//  7. compare the stored registry against the manifest's normalized form.
//  8. gate: nothing dirty and no drift ⇒ return having written no overlay
//     CONTENT at all; otherwise exactly one whole-pass wsresolve.Resolve.
//
// # What a clean pass does and does not entail
//
// Nothing dirty and no drift means every member that is STILL AVAILABLE folds
// to the root its stamp records, and the manifest still matches the stored
// registry. It does NOT mean the overlay's derived set is what a fresh
// wsresolve.Resolve would produce right now. There is one known exception, and
// it is the available -> unavailable transition.
//
// # Known limitation: a member that goes from available to unavailable
//
// Suppose lib is resolved and stamped, and app carries cross-edges into it.
// Then lib's .codeindex/graph.db is deleted, or lib's root is removed, or a
// repo-wide schema bump makes graph.OpenExisting reject it. On the next pass
// lib is counted unindexed and skipped at 5a, so its stamp is never read — step
// 6 only reads ov.Stamp for members that opened. The manifest is untouched, so
// there is no registry drift either. The gate therefore holds, Report says
// Resolved false with an empty Dirty, and the overlay goes on serving
// app -> lib cross-edges into a member that no longer exists. A
// wsresolve.Resolve run at that moment would clear app's rows and re-derive
// them WITHOUT lib as a candidate, dropping those edges. Report has no field
// that separates "clean" from "clean but still serving edges into a vanished
// member"; callers must not read Resolved false as "the overlay equals a fresh
// resolution".
//
// This is deliberate, not an oversight, and the repair is NOT local to this
// function. The detection signal already exists and is deliberately unread: a
// stamp present for a member that is now unavailable. Acting on it here —
// reading the stamp of a member that failed 5a and marking it dirty — makes
// that member dirty on EVERY subsequent pass, because wsresolve.Resolve never
// PRUNES the stamp of a member it could not open. That is a perpetual
// re-resolution of the whole workspace with no source ever changing: precisely
// the non-convergence Assumption 10 and plan test 7
// (TestFreshenConvergesWithABadVersionMember) exist to forbid.
//
// So the honest fix is stamp pruning inside wsresolve.Resolve — prune, or
// otherwise retire, the stamp of a member that is unavailable at resolution
// time — and only THEN may this pass treat a surviving stamp for an
// unavailable member as dirty. wsresolve.Resolve is frozen by change 0013, so
// that ordering is a hard prerequisite. A later slice (§4.1) must not build on
// the stronger entailment claim, and must not "fix" this by reading the stamp
// here alone.
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
// split, an unopenable overlay, or a fold/stamp read that errors on an
// otherwise-available member stops the pass.
//
// # Single-threaded, manifest order
//
// query.Fresh holds a package-level mutex for its entire body, so a parallel
// loop would serialize to nothing; determinism of Dirty and MembersMissing is
// a stated bar, and both are in manifest order.
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

	// 5-6. Per present member, in manifest order.
	for _, rm := range present {
		id := rm.Member.ID

		// 5a. Availability. Every failure class is one answer: unindexed,
		// skipped, no error, overlay rows left alone.
		st, err := graph.OpenExisting(memberIndexPath(rm.AbsRoot))
		if err != nil {
			rep.MembersUnindexed++
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

	// 8. The gate. Nothing dirty and no drift ⇒ every member that is still
	// AVAILABLE folds to its stamped root and the manifest still matches the
	// stored registry, so the pass ends having written no CONTENT at all: no
	// registry row, no edge, no ambiguity, no suppression, no stamp.
	//
	// That is strictly weaker than "the derived set is entailed by what is on
	// disk", and deliberately so — see the known limitation in the doc comment:
	// a member that was available and stamped and has since become unavailable
	// is skipped before its stamp is ever read, so the gate holds while the
	// overlay keeps serving cross-edges into it. Closing that needs stamp
	// pruning in wsresolve.Resolve first; do not close it here. (The overlay was opened, which re-executes schema and PRAGMA
	// user_version; the file is therefore never a no-write witness, and
	// content is.)
	//
	// An unindexed, freshen-failed or missing member does not by itself make
	// the pass dirty:
	// there is nothing new to derive from a member with no usable index, and
	// treating it as dirty would re-resolve the workspace on every pass for as
	// long as it stays unavailable.
	if len(rep.Dirty) == 0 && !drift {
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

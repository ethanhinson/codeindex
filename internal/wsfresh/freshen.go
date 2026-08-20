package wsfresh

import (
	"fmt"
	"path/filepath"

	"codeindex/internal/config"
	"codeindex/internal/engine"
	"codeindex/internal/graph"
	"codeindex/internal/overlay"
	"codeindex/internal/query"
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
//
// Steps 7 and 8 — registry drift and the resolution gate — are NOT here yet;
// see the seam at the end of this function. Until they land, Freshen always
// returns Resolved false and writes no overlay content at all.
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

		// 5c. The member's own freshen. A failure here is not fatal.
		if _, err := query.Fresh(rm.AbsRoot); err != nil {
			rep.MembersUnindexed++
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

	// SEAM — Task 4 continues here with step 7 (registry drift: ov.Registry()
	// against overlay.NormalizeMembers(ws.Members), whole records, member
	// order included) and step 8 (the gate: no dirty member and no drift ⇒
	// return with Resolved false and zero overlay content writes; otherwise
	// exactly one whole-pass wsresolve.Resolve(wsRoot), Resolved true, its
	// Stats carried into the Report). Until then the dirty list is computed
	// and reported but not acted on.
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

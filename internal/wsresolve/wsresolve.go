// Package wsresolve implements the workspace resolution ladder: resolving
// cross-member import hints against declared member namespaces.
//
// # Recorded obligation to the union-graph query layer (openspec §4.1)
//
// Member-over-dep precedence (see Suppress) does NOT mutate the consumer's
// graph.db: when member C vendors a namespace member O owns, C's intra-repo
// edge into its own tier-1 copy survives, and the overlay MAY ADDITIONALLY
// carry a cross-edge from the same call site into O. Where it does, a
// union-graph query over C that simply unions the two sources would count that
// one call TWICE.
//
// "May", not "does": a suppression is recorded on owner-uniqueness alone, but
// the re-pointed edge only becomes a cross-edge when the name also resolves in
// O. When it does not, the edge falls through rungs 2-4 (see Suppressed.Repoint)
// and the call site's ONLY edge is the surviving intra-repo one — a suppression
// record can therefore exist with no cross-edge behind it.
//
// §4.1 must read dep_suppressions and filter out an intra-repo edge whose
// resolved target is a tier-1 symbol in a suppressed namespace ONLY WHEN the
// overlay carries a cross-edge from the same call site — same source key
// (src_file, src_name, src_parent), same kind, same line. Absent such a
// cross-edge there is nothing to double-count, and filtering would delete the
// consumer's still-correct tier-1 edge with nothing replacing it. This package
// writes the record that makes that filter possible and does nothing else about
// it; nothing reads the overlay yet, so no double-count is observable before
// §4.1 exists. This is a stated obligation, not a side effect — deleting the
// filter requirement without replacing it reintroduces the double-count, and
// widening it past the same-call-site condition drops correct edges.
package wsresolve

import (
	"fmt"
	"path/filepath"

	"codeindex/internal/config"
	"codeindex/internal/engine"
	"codeindex/internal/graph"
	"codeindex/internal/overlay"
)

// Stats reports one resolution pass. MembersResolved + MembersUnavailable is
// always the number of members the manifest DECLARES: every declared member is
// either re-derived or accounted for as unusable, and there is no third
// outcome.
type Stats struct {
	// MembersResolved counts the members whose overlay contribution this pass
	// rewrote — present on disk with a usable index.
	MembersResolved int
	// MembersUnavailable counts declared members with no usable index: absent
	// from disk, indexed at a different schema version, or otherwise
	// unopenable. Not an error — see Resolve. These members are PRUNED from
	// the overlay by step 9a: the pass deletes their incident records and then
	// their stamp, so a member the resolver could not see contributes nothing
	// and leaves nothing behind.
	MembersUnavailable int
	// CrossEdges, Ambiguities and Suppressions count the records this pass
	// derived. Two derived records can share the same storage key
	// (PutCrossEdges upserts on (src, dst, kind, line); putAmbiguities
	// deletes-then-inserts on its natural key), so these counts can exceed
	// the number of rows actually persisted.
	CrossEdges   int
	Ambiguities  int
	Suppressions int
}

// Resolve runs one whole-workspace resolution pass over wsRoot: it re-derives
// every available member's cross-repo contribution and writes it to the
// workspace overlay. It is this package's only exported entry point, and it
// has no in-tree caller yet — §4.1's union-graph query layer is the consumer.
//
// The pass, in order:
//
//  1. wsRoot must be a workspace root.
//  2. load the manifest, and
//  3. split it into members present on disk and members that are not.
//  4. open the overlay, and
//  5. mirror the manifest into it as-built.
//  6. open each present member's index with graph.OpenExisting.
//  7. derive member-over-dep suppressions across the available members.
//  8. run the ladder for each available member, accumulating in MEMORY.
//  9. prune every DECLARED member that is not available this pass — its
//     records first, its stamp last (step 9a) — then clear every available
//     member's prior rows,
//  10. write the whole derived set, and
//  11. stamp every available member LAST.
//
// # Why steps 9 and 10 are separate
//
// This is the load-bearing ordering decision of the whole slice.
// overlay.ReplaceMemberEdges(M, …) deletes rows incident to M on EITHER end,
// so the obvious derive-and-write-per-member loop is wrong: the call for M₂
// deletes the S₁ → M₂ edges the call for M₁ just wrote, and a full pass silently
// loses every cross-edge whose destination member is processed after its
// source. Clearing every available member first with empty inputs — which the
// overlay's own validation accepts trivially — and then writing the accumulated
// set through the NON-deleting Put* calls is the only composition of the
// existing overlay API that is correct for a full pass. No new overlay method
// exists for this, deliberately.
//
// # Missing and unavailable members
//
// A member with no usable index contributes no candidates, is not a candidate
// target, and is not stamped by this pass. It is counted in
// Stats.MembersUnavailable and the pass returns NO error: an unbuilt or
// checked-out-elsewhere member is a runtime condition, and a workspace must
// keep answering while one member is unavailable. An absent member and a member
// whose index is absent, version-mismatched or unopenable are the same case
// here.
//
// Its overlay rows, and any stamp an earlier pass left it, do NOT stay behind.
// Step 9a PRUNES every member the manifest DECLARES that is not available this
// pass — the set Stats.MembersUnavailable counts, which is the members absent
// from disk plus every present member graph.OpenExisting could not open —
// running before step 9 clears anything and therefore before any of step 10's
// writes. Rows joining two unavailable
// members go with the rest: an edge between two members the resolver could not
// see is precisely an edge the overlay must not carry, and it is the one class
// nothing else would ever clear, since step 9 only reaches rows incident to an
// available member. The prune set is DECLARED-scoped on purpose. A stamp or a
// row for an id the manifest no longer declares is not touched here; that is
// ReplaceRegistry's orphan prune (step 5). Widening step 9a to "every stamped
// member not available this pass" would reach past this pass's own subject
// matter into ids the manifest never named, and internal/wsfresh plants an
// orphan stamp for one such id as a tripwire against exactly that.
//
// Step 9 still collaterally deletes an unavailable member's rows that are
// incident to an available one. That remains unavoidable and correct: the
// available member's whole contribution is being rewritten and such an edge
// cannot be re-derived at this pass.
//
// # Why the prune deletes records first and the stamp last
//
// Per member the order is fixed: ReplaceMemberEdges first, DeleteStamp last.
// It is the same stamp-last rule step 11 follows for writes, applied to
// deletes, and it is what makes a mid-prune crash recoverable.
//
//   - Completed prune: the member is unstamped and carries no rows. Its later
//     RETURN to availability is caught by the absent-stamp-is-dirty rule in
//     internal/wsfresh and re-derives cleanly.
//   - Crash between the two calls: the member is STILL STAMPED and its rows are
//     already gone. That surviving stamp is the signal — a declared member that
//     is unavailable yet still stamped lands in wsfresh.Report.StaleStamped,
//     which trips the freshen gate, so the next freshen pass re-runs Resolve
//     and the prune completes. Under-serving, and self-healing.
//
// The reverse order is unsafe, which is why it is spelled out here. An
// unavailable member is skipped before its stamp is read, so it can never enter
// wsfresh's Dirty set by the absent-stamp route; deleting the stamp first and
// then dying would leave the stale rows served with NO signal left that could
// ever fire again, silently restoring the hole. Records-first fails towards
// under-serving; stamp-first fails towards over-serving stale cross-edges.
//
// # Crash safety
//
// The pass is not one transaction and does not need to be. Each overlay call is
// individually atomic, the overlay holds no primary data, and every row is
// re-derivable by re-running. Stamps go last, so a pass that dies part-way
// leaves the affected members stampless and a later staleness gate re-resolves
// them: crash-safety carried by the ordering, not by a transaction the overlay
// API does not offer. Step 9a carries the same ordering in the other direction
// — see "Why the prune deletes records first and the stamp last" above — so a
// crash inside the prune also leaves a stamp the gate can fire on.
//
// No member graph.db is ever written. Indexes are opened with
// graph.OpenExisting, never graph.Open, which would create an absent index and
// delete-and-rebuild a version-mismatched one.
func Resolve(wsRoot string) (Stats, error) {
	var stats Stats

	// 1. Root kind. Checked before the manifest load so a repo root reports
	// what it actually is: LoadWorkspace would report the same directory as a
	// bare fs.ErrNotExist on a path the caller never named.
	kind, err := engine.DetectRootKind(wsRoot)
	if err != nil {
		return stats, fmt.Errorf("wsresolve: %s: %w", wsRoot, err)
	}
	if kind != engine.RootWorkspace {
		return stats, fmt.Errorf("wsresolve: %s is not a workspace root (no %s)",
			wsRoot, config.WorkspaceFile)
	}

	// 2-3. Manifest, then presence.
	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		return stats, fmt.Errorf("wsresolve: %w", err)
	}
	present, missing, err := ws.Resolve(wsRoot)
	if err != nil {
		return stats, fmt.Errorf("wsresolve: %s: %w", wsRoot, err)
	}
	stats.MembersUnavailable = len(missing)

	// 4-5. Overlay, and the registry mirrored as-built. The registry carries
	// every DECLARED member, missing ones included: presence is a runtime
	// condition, so it is not persisted.
	ov, err := overlay.Open(overlay.Path(wsRoot))
	if err != nil {
		return stats, fmt.Errorf("wsresolve: %w", err)
	}
	defer ov.Close()
	if err := ov.ReplaceRegistry(ws); err != nil {
		return stats, fmt.Errorf("wsresolve: %w", err)
	}

	// 6. Open each present member's index, read-only and non-creating. Every
	// failure class is the same answer: unavailable, skipped, no error.
	var available []Member
	for _, rm := range present {
		st, err := graph.OpenExisting(memberIndexPath(rm.AbsRoot))
		if err != nil {
			stats.MembersUnavailable++
			continue
		}
		defer st.Close()
		available = append(available, Member{
			ID:         rm.Member.ID,
			Namespaces: rm.Member.Namespaces,
			Deps:       rm.Member.Deps,
			Store:      st,
		})
	}
	stats.MembersResolved = len(available)

	// 7. Member-over-dep precedence across every available member.
	sup, err := Suppress(available)
	if err != nil {
		return stats, fmt.Errorf("wsresolve: %w", err)
	}

	// 8. Ladder every available member, in manifest order, into memory. The
	// same `available` slice is the candidate set for every source: Ladder
	// filters the source out itself. One Pass is created for the whole loop, so
	// the defs(X) cache is shared across sources — every source asks the same
	// targets the same questions, and a per-source cache would re-issue each
	// lookup once per source.
	pass := NewPass()
	var (
		crossEdges  []overlay.CrossEdge
		ambiguities []overlay.Ambiguity
	)
	for _, m := range available {
		edges, err := m.Store.UnresolvedEdges()
		if err != nil {
			return stats, fmt.Errorf("wsresolve: member %q: %w", m.ID, err)
		}
		// The re-pointed tier-1 edges are the named widening of the candidate
		// set: they are not today's unresolved edges, but suppression made them
		// resolvable, and they enter the ladder at rung 1 on the suppressed
		// namespace.
		edges = append(edges, sup.Repoint[m.ID]...)

		recs, err := pass.Ladder(m, available, edges)
		if err != nil {
			return stats, fmt.Errorf("wsresolve: member %q: %w", m.ID, err)
		}
		crossEdges = append(crossEdges, recs.CrossEdges...)
		ambiguities = append(ambiguities, recs.Ambiguities...)
	}

	// 9a. Prune every DECLARED member that is not available this pass, in
	// MANIFEST ORDER, before the clear loop and therefore before any of step
	// 10's writes. The set is ws.Members ∖ available: exactly what
	// Stats.MembersUnavailable counts, and exactly the set wsfresh walks to
	// fill Report.StaleStamped, because availability is the one
	// graph.OpenExisting predicate both sites share.
	//
	// The order is derived by walking ws.Members ONCE and filtering. It is not
	// `missing` concatenated with the ids that failed to open above: those are
	// two disjoint manifest-ordered subsequences, and their concatenation is
	// not manifest order.
	//
	// Records first, stamp last, per member — the safety argument is in the doc
	// comment and must not be reversed. The prune is unconditional: DeleteStamp
	// and an empty ReplaceMemberEdges are both no-ops when there is nothing to
	// remove, so no "does it have rows?" pre-check buys anything, and a second
	// pass over an already-pruned member deletes nothing.
	//
	// The loop cannot clobber itself the way a per-member derive-and-write loop
	// would (see above): it passes empty inputs, so there is no write for a
	// later iteration's either-end delete to take back.
	//
	// Not a gap: ReplaceMemberEdges deletes dep_suppressions on consumer_member
	// only, never owner_member (deliberately — internal/overlay/edges.go), so
	// rows where the pruned member is merely the OWNER survive this call. The
	// prune is still complete, by a different argument. Every surviving such
	// row's consumer is one of three things: available — cleared at step 9 and
	// re-derived at step 10 without the pruned member, since Suppress draws
	// only from `available`; itself unavailable — cleared by its own iteration
	// here; or no longer declared at all — in which case ReplaceRegistry's
	// orphan prune already deleted it at step 5, on EITHER column. Do not
	// "fix" this by widening the delete.
	avail := make(map[string]bool, len(available))
	for _, m := range available {
		avail[m.ID] = true
	}
	for _, dm := range ws.Members {
		if avail[dm.ID] {
			continue
		}
		// Records first ...
		if err := ov.ReplaceMemberEdges(dm.ID, nil, nil, nil); err != nil {
			return stats, fmt.Errorf("wsresolve: pruning member %q: %w", dm.ID, err)
		}
		// ... stamp last.
		if err := ov.DeleteStamp(dm.ID); err != nil {
			return stats, fmt.Errorf("wsresolve: pruning member %q: %w", dm.ID, err)
		}
	}

	// 9. Clear every available member's prior contribution — all of it, before
	// any of the new set is written. See the doc comment above.
	for _, m := range available {
		if err := ov.ReplaceMemberEdges(m.ID, nil, nil, nil); err != nil {
			return stats, fmt.Errorf("wsresolve: clearing member %q: %w", m.ID, err)
		}
	}

	// 10. Write the whole derived set with the non-deleting writers.
	if err := ov.PutCrossEdges(crossEdges); err != nil {
		return stats, fmt.Errorf("wsresolve: %w", err)
	}
	if err := ov.PutAmbiguities(ambiguities); err != nil {
		return stats, fmt.Errorf("wsresolve: %w", err)
	}
	if err := ov.PutSuppressions(sup.Records); err != nil {
		return stats, fmt.Errorf("wsresolve: %w", err)
	}

	// 11. Stamps last: a pass that dies before here leaves the affected members
	// stampless, which is exactly what makes a later staleness gate re-resolve
	// them.
	for _, m := range available {
		root, err := m.Store.MemberMerkleRoot()
		if err != nil {
			return stats, fmt.Errorf("wsresolve: member %q: %w", m.ID, err)
		}
		if err := ov.PutStamp(m.ID, root); err != nil {
			return stats, fmt.Errorf("wsresolve: member %q: %w", m.ID, err)
		}
	}

	stats.CrossEdges = len(crossEdges)
	stats.Ambiguities = len(ambiguities)
	stats.Suppressions = len(sup.Records)
	return stats, nil
}

// memberIndexPath is a member's own index inside its repo root. It mirrors the
// layout every other reader uses; there is no exported constant for it.
//
// TODO: this is the fifth open-coded copy of the ".codeindex/graph.db" join.
// Siblings: internal/webserver/graphstore.go, internal/readmodel/graph.go,
// internal/query/query.go, internal/engine/artifact.go. Consolidating them
// into one exported constant is deliberately out of scope for this slice —
// a future change should do it across all five sites at once.
func memberIndexPath(absRoot string) string {
	return filepath.Join(absRoot, ".codeindex", "graph.db")
}

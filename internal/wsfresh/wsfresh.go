// Package wsfresh is the workspace freshen pass: per-member freshen plus
// stamp-gated re-resolution of the workspace overlay.
//
// # This package is UNWIRED
//
// It has no verb and no non-test caller. That is deliberate, not an oversight.
// The `workspace-status` verb is NOT here: it is gated at verb wiring by the
// D7 second amendment of the workspace-graph design (owner ruling 2026-08-19),
// and rides a later gated change together with openspec §4. Nothing in this
// package may add a verb, a CLI flag, or an MCP surface; a caller arrives with
// that later change or not at all.
package wsfresh

import (
	"codeindex/internal/wsresolve"
)

// Report describes one freshen pass. It is PLAIN DATA and nothing more: no
// coverage clause, no confidence vocabulary, no status string, and no method
// that shapes an answer. Naming stale or missing members to a user belongs to
// the later query slice (§4.1), which owns the vocabulary for it.
//
// # Report and wsresolve.Stats count different things — read both
//
// A Report can carry a wsresolve.Stats, and the two use similar words over
// DIFFERENT denominators. Read this whole comment before adding a field or
// reusing a name from the other type.
//
// wsresolve.Stats partitions the DECLARED members: its stated invariant is
// MembersResolved + MembersUnavailable == len(declared), so its
// MembersUnavailable folds together every declared member the resolver could
// not use — absent from disk, unindexed, version-mismatched, unopenable.
//
// Report partitions the same declared members FOUR ways instead of two:
// MembersFreshened, MembersFreshenFailed, MembersUnindexed, and
// len(MembersMissing). The split is what keeps the two types readable against
// each other, because the resolver's two-way partition cuts across it:
//
//	Stats.MembersUnavailable == MembersUnindexed + len(MembersMissing)
//	Stats.MembersResolved    == MembersFreshened + MembersFreshenFailed
//
// Both identities hold only when Resolved is true (otherwise Stats was never
// filled in) and assume the workspace on disk did not change between this
// pass's member loop and the resolution it triggered. The point of the fourth
// counter is that the second identity is TRUE: a member whose own freshen
// failed still opened, so the resolver counts it RESOLVED, and folding it into
// MembersUnindexed would have made both identities false at once.
type Report struct {
	// MembersFreshened counts the available members whose per-repo freshen
	// actually ran — present on disk, index openable, freshen returned no
	// error.
	MembersFreshened int

	// MembersFreshenFailed counts members that were AVAILABLE — present on
	// disk, graph.OpenExisting succeeded — but whose per-repo query.Fresh
	// returned an error, so the pass skipped them without folding or stamping.
	//
	// It is separate from MembersUnindexed on purpose. Such a member IS
	// indexed; only the patch failed. The resolver's one availability
	// predicate is graph.OpenExisting, which this member passes, so
	// wsresolve counts it in Stats.MembersResolved, not in
	// Stats.MembersUnavailable — see the identities in the type comment.
	MembersFreshenFailed int

	// MembersFreshenFailedIDs lists the ids behind the MembersFreshenFailed
	// count, in manifest order. It is written at the SAME site as the count,
	// so the two cannot disagree: len(MembersFreshenFailedIDs) ==
	// MembersFreshenFailed always holds.
	MembersFreshenFailedIDs []string

	// MembersUnindexed counts members PRESENT on disk whose graph.OpenExisting
	// failed: no index, schema-version mismatch, otherwise unopenable. It
	// EXCLUDES members absent from disk (those are in MembersMissing) and
	// members that opened but failed to freshen (those are in
	// MembersFreshenFailed).
	//
	// It is deliberately NOT called MembersUnavailable. The name
	// MembersUnavailable is already spoken for by wsresolve.Stats over a
	// LARGER set: the resolver's MembersUnavailable counts unindexed AND
	// missing members together, so that MembersResolved + MembersUnavailable
	// == len(declared). Naming this field the same would put two denominators
	// under one name, which is the drift shape this repo has already paid for
	// twice — the tell being two doc comments in the same area that explain
	// opposite treatments of the same data and each read fine alone. Expect
	// MembersUnindexed <= Stats.MembersUnavailable, with equality exactly when
	// MembersMissing is empty.
	MembersUnindexed int

	// MembersUnindexedIDs lists the ids behind the MembersUnindexed count, in
	// manifest order. It is written at the SAME site as the count, so the two
	// cannot disagree: len(MembersUnindexedIDs) == MembersUnindexed always
	// holds. That is the one-name-one-denominator discipline
	// MembersFreshenFailedIDs already follows for its own count, and it is
	// ADDITIVE — MembersUnindexed keeps its type and its meaning.
	//
	// The ids are LOAD-BEARING rather than merely convenient. A declared member
	// that will not open has its rows omitted from every workspace answer, and
	// no other field of this Report discloses that DURABLY: StaleStamped is
	// non-empty for at most one pass per transition and is then pruned, and
	// MembersMissing does not cover a member still present on disk. Without
	// these ids the query surface has a count it cannot attribute, which is a
	// disclosure it cannot make — see wsquery.session.staleMembers.
	MembersUnindexedIDs []string

	// MembersMissing lists the ids of members the manifest DECLARES but which
	// are absent from disk, or present but not a directory — as
	// config.Workspace.Resolve defines it — in manifest order. Disjoint from
	// the MembersUnindexed count; together the two make up the resolver's
	// Stats.MembersUnavailable exactly.
	MembersMissing []string

	// Dirty lists the ids of members whose overlay stamp was absent or had
	// moved away from the member's re-folded merkle root, in manifest order.
	// An absent stamp counts as dirty: that is the self-healing signal the
	// resolver's stamps-last write order leaves behind after a pass that died
	// part-way.
	Dirty []string

	// StaleStamped lists the ids of members the manifest DECLARES that are NOT
	// available this pass — absent from disk, or present with an index
	// graph.OpenExisting could not open — and yet still carry an overlay stamp
	// some earlier pass left behind, in manifest order. It is the
	// available -> unavailable transition, and like Dirty it trips the gate:
	// such a member's cross-member rows are still in the overlay being served,
	// and only a whole-pass wsresolve.Resolve retires them.
	//
	// It is deliberately NOT folded into Dirty, and that is a correctness point
	// rather than a taste one. Dirty's stated definition is "stamp absent or
	// moved away from the member's RE-FOLDED merkle root" — an unavailable
	// member has no re-fold, so there is nothing for its stamp to have moved
	// away from. Putting it in Dirty would make that definition false and break
	// Dirty ⊆ the members this pass freshened; three live assertions require
	// that an unavailable member never appears in Dirty. This package has
	// already paid twice for one name carrying two denominators (see
	// MembersUnindexed), so the discipline here is to split.
	//
	// It is DECLARED-scoped. A stamp for an id the manifest no longer names is
	// not this field's subject at all: that is an orphan, and orphans belong to
	// overlay.ReplaceRegistry's prune.
	//
	// It is non-empty for at most ONE pass per transition, which is what makes
	// acting on it converge rather than re-resolve the workspace forever:
	// wsresolve.Resolve deletes each unavailable declared member's records at
	// its step 9a and THEN, once the rest of the pass has succeeded, its stamp
	// at its step 11a, so the next pass finds nothing stamped, this
	// field is empty, and the gate holds. Reading a stamp is not a content
	// write, so a pass that finds nothing here still writes nothing.
	StaleStamped []string

	// Resolved records whether this pass ran wsresolve.Resolve. False means
	// the gate held on all THREE of its clauses: no dirty member, no
	// stale-stamped member, and no registry drift — so no overlay content was
	// written.
	Resolved bool

	// Stats is the resolver pass's own report. It is MEANINGFUL ONLY WHEN
	// Resolved is true; when Resolved is false no resolution ran and this is
	// the zero value, which must not be read as "zero members resolved,
	// zero unavailable" — nothing was counted at all. When it is meaningful,
	// its counts are over the resolver's denominators, not this Report's: see
	// the type comment above, and Stats' own field comments in
	// internal/wsresolve.
	Stats wsresolve.Stats
}

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
// Report partitions the same declared members three ways instead of two:
// MembersFreshened, MembersUnindexed, and len(MembersMissing). Its
// MembersUnindexed is therefore a STRICTLY NARROWER count than the resolver's
// MembersUnavailable, which is why it does not carry that name.
type Report struct {
	// MembersFreshened counts the available members whose per-repo freshen
	// actually ran — present on disk, index openable, freshen returned no
	// error.
	MembersFreshened int

	// MembersUnindexed counts members PRESENT on disk whose graph.OpenExisting
	// failed (no index, schema-version mismatch, otherwise unopenable), plus
	// those whose per-repo freshen failed. It EXCLUDES members that are absent
	// from disk — those are in MembersMissing.
	//
	// It is deliberately NOT called MembersUnavailable. The name
	// MembersUnavailable is already spoken for by wsresolve.Stats over a
	// LARGER set: the resolver's MembersUnavailable counts unindexed AND
	// missing members together, so that MembersResolved + MembersUnavailable
	// == len(declared). Naming this field the same would put two denominators
	// under one name, which is the drift shape this repo has already paid for
	// twice — the tell being two doc comments in the same area that explain
	// opposite treatments of the same data and each read fine alone. Expect
	// MembersUnindexed <= Stats.MembersUnavailable, never equality in general.
	MembersUnindexed int

	// MembersMissing lists the ids of members the manifest DECLARES but which
	// are absent from disk, in manifest order. Disjoint from the
	// MembersUnindexed count; both are inside the resolver's
	// Stats.MembersUnavailable.
	MembersMissing []string

	// Dirty lists the ids of members whose overlay stamp was absent or had
	// moved away from the member's re-folded merkle root, in manifest order.
	// An absent stamp counts as dirty: that is the self-healing signal the
	// resolver's stamps-last write order leaves behind after a pass that died
	// part-way.
	Dirty []string

	// Resolved records whether this pass ran wsresolve.Resolve. False means
	// the gate held: no dirty member and no registry drift, so no overlay
	// content was written.
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

---
id: 13
slug: unindexed-member-named-stale-every-pass
title: An unindexed workspace member is named stale on every pass, not just the transition
status: Accepted
date: 2026-08-20
supersedes: []
reverses: []
relates_to: [11, 12]
change: 16
---

## Context

The workspace coverage clause reports `members_stale` so an agent can tell an incomplete
cross-repo answer from a complete one. Design spec
`2026-08-20-workspace-query-surfaces-gated-design.md` §4.3 defined that set as a **four-way**
union of `wsfresh.Report`'s `Dirty`, `StaleStamped`, `MembersMissing`, and the freshen-failed
ids — and deliberately **excluded** `MembersUnindexed` (a member present on disk whose index
cannot be opened), on the stated ground that such a member is "covered by StaleStamped when it
previously contributed rows, and by boundary when it never did."

Whole-branch review established that **both halves of that cover argument are false**:

- `StaleStamped` is **one-shot** — `wsfresh.Report.StaleStamped`'s own doc comment says it is
  "non-empty for at most ONE pass per transition." The `wsresolve.Resolve` triggered by the
  transition pass prunes the very stamp that fired it.
- `boundary` is a **fixed constant string** ("symbols outside this workspace are unknown to
  it"). It says nothing about a *declared* member *inside* the workspace whose rows were omitted.

So the steady state was: a present, declared, unbuilt member whose rows are silently absent from
every answer while `members_stale` reads `(none)`. A freshly cloned member that has never been
built reaches that state on the very first query. This is **silent staleness**, which the
campaign's pre-registered D7 evidence gate treats as a HARD FAIL — so the slice would have
shipped, under its own gate, the exact condition that gate refuses. The build had initially
characterized this as a known limitation (a `TestBYDESIGN…` test) rather than closing it.

## Decision

`members_stale` is a **five-way** union: `Dirty` (dropped only when `Report.Resolved` is true),
`StaleStamped`, `MembersMissing`, `MembersFreshenFailedIDs`, and `MembersUnindexedIDs`. Ids are
de-duplicated by set union, which is also what answers §4.3's original double-counting concern.

`wsfresh.Report` gained `MembersUnindexedIDs []string`, written **additively at the same site**
as the existing `MembersUnindexed` count increment so the two cannot disagree — the same
one-name-one-denominator pattern the package already uses for
`MembersFreshenFailed`/`MembersFreshenFailedIDs`. Neither count was re-typed or renamed.

The general rule worth carrying: **a disclosure field whose population depends on a one-shot
transition signal does not disclose a steady state.** Any staleness set built from
transition-triggered report fields must be checked for what it reports on the second and
subsequent passes, not only on the pass that produced the transition.

## Consequences

- Closes the silent-staleness hole **before** the D7 gate rather than characterizing it; the D7
  freshness property now asserts the member is named stale on **both** the transition pass and
  the steady-state pass. A single-pass test cannot catch a one-shot disclosure, which is how the
  gap survived the first time.
- A permanently unindexed member is now permanently listed in `members_stale`. That is
  deliberate and is the honest report; it is **not** the ADR-0012-adjacent failure mode of a
  member being permanently *dirty* (which would re-resolve the whole workspace on every pass
  forever). **Staleness disclosure is a read; dirtiness is a trigger.** Worth stating explicitly
  because the two look similar and the second is the one that must never be permanent.
- Spec §4.3 and its assumption 6 still say "four-way" and still carry the false cover argument.
  This ADR supersedes that text; the code cross-references the ADR at
  `internal/wsquery/session.go` (`staleMembers`), `internal/wsquery/coverage.go`
  (`Clause.MembersStale`) and `internal/wsfresh/wsfresh.go` (`MembersUnindexedIDs`).
- Cost: `MembersUnindexed` (the count) and `MembersUnindexedIDs` (the slice) must stay written
  at one site. `cmd/codeindex`'s `workspace-status` still prints only the unindexed **count**
  while printing ids for freshen-failed and missing — a known, deliberate asymmetry left for a
  follow-up.

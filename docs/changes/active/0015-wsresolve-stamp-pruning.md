---
id: 15
slug: wsresolve-stamp-pruning
title: Stamp pruning for unavailable members — close the stale-edges-after-unavailability hole
status: proposed
priority: high
type: fix
created: 2026-08-20
updated: 2026-08-20
depends_on: [14]
related: [13, 14]
discovered_from: [14]
adrs: []
spec: docs/superpowers/specs/2026-08-20-wsresolve-stamp-pruning-design.md
plan:
results:
trivial: false
auto_groomable: true
branch:
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-20-wsresolve-stamp-pruning-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-20-wsresolve-stamp-pruning-design.md) |
<!-- docket:artifacts:end -->

## Why

Change 0014 shipped `wsfresh.Freshen` with a recorded limitation
(pinned by a labelled characterization test and `Freshen`'s doc
comment): a member that goes available → unavailable *after* being
stamped leaves its stale cross-edges in the overlay while the gate
reports clean — the member can't be freshened or folded, its stamp
never moves, so nothing looks dirty. The naive fix (treat unavailable
as dirty) loops forever: an unavailable member can never be re-stamped,
so every subsequent freshen re-resolves the whole workspace.

While nothing is wired this is unreachable; but the gated §4 unit wires
queries to the overlay, and D7 makes silent staleness a hard fail —
this hole must close before §4 builds. Owner decision 2026-08-20
(merge-gate review of PR #12): file as its own high-priority change,
sequenced ahead of §4 via `depends_on`.

## What changes

Three coordinated edits; the design is settled in the linked spec.

- `internal/overlay`: add `(*Store).DeleteStamp(memberID)` — the only
  new overlay surface. Additive and idempotent.
- `internal/wsresolve.Resolve`: signature stays frozen. A new step 9a,
  before the clear/write phases, prunes every member in
  `DECLARED ∖ available` (availability still the single
  `graph.OpenExisting` predicate): records first via
  `ReplaceMemberEdges(U, nil, nil, nil)` — never-thin unchanged — then
  the stamp. Records-first/stamp-last is the same stamp-last rule 0013
  records for writes, applied to deletes: a mid-prune crash must leave a
  stamp that contradicts the rows, because that surviving stamp is the
  only remaining trigger.
- `internal/wsfresh.Freshen`: `Report` gains `StaleStamped` — declared
  members unavailable this pass that still carry a stamp — and the gate
  trips on it. Semantics become prune-then-clean: pass 1 resolves and
  prunes, pass 2 onward is clean and writes nothing, and the member's
  later return is caught by 0014's absent-stamp-is-dirty rule.

0014's characterization test flips from pinning the hole to asserting
the fix. `wsresolve.TestMissingMembersLeftAlone`'s seeded orphan row
joining two unavailable members is knowingly reversed — that row is
exactly the class of edge the prune exists to remove. Six doc/test sites
carry the old invariant and all move together.

ADR-0012 stands unamended; no new ADR.

## Out of scope

- Any verb wiring (D7 second amendment — the gate binds at wiring).
- Union-graph queries, coverage clauses (§4.x).
- Distinguishing "member root deleted" from "member DB version
  mismatch" beyond what `graph.OpenExisting` already reports — both are
  unavailable; finer states are §4's coverage-clause vocabulary if ever.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

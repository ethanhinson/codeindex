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
spec:
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

Inside `internal/wsresolve` (0013's API — this change may amend it,
that is the point): when a resolve pass runs and a *declared* member is
unavailable, prune — remove the member's overlay stamp and its incident
cross-edge/ambiguity/suppression records (never-thin applies: records
delete whole) — so the overlay never carries edges for members the
resolver could not see, and a member's return to availability makes it
dirty by the absent-stamp rule (0014's crash-healing signal) and
re-derives cleanly.

`wsfresh.Freshen`'s gate semantics follow: an unavailable-but-stamped
member becomes prune-then-clean rather than silently clean. Flip 0014's
characterization test from pinning the hole to asserting the fix;
convergence must hold (prune is idempotent — a second pass with the
member still unavailable writes nothing).

## Out of scope

- Any verb wiring (D7 second amendment — the gate binds at wiring).
- Union-graph queries, coverage clauses (§4.x).
- Distinguishing "member root deleted" from "member DB version
  mismatch" beyond what `graph.OpenExisting` already reports — both are
  unavailable; finer states are §4's coverage-clause vocabulary if ever.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

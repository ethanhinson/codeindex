---
id: 14
slug: workspace-freshen-internals
title: Workspace freshen internals — per-member freshen + stamp-gated incident-edge re-resolution
status: proposed
priority: high
type: feat
created: 2026-08-20
updated: 2026-08-20
depends_on: [13]
related: [12, 13]
discovered_from: []
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

§3.3 (change 0013, merged) gave the resolution ladder as an unwired
internal entry point. What makes the workspace honest at query time is
freshness: design D2's contract is that workspace `Fresh` = per-member
freshen, then stamp-gated re-resolution of overlay edges incident to any
member whose merkle root moved — unchanged members cost one stamp
comparison. Without this, §4's union-graph answers would serve stale
cross-edges, violating the always-fresh trust story (D7 even makes
silent staleness a hard fail).

This change implements the *internals* half of task §3.4 of
`openspec/changes/workspace-graph/tasks.md`. The split follows the D7
second amendment (owner ruling 2026-08-19, design.md Amendments): the
merge block binds at verb wiring, so the `workspace-status` verb — §3.4's
other half — rides the final gated change with §4, while these internals
ship unwired and mergeable like §3.2/§3.3.

## What changes

Per design D2 (freshness) and the recorded 0012/0013 hand-offs:

- **Workspace freshen pass** (internal entry point, no verb): for each
  available member, run the existing per-repo freshen; recompute the
  member's root via the canonical `graph.MemberMerkleRoot` fold (0013 —
  the single fold both writer and gate reuse); compare against the
  overlay stamp.
- **Stamp-gated incident re-resolution**: only members whose root moved
  get their overlay records re-derived — via the 0013 ladder, under the
  never-thin semantic (stale ambiguity records deleted whole, re-derived)
  and the established clear→put→stamp-last write order, scoped to
  incident edges rather than a full-workspace pass where the design
  permits. Unchanged members cost exactly one stamp comparison.
- **Stamp gating tests** (the §3.5 bar deferred out of 0013), including
  crash self-healing: a pass that died before stamps wrote must re-derive
  on the next freshen.
- Missing/unavailable members surface through `config.Resolve` (0009,
  still uncalled — this slice is a natural first caller) so a later
  coverage clause can name stale/missing members; no answer shaping here.

## Out of scope

- The `workspace-status` verb — gated at verb wiring by the D7 second
  amendment; rides the final gated change with §4.
- Union-graph query paths, coverage-clause emission, CLI/MCP surfaces
  (§4.x); confidence-vocabulary reconciliation stays §4.1's.
- The evidence gate run (§5.x).
- Repo-level merkle *redesign* — the 0013 fold is canonical; this slice
  reuses it, never forks it.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

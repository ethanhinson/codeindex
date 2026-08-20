---
id: 14
slug: workspace-freshen-internals
title: Workspace freshen internals — per-member freshen + stamp-gated re-resolution
status: proposed
priority: high
type: feat
created: 2026-08-20
updated: 2026-08-20
depends_on: [13]
related: [12, 13]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-20-workspace-freshen-internals-design.md
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
| Spec | [2026-08-20-workspace-freshen-internals-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-20-workspace-freshen-internals-design.md) |
<!-- docket:artifacts:end -->

## Why

§3.3 (change 0013, merged) gave the resolution ladder as an unwired
internal entry point. What makes the workspace honest at query time is
freshness: design D2's contract is that workspace `Fresh` = per-member
freshen, then re-resolution gated on whether any member's merkle root
moved away from its overlay stamp. Without this, §4's union-graph
answers would serve stale cross-edges, violating the always-fresh trust
story (D7 even makes silent staleness a hard fail).

The spec settles D2's "only edges incident to that member" as
**whole-pass re-resolution in the dirty case**: on the frozen 0013 API
the incident set is not closed under re-derivation (the deriving call is
scoped by edge *source*, and a change in one member can flip a
cross-edge between two clean ones into an ambiguity). D2's own risk note
bounds that worst case at a full overlay rebuild; the scoped
optimization is deferred to a measured slice. What the gate does buy is
the clean branch: no dirty member ⇒ **no overlay content write at all**.
The literal "unchanged members cost one stamp comparison" does not hold
as stated — a clean member still pays its per-repo freshen and one
merkle fold, which is the honest number for the D7 latency measurement.

This change implements the *internals* half of task §3.4 of
`openspec/changes/workspace-graph/tasks.md`. The split follows the D7
second amendment (owner ruling 2026-08-19, design.md Amendments): the
merge block binds at verb wiring, so the `workspace-status` verb — §3.4's
other half — rides the final gated change with §4, while these internals
ship unwired and mergeable like §3.2/§3.3.

## What changes

Per design D2 (freshness) and the recorded 0012/0013 hand-offs:

- **Workspace freshen pass** — new `internal/wsfresh.Freshen(wsRoot)`,
  an internal entry point with no verb and no non-test caller: for each
  available member (availability = `graph.OpenExisting` succeeds, the
  resolver's own predicate), run the existing per-repo freshen; recompute
  the member's root via the canonical `graph.MemberMerkleRoot` fold
  (0013 — the single fold both writer and gate reuse); compare against
  the overlay stamp. An absent stamp counts as dirty; that is the crash
  self-healing signal.
- **The gate**: no dirty member and no registry drift ⇒ no overlay
  content write. Otherwise ⇒ one `wsresolve.Resolve` pass, which carries
  the never-thin semantic and the clear→put→stamp-last write order
  unchanged. Registry drift compares whole member records (namespaces and
  deps are ladder inputs, so an id-set comparison would let a manifest
  edit go silently stale).
- **Stamp-gating tests** — the stamp-gating item of the §3.5 bar, which
  0013 deferred — including crash self-healing, cross-member freshness,
  manifest-only drift, and convergence (two clean passes in a row must
  stay clean).
- Missing members surface through `config.Resolve` (0009); this is its
  second call site and the first to hand the ids to its caller rather
  than collapse them into a count. Data only — a later coverage clause
  names stale/missing members; no answer shaping here.

## Out of scope

- The `workspace-status` verb — gated at verb wiring by the D7 second
  amendment; rides the final gated change with §4.
- Union-graph query paths, coverage-clause emission, CLI/MCP surfaces
  (§4.x); confidence-vocabulary reconciliation stays §4.1's.
- The evidence gate run (§5.x).
- Repo-level merkle *redesign* — the 0013 fold is canonical; this slice
  reuses it, never forks it.
- Scoped (source-closure) re-resolution — deferred to a measured slice;
  the D7 gate is where its cost would be justified.
- Single-member-workspace ≡ single-repo equivalence — §4.2's.
- Cold-building an unindexed member: a freshness check never triggers a
  full index build; the member is reported unindexed instead.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

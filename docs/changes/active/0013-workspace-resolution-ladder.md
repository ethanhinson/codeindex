---
id: 13
slug: workspace-resolution-ladder
title: Cross-repo resolution ladder — import-mediated exact, bare-name inferred, ambiguity, suppression
status: in-progress
priority: high
type: feat
created: 2026-08-19
updated: 2026-08-19
depends_on: [12]
related: [9, 12]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-19-workspace-resolution-ladder-design.md
plan:
results:
trivial: false
auto_groomable: true
branch: feat/workspace-resolution-ladder
pr:
blocked_by:
claimed_at: 2026-08-19T21:59:01Z
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-19-workspace-resolution-ladder-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-19-workspace-resolution-ladder-design.md) |
<!-- docket:artifacts:end -->

## Why

§3.1 (change 0009) gave the workspace identity layer and §3.2 (change
0012) the overlay store — which deliberately shipped unwired: this
change owns its first caller. The ladder is the semantic heart of the
workspace-graph campaign: it turns each member's existing unresolved
edges into cross-repo edges with honest confidence, which is what the
union-graph query paths (§4) will surface and what the D7 evidence gate
ultimately measures.

This change implements task §3.3 of
`openspec/changes/workspace-graph/tasks.md`, per frozen design D3
(`openspec/changes/workspace-graph/design.md`) — the ladder order is
frozen.

## What changes

Per design D3, for each unresolved edge in member S (name N, namespace
hint H) — resolution *inside* a member is unchanged and always wins
first; candidates are exactly today's unresolved edges:

1. **Import-mediated (exact-class):** H prefix-matches (on namespace
   boundaries) exactly one member M's declared namespace and N resolves
   uniquely inside M → provenance `cross_repo_import`, confidence
   `exact`. The only rung that can produce exact.
2. **Unique bare name (inferred):** no H; N resolves in exactly one
   member other than S → provenance `cross_repo_name`, confidence
   `inferred`.
3. **Ambiguous:** N resolves in multiple members → candidates recorded
   with count; S's manifest `deps` naming exactly one candidate lists it
   first (tiebreaker, still ambiguous).
4. **Unresolved:** stays unresolved, exactly as today.

Plus **member-over-dep precedence**: a namespace both claimed by a
member and attached as a tier-1 depmap inside another member → the
member wins, the tier-1 attachment is suppressed for that namespace,
and the suppression is recorded (consumer-member-scoped, for later
`workspace-status` skew reporting).

The ladder writes through the `internal/overlay` API (0012):
cross-edges by stable key, ambiguity records under the confirmed
**never-thin** semantic (owner sign-off on change 0012 — stale records
are deleted whole and re-derived, never candidate-thinned), suppression
records, and member stamps at resolution time.

The pass ships as an internal entry point (`internal/wsresolve.Resolve`)
with no CLI verb — D5 names `init-workspace` and `workspace-status` as
the only new verbs, and `workspace-status` is §3.4's — so the first
caller arrives with workspace freshen. It also adds the handful of
`internal/graph` readers the ladder needs and the package lacks
(unresolved-edge and tier-1-edge enumeration, tier-0 definition lookup,
a member merkle fold for stamps, and a **non-creating** index open, so a
resolution pass can never create or rebuild a member's `graph.db`).

Unit-level bars from tasks.md §3.5 that belong to the ladder itself
(ladder order; stable-key survival across member rebuild) land with this
slice; stamp *gating* tests belong to §3.4, and the
single-member-workspace ≡ single-repo bar needs a query path, so it
belongs to §4.2.

## Out of scope

- Workspace freshen policy and `workspace-status` verb (§3.4) — this
  slice may run resolution when invoked; the stamp-gated incremental
  re-resolution policy and repo-level merkle aggregation are §3.4's.
- Union-graph query paths and surfaces (§4.x) — no query verb reads the
  overlay yet; confidence-vocabulary reconciliation with
  `graph.Confidence` is §4.1's recorded hand-off.
- The evidence gate run (§5.x).
- Member discovery changes (change 0010).

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

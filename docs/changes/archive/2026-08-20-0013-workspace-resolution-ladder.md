---
id: 13
slug: workspace-resolution-ladder
title: Cross-repo resolution ladder — import-mediated exact, bare-name inferred, ambiguity, suppression
status: done
priority: high
type: feat
created: 2026-08-19
updated: 2026-08-20
depends_on: [12]
related: [9, 12]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-19-workspace-resolution-ladder-design.md
plan: docs/superpowers/plans/2026-08-19-workspace-resolution-ladder-plan.md
results: docs/results/2026-08-19-workspace-resolution-ladder-results.md
trivial: false
auto_groomable: true
branch: feat/workspace-resolution-ladder
pr: https://github.com/ethanhinson/codeindex/pull/11
blocked_by:
claimed_at: 
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-19-workspace-resolution-ladder-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-19-workspace-resolution-ladder-design.md) |
| Plan | [2026-08-19-workspace-resolution-ladder-plan.md](https://github.com/ethanhinson/codeindex/blob/main/docs/superpowers/plans/2026-08-19-workspace-resolution-ladder-plan.md) |
| Results | [2026-08-19-workspace-resolution-ladder-results.md](https://github.com/ethanhinson/codeindex/blob/main/docs/results/2026-08-19-workspace-resolution-ladder-results.md) |
| PR | [#11](https://github.com/ethanhinson/codeindex/pull/11) |
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

## Owner sign-off (2026-08-19, merge gate)

The D7 merge-gate conflict is resolved by owner ruling: a second dated
amendment (on this change's PR branch, commit a75046d) narrows the
2026-08-18 amendment's explicit block list to its own criterion — the
hard-block binds where a verb gets wired to workspace/overlay data
(§3.4's `workspace-status`, §4.x surfaces/goldens), not to unwired
engine internals. The §3.3 ladder ships unwired and may merge. All D7
bars and the kill condition are untouched. The `MemberMerkleRoot` fold
was checked at the gate: deterministic, documented opaque/equality-only,
canonical for §3.4 reuse.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

### 2026-08-19

Reconciled against `origin/main` `f6d4dee` and the `docket` tip before planning. The
design survives intact — **no scope change, no rung change, no deliverable added or
dropped**. What the pass established:

- **Dependency re-confirmed.** `depends_on: [12]` is satisfied (0012 `done`, archived as
  `2026-08-19-0012-workspace-overlay-store.md`); `related: [9, 12]` are both merged.
- **The slice is still entirely unbuilt.** `internal/wsresolve` does not exist, and none
  of `graph.UnresolvedEdges`, `graph.ProjectDefs`, `graph.TierOneEdges`,
  `graph.MemberMerkleRoot`, or `graph.OpenExisting` has been added by any other change —
  no work to drop.
- **`internal/overlay` is still unwired** (no non-test caller), and its API, its four
  record types, and `ReplaceMemberEdges`' either-end delete are byte-for-byte as the spec
  describes. The empty-input clear the write order depends on is already exercised by
  0012's own `TestReplaceMemberEdgesEmptyClears`, so §6 step 9 rests on tested behavior.
- **`config.Resolve` still has no non-test caller** — this change still owns its first,
  as `## Why` claims.

Four spec facts had drifted in detail and were corrected in place; none changes a
decision:

1. `PutFile`'s `bind` map is gated on `d.Source != "" && d.Target != ""`, not on
   `d.Source != ""` alone. Strictly narrower, so the Go-hint blind spot the spec calls out
   is if anything slightly wider than described — the ladder still must not assume H is
   universal.
2. `graph.Open` is at `store.go:139` (spec said `136-174`); `OpenRaw` at `:180` holds.
   Additionally `SchemaVersion() int` and `FileSchemaVersion(path) (int, error)` are
   **already exported** at `:182`/`:186`, so §6's availability check needs no new version
   accessor — one less thing for `OpenExisting` to invent.
3. `phpNamespaces` spans `namespaces.go:120-151` (spec said `120-141`) and
   `Symbol.QName` is at `types.go:85` (spec said `:83`). Both facts hold.
4. `DepFileState` is `{Path, Namespace, Version, MapHash, CurHash, Size, Mtime}` — it has
   **no `Modified` field**, though the `depfiles` table does. §7's fold reads
   `path`/`namespace`/`version`/`curhash`, all of which the struct exposes, so the stamp
   design is unaffected; the spec now says so explicitly rather than leaving a builder to
   discover it.

`openspec/changes/workspace-graph/tasks.md` still shows `- [ ] 3.2` unchecked, exactly as
the spec predicted — the builder ticks both §3.2 and §3.3.

No follow-up work was surfaced that warranted capture, and `auto_capture` is disabled in
this repo in any case.

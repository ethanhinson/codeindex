---
id: 16
slug: workspace-query-surfaces-gated
title: Workspace query surfaces — union-graph verbs, CLI/MCP wiring, workspace-status; merge gated on the D7 evidence run
status: proposed
priority: high
type: feat
created: 2026-08-20
updated: 2026-08-20
depends_on: [15]
related: [9, 12, 13, 14, 15, 10]
discovered_from: []
adrs: [12]
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

The workspace engine is complete and merged (changes 0009, 0012–0015)
but deliberately unwired: no verb reads the overlay, so a user still
cannot ask a single cross-repo question. This change is the wiring —
the last slice of the workspace-graph campaign
(`openspec/changes/workspace-graph/tasks.md` §4.1–§4.4 + the
`workspace-status` half of §3.4, plus hosting the §5 gate run).

Per the D7 second amendment (design.md Amendments, owner ruling
2026-08-19), this is exactly the slice the merge block binds to: **its
PR merges only if the pre-registered D7 evidence gate passes.** If the
grep-across control wins, the verdict is recorded in
`bench/engine/FINDINGS-workspace-graph.md` and the change closes —
that is a legitimate outcome (kill condition, frozen in D7).

## What changes

Per frozen design D4 (query semantics) and D5 (surfaces):

- **Union-graph verbs (§4.1):** `callers`/`callees`/`impact`/`nav` over
  member graph + overlay in-edges; `impact` crosses member boundaries by
  default; `find`/`grep` fan out across members (complete sets, no
  rank-merge; members in manifest order). Workspace-relative paths;
  every reference carries `repo: <member-id>`; anchors accept an
  optional `<member-id>:` prefix. Two recorded obligations from the
  engine slices are discharged here: filter the surviving intra-repo
  tier-1 edge via `dep_suppressions` (conditioned on a same-call-site
  cross-edge, else double-count), and reconcile the overlay's
  `exact`/`inferred` confidence vocabulary with `graph.Confidence` at
  the surface.
- **CLI root-kind wiring (§4.2):** every verb's root argument accepts a
  workspace root via 0009's `DetectRootKind` (first real call site);
  single-repo goldens stay byte-identical; the
  single-member-workspace ≡ single-repo bar (deferred from §3.5) lands
  here.
- **MCP (§4.3):** `codeindex mcp <workspace-root>` serves the union
  graph; `repo` field in result schemas; no new tools; plugin note
  untouched (house rule).
- **`workspace-status` verb** (§3.4's gated half): per-member
  build/stamp state, member/vendor version skew from suppression
  records.
- **Workspace goldens + freshness property (§4.4):** workspace answers
  pinned; the D7 freshness scenario (mutate one member, query from
  another, no explicit rebuild → answer reflects it or the coverage
  clause names the member stale) as an executable property test.
  Freshen wiring calls `wsfresh.Freshen` on query entry (D2: lazy,
  members consulted by the query where the verb permits).

**Merge gate (§5, runs on this PR before merge):** re-run the
four-class leak audit over campaign transcripts (standing pre-verdict
gate), then the pre-registered D7 run — arm A (shell + checkouts) vs
arm B (A + workspace MCP pointed at this branch's binary via
`CODEINDEX_WS_MCP_BIN`) on the frozen 65-task corpus; bars read from
`bench/workspace/README.md` verbatim. Verdict + residuals recorded in
`bench/engine/FINDINGS-workspace-graph.md` either way. Note: the bench
harness and corpus live in local-main-only commits (unpushed), so the
gate runs from the local main tree against the branch-built binary —
the run is an owner-attended step, not part of the autonomous build.

## Out of scope

- Corpus growth (change 0010) — the gate runs on the frozen 65-task
  corpus as registered.
- Cross-workspace semantic search / vectors, UI, git-remote identity,
  language expansion, re-ranking (frozen non-goals).
- Scoped incident re-resolution (ADR-0012 stands; a D7-measured
  follow-up if latency demands).

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

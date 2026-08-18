---
id: 9
slug: workspace-manifest-init-scan
title: Workspace manifest load/validate + init-workspace --scan
status: proposed
priority: high
type: feat
created: 2026-08-17
updated: 2026-08-17
depends_on: []
related: []
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

The workspace-graph campaign (query across a workspace of repos as if it
were a single graph) registered its GO on 2026-08-17 and its bench-first
phase is fully done: frozen 65-task OSS corpus across all four indexer
languages, arm-A control harness wired and smoked, four-class leak audit
PASS (`bench/workspace/`). The evidence gate (design D7) now waits on the
engine — arm B refuses to run until workspace support exists. This change
is the first engine slice: without a manifest and a way to create one,
nothing downstream (overlay store, resolution ladder, union-graph queries)
can start.

The full design is settled and frozen in
`openspec/changes/workspace-graph/design.md` (owner sign-off 2026-08-17,
open questions: none). This change implements task §3.1 of
`openspec/changes/workspace-graph/tasks.md`.

## What changes

Per design D1 (identity) and D5 (surfaces):

- Load + validate `<workspace-root>/.codeindex/workspace.json` (version,
  member id/root/namespaces/optional deps; roots relative to the
  workspace root, may point outside it).
- New verb `init-workspace --scan`: namespace auto-discovery per language
  (go.mod / package.json / composer.json / Python top-level modules) and
  monorepo member discovery via go.work, pnpm-workspace.yaml, and
  composer path repositories. Manifest overrides always win over
  scanned values.
- Root-kind detection groundwork: a root containing `workspace.json` is a
  workspace; a root with neither manifest nor indexable source errors
  naming both possibilities. Single-repo mode is the absence of a
  manifest, not a flag — existing single-repo behavior stays
  byte-identical.

## Out of scope

- Overlay store (`workspace.db`: registry, cross-edges, stamps) — task
  §3.2.
- Cross-repo resolution ladder — task §3.3 (import-mediated exact only,
  per design D3).
- Workspace freshen + `workspace-status` verb — task §3.4.
- Union-graph query paths, CLI/MCP surface changes beyond the new verb —
  tasks §4.x.
- The evidence gate run — task §5.x.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

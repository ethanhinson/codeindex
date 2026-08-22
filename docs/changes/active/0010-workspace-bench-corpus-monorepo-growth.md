---
id: 10
slug: workspace-bench-corpus-monorepo-growth
title: Grow the workspace bench corpus — monorepo declaration coverage in every supported language
status: proposed
priority: medium
type: chore
created: 2026-08-18
updated: 2026-08-22
depends_on: []
related: [9]
discovered_from: [9]
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

Grooming change 0009 surfaced that the frozen 65-task workspace bench
corpus contains exactly one monorepo (nest), and it declares members
solely via `lerna.json` — a source the frozen design didn't even list.
Member discovery (`init-workspace --scan`) therefore has effectively one
data point per declaration format, and most formats (go.work,
pnpm-workspace.yaml, composer path repositories, npm/yarn `workspaces`)
have zero organic coverage. Owner decision 2026-08-18: grow the corpus
significantly with monorepo examples in every supported language, then
measure discovery coverage.

## What changes

- Add pinned OSS monorepo members to the workspace bench corpus so every
  supported declaration format has at least one organic example: go.work
  (Go), pnpm-workspace.yaml and npm/yarn `workspaces` (TS/JS, alongside
  the existing lerna case), composer path repositories (PHP), plus a
  Python multi-member layout.
- Re-mine tasks over the grown corpus (extend `build_tasks_ws.py`),
  re-freeze the task set, and re-run the four-class leak audit
  (`leak_audit_ws.py`) as the standing pre-verdict gate.
- Measure and record member-discovery coverage per declaration format
  (which formats are exercised, per-member quota) in
  `bench/workspace/README.md`.

## Owner re-direction (2026-08-22, post-gate pivot)

The D7 gate runs (see `bench/engine/FINDINGS-workspace-graph.md`) showed
the frozen 65-task corpus is greppable by construction — both arms tie
on import-mediated shapes at every model tier — while the one structural
shape (`xsubtypes`, n=7) showed the index lifting haiku +14pp. The
corpus growth this change owns is therefore re-aimed: **structural,
grep-hostile cross-repo tasks are the priority**, monorepo declaration
coverage second.

- Task shapes to mine, with auditable GT: transitive impact chains
  (A→B→C across members), subtype maps (all implementors/extenders of a
  cross-repo interface/class — blocked on change 0017's hint fix landing
  first), cross-repo name collisions (same bare name in ≥2 members,
  where grep returns the union and the correct answer needs the import
  binding), and aliased/renamed imports.
- Grow the subtype set well beyond n=7 so the signal is statistically
  meaningful.
- Register a NEW gate (bars registered before any scored run, m5/D7
  form) on the grown corpus; a pass revives the killed 0016 query
  surfaces as a fresh change.
- Original monorepo-format coverage ask (go.work, pnpm, npm/yarn
  workspaces, composer path repos) stands as the secondary goal.

## Out of scope

- Any engine change — discovery source list changes belong to change
  0009 and its successors.
- Private-repo material — corpus stays pinned OSS (owner rule).
- Re-running the D7 evidence gate itself (§5 of the workspace-graph
  plan); this change only grows the corpus it will run over.

## Open questions

- Which specific OSS repos to pin per format, and target corpus size
  ("significantly" needs a number at brainstorm time).
- Whether grown-corpus task mining changes the per-member quota rules
  recorded in the bench README.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

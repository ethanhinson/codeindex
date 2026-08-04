---
id: 1
slug: migrate-keeper-lore-decisions-to-adrs
title: Migrate keeper lore decisions to docket ADRs
status: in-progress
priority: high
type: docs
created: 2026-08-03
updated: 2026-08-04
depends_on: []
related: [4]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md
plan:
results:
trivial: false
auto_groomable:
branch: feat/migrate-keeper-lore-decisions-to-adrs
claimed_at: 2026-08-04T03:42:50Z
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-03-back-out-lore-lean-into-docket-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md) |
<!-- docket:artifacts:end -->

## Why

Backing lore out of codeindex deletes `.lore/`, which holds real architectural
history. The durable *engine* decisions must survive the teardown as docket ADRs
before the directory is removed (change 0004 deletes it). This is Phase 0 of the
lore→docket pivot and a hard prerequisite for the `.lore/` deletion.

## What changes

- Author docket ADRs from the keeper `.lore/decisions/*`: tree-sitter parsing +
  own edge resolution · sqlite graph-db storage · Go single-static engine ·
  config-driven index include/exclude · on-demand/lazy freshness · flat per-file
  content-hash change detection · references-only output contract.
- Author one **reversal ADR** capturing the `openspec → lore → docket` lineage —
  "lore replaces openspec" is now superseded by "docket replaces lore" — so the
  history is not silently lost.
- Use the `docket-adr` skill so the ADR index stays valid.

## Out of scope

- Deleting `.lore/` (change 0004).
- Migrating lore-/UI-specific decisions (graph-UI aggregation, v3 two-state model,
  lore-is-a-sidecar, free-form records, private overlay, separate lore.db) — these
  die with lore and are intentionally not preserved.
- Any code change.

## Open questions

- Final per-decision keep/drop calls are triaged against `.lore/decisions/` at
  build time; the spec's Migration section is the selection rule.

## Reconcile log

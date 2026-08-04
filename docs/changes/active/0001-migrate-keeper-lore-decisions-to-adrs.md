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
adrs: [1, 2, 3, 4, 5, 6, 7, 8]
spec: docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md
plan: docs/superpowers/plans/2026-08-04-migrate-keeper-lore-decisions-to-adrs-plan.md
results:
trivial: false
auto_groomable:
branch: feat/migrate-keeper-lore-decisions-to-adrs
claimed_at: 2026-08-04T04:05:00Z
pr:
blocked_by:
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-03-back-out-lore-lean-into-docket-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md) |
| Plan | [2026-08-04-migrate-keeper-lore-decisions-to-adrs-plan.md](https://github.com/ethanhinson/codeindex/blob/feat/migrate-keeper-lore-decisions-to-adrs/docs/superpowers/plans/2026-08-04-migrate-keeper-lore-decisions-to-adrs-plan.md) |
| ADRs | [ADR-0001](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0001-parsing-via-tree-sitter-with-edge-resolver.md), [ADR-0002](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0002-storage-sqlite-graph-db-transactional-incremental.md), [ADR-0003](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0003-engine-language-go-single-static-binary.md), [ADR-0004](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0004-config-driven-index-include-exclude-defaults.md), [ADR-0005](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0005-freshness-on-demand-build-lazy-per-query-recheck.md), [ADR-0006](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0006-change-detection-flat-per-file-hashes-not-merkle-tree.md), [ADR-0007](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0007-output-contract-references-only-not-source.md), [ADR-0008](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0008-docket-replaces-lore.md) |
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

### 2026-08-04

Reconciled against current reality (spec, related change 0004, existing ADRs, and
`.lore/decisions/` on disk). Findings:

- Design **valid** — no re-brainstorm needed. The spec's `.lore/` migration section
  maps 1:1 onto the keeper decision files present in `.lore/decisions/`.
- **8 keeper decisions confirmed on disk**, matching the spec's keep list — the 7
  engine decisions (tree-sitter parsing + own edge resolver; SQLite `.codeindex/graph.db`
  transactional storage; Go single static binary; config-driven index include/exclude;
  on-demand build + lazy per-query freshness; flat per-file content-hash change
  detection; references-only output contract) plus the lineage decision
  `2026-07-29-lore-replaces-openspec.md` (source for the reversal ADR).
- **6 decisions correctly dropped** (graph-UI aggregation, graph-UI v3 two-state,
  lore-is-a-sidecar, free-form records, in-repo records + private overlay,
  Go-side scoring + separate lore.db) — lore/UI-specific, die with lore.
- All ADR anchor code paths (`internal/adapter`, `internal/graph`, `cmd/codeindex`,
  `internal/config/filter.go`, `internal/query`, `internal/merkle`) still **exist on
  `origin/main`** — the ADRs can cite live code.
- **No ADRs exist yet** (`docs/adrs/` empty) — this change authors the repo's first
  ADRs; the `docket-adr` skill assigns numbers 0001–0008.
- No overlapping work done elsewhere; change 0004 (`.lore/` deletion) correctly
  `depends_on: [1]`, so this migration remains its hard prerequisite.

Scope unchanged: author 7 engine ADRs + 1 reversal ADR. Markdown-only, no code.
No adjacent follow-up work surfaced (auto-capture disabled this repo).

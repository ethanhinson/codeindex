---
id: 2
slug: rip-out-lore-product-surface
title: Rip out the lore product surface (engine, CLI, MCP, plugin skills)
status: proposed
priority: high
type: refactor
created: 2026-08-03
updated: 2026-08-03
depends_on: []
related: [1, 3, 4]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md
plan:
results:
trivial: false
auto_groomable:
branch:
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

codeindex accreted a second product — lore — inside it. This change (Phase 1 of
the pivot) removes the lore engine, CLI, MCP, and plugin surface, returning
codeindex toward a single purpose: a blast-radius/impact tool. Work-tracking moves
to docket.

## What changes

- Delete the entire `internal/lore/**` tree; remove lore code from
  `internal/graph/store.go`; de-lore `cmd/codeindex/main.go` (drop the lore import
  and dispatch).
- Remove CLI subcommands `lore` (`cmd/codeindex/lore.go`), `tree`
  (`cmd/codeindex/tree.go`), and `attach`.
- Remove `internal/mcpserver/lore_tools.go` (`lore_search`, `lore_for_symbol`,
  `lore_backlog`, `lore_show`, `lore_add`); keep `impact`/`callers`/`callees`/
  `dependents`/`find`/`grep`.
- Strip `related_lore` / `--related-depth` enrichment from `impact` (CLI + MCP +
  engine).
- Remove the `decide.md` and `lore.md` plugin skills; keep `impact.md`.
- `go build ./...` and `go test ./...` stay green after each removal (deleting a
  package deletes its tests).

## Out of scope

- The web UI and the graph read-model/API (Phase 2, change 0003).
- Deleting `.lore/` data and README rewrite (Phase 3, change 0004).

## Open questions

- Depth of lore coupling inside `internal/graph/store.go` — resolve incrementally
  with green-build-per-step rather than one cut.

## Reconcile log

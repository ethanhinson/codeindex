---
id: 3
slug: decouple-graph-query-layer
title: Decouple the symbol-graph query layer (headless JSON API + CLI)
status: proposed
priority: high
type: refactor
created: 2026-08-03
updated: 2026-08-03
depends_on: [2]
related: [2, 4]
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

The `web/` React app existed only to visualize the lore graph. Instead of a coupled
UI, codeindex should expose the *symbol* graph as a clean, decoupled, versioned API
+ CLI that any external consumer (a future viewer, another tool) can query. This is
Phase 2 of the pivot.

## What changes

- Delete the entire `web/` app (including the galaxy retheme) and the webserver's
  static-file handler + `internal/webserver/dist`.
- Strip the lore overlay from `internal/readmodel`: remove the `FullGraph` lore
  branch, `RecordNeighborhood`, and `loreNode`; keep `SymbolNeighborhood` and a
  symbol-only `FullGraph`.
- `serve` becomes a **headless JSON graph API** (no static hosting):
  `GET /api/health`, `GET /api/graph?symbol=…&parent=…`, `GET /api/graph/full`.
- Pin a top-level `schemaVersion` on responses (Node = symbol-only:
  `{ID, Kind:"symbol", Label, File, Line, Signature, Group}`); document the contract
  in `docs/graph-api.md`.
- Update `internal/webserver/server_test.go` to assert the symbol-only shape,
  `schemaVersion`, and that static hosting is gone (root path 404s).

## Out of scope

- Lore engine/CLI/MCP removal (change 0002, prerequisite).
- `.lore/` deletion, config excludes, README rewrite (change 0004).
- Building any new viewer against the API.

## Open questions

- Whether `/api/graph/full` should paginate for very large repos — defer unless a
  consumer needs it; document the current whole-graph behavior.

## Reconcile log

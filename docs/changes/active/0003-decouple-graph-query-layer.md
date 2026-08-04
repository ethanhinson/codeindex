---
id: 3
slug: decouple-graph-query-layer
title: Decouple the symbol-graph query layer (headless JSON API + CLI)
status: implemented
priority: high
type: refactor
created: 2026-08-03
updated: 2026-08-04
depends_on: [2]
related: [2, 4]
discovered_from: []
adrs: [9]
spec: docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md
plan: docs/superpowers/plans/2026-08-04-decouple-graph-query-layer-plan.md
results: docs/results/2026-08-04-decouple-graph-query-layer-results.md
trivial: false
auto_groomable:
branch: feat/decouple-graph-query-layer
pr: https://github.com/ethanhinson/codeindex/pull/5
blocked_by:
reconciled: true
claimed_at: 2026-08-04T17:22:49Z
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-03-back-out-lore-lean-into-docket-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md) |
| Plan | [2026-08-04-decouple-graph-query-layer-plan.md](https://github.com/ethanhinson/codeindex/blob/feat/decouple-graph-query-layer/docs/superpowers/plans/2026-08-04-decouple-graph-query-layer-plan.md) |
| Results | [2026-08-04-decouple-graph-query-layer-results.md](https://github.com/ethanhinson/codeindex/blob/feat/decouple-graph-query-layer/docs/results/2026-08-04-decouple-graph-query-layer-results.md) |
| PR | [#5](https://github.com/ethanhinson/codeindex/pull/5) |
| ADRs | [ADR-0009](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0009-graph-api-schema-version-contract.md) |
<!-- docket:artifacts:end -->

## Why

The `web/` React app existed only to visualize the lore graph. Instead of a coupled
UI, codeindex should expose the *symbol* graph as a clean, decoupled, versioned API
+ CLI that any external consumer (a future viewer, another tool) can query. This is
Phase 2 of the pivot.

## What changes

- Delete the entire `web/` app (including the galaxy retheme) and the webserver's
  static-file handler (`internal/webserver/static.go`) + `internal/webserver/dist`.
- Strip the lore overlay from `internal/readmodel`: remove the `FullGraph` lore
  branch, `RecordNeighborhood`, and `loreNode` — and the lore-coupled helpers that
  fall dead with them (`Neighborhood`, `openLore`, `AttachAnchoredLore`); reduce
  `model.go` to the symbol-only `Node`/`Edge`/`Graph` shape (drop the lore
  `NodeKind`s, `EdgeAnchors`/`EdgeBlockedBy`, and the `Status`/`Priority` node
  fields). Keep `SymbolNeighborhood`, `openGraph`, `pkgOf`, `symNodeID`, and a
  symbol-only `FullGraph`.
- `serve` becomes a **headless JSON graph API** (no static hosting):
  `GET /api/health`, `GET /api/graph?symbol=…&parent=…`, `GET /api/graph/full`.
  `/api/graph` moves from the old `?focus=` + `Neighborhood` wiring to
  `?symbol=&parent=` + `SymbolNeighborhood`; the root `/` handler (static SPA) is
  removed so unknown paths 404.
- Pin a top-level `schemaVersion` on responses (Node = symbol-only:
  `{ID, Kind:"symbol", Label, File, Line, Signature, Group}`); document the contract
  in `docs/graph-api.md`.
- Update `internal/webserver/server_test.go` to assert the symbol-only shape,
  `schemaVersion`, and that static hosting is gone (root path 404s).
- CARRYOVER FROM 0002: delete the entire remaining `internal/lore/**` tree (root
  `layout.go`/`record.go`, `gitinfo/`, `index/`) now that readmodel/webserver no
  longer import it; strip any remaining lore imports across the codebase.
- CARRYOVER FROM 0002: remove the now-orphaned `internal/tui/tree` package (its sole
  consumer, the `tree` CLI command, was deleted in 0002; `internal/tui` has no other
  live consumers, so the whole `internal/tui` subtree goes).

## Out of scope

- Lore engine/CLI/MCP removal (change 0002, prerequisite — now merged).
- `.lore/` deletion, config indexing excludes (incl. the cosmetic
  `internal/webserver/dist` doc-comment example in `internal/config/config.go`),
  `lore.db` handling, and README rewrite (all change 0004).
- Building any new viewer against the API.

## Open questions

- Whether `/api/graph/full` should paginate for very large repos — defer unless a
  consumer needs it; document the current whole-graph behavior.

## Reconcile log

### 2026-08-04 — reconcile (docket-implement-next)

Reconciled against `origin/main` (the feature-branch base), the spec, and the
current code. Dependency 0002 is merged (`done`); confirmed on `origin/main`:
`internal/lore/**` still carries `layout.go`/`record.go`/`gitinfo/`/`index/` (0002
already removed `ghsync/` + `capture.go`), and `internal/tui/tree` still exists with
the `tree` command already gone — both carryovers are accurate and folded into scope.

Scope refinements discovered (change body updated above):
- **readmodel is more lore-coupled than the original body listed.** Beyond the
  `FullGraph` lore branch / `RecordNeighborhood` / `loreNode`, the helpers
  `Neighborhood`, `openLore`, and `AttachAnchoredLore` (in `graph.go`) also import
  and depend on `internal/lore` + `internal/lore/index`; they must be removed or
  they leave dangling lore imports. `model.go` also carries lore-only `NodeKind`s
  (`decision`/`item`/`note`/`path`), `EdgeAnchors`/`EdgeBlockedBy`, and the
  `Status`/`Priority` node fields — all reduced to the symbol-only shape.
- **`/api/graph` contract change is a real behavior change**, not just a rename:
  today `/api/graph?focus=sym:…` calls `readmodel.Neighborhood`; the contract is
  `/api/graph?symbol=…&parent=…` calling `SymbolNeighborhood`. `server_test.go`'s
  existing `TestGraphEndpoint`/`TestGraphEndpointMissingFocus`/`TestFullGraphEndpoint`
  all assert the lore-join shape and must be rewritten to the symbol-only contract;
  `TestStaticIndexServed` is deleted and replaced by a root-404 assertion.
- **`readmodel/graph_test.go`** contains lore-dependent tests
  (`TestAttachAnchoredLore`, `TestNeighborhood*`, `TestRecordNeighborhood`) that go
  with their subjects; `TestSymbolNeighborhood` stays.
- **`serve.go` (cmd)** needs no structural change — it still calls `webserver.Run`.
- **schemaVersion**: introduce a top-level `schemaVersion` on API responses (a small
  wrapper over `Graph`, or a field on `Graph` emitted by the HTTP layer).

Out-of-scope confirmations: the `internal/config/config.go:26` doc comment that uses
`internal/webserver/dist` as a prefix-match *example* is cosmetic (not a functional
web exclude) and belongs to change 0004's config-excludes work — left untouched here.
`.lore/` data dir and README are 0004. `AUTO_CAPTURE_ENABLED=false`, so no stubs
minted; no adjacent follow-up work surfaced that would warrant one anyway.

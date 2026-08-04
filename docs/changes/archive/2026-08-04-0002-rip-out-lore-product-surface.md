---
id: 2
slug: rip-out-lore-product-surface
title: Rip out the lore product surface (engine, CLI, MCP, plugin skills)
status: done
priority: high
type: refactor
created: 2026-08-03
updated: 2026-08-04
depends_on: []
related: [1, 3, 4]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md
plan: docs/superpowers/plans/2026-08-04-rip-out-lore-product-surface-plan.md
results: docs/results/2026-08-04-rip-out-lore-product-surface-results.md
trivial: false
auto_groomable:
branch: feat/rip-out-lore-product-surface
pr: https://github.com/ethanhinson/codeindex/pull/4
claimed_at: 
blocked_by:
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-03-back-out-lore-lean-into-docket-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md) |
| Plan | [2026-08-04-rip-out-lore-product-surface-plan.md](https://github.com/ethanhinson/codeindex/blob/main/docs/superpowers/plans/2026-08-04-rip-out-lore-product-surface-plan.md) |
| Results | [2026-08-04-rip-out-lore-product-surface-results.md](https://github.com/ethanhinson/codeindex/blob/main/docs/results/2026-08-04-rip-out-lore-product-surface-results.md) |
| PR | [#4](https://github.com/ethanhinson/codeindex/pull/4) |
<!-- docket:artifacts:end -->

## Why

codeindex accreted a second product — lore — inside it. This change (Phase 1 of
the pivot) removes the lore engine, CLI, MCP, and plugin surface, returning
codeindex toward a single purpose: a blast-radius/impact tool. Work-tracking moves
to docket.

- Remove the lore **product surface** — CLI, MCP, plugin skills, and the lore-only
  parts of `internal/lore` — while keeping `go build ./...` / `go test ./...` green
  after each removal (deleting a package deletes its tests).
- Remove CLI subcommands `lore` (`cmd/codeindex/lore.go` + `lore_test.go`), `tree`
  (`cmd/codeindex/tree.go`), and `attach`; de-lore `cmd/codeindex/main.go` (drop the
  `internal/lore/index` import, the `lore`/`tree`/`attach` dispatch cases, the
  `--related-depth` flag on `impact`, and the `relatedLoreForImpact`/`loreReindex`
  helpers).
- Remove `internal/mcpserver/lore_tools.go` (+ test) — the `lore_search`,
  `lore_for_symbol`, `lore_backlog`, `lore_show`, `lore_add` tools plus the shared
  `relatedLoreBlock`/`addLoreTools` helpers — and drop the two `relatedLoreBlock`
  calls + `addLoreTools` registration from `mcpserver.go`, stripping the "Related
  lore" enrichment from the kept `impact`/`callers`/`callees`/`dependents`/`find`/
  `grep` tools.
- Delete the lore-only slices of `internal/lore`: `ghsync/` (only `lore.go` used it)
  and `capture.go` + `capture_test.go` (the `attach`/session-capture flow).
- Remove the `decide.md` and `lore.md` plugin skills; keep `impact.md`.

**Reconciled boundary (2026-08-04):** the change file originally said "delete the
entire `internal/lore/**` tree" and "remove lore code from `internal/graph/store.go`."
Reconcile against current code corrected both:
- `internal/lore` (root: `layout.go`, `record.go`), `internal/lore/index/`, and
  `internal/lore/gitinfo/` are still imported by `internal/readmodel` (the Phase-2
  lore overlay: `openLore`, `loreNode`, `RecordNeighborhood`) and by
  `internal/webserver/server_test.go`. Deleting them now would break `go build`.
  Since readmodel/web are **out of scope** here (Phase 2 / change 0003), those
  packages **survive** into 0003, which removes the overlay and then deletes what it
  orphans. Only the lore-product-only pieces (`ghsync/`, `capture.go`) are removed now.
- `internal/graph/store.go` carries **no functional lore code** — only two comments
  ("tree explorer", "attached-map"). The single available cleanup is the now-dead
  `ProjectSymbols` (+ its `store_test.go` test), used solely by the deleted `tree.go`;
  the comment on the resolver's attached-map ladder stays (unrelated to lore).

## Out of scope

- The web UI and the graph read-model/API (Phase 2, change 0003) — including the
  `internal/lore`, `internal/lore/index`, and `internal/lore/gitinfo` packages that
  readmodel still imports (their deletion belongs to 0003, after the overlay is gone).
- Deleting `.lore/` data and README rewrite (Phase 3, change 0004).

## Open questions

- (Resolved during reconcile) Depth of lore coupling inside `internal/graph/store.go`
  — none functional; only the dead `ProjectSymbols` is removable here. The
  `internal/lore` bulk deletion moves to change 0003 because readmodel imports it.

## Reconcile log

### 2026-08-04
Reconciled against current `origin/main`. Findings:
- **Scope corrected — `internal/lore` bulk deletion deferred to 0003.**
  `internal/readmodel/graph.go`/`fullgraph.go` import `internal/lore` +
  `internal/lore/index`, and `internal/lore/index/reindex.go` imports
  `internal/lore/gitinfo`; `internal/webserver/server_test.go` imports `internal/lore`
  too. readmodel/web are explicitly Phase 2. Deleting `internal/lore/**` now would
  break `go build ./...`, contradicting this change's own green-build constraint and
  its own out-of-scope line. Resolution: keep those packages, delete only the
  lore-product-only pieces (`ghsync/`, `capture.go`). The bulk deletion rides with
  the overlay removal in change 0003.
- **`internal/graph/store.go` has no functional lore code** — two comments only. The
  only cleanup is the now-orphaned `ProjectSymbols` (used solely by `tree.go`) + its
  test.
- **`attach`** uses `depmap.Attach`/`AutoAttach`, not lore; removing the CLI dispatch
  leaves those `internal/depmap` exports intact (still used by the `depmap` command).
- `mcpserver.go` (kept) calls `relatedLoreBlock`/`addLoreTools` from `lore_tools.go`,
  so de-loring MCP is an edit to `mcpserver.go` plus deleting `lore_tools.go`.
- No new ADRs cited; `related: [1, 3, 4]` unchanged. No adjacent follow-up work to
  capture (auto-capture is disabled anyway).

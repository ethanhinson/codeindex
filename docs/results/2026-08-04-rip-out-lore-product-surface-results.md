<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0002 — Rip out the lore product surface (engine, CLI, MCP, plugin skills)](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0002-rip-out-lore-product-surface.md)**
<!-- docket:backlink:end -->

# Rip out the lore product surface — results

Change: #0002 · Branch: feat/rip-out-lore-product-surface · PR: (opened in step 7) · Plan: docs/superpowers/plans/2026-08-04-rip-out-lore-product-surface-plan.md · ADRs: none

## Verify (human)

No manual checks required beyond CI — this is a pure code-removal change with the full `go build ./...` / `go test ./...` suite green after every task and over the whole branch. Optional sanity: `go run ./cmd/codeindex build . && go run ./cmd/codeindex impact . <symbol> --limit 5` prints an impact summary with no "Related lore" section.

## Findings

- **Reconcile scope adjustment (recorded in the change's Reconcile log, not an ADR).** The change file originally said "delete the entire `internal/lore/**` tree" and "remove lore code from `internal/graph/store.go`." Reconcile against current code corrected both, since they conflict with this change's own out-of-scope constraint:
  - `internal/lore` (root: `layout.go`, `record.go`), `internal/lore/index/`, and `internal/lore/gitinfo/` are still imported by `internal/readmodel` (the Phase-2 lore overlay) and `internal/webserver/server_test.go`. Deleting them now would break `go build`. They **survive into change 0003**, which removes the overlay and then deletes what it orphans. Only the lore-product-only pieces (`ghsync/`, `capture.go`) were removed here.
  - `internal/graph/store.go` carries **no functional lore code** — only comments. The single available cleanup was the now-dead `ProjectSymbols` method (used solely by the deleted `tree.go`) + its test.
- This is a scope/phasing refinement the spec already anticipates ("resolve incrementally with green-build-per-step"), not a new architecture decision — hence no ADR.

## Follow-ups

- **Orphaned `internal/tui/tree/` package (dead-code cleanup, not filed as a change).** `cmd/codeindex/tree.go` was the sole consumer of `internal/tui/tree` (`tuitree.BuildTree/Static/NewModel`); after its deletion nothing imports the package. It still builds and tests green, so it is not a bug in this branch — the plan scoped only `cmd/codeindex/tree.go` for deletion. Flagging for a future dead-code sweep (naturally folds into Phase 2 / change 0003, which is already tearing down the web/graph surface). AUTO_CAPTURE is disabled in this repo, so this is reported here rather than minted as a stub.

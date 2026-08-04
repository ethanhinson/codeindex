<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0003 — Decouple the symbol-graph query layer (headless JSON API + CLI)](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0003-decouple-graph-query-layer.md)**
<!-- docket:backlink:end -->

# Decouple the symbol-graph query layer — results

Change: #0003 · Branch: feat/decouple-graph-query-layer · PR: (opened in step 7) · Plan: docs/superpowers/plans/2026-08-04-decouple-graph-query-layer-plan.md · ADRs: ADR-0009

## Verify (human)

Automated coverage is complete: `go build ./...` and `go test ./...` are green after every task and over the whole branch, and the whole-branch review passed. The suite includes `internal/webserver/server_test.go` asserting the symbol-only shape, `schemaVersion == "1"`, missing-`symbol` → 400, and root-path → 404 (static hosting gone).

Optional manual API smoke on this repo (the spec's final gate) — confirms the headless contract end to end:

```
go run ./cmd/codeindex build .
# start the headless API on loopback
go run ./cmd/codeindex serve &   # note the printed http://127.0.0.1:PORT
curl -s localhost:PORT/api/health                 # {"status":"ok","version":"0.2.0","root":"..."}
curl -s 'localhost:PORT/api/graph?symbol=SomeSymbol' | head   # {"schemaVersion":"1","focus":"sym:...",...}
curl -s localhost:PORT/api/graph/full | head                  # {"schemaVersion":"1",...} symbol-only nodes
curl -s -o /dev/null -w '%{http_code}\n' localhost:PORT/       # 404 (no static hosting)
curl -s -o /dev/null -w '%{http_code}\n' localhost:PORT/api/graph  # 400 (missing symbol)
```

## Findings

- **New ADR-0009 — Versioned symbol-graph JSON API contract for `serve`.** The concrete decision (top-level `schemaVersion` string via a `graphResponse` struct embedding `readmodel.Graph`; symbol-only `Node`/`Edge`; `sym:<qname>` vs `sym#<id>` id schemes across the two endpoints; version-bump-on-incompatible policy) is recorded as ADR-0009 on the docket branch and documented in `docs/graph-api.md`. Cheap insurance adopted now while the only prior consumer (the deleted web app) is gone.
- **Reconcile scope refinement (recorded in the change's Reconcile log, not an ADR).** The readmodel lore coupling was broader than the change body first listed: beyond the `FullGraph` lore branch / `RecordNeighborhood` / `loreNode`, the helpers `Neighborhood`, `openLore`, and `AttachAnchoredLore` also imported lore and were removed; `model.go` was reduced (dropped lore `NodeKind`s, `EdgeAnchors`/`EdgeBlockedBy`, and `Status`/`Priority`). The `/api/graph` param change (`?focus=` + `Neighborhood` → `?symbol=&parent=` + `SymbolNeighborhood`) is a real behavior change, not a rename.
- **Both 0002 carryovers handled here:** the remaining `internal/lore/**` tree and the orphaned `internal/tui/tree` (flagged in change 0002's own results as a 0003 follow-up) are deleted; `grep` for `internal/lore` / `internal/tui` across `*.go` is clean.

## Follow-ups

- **Two deferred minors (pre-existing, non-blocking; from the whole-branch review).** Neither introduced by this change; both are ledger-noted, not filed as stubs (AUTO_CAPTURE is disabled in this repo):
  - `internal/webserver/graphstore.go` — `openGraph` runs `query.Fresh(root)` on every `/api/graph` request (re-index per hit). This is the documented on-demand-freshness design (same as the deleted `Neighborhood`); a response cache would be a separate future change.
  - `internal/webserver/server.go` — `/api/graph/full` discards `*http.Request` (idiomatic; no bug).
- **Out of scope, deferred to change 0004 (not a defect here):** the `.lore/` data dir, README rewrite, config indexing excludes, and the `internal/webserver/dist` string used as a path-prefix example in `internal/config/config.go` (doc comment), `internal/config/filter_test.go`, and `internal/merkle/walk_test.go` (arbitrary fixture path strings, unaffected by deleting the real `dist/`).

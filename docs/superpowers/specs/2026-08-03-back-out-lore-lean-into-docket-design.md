<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0004 — Cleanup — delete .lore/, drop lore config, rewrite README](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/archive/2026-08-04-0004-cleanup-delete-lore-rewrite-readme.md)**
<!-- docket:backlink:end -->

# Back out lore, lean into docket — design

**Date:** 2026-08-03
**Status:** Approved (brainstorm)
**Driver of execution:** docket (this spec is the design; the teardown is tracked as docket changes)

## Summary

codeindex accreted a second, unrelated product inside it: **lore** — an in-repo
work-tracking / decisions / notes engine with a symbol-graph overlay and a React
graph UI (the "lore-graph"). This change **removes lore entirely** and returns
codeindex to a single, sharp purpose: a **blast-radius / impact tool** with a
**decoupled graph query layer** (CLI + headless JSON API) that other systems can
consume. Work-tracking moves to **docket** (danielhanold/docket), which we adopt
in this repo as part of the change.

Non-goals: no new engine features, no rewrite of the impact core, no new UI. This
is a subtraction plus a decoupling and a tooling swap.

## The keep / remove / decouple boundary

### KEEP — the blast-radius core + query layer
- **CLI:** `build`, `refresh`, `status`, `callers`, `callees`, `impact`,
  `dependents`/`deps`, `depmap`, `find`, `grep`, `enclosing`, `export`, `import`,
  `serve`, `mcp`, `bench`
- **MCP tools:** `impact`, `callers`, `callees`, `dependents`, `find`, `grep`
- **Engine packages:** `graph`, `depmap`, `engine`, `search`, `merkle`, `config`,
  `adapter`, `query`, `progress`
- **Read model / API:** `readmodel.SymbolNeighborhood` and the `/api/graph`,
  `/api/graph/full` (symbol-only), `/api/health` endpoints
- **Plugin skill:** `impact.md`

### REMOVE — all of lore
- **Go:** the entire `internal/lore/**` tree; lore code in
  `internal/graph/store.go`; the lore overlay in `readmodel` (`FullGraph` lore
  branch, `RecordNeighborhood`, `loreNode`); lore imports in `cmd/codeindex/main.go`
- **CLI:** the `lore` subcommand (`cmd/codeindex/lore.go`) and `tree` (the TUI over
  lore, `cmd/codeindex/tree.go`); the `--related-depth` / related-lore flag on
  `impact`; the `attach` subcommand
- **MCP:** `internal/mcpserver/lore_tools.go` (`lore_search`, `lore_for_symbol`,
  `lore_backlog`, `lore_show`, `lore_add`)
- **Web:** the entire `web/` React lore-graph UI (including the galaxy retheme) and
  the webserver's static-file handler + `internal/webserver/dist`
- **Plugin skills:** `decide.md`, `lore.md`
- **Data / config:** `.lore/` (after migration), lore/web indexing excludes, and
  `lore.db` handling

### DECOUPLE — "a nice API + CLI other systems can query"
- `serve` becomes a **headless JSON graph API** (no static hosting): `/api/health`,
  `/api/graph?symbol=…`, `/api/graph/full`, with a documented, versioned response
  contract.
- The existing query CLI (`impact`/`callers`/`callees`/`dependents`/`depmap`/
  `find`/`grep`) **is** the decoupled CLI surface — kept, minus lore flags.

## Phasing (tracked in docket)

Docket is bootstrapped first, then the teardown is filed as docket changes so the
pivot is dogfooded immediately.

**Phase 0 — Bootstrap docket + migrate history** (one change)
- Create `.docket.yml`, the docket directory layout (`docs/adrs/`, changes dir,
  `BOARD.md`), and satisfy the bootstrap guard.
- Migrate keeper `.lore/decisions/*` into docket ADRs (see Migration).
- Harvest still-live `.lore/items/*` into docket proposed changes.
- Produces the backlog that tracks Phases 1–3.

**Phase 1 — Rip out the lore product surface** (change)
- Delete `internal/lore/**`, `lore_tools.go`, `cmd/codeindex/lore.go`, `tree.go`,
  `attach`; de-lore `main.go` and `internal/graph/store.go`.
- Remove `decide.md` / `lore.md` plugin skills; strip `related_lore` from `impact`
  (CLI + MCP + engine).
- Green build + tests after each removal.

**Phase 2 — Decouple the graph query layer** (change)
- Delete `web/` and the webserver static handler + `dist`.
- Strip the lore overlay from `readmodel` (`FullGraph`, `RecordNeighborhood`,
  `loreNode`); `serve` becomes the headless JSON API.
- Document + version the API contract (`docs/graph-api.md`).

**Phase 3 — Cleanup & docs** (change)
- Delete `.lore/`; drop lore/web indexing excludes and `lore.db` handling from
  config.
- Rewrite `README.md` to the pure blast-radius positioning (drop lore/graph-UI
  sections).

## `.lore/` migration (Phase 0 detail)

**Decisions → docket ADRs.** Migrate only the durable *engine* decisions:
- tree-sitter parsing + own edge resolution
- sqlite graph-db storage
- Go single-static engine
- config-driven index include/exclude
- on-demand / lazy freshness
- flat per-file content-hash change detection
- references-only output contract (path/line/signature)

**Dropped (lore- or UI-specific):** graph-UI smoothness/aggregation · graph-UI v3
two-state model · lore-is-a-sidecar · lore free-form records · in-repo records +
private overlay · separate lore.db.

**One reversal ADR:** record the `openspec → lore → docket` lineage — "lore replaces
openspec" is now itself superseded by "docket replaces lore" — so the history isn't
silently lost.

**Items → docket changes.** Harvest only items describing engine work still wanted
post-pivot (triaged individually; near-zero expected). Everything lore/graph-UI/
claim-lease dies with lore.

**Notes:** dropped (session-capture), except anything durable folded into the
relevant ADR.

## The graph API contract (Phase 2 detail)

`serve` = headless JSON API, versioned so external consumers are insulated from
internal changes:

- `GET /api/health` → `{ "status": "ok", "version": "<build>" }`
- `GET /api/graph?symbol=<name>&parent=<optional>` → symbol neighborhood: focus +
  direct callers + callees
- `GET /api/graph/full` → whole symbol graph (all tier-0 symbols + resolved call
  edges), nodes grouped by package dir

Response shape = the existing `readmodel.Graph` **stripped to symbol-only**:
`Node{ID, Kind:"symbol", Label, File, Line, Signature, Group}`, `Edge{…}`, `Focus`,
plus a top-level `"schemaVersion"`. Documented in `docs/graph-api.md` so a future
viewer or other tooling can build against it without reading Go.

## Testing

- After each removal in Phases 1–2: `go build ./...` and `go test ./...` stay green.
  Deleting a package means deleting its tests; the remaining suite must pass with no
  dangling references.
- API contract: keep/adjust `internal/webserver/server_test.go` to assert the
  symbol-only response shape and the `schemaVersion` field; assert static hosting is
  gone (root path 404s).
- `impact` without `related_lore`: update its tests to the reference-only output.
- Final gate: full `go test ./...` green, `codeindex build` + a sample `impact`
  query + `serve` API smoke on this repo.

## Risks

- **Hidden lore coupling** in `internal/graph/store.go` or the engine — mitigated by
  incremental delete + green-build-per-step, not one big cut.
- **Migration loss** — the reversal ADR + keeper-ADR list guard against silently
  discarding real history.
- **API consumers** — none exist yet beyond the now-deleted web app, so versioning
  is cheap insurance rather than a compatibility burden.

## Why

Indexing is invisible. A cold build blocks the first query for up to 82s
(kubernetes) with zero feedback — in a terminal it looks slow, through an MCP
client it looks hung. The IDE extension (companion change) needs
machine-readable progress and index state to exist at all, and the CLI
deserves first-class output: a tool whose headline is honest measurement
should *look* like it's measuring.

## What Changes

- **Engine progress events**: Build/Patch variants emit phase events
  (walk → parse → write → resolve) with done/total counts; existing
  signatures remain as zero-overhead wrappers.
- **Status sidecar**: during builds the engine maintains `status.json` next
  to graph.db ({state, phase, done, total, started_at}; terminal state
  records files/symbols/indexed_at) — pollable by any surface.
- **`codeindex status <root> [--json]`**: unindexed / building / fresh-ness,
  schema version, file/symbol/edge counts, index size. The extension's
  detection primitive.
- **Pretty TTY output** for build/export/import: braille spinner, unicode
  progress bar, live rate and ETA, per-phase; final summary line. Plain
  throttled lines when stderr is not a TTY.
- **`--progress` flag**: JSON-lines events on stdout (the extension's feed).
- **First-query annotation**: when a query path triggers a cold build or
  non-trivial patch, the result says so (CLI: stderr note; MCP: leading line
  in the tool result — "indexed 11,005 files in 82s; subsequent queries are
  fast").

**Validation**: unit tests for status JSON shape, JSONL event stream
(monotonic, well-formed, terminal event), sidecar lifecycle; six-repo bench
re-run confirms no build-throughput regression (reporters are nil in bench);
recorded sample output in the findings doc. No agent A/B (UX/infrastructure).

Non-goals: TUI dependencies (hand-rolled ANSI only); progress for
sub-second operations; changing any query output beyond the freshness
annotation.

## Capabilities

### New Capabilities
- `index-progress`: progress events, status sidecar + verb, TTY rendering,
  JSONL feed, freshness annotations.

### Modified Capabilities
None at requirement level.

## Impact

- internal/progress (new), internal/engine (event emission, sidecar),
  internal/graph (re-resolve progress callback), internal/query (Fresh
  returns what it did), cmd/codeindex (status verb, renderers, --progress),
  internal/mcpserver (annotation).

## Why

The token-savings and re-index assumptions have been validated with proxy spikes
(`bench/`), but two things remain **engine-only and unmeasured**: (1) parse+patch
throughput — how fast the real engine re-parses a changed file with tree-sitter
and writes SQLite — and (2) incremental correctness — that an incremental update
produces a graph identical to a full rebuild. This change builds the thinnest
real vertical slice that answers both, before committing to the full
`core-indexing-engine` build. It is a walking skeleton: minimal, but real code
that the full engine extends, not a throwaway.

## What Changes

- New Go module and `codeindex` binary skeleton (the foundation
  `core-indexing-engine` builds on).
- A single language: **Go** (parsed with tree-sitter-go). One language keeps the
  slice thin while exercising the whole pipeline.
- Extract a minimal symbol set (function/method/type definitions) and **call**
  edges only, resolved **name-based** (matching the MVP resolver).
- Persist to SQLite using the target schema subset (`files`, `symbols`, `edges`,
  `merkle`).
- File-level Merkle hashing with the **mandatory size+mtime fast path**; detect
  changed files and patch only those in one transaction.
- `codeindex build` (full index) and an incremental path that re-parses only
  changed files and patches their symbols/edges.
- `codeindex bench` (or `make bench`) that measures **cold build throughput**
  (files/s, LOC/s), **single-file incremental patch latency**, and asserts
  **incremental == full rebuild** by diffing the two graphs.

Non-goals: the four query commands, other languages, precise resolution,
dependency/outline edges, directory-mtime shortcutting, the MCP server, and the
plugin — all remain in `core-indexing-engine` or later changes. This slice exists
to produce numbers and a correctness proof, not features.

## Capabilities

### New Capabilities

- `engine-skeleton`: The minimal real engine slice — Go tree-sitter parsing,
  name-based symbols + call edges, SQLite persistence, file-level Merkle with
  fast-path incremental patching, and a benchmark that measures parse/patch
  throughput and proves incremental-equals-full-rebuild.

### Modified Capabilities

None (new project; `core-indexing-engine` specs are not yet archived, so this
change introduces its own capability rather than modifying an existing one).

## Impact

- New Go module, `codeindex` binary, and dependencies: `go-tree-sitter` +
  tree-sitter-go grammar, a SQLite driver.
- Establishes the concrete package layout, adapter seam, and SQLite schema that
  `core-indexing-engine` will extend (rather than redesign).
- Produces `bench/engine/` results that fill the parse/patch-throughput and
  incremental-correctness gaps left open in `core-indexing-engine`'s tasks
  (9.2, 10.1).
- No user-facing query surface yet; output is limited to build stats and the
  bench report.

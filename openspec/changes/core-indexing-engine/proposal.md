## Why

When Claude navigates a codebase, structural questions ("who calls this?",
"what depends on this class?", "where is this defined?") are answered by
grepping and then reading whole files into context — burning tokens to surface
a few relevant lines, and getting slower as the repo grows. A pre-built symbol
relationship graph can answer these questions with compact `file:line +
signature` references instead of full-file dumps.

## What Changes

- New `codeindex` Go CLI that indexes a codebase into a SQLite-backed symbol
  graph and answers navigation queries.
- `codeindex build` walks the repo, parses source with tree-sitter, resolves
  relationships, and writes symbols + edges + a Merkle content index to
  `.codeindex/graph.db`.
- A Merkle content-hash tree drives incremental updates: each query recomputes
  the root, diffs it against the stored root, re-parses only changed files, and
  patches only the affected symbols/edges before answering (lazy re-check, no
  daemon).
- Language adapters (pluggable interface) for **TypeScript/JavaScript** and
  **Go** in this change. Edge resolution is **name-based** for the MVP, with
  every edge carrying a `resolved_confidence` value.
- Four query commands: callers/callees, definition+signature lookup,
  dependencies/dependents, and symbol search/outline.
- Output is reference-based (`path:line  signature`) with an opt-in `--json`
  format and an opt-in `--context N` for pulling limited source lines.
- A reproducible benchmark harness with measurable performance targets across
  three repository tiers (small ~50k LOC, medium ~500k LOC, large ~5M LOC) for
  cold build throughput, incremental update latency, query latency (including
  the lazy re-check), token savings vs. grep+read, and index size — enforced as
  a CI regression guard.

Non-goals for this change: Python/PHP/.NET adapters, import/scope-aware precise
resolution, the MCP server, and the Claude plugin packaging (all deferred to
later changes).

## Capabilities

### New Capabilities

- `code-indexing`: Building and incrementally maintaining the on-disk index —
  repo walk, tree-sitter parsing via language adapters, Merkle content tree,
  SQLite persistence, and lazy per-query re-check.
- `symbol-graph`: The graph data model and storage — symbols, edges (calls,
  imports, extends, implements, references), resolution confidence, and the
  name-based resolver that produces edges.
- `graph-queries`: The CLI query surface over the graph — callers/callees,
  definition/signature lookup, dependencies/dependents, symbol search/outline,
  and the reference-based output contract (`--json`, `--context`).
- `performance`: Measurable performance characteristics and the benchmark
  harness — per-tier targets for cold build, incremental update, query latency,
  token savings, and index size, plus build parallelism, memory bounds, and a
  CI regression guard.

### Modified Capabilities

None (greenfield project).

## Impact

- New Go module and binary (`codeindex`); dependencies: `go-tree-sitter` (+
  TypeScript/TSX/JavaScript/Go grammars), `mattn/go-sqlite3` (or equivalent).
- New on-disk artifact `.codeindex/graph.db` (gitignored).
- New benchmark harness and reference corpora (representative OSS repos per
  tier) plus a CI job that records baselines and fails on regression.
- Establishes the language-adapter interface and SQLite schema that changes 2
  (more languages, precise resolution) and 3 (MCP + plugin) build on.
- No existing code affected — greenfield.

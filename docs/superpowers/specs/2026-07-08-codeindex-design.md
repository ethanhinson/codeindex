# codeindex — Design

**Date:** 2026-07-08
**Status:** Approved (brainstorming complete)
**Planning:** OpenSpec (`openspec/changes/`)

## Problem

When Claude works in a codebase, answering structural questions ("who calls
`AuthService.login`?", "what depends on this class?", "where is this defined?")
means grepping and then reading whole files into context. That burns tokens —
often reading 8 files to find 3 relevant lines — and gets slower as the repo
grows.

## Goal

A local tool that pre-indexes a codebase into a **symbol relationship graph**
(call graph + dependency graph) and answers navigation queries with compact
`file:line + signature` references instead of full-file dumps. Kept fresh
cheaply via a **Merkle-tree incremental layer**.

Non-goals: semantic code understanding/summarization, executing code, replacing
grep for arbitrary text search, and (for the MVP) perfect cross-file type
resolution.

## Key Concepts

**Symbol graph vs. change-detection index — two distinct structures:**

- **Symbol graph** — nodes are symbols (functions, methods, classes, types),
  edges are relationships (calls, imports, extends, implements, references).
  This is the queryable data.
- **Content-hash change detection** — per-file content hashes with a size+mtime
  fast path. Diff against stored state to get exactly which files were added,
  modified, or deleted — so you re-parse only those and patch only the affected
  graph edges. It is not the relationship data itself.
  *(Revised by measurement: the original design was a full Merkle tree with
  directory nodes and a root hash. The built engine proved a flat per-file
  table meets every latency target, and interior nodes add nothing locally —
  directory mtime provably does not change on content edits inside it, so the
  natural subtree shortcut is incorrect. True Merkle interior nodes are
  deferred to a possible future index-sharing/dedup capability.)*

## Architecture

Bottom-up layers:

```
┌─ Claude plugin (slash cmds + skill) ─┐   change 3
├─ MCP server (Go MCP SDK)             ┤   change 3
├─ CLI (codeindex build|callers|deps…) ┤   change 1
├─ Query engine (graph traversals)     ┤
├─ Symbol graph (SQLite schema)        ┤
├─ Merkle index (change detection)     ┤
├─ Resolver (imports/scope → edges)    ┤
└─ Language adapters (tree-sitter)     ┘   pluggable per language
```

**Language adapter interface** — each language implements roughly:

- `Parse(file) → symbols[]`
- `ExtractEdges(tree) → rawEdges[]`
- `ResolveImports(file) → importMap`

This isolates per-language logic so adding a language never touches another
language's adapter.

## Data Flow

- **`codeindex build`**: walk repo → hash files/dirs into a Merkle tree →
  parse each file via its adapter → resolve edges → write symbols, edges, and
  Merkle state into SQLite.
- **Every query (lazy re-check)**: recompute the Merkle root → diff against the
  stored root → for each changed leaf, re-parse that file and patch its
  symbols/edges → then answer. Always correct, no background daemon.

## Storage — SQLite (sketch, updated to the proven skeleton schema)

- `files(id, path, hash, size, mtime, lang)` — doubles as the change-detection
  state (per-file leaves; no directory/root rows)
- `symbols(id, file_id, name, kind, signature, start_line, end_line, parent_id)`
  — `parent_id` enables qualified names (`Type.Method`)
- `edges(src_symbol_id, dst_symbol_id, dst_name, kind, resolved_confidence, line, src_file_id)`
  where `kind ∈ {calls, imports, extends, implements, references}`; `dst_name`
  makes unresolved edges representable and re-resolution cheap; `src_file_id`
  makes per-file replacement cheap; `line` gives call-site references
- Indexes on `symbols.name`, `edges.src_symbol_id`, `edges.dst_symbol_id`,
  `edges.dst_name` for fast both-direction traversal and name re-resolution.
- String interning / `file_id` normalization is the index-size lever (skeleton
  measured 1.7–2.2× source unoptimized; target ≤2×).

Stored under `.codeindex/graph.db` (gitignored).

## Query Surface (MVP)

1. **Callers / callees** — both directions of the call graph.
2. **Definition & signature lookup** — `file:line` + signature (optionally
   docstring), no full-file dump.
3. **Dependencies / dependents** — import/type/class-level edges, including
   blast-radius / impact analysis.
4. **Symbol search / outline** — fuzzy find by name; compact outline (all
   symbols + signatures) of a file/module.

## Output Contract (the token win)

- CLI prints compact text by default (`path:line  signature`); `--json` for
  structured output (used later by MCP).
- Every result is a **reference**, never full source. `--context N` pulls N
  lines only when explicitly asked.
- Edges carry `resolved_confidence` so ambiguous (name-only) matches are
  flagged, not silently presented as certain.

## Performance & Benchmarks

Performance is the entire justification for the tool, so targets are
first-class, measurable requirements with a reproducible harness and a CI
regression guard — not assumptions. Targets are defined against a baseline
machine (8 performance cores, NVMe SSD, warm file cache) across three tiers.

| Tier   | LOC  | Files | Cold build | Incremental (1 file) | Query p95 (unchanged) | Index size |
| ------ | ---- | ----- | ---------- | -------------------- | --------------------- | ---------- |
| Small  | 50k  | 500   | ≤ 3 s      | ≤ 150 ms             | ≤ 75 ms               | reported   |
| Medium | 500k | 5k    | ≤ 30 s     | ≤ 300 ms             | ≤ 150 ms              | ≤ 2× src   |
| Large  | 5M   | 50k   | ≤ 5 min    | ≤ 750 ms             | ≤ 400 ms              | ≤ 2× src   |

Invariants: incremental work scales with changed-file count (not repo size);
token savings ≥ 10× median vs. grep+read; peak build memory ≤ 1 GB; CI fails on
>20% regression. (Two original invariants were revised by measurement: index
size ≤25% was falsified — unoptimized skeleton measured 1.7–2.2× source; and
the parallel-efficiency gate was dropped — the cold-build budget is the outcome
and is met with headroom.) Full detail lives in the `performance` capability
spec of the `core-indexing-engine` change. Walking-skeleton status: cold build,
incremental latency, and incremental==full-rebuild are all measured/proven
(`bench/engine/FINDINGS.md`).

## Technology Decisions

- **Implementation:** Go — single static binary, fast concurrent parsing,
  `go-tree-sitter` bindings, `mattn/go-sqlite3`, official Go MCP SDK for later.
- **Parsing:** tree-sitter for breadth + speed. Resolution is our own logic;
  MVP is name-based (edges matched by symbol name, flagged with confidence),
  later upgraded to import/scope-aware precise edges.
- **Freshness:** on-demand build + lazy Merkle re-check per query. No daemon.

## Roadmap — OpenSpec changes (risk-ordered after review)

0. **`engine-walking-skeleton`** — DONE. Real Go slice: tree-sitter → name-based
   symbols+call edges → SQLite → content-hash change detection → build +
   incremental patch + bench. Proved throughput and incremental==full-rebuild.
1. **`agent-ab-efficacy`** — NEXT. A/B harness running real Claude agents on
   real issues, with vs. without codeindex, measuring total task tokens, tool
   calls, adoption, and task success. This is the goal metric; it gates
   everything downstream.
2. **`core-indexing-engine` (MVP):** SQLite graph + adapters for **TS/JS + Go**
   + name-based resolver + query surface + lazy re-check — breadth shaped by
   what agents actually queried in the A/B runs.
3. **`language-coverage-and-resolution`:** add **Python, PHP, .NET/C#**
   adapters; upgrade resolution *as the precision data demands* (measure
   name-based precision/recall against a compiler-grade oracle first;
   receiver-aware resolution may capture most of the win cheaply).
4. **`mcp-and-plugin`:** MCP server wrapper + Claude plugin (slash commands +
   skill). Must address concurrent-query serialization for the long-lived
   server.

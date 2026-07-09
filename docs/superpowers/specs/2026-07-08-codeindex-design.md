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

**Symbol graph vs. Merkle tree — two distinct structures:**

- **Symbol graph** — nodes are symbols (functions, methods, classes, types),
  edges are relationships (calls, imports, extends, implements, references).
  This is the queryable data.
- **Merkle tree** — a tree of content hashes (hash each file, then each
  directory from its children's hashes, up to a root). This is *only* for
  change detection: diff two roots, walk down the branches whose hashes
  differ, and you know exactly which files changed — so you re-parse only
  those and patch only the affected graph edges. It is not the relationship
  data itself.

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

## Storage — SQLite (sketch)

- `files(id, path, hash, lang, mtime)`
- `symbols(id, file_id, name, kind, signature, start_line, end_line, parent_id)`
- `edges(src_symbol_id, dst_symbol_id, kind, resolved_confidence)`
  where `kind ∈ {calls, imports, extends, implements, references}`
- `merkle(path, hash, parent_path)`
- Indexes on `symbols.name`, `edges.src_symbol_id`, `edges.dst_symbol_id`
  for fast both-direction traversal.

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

## Technology Decisions

- **Implementation:** Go — single static binary, fast concurrent parsing,
  `go-tree-sitter` bindings, `mattn/go-sqlite3`, official Go MCP SDK for later.
- **Parsing:** tree-sitter for breadth + speed. Resolution is our own logic;
  MVP is name-based (edges matched by symbol name, flagged with confidence),
  later upgraded to import/scope-aware precise edges.
- **Freshness:** on-demand build + lazy Merkle re-check per query. No daemon.

## Roadmap — three OpenSpec changes

1. **`core-indexing-engine` (MVP):** Merkle layer + SQLite graph + tree-sitter
   adapters for **TypeScript/JS + Go** + name-based resolver + all four query
   types + on-demand/lazy-recheck. Proves the full vertical slice.
2. **`language-coverage-and-resolution`:** add **Python, PHP, .NET/C#**
   adapters; upgrade resolver from name-based to import/scope-aware.
3. **`mcp-and-plugin`:** MCP server wrapper + Claude plugin (slash commands
   `/callers`, `/deps`, … + a skill teaching Claude to query before grepping).

Change 1 is specced fully first; 2 and 3 are outlined and detailed when
reached.

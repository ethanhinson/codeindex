## Context

Greenfield project. `codeindex` is a Go CLI that indexes a codebase into a
SQLite-backed symbol relationship graph so Claude can answer navigation
questions with compact references instead of grepping and reading whole files.
This change (`core-indexing-engine`) is the MVP vertical slice: the indexing
engine, the graph model, and the CLI query surface for TypeScript/JS + Go.

Constraints:

- Must run as a single self-contained binary with no language toolchains or
  running services required.
- Answers must always reflect the current working tree, including uncommitted
  edits, without a background daemon.
- Per-query overhead must stay small on an unchanged repo (the common case).

The full product vision and layer diagram live in
`docs/superpowers/specs/2026-07-08-codeindex-design.md`.

## Goals / Non-Goals

**Goals**

- Build and persist a symbol graph (symbols + edges) to `.codeindex/graph.db`.
- Incrementally maintain it via a Merkle content tree — re-parse only changed
  files.
- Answer four query types from the CLI: callers/callees, definition/signature,
  dependencies/dependents, symbol search/outline.
- Establish the language-adapter interface and SQLite schema for later changes.

**Non-Goals**

- Python/PHP/.NET adapters (change 2).
- Import/scope-aware precise resolution (change 2). MVP resolution is
  name-based, with confidence flags.
- MCP server and Claude plugin packaging (change 3).
- Semantic understanding/summarization, code execution, general text search.

## Decisions

**D1 — Go + tree-sitter.** Single static binary, fast concurrent parsing,
`go-tree-sitter` bindings cover TS/TSX/JS/Go grammars.
*Alternatives:* LSP servers (most accurate cross-file but heavy per-language
runtime, slow batch indexing, fragile orchestration — rejected for MVP);
Rust (comparable, team picked Go); Node/Python (slower for large-repo
indexing, harder single-binary distribution).

**D2 — SQLite storage.** One portable file, transactional incremental patches,
indexed both-direction edge traversal, trivially inspectable.
*Alternatives:* embedded graph DB (Kùzu/DuckDB — nicer multi-hop traversal,
heavier/less familiar dependency); flat JSON/Parquet (awkward incremental
updates); in-memory (loses persistence and the Merkle benefit).

**D3 — Merkle tree for change detection only.** Leaves hash file contents;
directory nodes hash their children's hashes up to a root. A query recomputes
the root, and when it differs walks down only the branches whose hashes changed
to find the exact set of changed files. The Merkle tree is *not* the
relationship graph — it decides *what to re-parse*, and the graph holds the
relationships.
*Alternatives:* mtime-only (misses touch-without-change and clock issues);
full rebuild each run (too slow); git-diff (misses uncommitted edits).

**D4 — Lazy per-query re-check, no daemon.** `codeindex build` creates the
initial index; every query does a Merkle diff first, patches changed files,
then answers. Always-correct with near-zero overhead when nothing changed.
*Alternatives:* watch-mode daemon (freshest but a process to manage — deferred);
manual-only updates (risks stale answers); git hooks (misses working-tree
edits).

**D5 — Name-based resolution with confidence, for the MVP.** An edge records
that a symbol *named* `save` is referenced; `resolved_confidence` marks whether
the target is unambiguous (single definition of that name) or ambiguous
(multiple candidates). This ships useful edges now; change 2 upgrades the
resolver to import/scope-aware precision without changing the schema.
*Alternatives:* precise resolution up front (much more per-language work,
blocks the MVP); hiding ambiguity (misleads consumers).

**D6 — Pluggable language-adapter interface.** Each language implements
`Parse`, `ExtractEdges`, `ResolveImports`. Adapters are registered by file
extension. Adding a language never touches another adapter.
*Alternatives:* one monolithic parser with per-language branches (poor
isolation, hard to test independently).

**D7 — Reference-based output contract.** Default output is
`path:line  signature` lines; `--json` emits structured records for later MCP
use; `--context N` opts into N source lines. Results are never full-file dumps —
this is the whole token-savings mechanism.

## Risks / Trade-offs

- **Name collisions inflate edges** → `resolved_confidence` flags ambiguous
  matches so consumers (and later the resolver) can distinguish them; change 2
  removes most ambiguity.
- **tree-sitter grammar drift / dynamic constructs** (reflection, dynamic
  dispatch, `eval`) miss edges → accept as a known limit; surface confidence so
  gaps aren't presented as certainties.
- **Large-repo initial build cost** → parse files concurrently (Go worker
  pool); persist so the cost is paid once, then only incrementally.
- **Merkle diff overhead on huge trees** → hashing on unchanged files can be
  skipped via a size+mtime fast-path before content hashing; only hash when the
  fast-path is inconclusive.
- **SQLite write contention** → the CLI is single-writer per invocation; wrap
  incremental patches in one transaction per query.
- **CGO dependency** (`go-tree-sitter`, `go-sqlite3`) complicates static builds
  → document the build toolchain; revisit pure-Go alternatives if distribution
  friction appears (deferred).

## Migration Plan

Greenfield — no migration. Deployment is `go build` producing the `codeindex`
binary. Rollback is deleting the binary and the `.codeindex/` directory; the
index is a derived artifact and safe to regenerate at any time.

## Open Questions

- Exact Merkle granularity: per-file leaves only, or also chunk large files?
  (Default: per-file leaves for the MVP.)
- Should `.codeindex/graph.db` be committed or always gitignored? (Default:
  gitignored; it is a derived artifact.)
- Ignore rules: honor `.gitignore` for the repo walk, plus a `.codeindexignore`?
  (Default: honor `.gitignore` + built-in defaults for the MVP.)

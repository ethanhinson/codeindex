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

**D8 — Performance is specced and benchmarked, not assumed.** Because the entire
justification for the tool is being faster and cheaper than grep+read, targets
are first-class, measurable requirements (see the `performance` spec) with a
reproducible harness and a CI regression guard.
*Methodology:*
- **Baseline machine:** 8 performance cores, NVMe SSD, warm OS file cache. All
  absolute targets are relative to this; other hardware reports ratios.
- **Tiers & corpora:** small (~50k LOC), medium (~500k LOC), large (~5M LOC),
  each pinned to representative open-source reference repositories (a mix of Go
  and TS/JS) at fixed commits for reproducibility.
- **Dimensions measured:** cold build time + parallel efficiency, incremental
  update latency (bounded by changed-file count), query p50/p95 including the
  lazy re-check, token-savings ratio vs. grep+read over a fixed navigation
  question set, index size, and peak build memory.
- **Token-savings measurement:** for each fixed question, compare tokens in the
  `codeindex` answer against the tokens of the source files a naive grep+read
  would load to answer it; report the median ratio (target ≥ 10×).
- **Regression guard:** the harness records a baseline; CI fails a run when any
  metric regresses > 20%, naming the metric and tier.

*Target table (baseline machine):*

| Tier   | LOC  | Files | Cold build | Incremental (1 file) | Query p95 (unchanged) | Index size |
| ------ | ---- | ----- | ---------- | -------------------- | --------------------- | ---------- |
| Small  | 50k  | 500   | ≤ 3 s      | ≤ 150 ms             | ≤ 75 ms               | ≤ 25% src  |
| Medium | 500k | 5k    | ≤ 30 s     | ≤ 300 ms             | ≤ 150 ms              | ≤ 25% src  |
| Large  | 5M   | 50k   | ≤ 5 min    | ≤ 750 ms             | ≤ 400 ms              | ≤ 25% src  |

*Alternatives:* leaving performance implicit (no way to catch regressions, and
no evidence the token story holds); micro-benchmarks only (miss end-to-end query
latency and the token-savings ratio that actually matters).

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
- **Lazy re-check walk cost at the large tier** (50k stat calls on every query)
  could blow the query-latency budget → size+mtime fast path avoids hashing, and
  directory-mtime shortcutting skips subtrees whose directory mtime is unchanged;
  if still too slow, fall back to an optional short-lived watch cache (deferred
  to change 3, not required here).
- **Token-savings target may not hold for broad queries** (e.g. a symbol with
  hundreds of callers) → cap default result counts with a `--limit`, keep output
  as references, and measure the ratio over a representative question set rather
  than worst case.

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

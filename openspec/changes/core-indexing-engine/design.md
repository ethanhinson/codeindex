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

**D3 — Per-file content-hash change detection (flat table, not a tree).**
Leaves hash file contents; a size+mtime fast path avoids hashing unchanged
files; the diff against stored state yields the exact added/modified/deleted
set. The change-detection layer is *not* the relationship graph — it decides
*what to re-parse*, and the graph holds the relationships.
**Revised by measurement:** the original design called for Merkle interior
directory nodes and a root hash. The walking skeleton proved a flat per-file
table meets every latency target (119 ms single-file patch on kubernetes), and
testing showed interior nodes add nothing locally — you must stat leaves to
notice changes anyway, and directory mtime provably does not change on content
edits inside it (so subtree-skipping by dir mtime would *miss edits*). Interior
hash nodes are deferred to a possible future index-sharing/dedup capability
where comparing subtree hashes across machines has value.
*Alternatives:* mtime-only (misses touch-without-change and clock issues);
full rebuild each run (too slow); git-diff (misses uncommitted edits); Merkle
tree with root diff (measured: no local benefit, and the natural subtree
shortcut is incorrect).

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

*Validation status:* a pre-implementation spike (`bench/`, `bench/FINDINGS.md`)
already validated the token assumption against real OSS repos (gin, prometheus,
nest, kubernetes) using `rg` grep-by-name as a faithful proxy for the name-based
MVP edges. Findings that shaped this design: (1) definition and callers savings
are 100–500× on large-file Go repos and hold at the kubernetes/large tier;
(2) savings scale with source file size — the win is smallest (~9–12×) on
well-factored small-file TS, so the token target is corpus-relative; (3) outline
saves less (~6–17×), so its target is ≥5×, not ≥10×; (4) JSON output costs
~1.5–1.7× the text form, so text is the default; (5) a full-repo grep on
kubernetes takes ~0.65s, reinforcing D2 (indexed SQLite lookups over per-query
scanning) for the interactive/IDE latency budget.

A second spike (`bench/reindex_bench.py`) validated the *re-index* path:
change-detection walk cost (stat ~185 ms vs full hash ~980 ms on kubernetes) and
edge blast-radius (median 2–7 inbound refs, 10–13% hot up to ~4000; real churn
median 1 file/commit). These made the fast path and vendored-tree exclusion
*required* rather than optional.

The `engine-walking-skeleton` change then built the real slice and closed the
remaining engine-only unknowns (`bench/engine/FINDINGS.md`): parse+patch
throughput meets every tier budget with headroom (kubernetes cold build 31.7 s,
single-file patch 119 ms), and **incremental == full rebuild is proven** on real
kubernetes (116k symbols) plus unit tests. A 109-symbol study over real GitHub
issues (`bench/efficacy-FINDINGS.md`) grounded the token claim end-to-end
statically (median 363× vs file-read, min 6×). Two originally-specced
constraints were **falsified by measurement and revised**: the ≤25% index-size
target (measured 1.7–2.2× source; target now ≤2× with interning as the known
optimization) and directory-mtime subtree skipping (provably misses content
edits; removed). Remaining unmeasured: query p50/p95 with the lazy re-check
wired in, build memory, and — the decisive one — end-to-end agent task savings,
covered by the `agent-ab-efficacy` change.

## Risks / Trade-offs

- **Name collisions inflate edges** → `resolved_confidence` flags ambiguous
  matches so consumers (and later the resolver) can distinguish them; change 2
  removes most ambiguity.
- **tree-sitter grammar drift / dynamic constructs** (reflection, dynamic
  dispatch, `eval`) miss edges → accept as a known limit; surface confidence so
  gaps aren't presented as certainties.
- **Large-repo initial build cost** → parse files concurrently (Go worker
  pool); persist so the cost is paid once, then only incrementally.
- **SQLite write contention** → the CLI is single-writer per invocation; wrap
  incremental patches in one transaction per query. NOTE for change 3: a
  long-lived MCP server serving concurrent queries must serialize re-check
  writes in-process (the lazy re-check design assumed a single-shot CLI) — spec
  this when change 3 is drafted.
- **CGO dependency** (`go-tree-sitter`, `go-sqlite3`) complicates static builds
  → document the build toolchain; revisit pure-Go alternatives if distribution
  friction appears (deferred).
- **Lazy re-check walk cost at the large tier** — *measured*: full content
  hashing on the query path (~980 ms on kubernetes) would exceed the 400 ms
  budget → the size+mtime fast path is **required** and vendored/generated trees
  are excluded. The real Go engine's stat walk measured ~119 ms serial on 11k
  files; parallel stat covers polyglot repos with far more files. Directory-mtime
  subtree skipping was considered and **rejected as incorrect** (dir mtime does
  not change on content edits inside it — empirically verified). If a repo's file
  count still blows the budget, the fallback is an opt-in filesystem-watch cache,
  never a correctness compromise.
- **Branch switching looks like a mass edit** — `git checkout` can change
  thousands of files at once, a daily event. Measured cold-build throughput
  (prometheus 253k LOC in 1.4 s) suggests even large change-sets patch in
  seconds, but this is untested → add an explicit branch-switch benchmark case,
  and fall back to a full rebuild above a changed-file threshold if patching is
  slower than rebuilding.
- **Hot-symbol edit ripple** — *measured*: the median symbol has 2–7 inbound
  references but 10–13% are "hot" (>100, up to ~4000). Editing a hot symbol
  re-resolves many edges → re-parse stays single-file, edge re-resolution runs
  only when the changed file's defined-name set changes, and uses indexed name
  lookups whose cost is proportional to reference count (not repo size). Real
  change-sets are tiny (median 1 file/commit), so this is a tail case, not the
  norm.
- **Token-savings target may not hold for broad queries** (e.g. a symbol with
  hundreds of callers) → cap default result counts with a `--limit`, keep output
  as references, and measure the ratio over a representative question set rather
  than worst case.

## Migration Plan

Greenfield — no migration. Deployment is `go build` producing the `codeindex`
binary. Rollback is deleting the binary and the `.codeindex/` directory; the
index is a derived artifact and safe to regenerate at any time.

## Open Questions

- Should `.codeindex/graph.db` be committed or always gitignored? (Default:
  gitignored; it is a derived artifact.)
- Ignore rules: honor `.gitignore` for the repo walk, plus a `.codeindexignore`?
  (Default: honor `.gitignore` + built-in defaults for the MVP.) The skeleton's
  hardcoded exclusion of `testdata/` is a decision to revisit — agents do ask
  about test fixtures.
- Schema normalization: the skeleton denormalizes file paths as TEXT on
  `symbols`/`edges`; the full engine should adopt `file_id` + string interning
  (also the main index-size lever) and add `parent_id` (required by the
  qualified-names requirement in `symbol-graph`).
- The skeleton's `query` command was an unspecced measurement aid: it does NOT
  perform the lazy re-check and returns empty on an unbuilt repo. Change-1 tasks
  7.1/7.2 must retrofit both behaviors before it becomes the real query path.
- Precise-resolution strategy for change 2: before hand-building per-language
  scope resolvers, measure name-based precision/recall against a compiler-grade
  oracle (gopls / `go/types` for Go), and consider optional compiler-front-end
  resolvers where available; receiver-aware method resolution may capture most
  of the win cheaply.

## 1. Project scaffolding

- [x] 1.1 Initialize Go module (`codeindex`) with the proven skeleton layout: `cmd/codeindex`, `internal/{adapter,graph,merkle,engine}` (resolver lives in the graph store; done in `engine-walking-skeleton`)
- [x] 1.2 Add core dependencies (done in skeleton: `smacker/go-tree-sitter` + Go grammar, `mattn/go-sqlite3`; stdlib arg parsing proved sufficient — add TS/TSX/JS grammars and a CLI framework here only if flag surface demands it)
- [x] 1.3 `.gitignore` entry for `.codeindex/` and `go build` target (done in skeleton)
- [ ] 1.4 Wire subcommands `callers`, `callees`, `def`, `deps`, `dependents`, `search`, `outline` (superseding the skeleton's combined `query`) with global `--json` / `--context N` / `--limit N` flags

## 2. Storage layer (symbol-graph)

- [ ] 2.1 Evolve the proven skeleton schema: normalize file paths to `file_id` + interned strings (the main index-size lever), add `parent_id` for qualified names, keep the proven `dst_name`/`line`/source-file linkage on edges
- [x] 2.2 Schema creation on first open of `.codeindex/graph.db` (done in skeleton; add migration when 2.1 changes the schema)
- [x] 2.3 Graph store CRUD in a transaction (done in skeleton)
- [x] 2.4 Test: per-file replace removes stale rows and leaves other files untouched (covered by skeleton engine tests)

## 3. Content-hash change detection (code-indexing)

- [ ] 3.1 Implement the repo walk honoring `.gitignore` + built-in ignore defaults (skeleton uses a hardcoded skiplist; revisit the `testdata/` exclusion)
- [x] 3.2 Per-file content hashing with a size+mtime fast path (done in skeleton; NO directory nodes/root — measured unnecessary, and dir-mtime skipping provably misses content edits)
- [x] 3.3 Diff (added/modified/deleted) against stored state (done in skeleton)
- [x] 3.4 Test: correct changed-file sets; fast path skips unchanged files (covered by skeleton tests)
- [ ] 3.5 Parallelize the stat walk and measure at polyglot scale (100k+ files); if the budget is still exceeded, design the opt-in watch cache fallback

## 4. Language adapters (code-indexing)

- [ ] 4.1 Define the adapter interface (`Parse`, `ExtractEdges`, `ResolveImports`) and an extension→adapter registry
- [ ] 4.2 Implement the Go adapter: extract funcs/methods/types/structs as symbols; extract call, import, and interface `implements`/embed edges
- [ ] 4.3 Implement the TypeScript/JavaScript adapter (`.ts/.tsx/.js/.jsx`): extract functions/classes/methods/types; extract call, import, `extends`, `implements` edges
- [ ] 4.4 Test each adapter against fixture files: assert the expected symbols (name/kind/signature/line span) and raw edges are produced

## 5. Name-based resolver (symbol-graph)

- [ ] 5.1 Resolve raw edges by symbol name across the index; set `resolved_confidence` = unambiguous / ambiguous
- [ ] 5.2 Retain unresolved references as edges with the name preserved (no fabricated target)
- [ ] 5.3 Test: single definition → unambiguous; multiple definitions → ambiguous with candidates; no definition → unresolved

## 6. Build orchestration (code-indexing)

- [ ] 6.1 Extend `codeindex build` (Go slice done in `engine-walking-skeleton`: walk → hash → concurrent parse → resolve → persist) to the TS/JS adapter and the evolved schema
- [ ] 6.2 Skip files with no registered adapter without error
- [ ] 6.3 Test: full build of a mixed Go+TS fixture repo produces the expected symbol/edge counts

## 7. Lazy re-check integration (code-indexing)

- [ ] 7.1 Implement a pre-query hook (diff → re-parse+patch changed files in one transaction, before answering) and retrofit it into the query path — the skeleton's `query` does NOT do this today
- [ ] 7.2 Build the index automatically if `.codeindex/graph.db` is missing when a query runs (the skeleton's `query` silently returns empty today)
- [ ] 7.3 Wire the parallel fast-path walk (task 3.5) + vendored/generated-tree exclusion into the pre-query hook; report excluded trees
- [ ] 7.4 Implement inbound-edge re-resolution: diff the changed file's set of defined symbol names; only when it changes, re-resolve edges referencing the affected names via indexed name lookups (cost ∝ reference count, not repo size)
- [ ] 7.5 Test: query after an edit reflects new content; query with no changes makes zero graph writes; query before any build triggers a build; editing a function body (no name-set change) triggers no inbound re-resolution; renaming a hot symbol re-resolves its referencing edges correctly

## 8. Query engine + output (graph-queries)

- [ ] 8.1 Implement callers/callees traversals over `calls` edges; flag ambiguous results; disambiguate when the requested name matches multiple definitions
- [ ] 8.2 Implement definition/signature lookup (with `--context N` bounded lines) and "not found" handling
- [ ] 8.3 Implement dependencies/dependents over `imports`/`extends`/`implements`/`references` edges (blast radius)
- [ ] 8.4 Implement fuzzy symbol search (ranked) and file/module outline
- [ ] 8.5 Implement the output layer: default `path:line  signature` text and `--json` structured records (path, line, signature, edge kind, confidence)
- [ ] 8.6 Test each query command end-to-end against the fixture repo in both text and JSON modes

## 9. Performance & benchmarks (performance)

- [x] 9.0 Pre-implementation token-savings spike validating the core assumption against real OSS repos (`bench/`, `bench/FINDINGS.md`) — done: 100–500× for def/callers on large-file Go incl. kubernetes; file-size dependent; outline weaker; JSON ~1.5–1.7× text
- [x] 9.0b Pre-implementation re-index spike (`bench/reindex_bench.py`) — done: change-detection walk cost (stat ~185ms vs hash ~980ms on kubernetes) and edge blast-radius (median 2–7 inbound, 10–13% hot up to ~4000; churn median 1 file/commit). Established fast-path/dir-shortcutting/vendored-exclusion as required; parse+patch throughput + incremental-correctness remain engine-only
- [ ] 9.1 Pin reference corpora at fixed commits for each tier (small ~50k, medium ~500k, large ~5M LOC; mix of Go + TS/JS) and a fetch script — extend `bench/repos.json`, fold the spike's `rg`-proxy corpora into the real harness
- [ ] 9.2 Extend `codeindex bench` (cold build + incremental done in `engine-walking-skeleton`, all targets met — `bench/engine/FINDINGS.md`) with query p50/p95 incl. lazy re-check, index size, peak build memory, and worker-count reporting
- [ ] 9.3 Define the fixed navigation-question set and measure the token-savings ratio (codeindex answer tokens vs. grep+read source tokens); assert median ≥ 10× (static ratio — end-to-end agent savings are the `agent-ab-efficacy` change)
- [ ] 9.4 Measure index size per tier against the ≤2× source bound (medium/large); implement string interning / `file_id` normalization if over (skeleton measured 1.7–2.2×)
- [ ] 9.5 Confirm typical `def`/`callers` answers stay ≤ ~500 tokens under default `--limit` (skeleton already has `--limit`; efficacy study measured median 449)
- [ ] 9.6 Record baseline results and add a CI job that fails on >20% regression, naming the metric and tier
- [ ] 9.7 Verify each per-tier target in the `performance` spec is met (or record a documented, justified deviation)
- [ ] 9.8 Benchmark the branch-switch case (thousands of files changed at once); add a full-rebuild fallback above a changed-file threshold if patching loses to rebuilding

## 10. Verification

- [ ] 10.1 Integration test: build → query → edit a file → re-query with query-level assertions (incremental==full-rebuild correctness already proven in `engine-walking-skeleton` on real kubernetes, 116k symbols)
- [ ] 10.2 Add a README documenting the CLI commands, the output contract, the performance targets, and the build toolchain (CGO note)
- [ ] 10.3 Run `openspec validate core-indexing-engine` and confirm the change is valid and complete

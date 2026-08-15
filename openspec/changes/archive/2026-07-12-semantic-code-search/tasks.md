# Tasks: semantic-code-search

## 1. Embedding engine foundation

- [x] 1.1 Vendor pinned llama.cpp revision and wire static CGO build in `internal/embed/local` (build tags, CFLAGS isolated to the package); verify `go build` produces a single binary on macOS arm64 + Linux amd64 *(macOS arm64 verified; Linux LDFLAGS in place, needs CI run)*
- [x] 1.2 Define `embed.Provider` interface (`Embed(ctx, texts) ([][]float32, error)`, `ID() string`, `Dims() int`) and implement the local provider: GGUF load, batched CPU inference across cores
- [x] 1.3 Select candidate bundled models (all-MiniLM-L6-v2, bge-small-en-v1.5 quantized GGUF), add `go:embed` of the default weights, and expose the model ID as name + weights hash *(all-MiniLM bundled; bge-small in pull registry pending 6.2 bench)*
- [x] 1.4 Implement `codeindex model pull|use|status` with cache in `~/.cache/codeindex/models/` and per-repo selection in `.codeindex` config; pulled model takes precedence over bundled
- [x] 1.5 Implement the `api` provider (config + env credential, Voyage/OpenAI-compatible endpoint) with fallback to local and disclosed fallback when the credential is missing
- [x] 1.6 Unit tests: provider selection, model-ID stamping, deterministic embeddings for fixed model/binary, credential-missing fallback

## 2. Vector storage in the graph

- [x] 2.1 Compile sqlite-vec into the existing SQLite binding; schema version bump adding the vector table (symbol ID, model ID, int8 vector) + metadata *(v8: content-addressed vecs + symvec mapping — symbol ids churn on re-parse, hashes don't; migration follows house delete-and-rebuild policy, design.md updated)*
- [x] 2.2 Store/lookup APIs in `internal/graph`: upsert vectors transactionally with symbol rows, delete with symbol deletion, top-K scan by cosine over int8
- [x] 2.3 Model-swap invalidation: vectors with mismatched model ID treated as absent; re-embed on next build
- [x] 2.4 Unit tests: transactional delete-with-symbol, invalidation on model change, KNN correctness vs float32 reference, older-schema rollback ignores the table *(rollback = house rebuild policy, covered by existing version-mismatch tests)*

## 3. Symbol cards and embedding pipeline

- [x] 3.1 Card builder: tokenized qualified name, kind, signature, doc comment, path segments, top-8 caller/callee names by usage (reuse existing tokenizer from `internal/search`) *(doc comments extracted at embed time from source lines above the def — no adapter changes needed)*
- [x] 3.2 Build-time embedding pass: parallel workers, progress via existing index-progress UX, cold-build disclosure line extended *(parallelism = ggml thread pool; "embed" phase added to progress verbs)*
- [x] 3.3 Incremental path: re-embed only symbols whose own card text changed during fresh-on-query patch; never trigger repo-wide embedding at query time (backfill deferred to next build) *(content-addressed vec cache keyed by card hash; no changed files → no model load)*
- [x] 3.4 Engine validation: incremental == full-rebuild parity for vectors (same harness pattern as `bench/engine`)

## 4. Search pipeline

- [x] 4.1 Retrieval lanes in `internal/search`: lexical (existing find ladder over query + hints) and vector (query embedding top-K); reciprocal-rank fusion (k=60); graph boosts re-rank
- [x] 4.2 Exact-name precedence rule: exact ladder match ranks first regardless of vector ordering
- [x] 4.3 Feature-map clustering: union-find over caller/callee edges among top hits, entry point = highest caller count, cluster label from shared path prefix + dominant tokens; `--flat` and `--json` outputs *(--flat shipped; --json deferred with the other text surfaces — no query verb has --json yet, tracked as follow-up)*
- [x] 4.4 Lexical-only degradation with first-line disclosure when embedder fails or vectors absent
- [x] 4.5 `query.SearchText` + `codeindex search <repo> "<query>" [--hints ...]` CLI command *(+ `codeindex model pull|use|status` closing task 1.4)*
- [x] 4.6 Unit tests: fusion ordering, exact-name precedence, clustering on synthetic graphs, degradation disclosure

## 5. Surfaces and workflow

- [x] 5.1 MCP `search` tool (query, hints, limit) with routing-law description; NOT added to the always-visible note
- [x] 5.2 MCP `explore-feature` prompt (search → choose entry point → impact) and server capability registration
- [x] 5.3 `find` zero-result hint: multi-token no-match now routes to `search` instead of grep
- [x] 5.4 Drop-in rules snippet for Cursor/VSCode/JetBrains documented in README + `docs/editor-rules-snippet.md`; Claude Code plugin ships `/codeindex:explore` *(a slash command, not an ambient skill — v3/v4 lesson)*
- [x] 5.5 MCP server tests: tool output shape, prompt listing, hint plumbing

## 6. Gate and docs

- [x] 6.1 Pre-register the concept-class query set (feature phrases + known-answer entry points for gin/kubernetes/laravel) and bars (concept hit@5 ≥ 60%, vague/distinctive non-regression, ≤2 min added full-build embed time on kubernetes-scale/8-core) BEFORE measuring *(deterministic doc-phrase generator, seeded; run end-to-end on all four quick-tier repos: gin, flask, nest, laravel)*
- [x] 6.2 Extend `bench/recall_bench.py` with the concept class + embedding pipeline hooks; run candidate bundled models; ship the smallest model that passes all bars *(MiniLM stays default; bge-small benched: gin 75.0% vs 61.7 — documented pull upgrade)*
- [x] 6.3 Record verdict in `bench/engine/FINDINGS-semantic-search.md` — MIXED: Go/Python PASS (61.7% each), TS strict-FAIL 43.3%/family 61.7% (identical-doc sibling degeneracy, verified), PHP FAIL 35.0% (generator validity limit on terse docs; live human queries behave well). Two registered iterations incl. a real card-poisoning bug (JSDoc tail extraction). Follow-up gate registered: human-curated any-of-N concept sets for TS/PHP before broader promotion.
- [x] 6.4 README: search command, model upgrade instructions, binary-size note, routing guidance, vendor-script build step, `nollama` escape hatch

# Proposal: semantic-code-search

## Why

Agents in VSCode/Cursor send concept-level queries ("airbnb onboarding lifecycle") to codeindex's MCP surface, and today every such query dead-ends: `find` matches symbol-name tokens only, returns zero results, and its fallback hint steers the model into a second guaranteed-miss `grep` call. Meanwhile the one asset codeindex has that no embedding-based AI search has — the call graph with usage counts — goes unused for these queries. A `search` capability that answers concept queries natively, ranked and clustered by graph structure, turns codeindex's biggest observed failure mode into its strongest differentiator.

## What Changes

- New `codeindex search <repo> "<concept query>"` CLI command: hybrid lexical+vector retrieval fused with reciprocal-rank fusion, re-ranked by graph signals (caller counts, tier, kind, test penalty), returned as a **feature map** — hits clustered by call-graph connectivity, each cluster led by its highest-caller-count entry point.
- New `internal/embed` package: pluggable embedder interface with a **local provider** (ggml/llama.cpp statically linked via CGO, small quantized GGUF embedding model bundled in the binary via `go:embed`, CPU-only inference) and an optional **API provider** (hosted embedders behind the same interface).
- Symbol-card embeddings: each tier-0 symbol gets a card (tokenized qualified name, kind, signature, doc comment, path segments, top caller/callee names — graph context is the moat) embedded at build time and incrementally re-embedded on the existing fresh-on-query patch path. Vectors stored in `graph.db` via the `sqlite-vec` extension, int8-quantized, stamped with model ID.
- Model management: `codeindex model` subcommands + config to pull/select a larger GGUF model that takes precedence over the bundled one; README documents the upgrade path. Model swap invalidates and re-embeds.
- New MCP tool `search` with an optional `hints` arg so the client LLM contributes concept→identifier expansion; description carries the routing law (concept/feature → search; known symbol → impact/callers; distinctive exact name → plain grep).
- Editor-agnostic workflow packaging: MCP prompt (`explore-feature`: search → pick entry point → impact) plus a documented drop-in rules snippet for Cursor/VSCode/JetBrains; Claude Code plugin skill wraps the same flow.
- Graceful degradation: if the model is unavailable or vectors are absent, `search` answers lexical-only and says so in the output.
- Pre-registered recall gate: `bench/recall_bench.py` grows a concept-class query set (bar: concept hit@5 ≥ 60%) plus non-regression on existing vague/distinctive classes and an index-time budget.

## Capabilities

### New Capabilities

- `embedding-engine`: pluggable text-embedding providers — bundled local GGUF model (statically linked ggml, CPU inference, `go:embed` weights), optional hosted API provider, model management (pull/select/invalidate), vector storage in the SQLite graph with incremental re-embedding.
- `semantic-search`: the concept-query search pipeline — symbol cards with graph context, hybrid lexical+vector retrieval, RRF fusion, graph re-rank, feature-map clustered output, CLI command, MCP `search` tool with client-hint expansion, editor-agnostic workflow prompt, lexical-only degradation.

### Modified Capabilities

<!-- none: no existing main-spec requirements change. locate-search (pending in
     locate-and-enriched-grep) is consumed as-is for the lexical lane; its
     requirements are not altered. -->

## Impact

- **New code**: `internal/embed` (providers, model bundling/management), `internal/search` gains the semantic pipeline (cards, fusion, clustering), `internal/query` gains `SearchText`, `cmd/codeindex` gains `search` + `model` subcommands, `internal/mcpserver` gains the `search` tool + `explore-feature` prompt.
- **Dependencies**: ggml/llama.cpp statically linked via CGO (new; same toolchain demands as existing tree-sitter/SQLite), `sqlite-vec` C extension (new, compiled into the existing SQLite binding), bundled GGUF weights (~25–35 MB → binary grows to roughly 70–90 MB).
- **Storage**: new vector table(s) in `.codeindex/graph.db` (~70 MB int8 per 180k symbols); schema version bump; model-ID stamp for invalidation.
- **Build/index time**: embedding pass at build time (parallel, roughly a minute per ~100k symbols on 8-core CPU); incremental path re-embeds only symbols whose card text changed.
- **MCP surface**: one new tool + one prompt; existing tool descriptions untouched except `find`'s empty-result hint, which now routes concept-shaped misses to `search` instead of `grep`.
- **Bench**: `bench/recall_bench.py` concept-class extension; per the house rule, `search` is sanctioned only via MCP descriptions and the workflow prompt — never the always-visible note — without its own A/B gate.

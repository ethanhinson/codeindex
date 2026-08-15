# Design: semantic-code-search

## Context

codeindex answers structural questions (callers/callees/dependents/impact) from
a symbol graph, plus lexical locate (`find`) and enriched `grep`. The v6 A/B
gate showed find/grep win their domains (+35% / +70% savings) and the recall
bench passed without embeddings ("the embeddings trigger does NOT fire") — for
*name-shaped* queries. But live MCP traffic from VSCode shows a query class we
never designed for: concept/feature phrases ("airbnb onboarding lifecycle").
Today these return zero results and the `find` fallback hint bounces the model
into a second dead `grep` call.

Existing AI code search (Cursor/Copilot embedding indexes) handles concept
queries but returns flat chunk lists with no structure. codeindex has two
assets they lack: the call graph with usage counts, and a client LLM already
sitting in the loop. This change composes both into a `search` capability.

Constraints inherited from the project:
- Single static binary, CGO already required (tree-sitter, SQLite). No runtime
  installs, no daemons, must work air-gapped and on CPU-only machines.
- Fresh-on-query index contract: answers always reflect the working tree.
- Output contract: compact references (path:line + signature), never source.
- Bench culture: pre-registered gates; the always-visible prompt note is a
  blunt instrument (measured 3×) — new tools are sanctioned via MCP
  descriptions/workflow prompts only, unless a dedicated A/B gate passes.

## Goals / Non-Goals

**Goals:**
- Concept-level queries return useful, ranked entry points with graph
  structure (feature map), one MCP call, in every MCP client.
- Fully bundled: embedding inference and default model weights ship inside the
  `codeindex` binary; CPU-only inference.
- Pluggable embedders: bundled local model default, larger local models and
  hosted APIs opt-in.
- Preserve determinism where it exists: lexical lane unchanged; embeddings
  deterministic for a fixed model + binary per platform.
- Pre-registered concept-class recall gate before the tool is promoted.

**Non-Goals:**
- Replacing `find`/`grep` — they remain the cheaper paths for name-shaped and
  content queries; `search`'s description routes accordingly.
- LLM-generated summaries at index time (breaks fast-fresh-index; revisit only
  if the concept-class gate fails).
- GPU acceleration, embedding server mode, cross-repo search.
- Changing the always-visible prompt note (requires its own gate).

## Decisions

### D1: Inference via ggml/llama.cpp statically linked (over ONNX Runtime)
ONNX Runtime effectively requires a shared library install — breaks the
single-binary contract. llama.cpp compiles to static libs with plain CMake +
the C toolchain we already demand, supports BERT-family embedding models in
GGUF, and is CPU-first (AVX/NEON kernels). Binding via CGO like tree-sitter.
Alternative considered: pure-Go inference (nlpodyssey/cybertron) — zero new C
deps but ~5–20× slower on CPU and a much smaller maintained ecosystem;
rejected because embedding 100k+ symbols at build time makes throughput a
product property.

### D2: Default model bundled via go:embed; upgrades opt-in
A small quantized GGUF (~25–35 MB, 384-dim class; candidate set
all-MiniLM-L6-v2 / bge-small-en-v1.5, final pick decided by the recall bench,
not by this design) is compiled into the binary. Truly single-file: works
offline/air-gapped, zero first-run surprise; binary grows to ~70–90 MB.
`codeindex model pull|use|status` manages larger GGUFs in
`~/.cache/codeindex/models/`; per-repo config selects one. Vectors are stamped
with a model ID (name + weights hash); mismatch triggers full re-embed.
Alternative considered: download-on-first-use — smaller binary but breaks
air-gapped installs and adds first-query latency cliffs; rejected per
distribution requirement.

### D3: Vectors in graph.db via sqlite-vec, int8-quantized
sqlite-vec is a single-file C extension compiled into the existing SQLite
binding — same pattern, no second storage engine, transactional with the
symbol tables it references (embeddings die with their symbol rows). int8
quantization: ~70 MB per 180k symbols, recall loss negligible at 384 dims.
Brute-force KNN is O(n) but sqlite-vec's scan handles ~200k×384 int8 in tens
of ms — same budget class as `find`'s in-memory scan (D1 of locate change).
Alternative considered: dedicated vector store (separate file, HNSW) — faster
at millions of symbols but adds a consistency problem with the fresh-on-query
patch path; rejected as premature at our scale.

### D4: Embed symbol cards with graph context (the moat)
Card text per tier-0 symbol: tokenized qualified name, kind, signature, doc
comment, file-path segments, and the names of up to N (default 8) top callers
and callees by usage. Neighbor names let a concept query match a symbol whose
*neighborhood* is about the concept. Re-embed policy: only when the symbol's
own card text changes; neighbor-name churn is tolerated drift (a full build
refreshes all cards). This bounds incremental cost to changed files, matching
the existing patch path. Alternative considered: chunk-level embedding of file
contents — what competitors do; loses the symbol-granular output contract and
costs far more index time; rejected.

### D5: Hybrid retrieval — RRF fusion of lexical and vector lanes, then graph re-rank
Query pipeline: (1) lexical lane = existing `find` ladder over the query plus
optional client-supplied `hints` tokens; (2) vector lane = query embedding vs
symbol-card vectors, top-K; (3) reciprocal-rank fusion (k=60) of both lanes;
(4) multiply existing graph boosts (log callers, tier, kind, test penalty).
The `hints` arg is how the client LLM's concept→identifier expansion folds in
— no second inference dependency, and it degrades to nothing when clients
omit it. Alternative considered: vector-only — simpler, but loses exact-name
precision and the deterministic floor; rejected.

### D6: Feature-map output — cluster by call-graph connectivity
Top fused hits are clustered by union-find over caller/callee edges among the
result set; each cluster is led by its highest-caller-count member as the
entry point, members listed as path:line + signature, cluster labeled by
shared path prefix + dominant name tokens. This is the structural answer no
embedding index can produce and the reason `search` beats "existing AI
search" rather than imitating it. Flat ranked list remains available via
`--flat`/JSON.

### D7: Graceful degradation, disclosed
If the embedder fails (corrupt model, OOM) or vectors are absent for the
current model ID, `search` answers from the lexical lane only and the first
line of output says so. Embedding backfill runs on the next `build`; `search`
never blocks on a full embedding pass at query time beyond the standard
fresh-on-query patch of changed files.

### D8: Workflow packaging is server-side (editor-agnostic)
The routing law and the explore-feature flow (search → pick entry point →
impact) ship where every client can see them: the MCP tool description and an
MCP prompt. A documented drop-in snippet for `.cursor/rules`/`AGENTS.md`
covers clients that under-surface MCP prompts. The Claude Code plugin skill
is a thin wrapper over the same flow. `find`'s zero-result hint changes to
route concept-shaped misses (multi-token, no symbol-name match) to `search`.
Per the measured meta-lesson, the always-visible note is NOT touched.

### D9: Pre-registered gate before promotion
`bench/recall_bench.py` grows a concept-class query set: feature-level phrases
with pre-registered known-answer entry points across the three bench repos
(gin, kubernetes, laravel). Bars: concept hit@5 ≥ 60%; existing vague-class
and distinctive-class results non-regressed; full-build embedding overhead
within budget (≤ 2 min added on kubernetes-scale, 8-core CPU). Model choice
(D2) is selected by this bench across the candidate set.

## Risks / Trade-offs

- [Binary grows ~40–55 MB] → acceptable per explicit distribution decision;
  keep the bundled model the smallest that passes the concept-class bar.
- [llama.cpp build complexity across platforms (macOS/Linux/arch matrix)] →
  vendor a pinned llama.cpp revision; CI builds the full matrix; CGO flags
  isolated in `internal/embed/local`.
- [Embedding pass slows cold build] → parallel workers saturate cores; embed
  pass is incremental thereafter; progress reported via the existing
  index-progress UX; disclosed once via the ConsumeColdBuild-style note.
- [Neighbor-drift makes stale cards (D4)] → bounded by full-build refresh;
  measure drift impact in the recall bench before tightening the policy.
- [Concept recall of a 25–35 MB model may miss the 60% bar] → candidate-set
  bench (D9) decides; escalation path is a larger pulled model (D2) and, only
  after a failed gate, LLM-summary cards (explicit non-goal for now).
- [Search over-adoption for name-shaped queries (v1/v3/v6 lesson)] → routing
  law in the tool description; `search` absent from the always-visible note;
  monitor via the existing A/B harness classes.
- [sqlite-vec extension adds schema surface] → schema version bump with
  auto-migration (drop + rebuild vector table; symbols unaffected).
- [Batched inference wobbles ~2e-4 cosine per batch composition] → below the
  int8 storage quantization step (~8e-3) and vectors are computed once per
  content hash, so index state is stable; "deterministic" is per-hash
  compute-once, not bit-identical across batch groupings.
- [Measured embed throughput ~10 ms/card (M-series, 8 threads, batched)] →
  ~2.5k-symbol repo adds ~25 s to a cold build; kubernetes-scale extrapolates
  past the 2-min budget — D9's bench measures it and parallel contexts are
  the known next lever if the gate demands it.

## Migration Plan

1. Schema bump v7→v8 adds symvec/vecs/vecmeta. Per house policy the index is
   a derived artifact: any version mismatch (upgrade OR rollback) deletes the
   db and rebuilds on the next query — no migration code, embeddings backfill
   on the next explicit `build`.
2. Until that build runs, `search` answers lexical-only and disclose it (D7).

## Open Questions

- Final bundled model pick (all-MiniLM-L6-v2 vs bge-small-en-v1.5 vs
  nomic-embed-text quantized) — decided by D9 bench, not by opinion.
- MCP prompt support across JetBrains/Cursor versions is uneven — the rules
  snippet is the fallback; verify per-client during rollout.

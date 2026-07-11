## Why

Locate is the one domain with a RED result (v1: +17% — grep won on
single-probe, distinctive-name tasks). But real locate work is messier than
v1's tasks, and each unit of mess is a full agent turn: probe iteration under
partial knowledge ("parseConfig? LoadConfig?"), common-name noise (hundreds of
unranked hits), and result attribution (every grep hit costs a Read to learn
"what function is this in?"). We attack only those three modes — grep keeps
its home turf, enforced by routing and a pre-registered gate.

## What Changes

- **`codeindex find <query>`** — ranked symbol search: convention-aware
  tokenization of symbol names (camelCase/snake_case/acronym runs — one shared
  splitter covers all four languages), deterministic synonym/stem expansion
  (~50 static groups: get/fetch/load, init/setup/create, …), subsequence-fuzzy
  matching, and ranking by signals only the graph has: match quality × caller
  count × tier (project > dep) × kind × prod-over-test. Filters: `--kind`,
  path substring. Deterministic output; no vector store (deferred behind a
  falsifiable trigger — see gate).
- **`codeindex grep <pattern>`** — grep, enriched: ripgrep underneath
  (internal Go-regexp fallback), every hit attributed to its enclosing symbol
  via the index (SQL, no parsing), definition-line hits marked, hits deduped
  by symbol with counts, ranked defs-first/prod-first, bounded reference
  output. Same search power; structured, cheap-to-consume results.
- **Routing update** (skill note + MCP): distinctive full name → plain grep;
  partial/vague/common name → `find`; understanding occurrences → `codeindex
  grep`. Two new MCP tools with routing baked into descriptions.
- **Two-level validation, pre-registered**:
  1. **Offline recall benchmark** (deterministic, free): generated vague
     queries from real symbols (token drop/reorder/synonym-swap/case) on
     kubernetes + laravel; bar: **hit@5 ≥ 70%** on the vague class. If tokens
     + synonyms miss the bar, THAT is the trigger to prototype an optional
     embeddings tier — not before.
  2. **Agent A/B v6** (~$8–10): locate task classes — distinctive-name
     (must NOT regress >10% — the v1 trap), vague-partial-name (≥30% savings),
     attribute-the-occurrences (≥30% savings), success parity.

Non-goals: vectors/embeddings in the default path; semantic concept→name
mapping; replacing plain grep for distinctive names; fuzzy file-content
search.

## Capabilities

### New Capabilities

- `locate-search`: the token/synonym/fuzzy matcher with graph-signal ranking,
  the enriched grep with symbol attribution, routing across skill/MCP, the
  offline recall benchmark, and the v6 gate.

### Modified Capabilities

None at requirement level (the v4-gated plugin note text changes routing
guidance; its gate thresholds are re-verified by v6's no-regression class).

## Impact

- New `internal/search` package; CLI `find` + `grep`; two MCP tools; plugin
  note/README routing text; no schema change (matching is an in-memory scan
  per query — 116k symbols score in tens of ms).
- `bench/recall_bench.py` (offline) and a v6 task set in `bench/agent_ab`.
- Spends ~$8–10 on the v6 gate.

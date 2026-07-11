## Context

v1 (RED) proved grep wins single-probe distinctive-name locate; v2–v5 proved
turns are the cost driver. Locate's real costs are probe iteration, noise
consumption, and hit attribution — all turn multipliers. We have unique
ranking signals (caller counts, tiers, kinds) and symbol spans for O(1)
attribution. The skill/note currently routes ALL locate to grep; this change
narrows that to distinctive-name only.

## Goals / Non-Goals

**Goals**: one-call ranked symbol search under partial knowledge; grep output
that arrives pre-attributed and deduped; routing that cannot reproduce the v1
regression (gated); vectors kept out unless the recall bar fails.

**Non-Goals**: semantic search, embeddings in the default path, replacing
grep for distinctive names, content (non-symbol) fuzzy search.

## Decisions

**D1 — In-memory scoring per query, no schema change.** `find` loads
(id, name, parent, ns, tier, kind, file, line, signature) plus a caller-count
map (one GROUP BY over edges) and scores in Go. kubernetes = 116k symbols →
tens of ms; well inside budget. *Alternative:* a persisted tokens table —
faster retrieval but poisons attach (INSERT..SELECT can't tokenize) and adds
schema churn; revisit only if profiling demands.

**D2 — One shared convention tokenizer.** Split on case humps, digits,
`_`/`-`/`.`, with acronym-run handling (`HTTPServer` → http, server). Covers
all four languages; no per-language grammar needed. Applied to symbol names at
score time and to the query.

**D3 — Deterministic match ladder × graph boosts.** Match quality: exact name
(100) > exact token set (90) > name prefix (80) > all query tokens present
(70) > subsequence of name (50); synonym-expanded token hits count at 0.8
weight. Score = quality × (1 + log10(1+callers)) × tier factor (project 1.0,
dep 0.6) × kind factor (type/func 1.0, method 0.9) × test-file penalty (0.7).
Ties broken by name, file, line. Fully deterministic and explainable — every
result can say why it ranked.

**D4 — Synonyms/stems are a static table in code (~50 groups).** get/fetch/
load/retrieve/read, init/setup/create/new/make, delete/remove/destroy/drop,
config/settings/options/conf, check/validate/verify/assert, send/emit/publish/
dispatch, handle/process/on, find/search/lookup/locate, … plus light stemming
(trailing s/ing/er/ed folds). Deterministic and inspectable; not a model.

**D5 — Enriched grep: ripgrep underneath, index on top.** Resolve `rg` from
PATH (agent environments ship it); fallback to an internal Go-regexp scan over
the indexed file set (correct, slower — documented). Hits are attributed by
loading each hit file's symbol spans once (SQL) and binary-searching lines;
`hit line == symbol start_line` marks a definition. Output groups by symbol
with hit counts, defs first, prod before test, bounded by `--limit`, and
reports `N raw hits → M symbols` so compression is visible. Comment/string
classification (needs parsing) is explicitly deferred.

**D6 — Routing.** Note/skill text: distinctive full name → grep; partial or
convention-uncertain name → `find`; "where is X used and in what functions" →
`codeindex grep`. MCP adds `find` and `grep` tools whose descriptions carry
the same routing so IDE agents inherit it. The v4 trust instruction applies
unchanged.

**D7 — Two-level validation, pre-registered before runs.**
- *Offline recall* (free, deterministic, seeded): sample symbols from
  kubernetes + laravel; generate vague queries per symbol (lowercased name,
  space-joined tokens, one-token drop (≥2 tokens), one synonym swap, token
  reorder); bar: **hit@5 ≥ 70%** on the vague classes. Failing THIS bar — and
  only this — triggers an optional-embeddings prototype as a separate change.
- *Agent A/B v6*: classes distinctive-name (gate: ≤10% regression — the v1
  trap re-armed), vague-partial (≥30% savings), attribute-occurrences (≥30%
  savings); success delta ≥ −5pp; plugin arm; ~$8–10.

## Risks / Trade-offs

- **`find` latency on huge indexes** → measured path is a single table scan +
  Go scoring; if >100 ms at k8s scale, cache the symbol slice per process
  (CLI is per-invocation; MCP server can memoize between calls with a dirty
  flag from Fresh).
- **Synonym table bias** → it's inspectable and versioned; misses are visible
  in the recall benchmark rather than hidden in a model.
- **rg absent** → internal fallback keeps correctness; speed difference
  documented in output.
- **Routing mis-fires** (agents overusing find on distinctive names) → exactly
  what the v6 distinctive class measures; wording iterates once (registered)
  if it regresses.

## Migration Plan

Additive: new commands/tools; no schema change; note text update. Rollback =
revert binary + note.

## Open Questions

- Whether MCP `find` should return structured JSON content instead of text —
  deferred with the broader `--json` output task (core 8.5).

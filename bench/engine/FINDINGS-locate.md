# Locate: find + enriched grep — findings (phase 1)

**Date:** 2026-07-10 · **Change:** `locate-and-enriched-grep`

## Shipped

- `codeindex find`: convention tokenizer (camel/snake/acronym/digit
  boundaries, one splitter for all four languages), ~50-group static
  synonym/stem table, deterministic match ladder (exact > token-set > prefix >
  all-tokens > subsequence; synonyms at 0.8) × graph boosts (log callers, tier,
  kind, test penalty). In-memory scan: kubernetes 182k symbols score inside a
  ~1.6 s end-to-end CLI call (incl. fresh-on-query patch + process start).
- `codeindex grep`: ripgrep (or internal fallback) + per-hit enclosing-symbol
  attribution (span binary search), def-line marking, dedup-by-symbol with
  counts, defs-first ranking, `N raw hits → M symbols` compression line.
- Routing shipped in the prompt note + two MCP tools (find/grep) whose
  descriptions carry it: distinctive full name → plain grep; partial/vague →
  find; occurrence understanding → codeindex grep.

## Offline recall (pre-registered bar: vague-class hit@5 ≥ 70%)

| Repo | casefold | token-join | reorder | synonym | token-drop | VAGUE hit@5 | Verdict |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gin | 100% | 100% | 100% | 90.9% | 90.0% | **94.4%** | PASS |
| kubernetes | 100% | 97.5% | 97.5% | 85.7% | 67.5% | **83.2%** | PASS |
| laravel | 100% | 100% | 100% | 100% | 57.5% | **81.5%** | PASS |

**The embeddings trigger does NOT fire.** Weakest class is token-drop (a
2-token name losing one token leaves a generic survivor among thousands);
caller-count ranking recovers most. Deterministic, seeded, reproducible
(`bench/recall_bench.py`, results in `bench/engine/recall-*.json`).

## Live spot-checks

- k8s `find "config load"` (reversed, vague) → `LoadConfig` first, then
  usage-ranked candidates (`ToRawKubeConfigLoader`, callers=69).
- k8s `find "fetch pod"` (synonym) → `getPod` (callers=73) first.
- gin `grep ServeHTTP` → 17 raw hits → 6 attributed symbols, definition first.

## Pending

Agent A/B v6 gate (distinctive ≤10% regression / vague ≥30% / occurrences
≥30%) — phase 2 of this change.

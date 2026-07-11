## 1. Matcher core (`internal/search`)

- [x] 1.1 Tokenizer: case humps, digits, `_`/`-`/`.` separators, acronym runs; unit tests across all four languages' conventions
- [x] 1.2 Synonym/stem table (~50 groups + s/ing/er/ed folds); expansion at 0.8 weight
- [x] 1.3 Scorer: match ladder (exact 100 > token-set 90 > prefix 80 > all-tokens 70 > subsequence 50) × caller-count/tier/kind/test boosts; deterministic ties; unit tests
- [x] 1.4 `Find(store, query, opts)`: single symbol scan + caller-count GROUP BY; kind/path filters; measure latency on kubernetes (target < 100 ms scan+score)

## 2. Enriched grep

- [x] 2.1 Search backend: `rg -n --no-heading` when on PATH; internal Go-regexp scan over indexed files as fallback (noted in output)
- [x] 2.2 Attribution: per-hit-file symbol spans (one SQL per file), line binary search; def-line marking; dedup by symbol with counts; defs-first/prod-first ranking; `N raw hits → M symbols` line
- [x] 2.3 Tests: dedup/def-marking on fixtures; fallback path

## 3. Surfaces

- [x] 3.1 CLI `find` + `grep` (both fresh-on-query, `--limit`, find `--kind`)
- [x] 3.2 MCP tools `find` + `grep` with routing descriptions; MCP test updated
- [x] 3.3 Plugin note routing (three-way; ≤250 tokens total) + plugin README

## 4. Offline recall benchmark

- [x] 4.1 `bench/recall_bench.py`: seeded symbol sampling (kubernetes, laravel), query generators (case fold, token join, one-token drop, synonym swap, reorder), hit@1/hit@5 via the CLI; pre-registered bar hit@5 ≥ 70% (vague classes)
- [x] 4.2 Run + record in `bench/engine/FINDINGS-locate.md`; if the bar fails, record the embeddings-tier trigger decision

## 5. Agent A/B v6

- [x] 5.1 Task classes in `tasks_v6.json` (pre-registered header): distinctive-name (≤10% regression gate), vague-partial (≥30%), attribute-occurrences (≥30%); ground truth arm-neutral (symbol locations / rg-derived)
- [x] 5.2 Smoke, then full run (plugin arm, reps 2, budget $15); grade/report/dashboard v6; verdict + consequence recorded

## 6. Close-out

- [x] 6.1 All tests green; gofmt; rebuild pinned binary; `openspec validate`
- [x] 6.2 core-indexing-engine 8.4 (search portion) cross-referenced; findings + READMEs

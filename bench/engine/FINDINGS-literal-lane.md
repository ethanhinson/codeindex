# Literal lane — gate findings

**Date:** 2026-07-12 · **Change:** `literal-lane-retrieval`

## Verdict: WITHHELD from defaults (pre-registered bars not met as a conjunction); explicit error_text path ships

Bars (registered in the residuals backlog bucket 4 + design D6 before
measurement): issues-v2 ≥ grep-control per repo (gin ≥20.5%, flask ≥43.3%),
zero curated regression (88.5/76.0/65.4/76.9), find classes, latency.
Iteration budget: 2, both spent.

| config | issues gin | issues flask | cur gin | cur flask | cur nest | cur laravel |
| --- | --- | --- | --- | --- | --- | --- |
| bars | ≥20.5 | ≥43.3 | ≥88.5 | ≥76.0 | ≥65.4 | ≥76.9 |
| lane v0 | 20.5 | 56.7 | **80.8 ✗** | 88.0 | 65.4 | 76.9 |
| iter 1 (skip test files) | 20.5 | **66.7** | **80.8 ✗** | 88.0 | 65.4 | 76.9 |
| iter 2 (+word hit-cap 100) | 20.5 | 63.3 | 88.5 | 88.0 | 65.4 | **73.1 ✗** |
| explicit error_text mode | 20.5 | 63.3 | (lane off: frozen) | | | |

Every config fails somewhere: v0/iter1 leak framework-common words into gin
concept rankings (test symbols first — iter 1's fix — then plain
domain-common words like "response"/"templates"); iter 2's fixed 100-hit
word cap fixes gin but is repo-size-dependent — on 10×-bigger laravel it
gates out words that were legitimately helping (−3.8). **Root cause of the
failure: absolute thresholds where the corpus demands relative ones** —
the same lesson as contrast's family-df RATIO, not respected here.

## What ships vs what's withheld (v6 precedent)

- **WITHHELD**: the always-on literal lane. Off by default; experimental
  flag `CODEINDEX_LITERAL_LANE=1` for gate development only. Withheld
  default verified bit-identical to the frozen pipeline on all four
  curated sets.
- **SHIPPED**: the `error_text` explicit path (MCP arg + `--error-text`) —
  lane and verbatim-phrase pin activate only on caller-supplied symptom
  evidence. Measured: never worse than grep-control (gin ties 20.5,
  flask 63.3 vs 43.3 ✓) and inert when absent. This mirrors the
  ambient-note meta-lesson exactly: automatic sanctioning over-applies;
  explicit invocation is safe.
- Where the lane genuinely shines (both iterations): flask issues 33.3 →
  66.7 peak (2× baseline, +23 over control) and flask curated +12 (76 →
  88) — literal evidence is decisively right for some corpora; the failure
  is the activation policy, not the evidence.

## Registered next gate (before any re-promotion)

One change, one gate: replace `maxWordHits=100` with a repo-relative
threshold (hits per indexed KLOC or per file count; constant frozen in that
change's design first), re-run this exact table one-shot. Prediction to
register there: iter-2's gin gains hold AND laravel returns to ≥76.9 AND
gin issues > 20.5 (strict dominance, ending the tie).

## Notes

- gin issues plateaued at 20.5% (= control) in every config: the lane
  converts grep's wins into search's ranking but adds nothing beyond them
  on gin — consistent with the miss anatomy (gin's misses are dominated by
  multi-hop, bucket C, which no lexical mechanism addresses).
- Rider shipped: `issues_corpus.py` reads GITHUB_TOKEN from env / repo
  `.env` (authenticated budget 2000) for future corpus expansion; fixtures
  remain frozen.

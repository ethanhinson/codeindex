# End-to-end efficacy — real GitHub issues, with vs without codeindex

**Date:** 2026-07-09
**Harness:** `bench/efficacy_batch.py` (WITH = the real `codeindex query`; WITHOUT
= grep+read), pooled by `bench/aggregate_efficacy.py`
**Sample:** **109 code symbols** referenced by **real GitHub issues** across three
Go repos — gin (small), prometheus (medium), kubernetes (large, ~3M LOC)
**Token counter:** Anthropic `count_tokens` (exact Claude) for gin + prometheus;
tiktoken cl100k for kubernetes (fast on huge files). Ratios are computed
within-symbol (same tokenizer both sides) and we separately verified Claude vs
tiktoken ratios match, so pooling ratios is valid.

## What it measures

To work an issue an agent must first *understand* the code it touches — where the
symbols it names are defined and who calls them. This measures the tokens for that
comprehension step, WITH the index versus WITHOUT it.

- **WITH (codeindex):** `codeindex query <symbol>` → compact definitions + resolved
  callers as `path:line` references.
- **WITHOUT baselines:**
  - `grep -n floor` — tokens of `rg -n -w <symbol>` output. The **cheapest possible
    way to even locate** the symbol without an index; unresolved and noisy (every
    textual mention, no signatures, no call/def distinction).
  - `smart file-read` — the top-8 files by match count, read whole.
  - `naive file-read` — all matching files (capped), read whole.

## Why this is hard to dispute

1. **No cherry-picking.** A transparent, scripted rule selects the symbols each
   issue references (CamelCase / backtick identifiers, minus a small stoplist),
   filtered to those the index actually defines. **Every** selected symbol is in
   the sample, including unfavorable ones. Raw per-symbol data is in
   `bench/results/efficacy-*.json`.
2. **A skeptic-proof baseline.** We compare not only against reading files but
   against the raw `grep -n` output — the cheapest locate there is. The index
   answer is *smaller than grep's own output in 97% of symbols*, while being
   resolved and structured.
3. **The whole distribution, worst case included.** We report min/median/max, and
   lead with the **minimum**.

## Results — pooled (109 symbols, 3 repos)

| Comparison | median | min (worst case) | max | index beats it |
| ---------- | ------ | ---------------- | --- | -------------- |
| vs `grep -n` floor | **43.6×** | 0.8× | 14,106× | 97.2% of symbols |
| vs smart file-read (top-8) | **362.6×** | **6.0×** | 168,018× | 100% |
| vs naive file-read (all) | 659× | 12.6× | — | 100% |

Median index answer: **449 tokens**.

**The headline: across 109 real symbols, the index never costs more than ~1/6 the
tokens of reading files to answer the same question, is usually 100–400× cheaper,
and even undercuts raw grep output 97% of the time.**

### Per repo

| Repo | Size | Symbols | vs grep floor (median / min) | vs file-read (median / min) |
| ---- | ---- | ------- | ---------------------------- | --------------------------- |
| gin | small | 18 | 3.8× / 1.0× | 282× / 12.6× |
| prometheus | medium | 10 | 4.7× / 0.8× | 149× / 7.6× |
| kubernetes | large | 81 | 88.9× / 0.9× | 404× / 6.0× |

The win grows with repo/file size (consistent with the symbol-level spike): on
kubernetes, grep for a common symbol returns thousands of unresolved lines, while
the index returns a bounded, resolved answer.

### Case study — prometheus #11505

"remote write: check that labels are sorted lexicographically," which explicitly
links `Labels` and `func (ls Labels) HasDuplicateLabelNames()`:

- `HasDuplicateLabelNames`: **382 tokens** via the index vs **87,402** to read the
  8 files that mention it → **229×**, and the index *directly* lists the 3
  definitions and 11 callers.
- `Labels` (common type): 5,507 vs 308,222 → 56×.

## Honesty / caveats

- **Comprehension phase only.** Editing and reasoning tokens are unaffected; this
  is the navigation step the plugin changes.
- **Name-based resolution is conservative here** — it inflates the WITH answer for
  common names (e.g. `Labels` → 25 same-named defs). Precise resolution
  (`core-indexing-engine` change 2) shrinks the index answer, so these numbers are
  a floor, not a ceiling.
- **The `grep -n` floor min of 0.8×** means a few ultra-rare symbols have a grep
  output smaller than the index answer; even then the index answer is resolved and
  structured. This is reported, not hidden.
- **Pinned index vs `main` issues:** only symbols present in the pinned index are
  tested (the `has_def` filter), i.e. stable, long-lived symbols. Recent-only
  prometheus issues yielded few matches for this reason; the good-first issues and
  kubernetes's long-lived APIs supplied the volume.
- **Selection heuristic is imperfect** (prose false-positives possible); every kept
  symbol is reported so the sample is auditable.

## Reproduce

```
codeindex build <repo-clone>
python3 bench/efficacy_batch.py --binary <codeindex> --repo <repo-clone> \
    --slug owner/repo --recent 80 --out bench/results/efficacy-<repo>.json
python3 bench/aggregate_efficacy.py     # pools all results/efficacy-*.json
```

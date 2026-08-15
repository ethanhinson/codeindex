# Corpus expansion — findings

**Date:** 2026-07-12 · **Change:** `selfheal-validation-harness` follow-on
(PAT-enabled). Two parallel subagents (miner, author); baselines run by the
main session. Frozen v1/v2 fixtures and all historical results untouched —
expansion is additive; old gate tables remain comparable.

## What now exists

- **Curated x2** (`bench/concept_sets/x2/`): 371 questions across 7 repos
  (~50–56 each; supersets of the frozen originals), authored from
  documentation knowledge, symbol-table-verified, zero retrieval runs
  before freeze.
- **Issues x** (`bench/selfheal/issues_x/`): 345 mined real-issue questions
  across 7 repos (210 tuning scored, 135 held-out BANKED UNSCORED), with
  the miss-analysis hygiene fixes in (comment-only hunks skipped, refactor
  titles filtered, merge-diff handling, per-language xfuncname incl. a
  custom TS driver). 469/2000 lifetime API requests used.

## New baselines (shipped binary, withhold-default config)

| repo | curated x2 (n) | old 26-q | issues x (n) | old v2 |
| --- | --- | --- | --- | --- |
| gin | **79.2%** (53) | 88.5 | **19.6%** (51) | 10.3 |
| flask | **84.0%** (50) | 76.0 | **23.1%** (39) | 33.3 |
| nest | **58.5%** (53) | 65.4 | **35.0%** (60) | — |
| laravel | **83.0%** (53) | 76.9 | **20.0%** (60) | — |

Held-out (prometheus/symfony/vscode): curated x2 51/56/55 questions and
issues 45/45/45 questions authored and banked; NOT scored — reserved for
one-shot gates.

**The point of the exercise, demonstrated:** every number moved 7–13 points
in one direction or the other purely from doubling n — the old 26-question
verdicts (including the literal-lane gate's one-question flips) were
operating at coin-flip granularity. These tables are the new bars for all
FUTURE gates; historical verdicts stand as recorded against their own
frozen fixtures.

The four-language picture at trustworthy n: concept/feature retrieval
~59–84% (nest still the laggard — entry-point residual), fault-finding
~20–35% everywhere (bucket-4/multi-hop territory, as diagnosed).

## Indexer gaps surfaced by authoring (filed, not fixed here)

1. **TS adapter misses abstract classes** (nest `ModuleRef`,
   `AbstractInstanceResolver` absent; their methods indexed with empty
   parent) and **TS enums** (`Scope`). Real adapter bug — affects accepts,
   cards, and graph edges.
2. **Module-level data is not indexed** (flask blinker signals, gin struct
   fields, nest constants) — some legitimate concept questions are
   unanswerable by construction; a "values/fields" symbol tier is a
   product question, not a bug.
3. **Locate-filter gap in the miner** (measured per repo, heaviest PHP):
   titles naming a mapped method's parent class slip through; parent-aware
   check required before the issues held-out gate.
4. Test-file symbols are tier-0 and inflate bare-name matches at 50k+
   symbol scale; parent-qualified accepts are the reliable fixture form
   (authoring convention adopted).

## Standing next gates (in recommended order)

1. Literal-lane re-promotion, if attempted again, now gates against
   curated-x2 + issues-x bars (single-question flips can no longer decide).
2. Runtime-evidence WordPress/Drupal gate (multi-hop bucket) — unchanged.
3. TS adapter abstract-class/enum fix — small, measurable via nest x2.

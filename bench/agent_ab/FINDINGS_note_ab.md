# agent-ab note A/B — nav sentence in the plugin prompt note

**Date:** 2026-08-16 · **Verdict: FLIP (shipped)** · **Cost:** $8.00 total
(incl. one wrong-repo restart), 80 valid paired runs, claude-sonnet-4-6.

## Question

The plugin's UserPromptSubmit note is a measured artifact (v2: always-visible
note → 100% adoption, −73% cost; v3: richer apparatus → net-negative), so
`codeindex nav` could not be added to it on faith — this was the last
A/B-gated follow-up in bench/scout/NEXT_STEPS.md item 6. Does one nav
sentence help, hurt, or wash?

## Setup

- Both arms run the REAL packaged plugin via `--plugin-dir`; the ONLY delta
  is one sentence in the note ("To orient on a symbol you know — where
  defined, who calls it, which files reference it — run `codeindex nav
  <repo-root> <Symbol>`: one call returns all three."), inserted so the
  existing trust instruction covers nav. Variant materialized from the live
  plugin source at run time (`note_ab.py`), verified byte-identical to what
  shipped.
- tasks_v6.json: 20 tasks (gin + prometheus, comprehension/occurrences/
  vague_find), 2 arms × 2 reps = 80 runs, arm order alternated per task.
  Graded by the harness's own arm-blind grader.
- Injection verified three ways: plugin path in the stream-json init event,
  the variant hook rendered against the work repo, and differential behavior
  (7 nav calls in the nav arm, 0 in cur).
- Pre-registered gate: flip only if success Δ ≥ −5pp AND median paired cost
  Δ ≤ +10% AND nav actually called.

## Result (40/40 pairs)

| arm | success | med F1 | med cost | med turns |
| --- | --- | --- | --- | --- |
| cur | 95% | 1.00 | $0.034 | 2 |
| nav | 95% | 1.00 | $0.035 | 2 |

- Success delta **+0.0pp** (the two misses are the same vague_find tasks in
  both arms). Median paired cost **+0.4%** — the ~30 extra note tokens are
  noise at task scale.
- nav adoption is **selective and correct**: 7 calls, all in comprehension
  (orientation-shaped) tasks, where the nav arm's median cost is ~5% lower;
  occurrences/vague_find agents keep using callers/grep per the note's
  existing anchors.
- Honest scope: both arms are at the success ceiling on this task set, so
  the gate demonstrates *no harm + real, well-targeted adoption*, not an
  outcome improvement. The flip's value is alignment: the note now
  advertises the verb the product actually ships for orientation.

## Process note (recorded because it cost a restart)

The first full run hardcoded the gin work clone for all tasks; tasks_v6 is
HALF prometheus, so 40 rows ran against the wrong repo (graded 0.0,
symmetrically). Caught by reading the tail of the run log, not by the
harness. Fixed to per-task repo resolution; bad rows pruned to
`note_ab_runs_wrongrepo.jsonl.bak`; prometheus re-run clean. Lesson: a
paired harness happily measures symmetric garbage — sanity-check per-row
task/repo pairing before trusting aggregates.

## Files

- `note_ab.py` — runner (materializes the variant plugin, paired matrix,
  crash-safe resume, budget guard).
- `note_ab_report.py` — paired report + the pre-registered gate.
- `results/note_ab_runs.jsonl` + `results/transcripts_noteab/` — raw rows.

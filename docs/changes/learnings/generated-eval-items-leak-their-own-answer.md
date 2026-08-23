---
slug: generated-eval-items-leak-their-own-answer
hook: "A generated benchmark task is only testing its shape if the shape's own work is required — check every prompt against the trivial baseline before freezing."
topics: [benchmarking, measurement, coverage, spec-fidelity]
changes: [10]
created: 2026-08-23
updated: 2026-08-23
promotion_state: candidate
promoted_to:
---

## Apply

When a miner emits evaluation tasks from real source, the emission code and the prompt template are
written at different moments, and nothing in a green suite connects them. The failure mode is that
the **prompt hands over the intermediate the task exists to make the system derive**, so the task
silently collapses into an easier, already-covered shape — and it still grades, still looks
plausible, and still counts toward the registered floor.

Two checks, both cheap, both mechanical, both belonging *in the selftest* rather than in a review
pass:

1. **Per-shape adversarial baseline.** For each task shape, ask what the *weakest* strategy that
   could answer it is — a single grep, a one-hop lookup, the shape one rung below. If that strategy
   answers the prompt as written, the task is mislabelled, not merely easy. A two-hop task whose
   prompt names the hop-1 *and* hop-2 symbols is a one-hop task with extra words.
2. **A per-task shape invariant asserted over the whole frozen set**, not sampled. The invariant is
   a property of the emitted pair (prompt, GT) — "the GT symbol does not appear in the prompt", "the
   GT satisfies this shape's semantics", "no task exceeds the registered GT-size cap." Assert it for
   every task; a cap or rule enforced only at the *candidate* level is not enforced at all, because
   the later assembly steps can reintroduce a violation.

**Corollary — keying choice changes the population, so re-derive the counts.** Re-keying the hop-2
task onto its hop-1 symbol both well-posed the shape and doubled its yield (22 → 46). A shape that
grows to ~40% of a scored subset makes per-shape reporting load-bearing rather than
belt-and-braces: the aggregate mean is now mostly one shape's mean.

## Why it bites

A corpus is **frozen and pre-registered specifically so the verdict cannot be argued afterward** —
which is exactly what makes a contaminated task unrecoverable later. Both defects here would have
inverted or trivialised the primary bar, and the bar had already been registered. There is no
post-hoc fix that isn't also a re-registration, so the only place these checks pay is *before the
first scored run*.

The generative case also under-counts itself. Task ids commonly embed the emission ordinal, so any
re-mine renumbers them: correcting the corpus stranded 39 of 65 prior ids and made every earlier
campaign transcript ungradeable. Budget for that — a late GT fix costs the baseline, not just the
fix.

## Provenance

- **#0010, PR #16** — growing the workspace bench corpus from 65 to 207 tasks. Whole-branch review
  (deep rung) returned 9 findings, **2 of them blockers, both of this class**: the `xchain` (two-hop)
  prompt named the hop-2 symbol, making it verbatim an `xcallers` (one-hop) task (`9803a11`); and
  registered floors were hardcoded at four sites and asserted nowhere (`784f177`). A third finding
  added the missing "GT satisfies its shape's semantics" assertion (`2206c80`). The response was to
  grow `--selftest` from 3 checks to 32 — including a per-task shape-invariant sweep over all 207
  tasks, a binding registered-floor check, and 5 KNOWN LIMITATION assertions — with every new guard
  mutation-tested. Separately, the *pre-existing* 65-task frozen set was found to already violate its
  own registered 40-file GT cap (`ws-ximpact-Reference-001`, 41 files), because the cap had only ever
  been applied to candidates (ADR-0015).

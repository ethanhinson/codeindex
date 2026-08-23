---
id: 15
slug: corpus-caps-evaluated-on-emitted-ground-truth
title: Corpus size caps and shape guards are evaluated on emitted ground truth, not on a candidate-level proxy
status: Accepted
date: 2026-08-22
supersedes: []
reverses: []
relates_to: []
change: 10
---

## Context

The workspace bench miner (`bench/workspace/build_tasks_ws.py`) mines cross-repo tasks from OSS
member checkouts. A registered corpus rule says "GT size is capped at 40 files (larger answers
measure listing stamina, not finding)". The miner has a primary loop that picks candidate symbols
and several sub-kind loops that emit derived shapes (`xsubtypes`, `xnew`, and now `xalias`,
`xchain`) whose ground truth is a SUBSET or a DIFFERENT SET from the candidate's own reference set.

Historically the cap was written only in the primary loop as
`if c["cross_files"] > MAX_GT_FILES: continue` — a predicate over the CANDIDATE's whole
cross-member reference count. The sub-kind loops inherited it only transitively, via a `picked`
guard that restricted them to already-picked candidates.

When change 0010 removed that `picked` guard for `xsubtypes` (to unblock the real subtype supply),
the two readings of "re-apply the cap" diverged materially and measurably on the unchanged
10-member corpus:

- candidate-level predicate (`c["cross_files"] > 40`): 68 tasks
- emitted-GT predicate (`len(gt) > 40`):               80 tasks

They are different rules whenever a sub-kind's GT is not the candidate's whole reference set: a
subset can pass while the candidate fails, and `ximpact` (which ADDS the definition file) can fail
while the candidate passes. The baseline frozen corpus in fact already contained one over-cap task
at 41 GT files — a latent violation of the registered rule caused by exactly this gap.

## Decision

The cap is evaluated on the ground truth a task ACTUALLY EMITS, via a single shared predicate
`gt_within_cap(gt)` called at EVERY emit site. The candidate-level check is retained only as a
cheap pre-filter and is explicitly commented as not being the gate.

The same principle generalizes to the shapes' emit guards: each shape's non-degeneracy guard
(proper-subset for `xnew`/`xsubtypes`/`xalias`, language-gated proper-subset for `xcollide`,
disjointness for `xchain`) is likewise evaluated on the emitted answer, not on a proxy.

Rationale: the cap exists to bound the ANSWER an agent must list, so the answer is the only correct
thing to measure. Routing it through one predicate also prevents the invariant from drifting across
what is now five-plus emit sites (the repo's `one-invariant-many-sites-drifts` learning).

## Consequences

- **Enables:** the registered rule now means what it says; the 40-file cap is enforced uniformly;
  a pre-existing 41-file violation was found and removed.
- **Costs / gives up:** the emitted-GT reading admits tasks whose CANDIDATE is very large but whose
  shape-specific answer is small — deliberate, since the agent is only asked for the small answer.
  Counts under the two readings differ substantially (68 vs 80 on the reference corpus), so any
  historical count computed under the old reading is not comparable.
- A future reader must not "simplify" the pre-filter and the gate into one predicate, nor conflate
  `MAX_GT_FILES = 40` with the separate registered floor `control >= 40` — they share a value by
  coincidence and are commented as such.

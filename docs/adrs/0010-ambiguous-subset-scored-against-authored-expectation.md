---
id: 10
slug: ambiguous-subset-scored-against-authored-expectation
title: Ambiguous-subset accuracy is scored against the authored expectation, not the tool's self-report
status: Accepted
date: 2026-08-04
supersedes: []
reverses: []
relates_to: []
change: 5
---

## Context

The blast-radius accuracy benchmark (`bench/impact_bench.py`, change 0005) reports, alongside overall impact-set recall/precision, a separately broken-out score over the subset of results codeindex tags `[ambiguous]` — the spec's stated goal is to "quantify how much the `[ambiguous]` flag can be trusted." A naive implementation intersected the ground-truth edge set with the set of edges the tool itself flagged `[ambiguous]` (`amb_truth = truth ∩ tool_flagged`, `amb_predicted = predicted ∩ tool_flagged`). This has a silent failure mode: when the tool flags NOTHING (which codeindex does for same-name collision fixtures in Go/JS/Python/TS), both intersections are empty and the empty-set scoring convention (recall=precision=1.0 when the denominator is 0) returns a vacuous perfect 1.000 — masking the very missing-flag gap the metric exists to detect. A symbol authored as a genuine ambiguity case whose flag the tool omits would score a spurious 1.0.

## Decision

The ambiguous-subset metric scores the tool's actually-flagged edges against the AUTHORED expectation, not against the tool's own self-report. Each fixture symbol carries an authored `ambiguous: true|false` in its `manifest.json`. The expected-ambiguous edge set is defined as `expected_ambiguous = truth if authored_ambiguous else set()` — i.e., for a symbol the fixture author declares ambiguous, ALL its true caller edges SHOULD be flagged `[ambiguous]`; for a non-ambiguous symbol, NONE should be. The subset score is then `score_sets(expected_ambiguous, tool_flagged_edges)`. Consequently an authored-ambiguous symbol whose flag the tool omits scores ambiguous-recall < 1.0 (the miss is surfaced), while a correctly-unflagged non-ambiguous symbol still scores 1.0. The authored ambiguity expectation therefore lives only in fixtures (the sole place ground truth for collisions is known by construction), consistent with change 0005's spec. The public `score_with_ambiguous(truth, predicted, ambiguous)` helper retains its original signature/behavior (unit tests depend on it); the expected-set logic lives in the harness's per-symbol runner.

## Consequences

- The ambiguous-subset number now measures real `[ambiguous]`-flag coverage fidelity and cannot be gamed by a tool that simply never flags anything. First fixture run surfaced the honest result: codeindex omits the `[ambiguous]` flag on authored collision fixtures for Go/JS/Python/TS (ambiguous-recall 0.000) while PHP flags them (1.000) — overall blast-radius recall remained 1.000 across all five.
- The metric depends on fixtures carrying a correct authored `ambiguous:` flag; real-repo sampling (restricted to unique names) contributes nothing to this subset, by design.
- The empty-set 1.0 convention is retained for the overall scorer and for correctly-unflagged non-ambiguous symbols — a deliberate asymmetry: absence of a flag is only penalized where the author expected a flag.

---
id: 5
slug: blast-radius-accuracy-benchmark
title: Blast-radius accuracy benchmark — impact-set recall vs. false positives
status: proposed
priority: high
type: feat
created: 2026-08-04
updated: 2026-08-04
depends_on: []
related: [6]
discovered_from: []
adrs: []
spec:
plan:
results:
trivial: false
auto_groomable:
branch:
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
<!-- docket:artifacts:end -->

## Why

The `bench/` evidence today proves codeindex saves tokens versus grep. It does not prove the impact sets are *correct* — and name-based call resolution (with its `[ambiguous]` flag) is exactly the part where correctness is in question. An agent that trusts an incomplete "who calls this?" answer and ships a breaking change is worse off than one that greps. We need a ground-truth accuracy benchmark to sit alongside the token-savings numbers.

Idea surfaced while mining the LLM-research vault: the recent coding-agent and static-analysis benchmarks (CoRe on code-reasoning/dependency tasks, AgentHPOBench and ExtractBench on scored completeness + grounding) all converge on measuring recall and false-positive rate against a ground-truth oracle rather than eyeballing output. Three independent passes over the vault flagged this as the highest-leverage idea.

## What changes

Add a benchmark that, for a corpus of repos across the supported languages:
- Picks a symbol, mutates or removes it, and derives a ground-truth impact set from what actually breaks (compile errors / failing tests).
- Runs codeindex's blast-radius query for that symbol and scores the returned set on **recall** (did it find every real dependent?) and **false-positive rate** (how much noise?).
- Reports per-language and aggregate scores, and specifically breaks out accuracy on `[ambiguous]`-flagged results so we can quantify how trustworthy the ambiguity signal is.

## Out of scope

- Semantic/embedding-based retrieval — this measures the existing deterministic resolver, it does not change it.
- Fixing any accuracy gaps the benchmark reveals (those become their own changes).

## Open questions

- How to source the ground-truth oracle cheaply — compile-break vs. test-break vs. an independent LSP/analyzer as a second opinion?
- Corpus selection: reuse the existing `bench/` repos or add purpose-built fixtures with known call structure?
- Is this one benchmark parameterized by language, or per-language harnesses sharing a scorer?

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

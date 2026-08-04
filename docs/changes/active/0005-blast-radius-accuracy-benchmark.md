---
id: 5
slug: blast-radius-accuracy-benchmark
title: Blast-radius accuracy benchmark — impact-set recall vs. false positives
status: in-progress
priority: high
type: feat
created: 2026-08-04
updated: 2026-08-04
depends_on: []
related: [6]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-04-blast-radius-accuracy-benchmark-design.md
plan: docs/superpowers/plans/2026-08-04-blast-radius-accuracy-benchmark.md
results:
trivial: false
auto_groomable:
branch: feat/blast-radius-accuracy-benchmark
pr:
claimed_at: 2026-08-04T19:36:01Z
blocked_by:
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-04-blast-radius-accuracy-benchmark-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-04-blast-radius-accuracy-benchmark-design.md) |
| Plan | [2026-08-04-blast-radius-accuracy-benchmark.md](https://github.com/ethanhinson/codeindex/blob/feat/blast-radius-accuracy-benchmark/docs/superpowers/plans/2026-08-04-blast-radius-accuracy-benchmark.md) |
<!-- docket:artifacts:end -->

## Why

The `bench/` evidence today proves codeindex saves tokens versus grep. It does not prove the impact sets are *correct* — and name-based call resolution (with its `[ambiguous]` flag) is exactly the part where correctness is in question. An agent that trusts an incomplete "who calls this?" answer and ships a breaking change is worse off than one that greps. We need a ground-truth accuracy benchmark to sit alongside the token-savings numbers.

Idea surfaced while mining the LLM-research vault: the recent coding-agent and static-analysis benchmarks (CoRe on code-reasoning/dependency tasks, AgentHPOBench and ExtractBench on scored completeness + grounding) all converge on measuring recall and false-positive rate against a ground-truth oracle rather than eyeballing output. Three independent passes over the vault flagged this as the highest-leverage idea.

## What changes

Add `bench/impact_bench.py` (mirroring the existing `token_bench.py` / `recall_bench.py` shape) that scores codeindex's impact/blast-radius answers against a ground truth, across all five supported languages:
- For a symbol S, compares codeindex's impact set against ground truth on **recall** (did it find every real dependent?) and **precision** (noise), per-language and aggregate, with a broken-out score for `[ambiguous]`-flagged results.
- **Hybrid oracle:** compile-break on real repos for Go/TS (rename the declaration, run `go build` / `tsc`, the sites that fail to resolve are the truth); authored known-truth fixtures for JS/Python/PHP (which have no static break signal), where the manifest declares each symbol's true dependents.
- **Ambiguity lives only in fixtures:** real-repo sampling is restricted to uniquely-named symbols (so the compiler oracle stays clean); same-name/shadowing/re-export cases are authored into fixtures where the truth is known.
- Pre-registered bar (fixed before running): aggregate recall ≥ 0.95, per-language recall ≥ 0.90; precision reported but ungated in v1.

Design detail — oracle interfaces, scoring normalization, outputs, error handling — is in the linked spec.

## Out of scope

- Changing the deterministic resolver — this measures it, it does not alter it.
- Fixing any accuracy gaps the benchmark reveals (each becomes its own change).
- Real-repo runs for JS/Python/PHP under a test-break oracle (a later change once the fixture number is trusted).
- Semantic/embedding-based retrieval.

## Open questions

_Resolved during grooming (2026-08-04) — see the linked spec: hybrid compile-break + fixture oracle; all five languages; ambiguity confined to authored fixtures._

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

### 2026-08-04 — reconcile (docket-implement-next)

Reconciled against current `origin/main`. No scope changes; the spec holds unchanged.

- **CLI surface verified present and matching the spec.** `codeindex impact <root> <symbol> [--limit N]` and `codeindex callers <root> <symbol>` both exist (`cmd/codeindex/main.go`); `internal/query/query.go` emits the parseable format the runner needs — a `callers (N):` header followed by `  <file>:<line>  <QName>[  [ambiguous]]` lines, where `QName()` yields the enclosing-symbol identity the scorer normalizes to. The `[ambiguous]` flag is driven by `graph.ConfAmbiguous`.
- **ADR-0007 (references-only output contract, `[ambiguous]` flag) is Accepted and current** — the spec's premise is intact. No new ADRs since supersede it.
- **`depends_on: []` satisfied.** Related change 6 (`delta-impact-query-mode`) is still needs-brainstorm and independent — no coupling. Recently-archived changes 0001–0004 (lore removal, graph-query decouple, README rewrite) do not touch `bench/` or the impact surface.
- **Additive only.** All work lands under `bench/` (mirroring `token_bench.py` / `recall_bench.py`); the deterministic resolver is measured, not altered (spec Out-of-scope).
- Auto-capture disabled (`AUTO_CAPTURE_ENABLED=false`); no stubs minted.

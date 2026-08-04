---
id: 7
slug: session-stable-query-cache
title: Session-stable query cache keyed by index version
status: proposed
priority: medium
type: perf
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

Agents resubmit near-identical context with small appends, and within one task they ask the same blast-radius questions many times. Recomputing an unchanged answer wastes work on our side and — more importantly — a byte-unstable answer defeats the model-side prompt cache, so the whole transcript re-prefills. When the index has not changed, the same query should return byte-identical output, ideally without touching the graph at all.

Idea surfaced while mining the LLM-research vault: TokTier (stateful tokenization for agentic serving — small appends to long transcripts) and LiveMem (state continuity across context turnover) both point at caching deterministic results keyed to a stable state token. codeindex already has the per-file hashes to build that key.

## What changes

Add a query result cache keyed by `(query, index_version)` where `index_version` derives from the existing content hashes:
- A repeated query against an unchanged index returns the cached, byte-identical result and skips graph traversal.
- The key invalidates precisely when the incremental engine patches the affected files — not on every unrelated edit.
- Applies to the read-heavy surfaces (graph API / `codeindex serve`, MCP).

## Out of scope

- Cross-session persistence of an agent's *reasoning* state — this caches query outputs, not agent memory.
- The delta query mode (0006) — related but separate; this change may provide the baseline-snapshot mechanism it can reuse.

## Open questions

- Cache scope and eviction: in-process LRU for `serve`, or a persisted table in the SQLite store?
- Granularity of `index_version` — whole-index hash (simple, coarse invalidation) vs. per-subgraph (precise, more bookkeeping)?
- Interaction with lazy per-query rebuild (ADR-0005): cache lookup must happen after the freshness recheck, not before.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

---
id: 5
slug: freshness-on-demand-build-lazy-per-query-recheck
title: Freshness is on-demand build + lazy per-query re-check, no daemon
status: Accepted
date: 2026-07-30
supersedes: []
reverses: []
relates_to: []
change: 1
---

## Context

Query answers must stay correct as source files change, without the operational cost and complexity of running a background daemon/watcher. Origin: decided 2026-07-08 in the openspec "Key decisions" block, migrated to `.lore/decisions/` on 2026-07-30, now migrated to a docket ADR as part of backing lore out (change 0001, before `.lore/` is deleted by change 0004).

## Decision

No background daemon. The index is built on demand (`build`), and every query does a lazy re-check of file hashes before answering, patching anything stale. This yields always-correct answers with minimal per-query overhead. Anchor: `internal/query/` (`query.Fresh` / the query layer).

## Consequences

Enables always-correct answers with no daemon to run, monitor, or keep resident. The cost is a small per-query staleness check (hash re-check of candidate files) instead of amortizing freshness in a persistent watcher.

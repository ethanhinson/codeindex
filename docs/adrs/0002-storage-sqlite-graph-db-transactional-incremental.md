---
id: 2
slug: storage-sqlite-graph-db-transactional-incremental
title: Storage is SQLite (.codeindex/graph.db), transactional incremental updates
status: Accepted
date: 2026-07-30
supersedes: []
reverses: []
relates_to: []
change: 1
---

## Context

The symbol graph needs durable storage that supports incremental updates and both-direction edge traversal (callers AND callees) without full scans.

Origin: decided 2026-07-08 in the openspec "Key decisions" block, migrated to `.lore/decisions/` on 2026-07-30, now migrated to a docket ADR as part of backing lore out (change 0001, before `.lore/` is deleted by change 0004). The `date:` above preserves the `.lore/decisions/` provenance date rather than the docket authoring date.

## Decision

The symbol graph lives in a single SQLite file (`.codeindex/graph.db`) with transactional incremental updates and indexed both-direction edge traversal, so callers and callees are answered without full scans. Anchor: `internal/graph/`.

## Consequences

Enables trivial distribution (a single file), atomic incremental patches, and fast bidirectional traversal via indexed edges. A single-file store bounds concurrency to SQLite's model. Historical note: lore formerly kept a SEPARATE `lore.db` that was required not to couple to `graph.db`; that lore-specific constraint dies with lore and is intentionally NOT carried forward into this decision.

---
id: itm-01KYTV4G77DRAQ82K69M6N7E9N
title: 'serve: guard per-request lore reindex against concurrent races'
status: open
date: "2026-07-31"
priority: p2
anchors:
    - symbol: Neighborhood
    - path: internal/readmodel/graph.go
---

The web read model calls loreindex.Reindex on every /api/graph request via openLore (internal/readmodel/graph.go). Unlike query.Fresh, which serializes with a package mutex, this reindex is unguarded: two concurrent requests each run a full upsert + delete-unseen pass on lore.db. WAL + busy_timeout avoids most SQLITE_BUSY, but the delete-unseen pass can race semantically. Low risk for a loopback single-user dev tool, but the graph handler does a write on every read. Fix: serialize the per-request freshen (shared mutex) or freshen once at startup / cache the store. Discovered in the final review of the serve+graph-API first slice. Relates to the existing concurrent-reindex-story item.

---
id: dec-01KYTPM3WGARYNNX1NJRSV9ET5
title: Storage is SQLite (.codeindex/graph.db), transactional incremental updates
status: active
date: "2026-07-30"
related: [note-01KYTPDHJH59PG2M0FFTEYC92Z]
tags: [engine]
anchors:
    - path: internal/graph/
refs:
    - url: docs/superpowers/specs/2026-07-08-codeindex-design.md
---

The symbol graph lives in a single SQLite file (.codeindex/graph.db) with
transactional incremental updates and indexed both-direction edge traversal
(callers and callees answered without full scans).

Migrated 2026-07-30 from openspec/config.yaml Key decisions (decided 2026-07-08),
per dec-01KYR17XEC208KMPSEGKBFT6Y7. Active. Note: lore keeps a SEPARATE lore.db
and must not couple to graph.db (dec-01KYR17XECDN2T35W7ERZ932Y8).

---
id: itm-01KYTGCATA2045YS4MC3XQ0HVD
title: Execute lore knowledge-graph-edges plan (related edge, Trace, lore-in-impact, graph-health)
status: done
date: "2026-07-30"
priority: p1
tags: [knowledge-graph]
related: [dec-01KYTG2C8BPFS0GV787Y8AA4QM]
anchors:
    - path: internal/lore/record.go
    - path: internal/lore/index/
    - path: cmd/codeindex/lore.go
refs:
    - url: docs/superpowers/plans/2026-07-30-lore-knowledge-graph-edges.md
    - gh-pr: ethanhinson/codeindex#1
---

Implement the 7-task plan at
docs/superpowers/plans/2026-07-30-lore-knowledge-graph-edges.md: add the
`related:` record→record edge (Task 1), `lore_links` index + schema v3 (Task 2),
ResolveID/Trace/Backlinks graph primitives (Task 3), the shared RelatedLoreBlock
formatter (Task 4), CLI `impact --related-depth` (Task 5), `lore related` +
Referenced-by in `lore show` (Task 6), and orphans/dangling-links/density in
`lore doctor` (Task 7).

This is the first "build dogfooding capability" deliverable: it makes codeindex
usable on itself as a knowledge graph and gives `lore doctor` a graph-health
baseline to measure future dogfooding against. Rationale and rejected
alternatives: dec-01KYTG2C8BPFS0GV787Y8AA4QM.

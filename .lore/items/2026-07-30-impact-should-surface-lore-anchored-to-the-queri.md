---
id: itm-01KYTMETKF3WSY4SQ03363S5DJ
title: impact should surface lore anchored to the queried symbol's file/dir, not just the exact symbol
status: open
date: "2026-07-30"
priority: p3
related: [dec-01KYTG2C8BPFS0GV787Y8AA4QM]
tags: [knowledge-graph]
anchors:
    - path: internal/lore/index/relatedblock.go
    - path: internal/query/
---

Surfaced while dogfooding Task 5. `impact <symbol>` currently surfaces only
records whose anchor is that exact symbol (or a path prefix of the query
string). A record anchored to the symbol's containing file or directory does
NOT appear, because RelatedLoreBlock's roots come from RecordsForAnchor(symbol)
with no symbol→file resolution. Example: the "no graph.db coupling" decision is
anchored to internal/lore/index/ and never shows up for `impact StaleRecords`
even though StaleRecords lives in that dir.

Fix stays query-time (no graph.db coupling): at impact time, resolve the
symbol's file via the code graph, then also match records whose path anchor
covers that file/dir. This is a lookup at query time, not a schema link, so it
respects dec-01KYR17XECDN2T35W7ERZ932Y8. Rationale: dec-01KYTG2C8BPFS0GV787Y8AA4QM.

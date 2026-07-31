---
id: itm-01KYTV59EAA8WHZZRP5BWPHESA
title: 'readmodel graph builder: normalize symbol-anchor join + dedup RecordNeighborhood nodes'
status: open
date: "2026-07-31"
priority: p3
anchors:
    - symbol: AttachAnchoredLore
    - symbol: RecordNeighborhood
---

Two low-risk correctness niceties in internal/readmodel/graph.go found in the serve+graph-API first-slice review. (1) AttachAnchoredLore matches lore anchors via RecordsForAnchor(recs, sym.Label) with sym.Label = dotted QName, but SplitAnchor also accepts Parent::Name; a record anchored with ::-form attaches on the record-focus path but silently not on the symbol-focus path (asymmetric join). Fix: normalize the anchor/label through SplitAnchor+qname (or match on resolved QName) before comparing. (2) RecordNeighborhood appends a node per anchor and per blocked_by with no present-map dedup (unlike AttachAnchoredLore), so repeated anchors yield duplicate node IDs; sortGraph sorts but does not dedup. Fix: reuse the present-map pattern. Neither is triggered by current repo records (all anchors are path: today).

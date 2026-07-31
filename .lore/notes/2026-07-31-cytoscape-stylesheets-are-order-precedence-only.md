---
id: note-01KYWFH3ZV61A9CA43XX7QSH54
title: Cytoscape stylesheets are order-precedence only — no specificity; rule order is load-bearing
date: "2026-07-31"
---

Root cause from the graph-smoothness final review (fixed in e09540e): the edge[?bundled] width rule (mapData(count)) was silently dead because the generic 'edge' rule appeared later in stylesheet() and overwrote width/color/opacity property-by-property. Cytoscape has no CSS-style specificity: the last matching rule wins per property. Any new rule in web/src/graph/style.ts must be placed deliberately: generic rules first, feature rules (bundled) after them, per-kind rules last only for the properties they should win. A regression e2e asserts max bundled-edge width > 1 at overview. Known accepted consequence: per-kind rules re-fix width 1.5 on bundled anchors/blocked_by edges (intended — kind color/dash wins there; count→width is a calls-bundle feature).

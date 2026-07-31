---
id: dec-01KYV5FRVTW6ACVNW4MQ8QGZ5V
title: 'Graph UI smoothness: client-side aggregation over single full payload, overview-first with ranked reveal'
status: active
date: "2026-07-31"
---

The SPA never renders the raw full graph. Client derives a package-level overview (~60 nodes) from /api/graph/full, expands packages on demand to top-12 symbols by degree + '+N more' chip, resolves edges by visibility (bundled otherwise). Deterministic layout: hash-seeded circle placement + fcose randomize:false; expansions pin existing nodes (fixedNodeConstraint). Hover highlights via neighborhood diff, not all-elements class sweep; LOD label/edge hiding on zoom-band change. Rejected: (1) server-side aggregation endpoints — round-trip per expand hurts perceived smoothness and payload isn't the bottleneck at ~1.5k nodes; revisit behind the same client abstraction at ~20k+ symbols. (2) sigma.js/WebGL renderer swap — rewrite cost; Cytoscape is fine once we stop rendering everything. Spec: docs/superpowers/specs/2026-07-30-graph-rendering-smoothness-design.md (branch worktree-lore-graph-ui).

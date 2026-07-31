---
id: dec-01KYWJEEHNAP5HB52AVA7WVB45
title: 'Graph UI v3: two-state model (overview ⇄ package focus), earned labels, HTML lore rail, anchored motion'
status: active
date: "2026-07-31"
---

Supersedes the in-place expand/chip interaction from dec-01KYV5FRVTW6ACVNW4MQ8QGZ5V's execution: in-place compound expansion is removed. Tap a package → focus view (whole canvas, real intra-package structure, neighbor packages as rim satellites, ego-graph navigation). Labels render only when earned (top-8 degree / hover / selection / near zoom). Lore leaves the canvas for an HTML rail grouped by kind with session notes off by default; cross-links are hover-sync, not drawn edges. Motion: deterministic anchors + Lissajous idle oscillation (hash-derived phase/period, 30fps, pauses on gesture/reduced-motion/?motion=0) and ~350ms state transitions. Rejected: list-panel tail reveal; dimmed-map focus; always-on truncated labels; anchor-adjacent lore placement; keeping both expand paradigms. Spec: docs/superpowers/specs/2026-07-31-graph-two-state-ui-design.md

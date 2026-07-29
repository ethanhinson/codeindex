---
id: itm-01KYR17XECFKKKJBWRY0A7RCF3
title: Execute Plan 1 — lore engine core (CLI-complete)
status: open
date: 2026-07-29
priority: p0
refs:
    - url: docs/superpowers/plans/2026-07-29-lore-engine-core.md
---
Implement the 12 tasks of Plan 1: record model, layout/overlay, derived
store, lazy reindex, search/ranking, anchor staleness, and the full CLI
(add, show, search, for, backlog, promote, supersede, doctor, init).
Done when `go test ./...` passes and the end-to-end smoke in Task 12 runs
against this repo's own .lore/ seed records.

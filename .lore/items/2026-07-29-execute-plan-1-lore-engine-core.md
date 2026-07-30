---
id: itm-01KYR17XECFKKKJBWRY0A7RCF3
title: Execute Plan 1 — lore engine core (CLI-complete)
status: done
date: 2026-07-29
priority: p0
refs:
    - url: docs/superpowers/plans/2026-07-29-lore-engine-core.md
    - branch: feature/lore-engine-core
---
Implement the 12 tasks of Plan 1: record model, layout/overlay, derived
store, lazy reindex, search/ranking, anchor staleness, and the full CLI
(add, show, search, for, backlog, promote, supersede, doctor, init).
Done when `go test ./...` passes and the end-to-end smoke in Task 12 runs
against this repo's own .lore/ seed records.

Completed 2026-07-29: 12 tasks executed TDD via subagent-driven development,
each spec+quality reviewed with fix rounds; final whole-branch review verdict
"ready to merge with fixes", both fixes applied. Dogfood smoke passed —
the binary queries this repo's own records. Follow-up gaps filed as
itm-01KYRS1YSWEV1CCEB1X8SGMDJ1 and itm-01KYRS1YSW68DHC9QKNRD0DNX6.

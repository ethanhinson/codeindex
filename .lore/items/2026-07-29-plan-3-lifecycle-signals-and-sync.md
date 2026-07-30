---
id: itm-01KYR17XECP2YV5YBQ7VT87NQF
title: Write and execute Plan 3 — lifecycle signals, events, gh sync
status: done
date: 2026-07-29
priority: p2
blocked_by: [itm-01KYR17XECTSCDR5DZX5DXAWTJ]
refs:
    - url: docs/superpowers/specs/2026-07-29-lore-engine-design.md
---
Git-derived evidence during reindex: ratification-by-merge labeling,
`closes <item-id>` commit references driving durable item transitions,
survival/churn confidence scoring, `lore event` CI ingestion, and
`lore sync github` / `lore push` via the gh CLI.

Completed 2026-07-30 on branch feature/lore-lifecycle-signals: eight tasks
TDD (plus the two integrity items closed in-commit: open wire struct,
duplicate-ID visibility), per-task gates with fix rounds (T7 needs-fixes
round on event matching), final whole-branch review "with fixes" — CWD
churn-counting bug, sync/push index freshness, ref-recording contract —
all applied. Closes rehearsal proven live: a commit subject flipped a
backlog item to done. Deferred findings filed as
itm-01KYTCQDP4DBJR8TE8GKBP4HRD, itm-01KYTCQDP48M7SX769JYP9GNF3,
itm-01KYTCQDP4BXQ48RBJF229GJ6Q.

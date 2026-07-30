---
id: itm-01KYRS1YSW68DHC9QKNRD0DNX6
title: Carry unknown frontmatter keys through Parse/Marshal (open wire struct)
status: done
date: 2026-07-29
priority: p2
tags: [format, forward-compat]
blocked_by: []
anchors:
    - path: internal/lore/record.go
refs:
    - url: docs/superpowers/plans/2026-07-29-lore-engine-core.md
---
Found by the Plan 1 final whole-branch review. The wire struct is closed:
supersede's Parse→Marshal rewrite drops any frontmatter key the current
binary doesn't know. Lossless today, but the moment Plans 2–3 add fields
(hook:, claimed_at:, promotion_state:), an older binary rewriting a record
silently strips them.

Fix direction: carry unknown keys via a yaml:",inline" map on the wire
struct so rewrites preserve fields from the future; add a round-trip test
with an unknown key. Must land with or before the first new-field plan
(reconcile/claim work) — sequence it ahead of the claim-lease item.

Completed 2026-07-30: implemented via second unmarshal pass into `Record.Extra`; `knownKeys` slice guards coverage; all tests green.

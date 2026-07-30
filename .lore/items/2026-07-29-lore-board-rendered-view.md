---
id: itm-01KYR5Z1KBBK2VW8AJ7E7CS9SC
title: lore board — rendered at-a-glance view with readiness cells
status: open
date: 2026-07-29
priority: p3
blocked_by: [itm-01KYR17XECFKKKJBWRY0A7RCF3]
refs:
    - url: https://github.com/danielhanold/docket
    - url: .lore/notes/docket-comparison-and-adoptable-ideas.md
---
Adopted from docket's BOARD.md. `lore board` renders a grouped view of all
records (status → priority → age) with readiness cells, including the
"waiting on <id> — needs your merge" distinction once branch/pr fields
exist. Deterministic and derived: stdout by default, `--write BOARD.md`
optional; the files remain the only source of truth and the board is never
hand-edited.

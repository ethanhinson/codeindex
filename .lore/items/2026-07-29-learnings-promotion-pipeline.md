---
id: itm-01KYR5Z1KB8PYV49601870F1VY
title: Notes promotion pipeline — promotion_state and graduation to AGENTS.md
status: open
date: 2026-07-29
priority: p2
blocked_by: [itm-01KYR17XECTSCDR5DZX5DXAWTJ]
refs:
    - url: https://github.com/danielhanold/docket
    - url: .lore/notes/docket-comparison-and-adoptable-ideas.md
---
Adopted from docket's learnings tiering. Notes gain optional fields:
`hook:` (one-line searchable trigger phrase), `promotion_state:`
(retained | candidate | promoted, default retained), `promoted_to:` (the
agent-instructions file that received the rule). /lore-review proposes
candidates ("this note's rule has proven itself across N items/sessions");
a human graduates the rule into AGENTS.md/CLAUDE.md and flips the state.
Promoted notes are kept as receipt + dedup memory. Curation is always a
human act — the loop flags, never auto-merges.

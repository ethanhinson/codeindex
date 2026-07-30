---
id: itm-01KYR5Z1KB4Z4DPZ31RZ914SS9
title: Add discovered_from provenance and branch/pr fields to items
status: open
date: 2026-07-29
priority: p2
blocked_by: [itm-01KYR17XECFKKKJBWRY0A7RCF3]
refs:
    - url: https://github.com/danielhanold/docket
    - url: .lore/notes/docket-comparison-and-adoptable-ideas.md
---
Adopted from docket. `discovered_from: [item-ids]` records which work
surfaced this item (agents constantly notice adjacent work — that
provenance is lore). `branch:`/`pr:` typed ref kinds link an item to its
implementation artifacts (pairs with the claim protocol and Plan 3's
`closes <id>` commit detection). Small frontmatter + index additions.

---
id: itm-01KYR17XECZJ5DYEV1VXQQ3TRD
title: Migrate openspec/ content into .lore/ and retire the directory
status: done
date: 2026-07-29
priority: p2
blocked_by: [itm-01KYR17XECFKKKJBWRY0A7RCF3]
related: [dec-01KYR17XEC208KMPSEGKBFT6Y7]
anchors:
    - path: .lore/
refs:
    - url: .lore/decisions/2026-07-29-lore-replaces-openspec.md
---
Convert the durable knowledge in openspec/ into lore records: the "Key
decisions" block in openspec/config.yaml becomes individual decision records
(with their original rationale); still-relevant roadmap entries become
backlog items; completed changes need no migration (git history keeps them).
Then remove openspec/ and update CI/docs references.

Update 2026-07-30 (drift check): this item is now fully actionable. Its
blocker (Plan 1 execution, itm-01KYR17XECFKKKJBWRY0A7RCF3) is done, and the
retirement precondition set by dec-01KYR17XEC208KMPSEGKBFT6Y7 ("nothing
deleted until the loop has proven itself through at least Plan 1") is met —
Plans 1/2/3 shipped plus the knowledge-graph-edges PR #1. openspec/ is still
present on disk; nothing else blocks retiring it.

---
id: itm-01KYR17XECZJ5DYEV1VXQQ3TRD
title: Migrate openspec/ content into .lore/ and retire the directory
status: open
date: 2026-07-29
priority: p2
blocked_by: [itm-01KYR17XECFKKKJBWRY0A7RCF3]
anchors:
    - path: openspec/
refs:
    - url: .lore/decisions/2026-07-29-lore-replaces-openspec.md
---
Convert the durable knowledge in openspec/ into lore records: the "Key
decisions" block in openspec/config.yaml becomes individual decision records
(with their original rationale); still-relevant roadmap entries become
backlog items; completed changes need no migration (git history keeps them).
Then remove openspec/ and update CI/docs references.

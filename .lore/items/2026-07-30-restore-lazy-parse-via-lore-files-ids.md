---
id: itm-01KYT53P7TM5B6SYWSAVBRZV0J
title: Restore lazy reindex — persist per-file IDs in lore_files
status: open
date: 2026-07-30
priority: p2
tags: [performance]
discovered_from: [itm-01KYRS1YSWEV1CCEB1X8SGMDJ1]
anchors:
    - path: internal/lore/index/reindex.go
    - path: internal/lore/index/store.go
---
Duplicate-ID detection (itm-01KYRS1YSWEV1CCEB1X8SGMDJ1) forced Reindex to
read+parse every record file on every run, because lore_records is keyed by
ID last-writer-wins and cannot yield both paths of a collision. That
regressed the lazy contract from O(changed files) to O(all files) — fine at
dogfood scale, but reindex runs on every CLI command and MCP tool call.

Fix (named by both the implementer and the reviewer): add an `ids TEXT`
column to lore_files, written at parse/upsert time; unchanged files then
contribute their IDs from the DB without being read, restoring stat+hash
laziness while keeping complete collision detection. No new deps.

Extended by the Plan 3 final review: the signals pass adds ~1 subprocess
per repo-layer record per reindex (FileOnBranch each) — batch it with one
`git ls-tree -r --name-only origin/<branch> -- .lore/` call, and skip
ratification recomputation entirely when HEAD and the record set are
unchanged since the last scan. Same milestone as the ids column.

Note: this item's `discovered_from` field uses the Plan-4 provenance
convention early (itm-01KYR5Z1KB4Z4DPZ31RZ914SS9) — harmless as an Extra
key until the field is formalized.

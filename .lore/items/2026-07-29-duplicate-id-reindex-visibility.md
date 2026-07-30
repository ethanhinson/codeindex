---
id: itm-01KYRS1YSWEV1CCEB1X8SGMDJ1
title: Duplicate-ID records can silently vanish from the lore index
status: done
date: 2026-07-29
priority: p2
tags: [integrity]
anchors:
    - path: internal/lore/index/reindex.go
    - path: cmd/codeindex/lore.go
refs:
    - url: docs/superpowers/plans/2026-07-29-lore-engine-core.md
---
Found by the Plan 1 final whole-branch review. Promote is copy-then-delete;
if the delete fails, one ID exists in two layers. Reindex is last-writer-wins
on ID, and if the file the record row points at is later deleted,
DeleteByFile removes the record while the surviving copy's unchanged hash
means it is never re-parsed — the record stays missing from the index until
edited or lore.db is deleted. Doctor has no duplicate-ID check.

Fix direction: track IDs seen per Reindex run and report duplicates in
Report (doctor prints them); optionally force a re-parse when DeleteByFile
removed rows whose ID survives in another tracked file. Acceptable to defer
for dogfood because lore.db is sanctioned-rebuildable, but must land before
multi-writer workflows (claim/lease).

completion: Report.Duplicates populated in Reindex (all files parsed for ID tracking); lore doctor surfaces duplicate-id findings.

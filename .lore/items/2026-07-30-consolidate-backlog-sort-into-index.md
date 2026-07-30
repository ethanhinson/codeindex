---
id: itm-01KYSZT2F9K5CZYYEXZKFFT2Y7
title: Consolidate backlog filter/sort into internal/lore/index
status: open
date: 2026-07-30
priority: p3
tags: [cleanup]
anchors:
    - path: cmd/codeindex/lore.go
    - path: internal/mcpserver/lore_tools.go
refs:
    - url: docs/superpowers/plans/2026-07-29-lore-host-integration.md
---
Found by the Plan 2 final whole-branch review. The open-items filter +
priority/blocked/date sort is duplicated between the CLI (loreBacklog) and
the MCP server (loreBacklogText) — deliberate v1 duplication because cmd
(package main) is not importable from internal. Consolidate by moving the
logic into internal/lore/index (e.g. index.Backlog(recs, anchor)) and
having both call it. Also fold in the smaller pairs while there:
writeNewRecord/writeNewLoreRecord. Do alongside Plan 3's item lifecycle
work, which touches the same sorting.

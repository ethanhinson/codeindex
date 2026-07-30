---
id: itm-01KYR17XECTSCDR5DZX5DXAWTJ
title: Write and execute Plan 2 — MCP tools, related_lore, capture, host packaging
status: done
date: 2026-07-29
priority: p1
blocked_by: [itm-01KYR17XECFKKKJBWRY0A7RCF3]
anchors:
    - path: internal/mcpserver/
    - path: plugin/
refs:
    - url: docs/superpowers/specs/2026-07-29-lore-engine-design.md
---
MCP tool family (lore_search, lore_add, lore_for_symbol, lore_backlog,
lore_show, lore_promote), the related_lore field on impact/callers
responses, `lore capture --stdin` for hook-driven session capture, and
packaging: Claude Code plugin skill + hooks + slash commands, Cursor rules
+ mcp.json, Codex AGENTS.md snippet, and per-host `lore init` scaffolding.

Completed 2026-07-30 on branch feature/lore-host-integration: six tasks
TDD via subagent-driven development, per-task gates + final whole-branch
review ("with fixes" — codex marker guard, MCP handshake coverage, lore.db
DSN hardening — all applied). Sanctioned deviations: no SKILL.md (measured
adoption data favors always-visible notes) and lore_promote kept CLI-only.
Follow-up cleanup filed as itm-01KYSZT2F9K5CZYYEXZKFFT2Y7.

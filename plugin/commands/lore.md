---
description: Search .lore/ for decisions, work items, and notes relevant to a query
argument-hint: <query>
allowed-tools: Bash(codeindex *)
---

## Lore search: `$1`

Search results:
!`codeindex lore "$CLAUDE_PROJECT_DIR" search "$1" --limit 8`

Active backlog:
!`codeindex lore "$CLAUDE_PROJECT_DIR" backlog`

## Task

Summarize what is relevant to the current conversation from the lore above:

1. **Decisions** that constrain the current work (active decisions are constraints — follow them).
2. **Open work items** from the backlog that overlap with what we are doing now.
3. **Notes** that provide useful context or explain non-obvious choices.

Be concise. Quote the lore record ID when referencing a specific entry so it can be retrieved later.

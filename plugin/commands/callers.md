---
description: Who calls this symbol (definitions + call sites as path:line references)
argument-hint: <symbol> [limit]
allowed-tools: Bash(codeindex *)
---

!`codeindex callers "$CLAUDE_PROJECT_DIR" "$1" --limit ${2:-50}`

Above: definitions of `$1` and its callers as `path:line` references.
`[ambiguous]` = name-collision match, verify by file before trusting. Answer the
user's question from these references without reading whole files unless a
specific site needs inspection.

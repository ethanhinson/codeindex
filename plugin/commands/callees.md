---
description: What this symbol calls (each callee resolved to its definition when possible)
argument-hint: <symbol> [limit]
allowed-tools: Bash(codeindex *)
---

!`codeindex callees "$CLAUDE_PROJECT_DIR" "$1" --limit ${2:-50}`

Above: what `$1` calls, each resolved to its definition where possible
(`unresolved` = stdlib/external or not in the index). Answer from these
references; only open a file if a specific call site needs inspection.

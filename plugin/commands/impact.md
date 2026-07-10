---
description: Blast-radius summary for a symbol — callers + callees, counts first. Run BEFORE modifying/renaming/deleting it.
argument-hint: <symbol>
allowed-tools: Bash(codeindex *)
---

## Graph data for `$1`

Callers and definitions:
!`codeindex callers "$CLAUDE_PROJECT_DIR" "$1" --limit 30`

Callees:
!`codeindex callees "$CLAUDE_PROJECT_DIR" "$1" --limit 30`

## Task

Produce a blast-radius summary for `$1` from the data above:

1. **Counts first**: N definitions, N callers (note how many are in test files), N callees.
2. **Callers** grouped by file (these break if `$1`'s signature/behavior changes).
3. **Callees** that resolve (what `$1` depends on).
4. Flag every `[ambiguous]` entry as needing per-file verification before trusting.
5. State coverage honestly: this covers call edges only — imports/type-dependents are not included yet.

Keep it compact (references, not file contents). If a definition wasn't found, say so and suggest checking the spelling with Grep — do not guess.

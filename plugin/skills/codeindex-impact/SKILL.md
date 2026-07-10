---
name: codeindex-impact
description: Use for ANY question shaped "who calls X", "which functions call/use X", "what does X call", "what is affected if X changes", refactor impact, affected callers, blast radius, or dead-code checks on a named Go function/method/type — run codeindex instead of grep-and-reading files (measured 73% cheaper). Do NOT use to find/locate things (where is X, which files mention Y) — use Grep for that.
---

# codeindex — branch out from a known anchor

This repository has a `codeindex` symbol graph (Go). It answers **structural**
questions about a symbol you can already name, far cheaper than grepping and
reading files (measured: −73% cost on caller-attribution tasks).

## The rule

**Anchor known → branch out → use codeindex.**
**Nothing to anchor on → finding things → use Grep/Glob.**

| Situation | Tool |
|---|---|
| About to modify/rename/delete function `X` | `/codeindex:impact X` FIRST |
| "Who calls `X`?" / "is `X` dead code?" | `codeindex callers <repo> X` |
| "What does `X` call?" (tracing downward) | `codeindex callees <repo> X` |
| Assessing what a diff touches | `/codeindex:impact` each changed symbol |
| "Where is `X` defined?" | **Grep** (`^func X`) — measured cheaper |
| "Which files mention Y?" | **Grep** `-l` — measured cheaper |
| Exploring unfamiliar code, no anchor | **Grep/Glob/Read** |

The negative rows are not optional style — using codeindex for locate-questions
measured **+17% cost** vs plain grep. Don't.

## Refactoring workflow

1. Anchor: name the symbol you're changing.
2. `/codeindex:impact <symbol>` — callers + callees, counts first.
3. Edit. After each edit, a hook may report the blast radius of what you touched.
4. Re-run `codeindex callers` on renamed/removed symbols to confirm zero
   remaining references (an unresolved count of 0 means done).

## CLI reference

```
codeindex callers <repo-root> <symbol> [--limit N]   # defs + who calls it
codeindex callees <repo-root> <symbol> [--limit N]   # what it calls
```

Output is `path:line` references. `[ambiguous]` marks name-collision matches —
verify those by file before trusting them. The index refreshes itself on every
query (safe mid-refactor). Go symbols only; for other languages fall back to
Grep.

# codeindex plugin for Claude Code

Blast-radius navigation for refactoring: when you know a function/method/type
and need **who calls it, what it calls, and what breaks if it changes**.

**What it is NOT:** a search tool. Locating definitions and finding files is
measurably cheaper with grep (A/B: codeindex on locate-questions cost **+17%**;
on branch-out questions it saved **−73%**). The skill encodes exactly that
boundary; the plugin's value depends on respecting it.

## Requirements

- The `codeindex` binary on PATH (or `CODEINDEX_BIN` set):
  `go build -o /usr/local/bin/codeindex ./cmd/codeindex` (CGO; needs a C toolchain)
- A Go repository (Go-only symbols for now). The index self-builds and
  self-refreshes on every query — no manual build step.

## Install

```
claude --plugin-dir /path/to/code-indexer/plugin
# or add to your plugin config permanently
```

## What you get

| Piece | What it does |
|---|---|
| **Skill** `codeindex-impact` | Teaches the agent the anchor rule: branch out from known symbols with codeindex; find things with grep. |
| **`/codeindex:impact <symbol>`** | Counts-first blast-radius summary (callers + callees) — run before modifying a symbol. |
| **`/codeindex:callers <symbol>`** | Definitions + call sites as `path:line` references. |
| **`/codeindex:callees <symbol>`** | What the symbol calls, resolved to definitions. |
| **Post-edit hook** | After the agent edits a Go function that has callers elsewhere, injects a ≤150-token note: symbol, caller count, where — once per symbol per session. |

## Hook controls

- Disable per-repo: `touch .codeindex/hook-disabled`
- Disable globally: `CODEINDEX_HOOK_DISABLE=1`
- The hook is silent on any failure and never blocks edits.

## Honest limits

- Go only; name-based resolution — `[ambiguous]` flags mark name collisions;
  verify those by file before trusting.
- Call edges only for now: `/impact` covers callers/callees, not
  import/type dependents (it says so in its output).

## MCP (IDEs) — Phase 2

`codeindex mcp <repo>` will expose `impact`/`callers`/`callees` to Cursor,
Claude Desktop, and VS Code. Config snippets will land here when it ships.

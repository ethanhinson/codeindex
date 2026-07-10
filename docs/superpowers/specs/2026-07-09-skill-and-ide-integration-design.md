# codeindex — Skill & IDE Integration Design

**Date:** 2026-07-09
**Status:** Approved (brainstorming complete)
**Planning:** OpenSpec change `skill-and-ide-integration` (supersedes the
roadmap placeholder `mcp-and-plugin`)

## Problem

The engine works and the A/B experiments mapped its value boundary: codeindex
**wins (−73% cost)** when the agent holds a known anchor and branches out
(callers, impact), and **loses (+17%)** when used for locate-questions grep
already answers. The consumption layer's job is to package the tool so agents
get the wins without reproducing the v1 mis-trigger losses — in Claude Code
first, then IDEs via MCP.

## The core rule (trigger discipline)

> Use codeindex when you hold a **known anchor** (a function/method/type you
> can name) and need to **branch out** — who calls it, what it calls, what
> breaks if it changes. Do **not** use it to *find* things (where is X, which
> files mention Y) — grep is cheaper there, measured.

Positive triggers: about to modify/rename/delete a function; assessing a diff's
impact; tracing callers while debugging; dead-code checks; sequencing a
refactor. Negative triggers are explicit in the skill because v1 measured the
cost of firing on them.

## Decisions (from brainstorming)

1. **Surfaces:** Claude Code plugin first (skill + slash commands + hook), MCP
   server second — one change, phased.
2. **Trigger model:** agent-initiated skill **plus** a PostToolUse edit-hook
   that auto-injects blast-radius context after the agent edits a known symbol
   (non-round-trip; cannot mis-trigger — reacts to edits, not questions).
3. **Command surface:** workflows + primitives — `/impact <symbol>` headline
   (callers + callees + dependents-when-available summary), `/callers`,
   `/callees` primitives.
4. **Validation:** a pre-registered v3 A/B gate on the packaged plugin. The
   change is not done until it passes.

## Architecture

```
Phase 0 (engine prereq): lazy re-check on the query path + auto-build
                         + `enclosing` lookup (file+range → symbols)
Phase 1 (plugin):        plugin/ = skill + /impact /callers /callees
                         + PostToolUse hook (git-diff hunks → enclosing →
                           compact caller injection, deduped, capped)
Gate    (v3):            mixed locate+branch-out task set, arm B = the real
                         plugin via --plugin-dir; thresholds pre-registered
Phase 2 (MCP):           `codeindex mcp` (stdio, official Go SDK), tools
                         impact/callers/callees with the anchor rule embedded
                         in descriptions; single-writer re-check serialization
```

**Phase 0 is a hard prerequisite:** refactoring means querying *while editing*;
today `query`/`callees` read a static index. Stale answers during a refactor
are wrong answers. Measured patch latency (3.9–119 ms) says freshness on the
query path is affordable.

**Edit-hook mechanics:** PostToolUse on Edit/Write of `.go` files → script gets
the file path from hook stdin JSON → `git diff -U0` for changed hunk ranges →
`codeindex enclosing <repo> <file> <start>:<end>` (lazy re-check makes it
fresh) → if the enclosing symbol has ≥1 caller outside the edited file, inject
≤150 tokens: symbol, caller count, top caller files, "run /impact for detail".
Noise controls: once per symbol per session (dedup file under `.codeindex/`),
hard token cap, plugin setting to disable.

**v3 gate thresholds (pre-registered):** locate tasks must not regress >10%
vs no-plugin (proves negative triggers); branch-out savings ≥50% retained;
hook fires on ≥80% of symbol edits, zero on non-symbol edits; trigger
precision/recall reported from transcripts.

## Risks

- **Skill mis-trigger** (the v1 trap) → explicit negative triggers + the v3
  locate-regression threshold as a hard gate.
- **Hook noise** → fires only with external callers, per-symbol dedup, token
  cap, off switch.
- **Stale index** → Phase 0 freshness prerequisite.
- **Ambiguous names in /impact** → `[ambiguous]` flags preserved; parent_id/
  qualified names (core-indexing-engine task 2.1) extends precision later.
- **MCP long-lived process vs single-writer SQLite** → in-process mutex
  serializing re-check writes.

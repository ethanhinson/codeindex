## Why

The A/B experiments mapped codeindex's value boundary precisely: **−73% cost**
when an agent branches out from a known anchor (v2 GREEN), **+17% cost** when it
fires on locate-questions grep already answers (v1 RED). The engine now serves
the winning queries (callers/callees), but nothing packages them for agents. The
consumption layer must deliver the wins while structurally preventing the v1
mis-trigger losses — first in Claude Code, then in IDEs via MCP.

## What Changes

- **Phase 0 (engine prerequisites):** wire the lazy re-check into the query
  path (queries auto-build a missing index and patch a stale one before
  answering — core-indexing-engine tasks 7.1/7.2, pulled in because refactoring
  means querying while editing) and add an `enclosing` lookup (file + line
  range → enclosing symbols with caller counts) that the edit-hook needs.
- **Phase 1 (Claude Code plugin, `plugin/`):**
  - A **skill** encoding the anchor rule: use codeindex to branch out from a
    known symbol (impact, callers, callees) — never to locate things (explicit
    negative triggers, because v1 measured their cost).
  - **Slash commands:** `/impact <symbol>` (headline workflow: callers +
    callees + dependents-when-available, bounded summary) plus `/callers` and
    `/callees` primitives.
  - A **PostToolUse edit-hook**: after the agent edits a `.go` file, map the
    changed hunks to enclosing symbols and inject a compact (≤150-token)
    blast-radius note when the symbol has external callers — deduped per
    symbol per session, with an off switch. Non-round-trip by construction.
- **v3 validation gate (pre-registered, blocking):** mixed locate + branch-out
  task set run through the A/B harness with arm B = the real packaged plugin
  (`--plugin-dir`). Thresholds: locate tasks regress ≤10%, branch-out savings
  ≥50% retained, hook fires on ≥80% of symbol edits and never on non-symbol
  edits. The change is not complete until the gate passes.
- **Phase 2 (MCP server):** `codeindex mcp` (stdio, official Go MCP SDK)
  exposing `impact`/`callers`/`callees` tools whose descriptions embed the
  anchor rule; in-process serialization of re-check writes (long-lived process
  vs SQLite single-writer); config snippets for Cursor, Claude Desktop, VS
  Code.

Non-goals: new languages or precise resolution (change 2 remains separate);
dependents/blast-radius edges beyond what the engine already has (the `/impact`
command degrades gracefully until deps land); native IDE extensions (LSP);
Windows.

## Capabilities

### New Capabilities

- `claude-plugin`: The Claude Code integration — trigger-disciplined skill,
  `/impact` + primitive commands, the post-edit blast-radius hook with its
  noise controls, and the Phase-0 engine support they depend on (fresh-on-query
  behavior and the `enclosing` lookup).
- `integration-validation`: The v3 pre-registered A/B gate for the packaged
  plugin — mixed task set, thresholds, trigger precision/recall reporting.
- `mcp-server`: The IDE-facing MCP server — tools, anchor-rule descriptions,
  concurrency safety, client configuration.

### Modified Capabilities

None (core-indexing-engine's `code-indexing` spec already requires lazy
freshness; Phase 0 implements it rather than changing its requirements).

## Impact

- New `plugin/` directory (Claude Code plugin: manifest, skill, commands,
  hook script) and new engine surface: `codeindex enclosing`, fresh-on-query
  behavior, later `codeindex mcp`.
- `bench/agent_ab/` gains the v3 mixed task set and a plugin-arm runner mode.
- Spends ~$10–20 API budget for the v3 gate.
- core-indexing-engine tasks 7.1/7.2 are satisfied by Phase 0 work here.

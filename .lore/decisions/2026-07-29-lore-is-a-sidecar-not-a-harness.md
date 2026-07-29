---
id: dec-01KYR17XECC2X5P01CQZN5D0YH
title: Lore is a sidecar service, not a mediating harness
status: active
date: 2026-07-29
refs:
    - url: docs/superpowers/specs/2026-07-29-lore-engine-design.md
---
The lore engine runs alongside the IDE's existing agent (Claude Code, Cursor,
Codex) and reaches it through their standard extension surfaces: MCP tools,
skills/rules files, and shell hooks. It never sits between the editor and the
model.

## Alternatives considered

**Mediating harness (ACP agent).** Speaking ACP and driving models ourselves
would capture every turn automatically, but it means building and maintaining
an agent harness, and the three target hosts would never route through us —
they ARE the agents. Rejected for v1; the engine's internal API keeps this
open as a later mode.

**Hybrid from day one.** Deferred: sidecar adoption first, harness only if
proven necessary.

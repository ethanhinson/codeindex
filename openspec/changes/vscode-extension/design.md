## Context

The engine already speaks machine progress (JSONL v1 events, status.json
sidecar, `status --json`). VS Code extensions activate on workspace events,
can spawn processes, and render progress via window.withProgress + status
bar items. Cursor/Windsurf consume VSIX-compatible extensions.

## Goals / Non-Goals

**Goals**: warm index before first agent query; zero indexing logic in
TypeScript; consent before first CPU burn; visible progress end-to-end.
**Non-Goals**: JetBrains, query surfaces, binary bundling, marketplace
publishing (needs release engineering first).

## Decisions

**D1 — The extension is a renderer of CLI truth.** Detection =
`status --json` (side-effect-free by spec). Indexing = `build --progress`
JSONL. Keep-warm = `refresh`. If the extension dies, nothing breaks; if the
CLI improves, the extension inherits it.

**D2 — Consent once, per workspace.** First detection prompts; the answer
persists in workspaceState ("declined" is permanent until the user runs the
codeindex.indexNow command). autoIndex=always skips the prompt (team
settings.json can opt whole repos in); never disables detection entirely.

**D3 — refresh verb, not build, for keep-warm.** `build` deletes and
rebuilds (correct for the explicit verb); the save hook needs the
incremental path: refresh = build-if-missing else patch. Debounce 1500ms,
serialize (never two concurrent refreshes; trailing-edge re-run if saves
arrived mid-refresh) — mirrors the SQLite single-writer reality.

**D4 — Progress rendering.** window.withProgress(Notification) for the
initial build (percent from JSONL done/total, phase in the message) + a
persistent status bar item ($(sync~spin) while running, $(check) codeindex
when fresh, $(warning) on failure with the error in the tooltip). Keep-warm
refreshes only touch the status bar (no notifications — saves are frequent).

**D5 — Binary discovery.** codeindex.binaryPath setting > PATH lookup (spawn
probe). Missing binary → one notification with a "How to install" button
(opens the repo README) and a "Set path" button (opens settings). Never
re-prompts within a session.

## Risks / Trade-offs

- **Cannot integration-test an IDE here** → pure logic (JSONL parse, status
  interpretation, debounce) extracted into testable functions; manual smoke
  checklist in the README is part of the change's definition of done.
- **Giant repos on first open** → consent prompt shows the file count from a
  bounded workspace scan before any indexing starts.
- **Extension host restarts mid-build** → orphaned build finishes writing
  the sidecar; next activation reads status fresh. The stale-building
  detection (>10min) in the status verb covers crashed builders.

## Migration Plan

Additive. Extension distributed as VSIX artifact initially (marketplace
publishing rides the release-engineering change).

## Open Questions

None.

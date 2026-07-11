# codeindex for VS Code / Cursor / Windsurf

Indexes your workspace on open so AI agents query the symbol graph in
milliseconds instead of cold-building mid-conversation (kubernetes-scale:
~80s once, here, visibly — never again during a chat).

The extension is a thin renderer: all indexing lives in the `codeindex`
binary. It detects, asks once, launches, shows progress, and keeps the index
warm on save.

## Install

1. Build or install the CLI: `go build ./cmd/codeindex` (repo root) and put
   it on PATH, or set `codeindex.binaryPath`.
2. Install the extension from the VSIX: `code --install-extension
   codeindex-0.1.0.vsix` (Cursor: `cursor --install-extension …`). Build the
   VSIX with `npm run package` (requires `npm i -g @vscode/vsce`).

## What it does

- **On workspace open**: `codeindex status --json` (side-effect-free). If
  the workspace has supported files (Go, TS/JS, Python, PHP) and no current
  index, you get one prompt with the file count. Your answer is remembered
  per workspace; `codeindex: Index This Workspace` overrides a decline.
- **While indexing**: progress notification + status bar
  (`⟳ codeindex 4213/11005` → `✓ codeindex`), driven by the CLI's JSONL
  progress feed. Cancellable.
- **On save** (keepFresh, default on): debounced, serialized
  `codeindex refresh` — incremental, typically milliseconds — so agent
  queries never wait on freshness.

## Settings

| setting | default | meaning |
|---|---|---|
| `codeindex.enable` | `true` | master switch |
| `codeindex.binaryPath` | `""` | path to binary; empty = PATH |
| `codeindex.autoIndex` | `prompt` | `prompt` / `always` (team repos: set in workspace settings) / `never` |
| `codeindex.keepFresh` | `true` | refresh index on save |

## Development

```sh
npm install
npm run compile   # tsc -> out/
npm test          # node --test (pure logic: JSONL parsing, status, debounce)
```

## Manual smoke checklist (per release, VS Code + Cursor)

1. Open an unindexed repo with Go/TS files → prompt appears with a plausible
   file count; **Index** → notification with phase + counts; status bar
   spinner → checkmark. `.codeindex/graph.db` exists.
2. Reload window → no prompt (consent remembered); status bar checkmark
   appears with file/symbol counts in the tooltip.
3. Open an unindexed repo → **Not this workspace** → reload → no prompt;
   `codeindex: Index This Workspace` command still indexes.
4. Save a tracked file 3× quickly → exactly one refresh (status bar flickers
   `refreshing` once); `codeindex status` shows updated timestamp.
5. Rename the binary away → reload → single warning with **How to install**
   / **Open settings** buttons; no repeat warning within the session.
6. `codeindex: Show Index Status` → counts match `codeindex status`.
7. Kill VS Code mid-build; reopen → status shows stale-builder detection or
   re-prompts; re-indexing recovers cleanly.
8. Set `codeindex.autoIndex: always` in workspace settings of a fresh clone
   → indexing starts without a prompt on open.

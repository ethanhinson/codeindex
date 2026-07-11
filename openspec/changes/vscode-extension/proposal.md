## Why

Indexing is a workspace concern, not an agent concern. Today the first agent
query pays the cold build mid-conversation — the worst moment. An IDE
extension indexes on workspace open, visibly (the JetBrains model), so the
index is warm before any agent exists. VS Code's extension format also covers
Cursor and Windsurf — most of the AI-IDE market in one artifact. The engine
side (status --json, --progress JSONL, status.json sidecar) shipped in
index-progress-ux; this change is its first renderer.

## What Changes

- **editors/vscode/**: a thin TypeScript extension. All indexing stays in the
  `codeindex` binary; the extension detects, prompts, launches, renders.
  - On workspace open: detect supported files, run `codeindex status --json`
    (side-effect-free). Unindexed or stale-schema → one consent prompt
    ("Index now / Not this workspace", choice persisted per workspace).
  - Indexing runs `codeindex build --progress`, parsing JSONL into a VS Code
    progress notification + status bar item (spinner → counts → checkmark).
  - Keep-warm: debounced re-index on save of supported files via the new
    `codeindex refresh` verb (incremental patch — milliseconds), so agent
    queries never wait on freshness.
  - Binary discovery: `codeindex.binaryPath` setting, else PATH; missing →
    actionable notification.
  - Settings: enable, binaryPath, autoIndex (prompt|always|never), keepFresh.
- **CLI: `codeindex refresh [--progress]`** — build-if-missing else
  incremental patch, with the standard progress surfaces (`build` is a
  from-scratch rebuild; the extension needs the cheap verb).

**Validation**: TypeScript compiles clean (tsc, no emit errors); `refresh`
verb covered by CLI-level test of its build/patch behavior; extension logic
that is pure (JSONL parsing, status interpretation) unit-tested with node;
manual smoke checklist recorded in editors/vscode/README.md (VS Code +
Cursor) since no harness here can drive an IDE. No agent A/B.

Non-goals: JetBrains (separate follow-on); query UI, tree views, or any
feature beyond detect→consent→index→render→keep-warm; bundling binaries
(requires the release-engineering change; PATH + setting until then).

## Capabilities

### New Capabilities
- `ide-extension`: workspace-open detection, consent, visible indexing,
  keep-warm on save, binary discovery.

### Modified Capabilities
None.

## Impact

- editors/vscode/ (new: package.json, src/extension.ts, tsconfig, README).
- cmd/codeindex: refresh verb.

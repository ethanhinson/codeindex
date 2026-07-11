## 1. CLI

- [x] 1.1 refresh verb (build-if-missing else patch, reporter wiring) + test

## 2. Extension

- [x] 2.1 Scaffold editors/vscode (package.json manifest: activation, settings, commands; tsconfig; .vscodeignore)
- [x] 2.2 extension.ts: binary discovery, status detection, consent flow (workspaceState), build with JSONL-driven notification + status bar, keep-warm debounced refresh, indexNow/showStatus commands
- [x] 2.3 Pure-logic unit tests (JSONL parse, status interpretation, debounce) runnable via node; tsc compiles clean

## 3. Close-out

- [x] 3.1 editors/vscode/README.md: install (VSIX), settings, manual smoke checklist (VS Code + Cursor)
- [x] 3.2 Full suite green; openspec validate; commit + push; archive

## 1. Verbs

- [x] 1.1 engine.Export(root, out): freshen (Build if missing, else Patch) + VACUUM INTO; engine.Import(root, artifact): version check via PRAGMA, install, Patch, return drift Stats
- [x] 1.2 CLI: export/import subcommands with drift reporting

## 2. Proof

- [x] 2.1 Tests: export/mutate/import == full rebuild (edit+add+delete); mtime-only drift patches 0 files; schema-mismatch rejected loudly
- [x] 2.2 kubernetes measurement: import + patch (touched mtimes + a real edit) vs 82.5s cold build; record in FINDINGS-shared-index.md

## 3. Docs and close-out

- [x] 3.1 docs/ci.md (GitHub Actions example); README pointer
- [x] 3.2 Full suite green; openspec validate; commit + push; archive

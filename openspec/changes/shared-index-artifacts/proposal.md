## Why

Every developer (and every CI job) currently pays the cold build — 82s on
kubernetes — for an index that is a pure function of the tree. The index is
already a portable derived artifact (repo-relative paths, schema-version
self-describing, content-hash change detection), and depmaps already proved
the ship-a-database mechanics. What's missing is the last mile: export/import
verbs, a proven import-then-patch==rebuild guarantee across tree states and
machines, and a CI recipe. This is the "index once, whole team queries" wedge
— the first product story beyond a single machine.

## What Changes

- `codeindex export <out.db>`: freshen the index (build or patch), then
  snapshot it compactly (`VACUUM INTO`) — a consistent artifact suitable for
  CI upload.
- `codeindex import <artifact.db>`: verify the artifact's schema version
  matches the binary (mismatch = clear error; rebuild beats silent wrongness),
  install it as `.codeindex/graph.db`, then **patch to the current tree** with
  the existing incremental engine, reporting drift (files re-parsed, symbols).
- Fresh-checkout property, proven: an artifact imported into a tree whose
  files differ only in mtime (git clone, CI cache restore) re-hashes and
  patches **zero** files' graphs.
- Docs: `docs/ci.md` with a GitHub Actions workflow (build+export keyed by
  commit, upload artifact; developers import).

**Validation (pre-registered)**: (1) import-then-patch == full rebuild —
export at tree state A, mutate to state B (edit + add + delete), import,
patch: snapshot equals a from-scratch build at B; (2) mtime-only drift patches
0 files; (3) schema-mismatch artifact is rejected with a clear message;
(4) measured on kubernetes: import + patch with real drift completes in a
small fraction of the 82.5s cold build. No agent A/B (infrastructure; engine
gates).

Non-goals: artifact hosting/transport (CI systems own that); index merging;
depmap bundling inside the artifact (attach remains separate); auth.

## Capabilities

### New Capabilities
- `shared-index`: export/import verbs, cross-state/machine equivalence
  guarantee, drift reporting, CI recipe.

### Modified Capabilities
None (Build/Patch/change-detection semantics untouched — import composes
existing proven pieces).

## Impact

- cmd/codeindex: export/import subcommands; internal/engine/artifact.go.
- docs/ci.md (new); README pointer.
- No schema change (rides v7).

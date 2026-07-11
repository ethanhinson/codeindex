## Context

`engine.Build` and `engine.Patch` share one code path: diff the tree against
stored per-file state (hash + size/mtime fast path), re-parse changed files,
re-resolve affected names — with incremental==full proven on six repos at
every schema generation. graph.db stores repo-relative paths only, and Open()
rejects/rebuilds on PRAGMA user_version mismatch. Depmaps already ship
SQLite files across machines with hash verification.

## Goals / Non-Goals

**Goals**: an artifact a CI job exports and any checkout imports, with the
same equivalence guarantee the incremental engine has; honest drift
reporting; a copy-pasteable CI recipe.
**Non-Goals**: transport, hosting, merging, depmap bundling.

## Decisions

**D1 — The artifact IS the database.** No wrapper format, no manifest:
graph.db is self-describing (user_version, files/merkle tables). Export =
freshen + `VACUUM INTO` (consistent, compact, no WAL loose ends). Import =
version-check, copy into place, `engine.Patch`.

**D2 — Import rejects schema mismatch outright.** No migration, no attempt
to salvage: the error names both versions and says to re-export or build.
Silent rebuild would hide that the team's CI artifact is stale — loud is
correct.

**D3 — Drift is handled by the existing Patch, and reported.** mtimes always
differ after checkout; the size+mtime fast path falls back to content hash,
so unchanged files cost one hash each and no graph writes. Real drift
re-parses only changed files. Import prints both numbers — the user sees
exactly what the artifact saved.

**D4 — Prove it with the same gate that proves everything else.** The new
test exports at state A, mutates (edit+add+delete), imports at B, and
compares the patched snapshot against a from-scratch build at B. This is the
equivalence gate crossing tree states — if it holds, CI-built indexes are
exactly as trustworthy as locally built ones.

## Risks / Trade-offs

- **Artifact from a different repo/tree** → Patch simply treats everything as
  changed and effectively rebuilds; correct, just slow. Drift report makes it
  visible.
- **VACUUM INTO target exists** → remove first (export owns its output path).
- **Depmap-attached artifacts** → tier-1 rows travel with the artifact;
  depfiles hashes still verify locally via the existing overlay mechanism.

## Migration Plan

None — new verbs, no schema change.

## Open Questions

None.

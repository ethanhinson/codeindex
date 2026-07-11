# Findings: shared index artifacts (export/import)

Date: 2026-07-11 · Schema v7 (no schema change — new verbs only)

## Headline

`codeindex export` / `codeindex import` turn the index into a CI artifact:
on kubernetes, importing an 80 MB artifact into a fully mtime-invalidated
checkout with one real local edit took **1.53s** against an **82.5s** cold
build — **54× faster**, re-parsing exactly the 1 changed file (drift report:
"1 files re-parsed, 0 deleted, 66 symbols").

## Pre-registered validations

1. **Import-then-patch == full rebuild** — export at state A, mutate to B
   (edit + add + delete), import, patch: snapshot identical to a
   from-scratch build at B. PASS (TestImportThenPatchEqualsRebuild). This is
   the incremental==full gate crossing tree states/machines: a CI-built
   index is exactly as trustworthy as a local one.
2. **mtime-only drift is free** — content-identical tree, every mtime
   touched: 0 files re-parsed. PASS (TestImportMtimeOnlyDriftIsFree).
   This is the property that makes fresh clones cheap: the size+mtime fast
   path falls back to hashing, hashing says nothing changed.
3. **Schema mismatch rejected loudly** — artifact at v6 vs binary v7: error
   names both versions, nothing installed. PASS
   (TestImportRejectsSchemaMismatch).
4. **kubernetes measurement** — above; hash-verifying all 11,005 files
   dominates the 1.53s.

## Mechanics

The artifact IS the database (D1): `VACUUM INTO` snapshot (94.4 → 80 MB,
free pages dropped), self-describing via PRAGMA user_version, portable via
repo-relative paths. Import = version check → install → `engine.Patch` —
zero new index logic; the guarantee is inherited, not re-implemented.

## Verdict

Ship. docs/ci.md carries the GitHub Actions recipe. Natural follow-on when
distribution work starts: a `codeindex fetch` that pulls the artifact for
the current HEAD from GitHub Actions artifacts directly.

## 1. Schema + model (v4)

- [x] 1.1 `symbols.namespace` TEXT, `symbols.tier` INT (0 project / 1 dep), `depfiles(path, namespace, version, hash)` table; schemaVersion 3→4; indexes on (name, tier) and namespace
- [x] 1.2 `graph.Symbol.{Namespace,Tier}`; snapshot normalization scoped to tier 0 (project equivalence gate unchanged) — dep-tier consistency gets its own round-trip check
- [x] 1.3 Resolver tier ordering: qualified t0 > qualified t1 > plain t0 > plain t1, deterministic within tiers; tests incl. project-beats-dep collision and no-degradation assertion

## 2. Defs-only parse mode + depmap generation

- [x] 2.1 Adapter mode flag (skip call/dep collection) threaded through all four adapters
- [x] 2.2 `codeindex depmap <dir> --namespace <ns> --version <v> -o <map.db>`: walk dir with registry extensions, defs-only parse, write symbols + per-file hashes + meta; per-`ns@version` cache under `~/.codeindex/depmaps/`
- [x] 2.3 Tests: map contains defs/hashes/meta, zero edges

## 3. Attach + auto-generation

- [x] 3.1 `codeindex attach <repo> <map.db>`: version check vs meta, bulk INSERT..SELECT into tier 1 + depfiles rows; re-attach same ns replaces prior rows
- [x] 3.2 `codeindex attach <repo> --auto`: Go via `vendor/modules.txt`, PHP via `composer.lock` + vendor tree; cache reuse; report attached modules/counts
- [x] 3.3 Re-resolve project edges whose dst_name matches newly attached symbols (upgrade unresolved → dep-resolved); test

## 4. Overlay (hacked-dep tracking)

- [x] 4.1 Covered in-tree files join the fresh walk under a 25k-file threshold; hash mismatch → defs-only re-parse replacing that file's tier-1 rows (mark modified); restore → map-equivalent rows
- [x] 4.2 Above threshold: attach/build-time verification only; documented in output + README
- [x] 4.3 Scripted round-trip test: edit vendor file → query reflects → restore → equivalent

## 5. Query surface + provenance

- [x] 5.1 Callees/impact display `[dep ns@ver]` (+` modified`); callers/dependents unchanged
- [x] 5.2 Tests: provenance rendering; dep symbols absent from caller lists

## 6. Validation

- [x] 6.1 Metric: attach maps to kubernetes (Go vendor auto) and laravel (composer auto); record unresolved-call share before/after vs baseline (19.6% / 27.5%) in `bench/engine/FINDINGS-depmaps.md`
- [x] 6.2 All existing gates re-run green (six-repo inc==full, full bench incl. query p95 with maps attached, adapter suites)
- [x] 6.3 READMEs + plugin note updated; `openspec validate dependency-maps`; size delta recorded

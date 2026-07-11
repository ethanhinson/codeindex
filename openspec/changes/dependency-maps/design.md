## Context

Every language namespaces dependencies canonically (Go import paths, PHP
namespaces, TS module specifiers, Python module paths) — we adopt those rather
than invent addressing. Deps are pinned-immutable in the common case (maps are
version-keyed caches) but locally mutable in the dev case (hash verification +
overlay). Existing machinery reused: adapters (new defs-only mode), the
content-hash change layer (extended to covered vendor files), the resolver
(tier ordering slot), `dst_name`-keyed queries (already list callers into deps
name-only today — this change upgrades their targets).

Baseline metric (measured, `FINDINGS-bench-full` era indexes): unresolved call
edges — gin 46.8%, prometheus 40.1%, kubernetes 19.6%, nest 30.0%, flask
37.1%, laravel 27.5%.

## Goals / Non-Goals

**Goals**: symbols-only versioned depmaps; attach + tiered resolution
(project always first); per-file hash overlay for hacked deps; provenance
display; measured unresolved-share reduction on kubernetes + laravel; all
existing gates re-verified.

**Non-Goals**: import-aware project resolution (follow-up change — decided);
stdlib maps; network distribution; node/py lockfile auto-gen; dep-internal
edges.

## Decisions

**D1 — Import-into-main, not ATTACH-at-query.** `attach` bulk-copies map rows
into the repo's graph.db (tier column) via SQLite ATTACH + INSERT..SELECT once.
Rationale: the resolver resolves at insert time against one queryer; UNIONing
external DBs would thread through every query and the write path. The map FILE
remains the distributable artifact; the repo index is its materialization.
*Alternative:* query-time ATTACH — cleaner dedup, far more invasive; revisit
if map sizes hurt.

**D2 — Tier is provenance + priority, content is truth.** `symbols.tier`:
0 = project, 1 = dep. Resolution ordering: qualified tier-0 > qualified
tier-1 > plain tier-0 > plain tier-1, deterministic within each. Dep rows have
namespace + version populated; project rows namespace '' (v1).

**D3 — Symbols-only parse mode.** Adapters expose defs-only parsing (skip
call/dep emission — cheapest correct implementation: parse then drop, or a
mode flag threaded to skip collection; flag preferred for speed on huge
trees). Dep symbols therefore never source edges; caller-attribution purity is
structural.

**D4 — Overlay = current content always wins, per file.** A `depfiles` table
records (path, namespace, version, hash) for every file a map covers. Covered
in-tree files join the fresh-on-query walk when the covered-file count is
under a threshold (default 25k — Go vendor scale passes, node_modules scale
doesn't); above threshold, verification runs at attach/build only (documented;
`codeindex build` always re-verifies). On hash mismatch: re-parse that file
defs-only, replace its dep-tier rows (namespace/version retained, provenance
shows `[modified]`). On restore, content re-parses back to map-equivalent
rows. No hidden state: git is the undo.

**D5 — Auto-generation where the ecosystem hands us the metadata.** Go:
`vendor/modules.txt` (module path + version per vendor dir). PHP:
`composer.lock` (+ `vendor/<vendor>/<pkg>`). Node/Python: manual `depmap`
command in v1 (lockfile auto-gen is mechanical follow-up). Namespace recorded
per map; per top-level module = one map = one cache key `ns@version`.

**D6 — Provenance display.** Resolved-to-dep targets render
`name  path:line  [dep ns@ver]` (or `[dep ns@ver modified]` when overlaid) in
callees/impact; `deps`/`dependents` unchanged. The trust instruction's
"[ambiguous] means verify" discipline extends: dep markers are informational,
not verification demands.

**D7 — Equivalence gate scoping.** incremental==full compares the PROJECT
tier exactly as today (bench builds don't attach maps — unchanged
comparisons). Dep-tier consistency has its own check: attach + overlay +
restore round-trip yields map-equivalent rows (scripted test). Rationale: the
full-rebuild path deliberately doesn't re-generate maps.

**D8 — Measured acceptance.** (1) unresolved-call share on kubernetes with
vendor maps attached and laravel with composer maps — report the drop against
the baseline table; (2) hacked-dep scenario test (edit vendor file → impact
reflects it → restore → map content returns); (3) all existing engine gates
re-run green; (4) query p95 with maps attached stays in budget.

## Risks / Trade-offs

- **Vendor walk cost at query time** → threshold + fast path; k8s vendor
  (4,161 files) adds ~single-digit ms of stats; node_modules excluded by
  threshold with attach-time verification (honest doc).
- **Index growth** → symbols-only keeps it a fraction of full indexing;
  recorded against the size bound; interning (core 9.4) remains the lever.
- **Same-name collisions across deps** → namespace keeps addresses canonical;
  unqualified matches surface all tiers' candidates deterministically with
  ambiguity flags, project first.
- **Map staleness vs dep upgrades** → version key mismatch on attach (lockfile
  says v1.10, map says v1.9) → regenerate; attach refuses silently-stale maps.

## Migration Plan

Schema v4 auto-rebuild (existing mechanism). Repos without maps behave exactly
as today. Rollback: revert binary; maps are ignored by older schemas.

## Open Questions

- Namespace-qualified anchor grammar (`ns::Type.method`) — deferred until the
  import-aware follow-up (PHP's `::` collision with our anchor syntax needs a
  considered answer, not a v1 hack).
- Whether depmap generation should parallelize across modules — measure first.

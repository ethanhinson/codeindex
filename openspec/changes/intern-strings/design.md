## Context

Read queries reference symbols/edges TEXT columns in 34 places across
resolution (hot: resolve()'s ladder probes), traversal, depmaps, and search.
Write statements number 9. SQLite flattens simple views, so predicates like
`name=?` against a view column backed by `strs` resolve via the UNIQUE index
into an int lookup on the base table.

## Goals / Non-Goals

**Goals**: laravel ≤2× source; every repo's index smaller in absolute bytes;
zero read-query rewrites; resolution semantics byte-identical (equivalence
gate + unchanged Snapshot keys).
**Non-Goals**: schema migration (rebuild is the mechanism); signature
compression; files/merkle dedup.

## Decisions

**D1 — Views preserve the read surface.** Physical tables `symbols_t`,
`edges_t` hold int references into `strs`; views named `symbols` and `edges`
reconstruct the original TEXT columns (including `id`). All existing SELECTs
— and their ORDER BY determinism — survive verbatim. Writes cannot go through
views: the 9 write statements move to the base tables.

**D2 — One shared strs table, empty string interned.** file/name/parent/
namespace/kind on symbols; src_file/dst_name/dst_qualifier/dst_ns/kind/
confidence on edges. "" gets a real id so inner joins never drop rows and
NOT-NULL semantics hold.

**D3 — Store-level intern cache.** `strs` is append-only for the life of a
schema generation; a map[string]int64 on Store (reset on schema init) makes
interning O(1) amortized; misses do INSERT OR IGNORE + SELECT.

**D4 — Planner risk is measured, with a recorded fallback.** If bench shows
view-flattening regressing the resolve() ladder or query p95, the fallback is
explicit two-step lookups (intern-lookup then int-indexed probe) in resolve()
only. Decision by measurement, not speculation.

**D5 — Int indexes replace TEXT indexes.** symbols_t(name_id),
(name_id,parent_id), (name_id,namespace_id), (file_id); edges_t(dst_name_id),
(src_file_id), (src_symbol_id), (dst_symbol_id). B-trees over ints are the
bulk of the size win beyond row shrinkage.

## Risks / Trade-offs

- **View flattening fails somewhere** (e.g. aggregate over view) → SQLite
  materializes; caught by the six-repo bench gates.
- **Depmap artifacts carry the old schema** → existing version-mismatch
  regeneration covers it; verified by depmap tests.
- **Interning adds insert-time lookups** → amortized by the cache; cold-build
  budget gates it.

## Migration Plan

v7 bump; delete-and-rebuild on open (existing mechanism). Depmap caches
regenerate on version mismatch.

## Open Questions

None — fallback for the only open risk (planner) is pre-registered.

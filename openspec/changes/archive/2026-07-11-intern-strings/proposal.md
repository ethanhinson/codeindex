## Why

The index stores every string inline: each of kubernetes' ~550k edges repeats
its source path, target name, qualifier, namespace hint, kind, and confidence
as TEXT, and TEXT indexes duplicate them again. Recorded deviation: laravel's
index is 3.28–3.5× its source (budget ≤2×, core-indexing-engine task 9.4
REQUIRED); kubernetes carries 227.9 MB. Deep PHP namespaces and long repo
paths make this structural, not incidental.

## What Changes

Schema v7 interns repeated strings into one `strs(id, s UNIQUE)` table.
`symbols` and `edges` become **views** that reconstruct the TEXT columns from
interned base tables (`symbols_t`, `edges_t`) — the entire read surface (34
queries across resolution, traversal, depmaps, search) keeps working verbatim;
only the 9 write statements change to intern-and-insert. Re-resolution updates
target the base table directly.

Interned: symbols file/name/parent/namespace/kind; edges src_file/dst_name/
dst_qualifier/dst_ns/kind/confidence. Not interned: signatures (mostly
unique), files/merkle paths (trivial share of bytes).

**Validation (pre-registered)**: incremental==full on all six pinned repos;
index bytes vs the recorded v6 baselines (laravel must reach ≤2× source by
the bench's own ratio definition); cold build and query p50/p95 within
existing budgets (views add joins — measured, with a recorded fallback to
explicit two-step id lookups in resolve() if the planner degrades); depmap
artifacts regenerate on version mismatch via the existing mechanism.

Non-goals: merging files/merkle tables; compressing signatures; any change to
resolution semantics (byte-identical Snapshot() content keys).

## Capabilities

### New Capabilities
- `index-compaction`: interned string storage behind view-preserved read
  compatibility, with measured size/latency gates.

### Modified Capabilities
None at requirement level (core-indexing-engine's size budget becomes MET
instead of deviated).

## Impact

- Schema v7 (delete-and-rebuild on version mismatch, as always); depmap cache
  artifacts regenerate.
- internal/graph store.go/depmaps.go write paths; no query-surface changes.
- bench/engine/FINDINGS-interning.md records sizes before/after.

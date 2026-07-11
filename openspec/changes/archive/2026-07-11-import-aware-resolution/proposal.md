## Why

The remaining precision ceiling is cross-package project ambiguity: after
lexical qualification and depmaps, kubernetes still carries ~355k ambiguous
call edges (dozens of packages each defining `New`/`Get`/`Run`/`validate`),
laravel ~64.6k, prometheus ~25.6k. Every `[ambiguous]` flag invites a
verification read. The evidence to fix it is already in the index: each file's
location implies its scope, and the import edges shipped in `dependency-edges`
say exactly which names a file binds from where. No type inference — the same
lexical discipline as everything else.

## What Changes

**Stage 1 — scope-preference resolution (no adapter changes).**
- Project symbols gain a derived `namespace`: Go = directory (≈ package),
  Python = dotted module path from the file path, TS/JS = the file path, PHP =
  the declared `namespace X;` when capturable (else directory).
- The resolution ladder inserts scope steps between qualified and global:
  qualified → **same-file** → **same-namespace** → global-project → dep tiers.
  Deterministic as always; a caller's unqualified `validate()` resolves to its
  own package's `validate` instead of flagging 40 candidates.
- **Measure immediately**: ambiguous-edge reduction per repo vs the recorded
  baselines. Stage 2 proceeds only against the measured residue.

**Stage 2 — import-binding resolution (contingent on Stage-1 residue).**
- Import edges carry their source (module specifier / import path / use-path)
  in a new `dst_ns` column; Go selector calls through an import alias carry a
  namespace hint (`util.Foo()` → the aliased import path).
- The ladder gains an import-bound step: a called name that the calling file
  imports resolves within the import's mapped namespace (TS relative
  specifiers → files; Go internal paths → directories by suffix; Python
  modules → dotted paths; PHP use-paths → declared namespaces).
- Re-resolution becomes per-(name, qualifier, source-file) where import
  context matters, preserving incremental==full.

**Validation (pre-registered)**: incremental==full on all six pinned repos at
each stage; ambiguity metric recorded per stage in
`bench/engine/FINDINGS-import-resolution.md`; qualified-anchor and v5/v7
behavior spot-checks unchanged; full bench re-run (query p95 within budget).
No paid agent A/B: precision improvements reduce `[ambiguous]` verification
pressure along the already-validated caller-attribution shape (v5 precedent);
the engine metric is the honest measure.

Non-goals: type inference; alias tracking through re-exports/barrel files
beyond one hop; wildcard imports (`from x import *`, `use A\B\{...}` groups
resolve per-clause only); cross-repo namespaces.

## Capabilities

### New Capabilities

- `scoped-resolution`: derived project namespaces, the scope-preference
  ladder, import-binding with persisted sources and hints, per-file
  re-resolution correctness, and the staged ambiguity measurements.

### Modified Capabilities

None at requirement level (refines `symbol-graph` resolution behavior strictly
within its confidence contract — matches can only become more precise).

## Impact

- Schema v5: `edges.dst_ns`; project symbols' `namespace` populated
  (previously dep-only); auto-rebuild on first touch.
- `internal/graph` resolve ladder + re-resolution grouping; PutFile derives
  namespaces; Go/TS/Python/PHP adapters touched only in Stage 2 (alias/source
  capture; PHP namespace declarations in Stage 1 if the grammar cooperates).
- Baselines for the metric are already recorded (precision-results.json +
  FINDINGS-depmaps).

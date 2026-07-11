# Dependency edges (imports/extends/implements) — findings

**Date:** 2026-07-10 · **Change:** `dependency-edges`

## What shipped

All four adapters emit dependency edges lexically: Go import specs (verbatim
paths) + struct embedding; TS/JS named+default imports, extends, implements
(class + interface); Python imports/from-imports + class bases; PHP `use`
imports, extends, implements, in-class trait use. Stored as typed edges in the
existing table; imports are file-level (`src_symbol_id=0` + `src_file`).
New queries: `dependents` / `deps` (CLI + MCP tool); `/impact` composes a
dependents section; coverage line now: "call + import/extends/implements
edges; type-usage references not included." Schema v3 (bump forces full
repopulation so unchanged files gain dep edges).

## Correctness

incremental == full rebuild passes on ALL SIX pinned repos with dependency
edges and file-level sources in the snapshot.

## Spot-checks (recorded)

- PHP: `dependents ServiceProvider` (laravel) → **103 extends** (the real
  subclass inventory).
- Go: `dependents codeindex/internal/graph` (this repo) → 16 importing files;
  last-segment mode (`dependents graph`) matches identically.
- TS: `dependents HttpException` (nest) → **64 extends** (the exception
  hierarchy).
- `/impact` composes: counts lead with defs/callers/callees/dependents.

## Index size — recorded deviation

With import edges added: kubernetes **1.98×** ✓, prometheus **2.01×** (at
bound), **laravel 3.28× — exceeds the ≤2× bound**. Cause: PHP's high symbol
density + one import edge per `use` statement, against comparatively small
source bytes. Justification for proceeding: the bound's purpose is
regression visibility, the measurement is recorded here and in
`precision-results`-style tracking, and the known lever (string interning +
`file_id` normalization, core-indexing-engine tasks 2.1/9.4) is unstarted —
it should recover well over the overage since dst_name/src_file TEXT
duplication dominates. Revisit the bound after interning lands.

## Known limits (by design)

- Go implicit interface satisfaction not captured (needs type checking).
- Module aliasing/path mapping not resolved; TS `import * as ns` skipped.
- `references` (type-usage) edges remain future work — disclosed in impact.
- No paid agent A/B: extends the validated impact query shape
  (language-adapters precedent); engine-level proof + spot-checks recorded.

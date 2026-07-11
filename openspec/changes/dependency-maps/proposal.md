## Why

20–47% of call edges are unresolved (measured across the six pinned repos),
and a large share point into dependencies we deliberately don't index —
`impact`/`callees` on any dep-touching code stops at a name. Meanwhile the
common dev reality cuts both ways: deps are pinned and immutable *almost*
always, but developers hack up a vendored dep to test a bug fix and expect
the index to follow. The design that serves both: **versioned, importable,
symbols-only dependency maps** — cached per `module@version`, addressed by
each language's native namespace scheme — **verified per-file by content hash**
so local modifications overlay the static map automatically.

## What Changes

- **Namespace addressing (deps tier)**: dep symbols carry the language-native
  namespace (Go import path, PHP namespace, TS module specifier, Python module
  path). Project symbols are unchanged in v1 (import-aware project resolution
  is a follow-up change, decided).
- **Depmap artifact**: `codeindex depmap <src-dir> --namespace <ns> --version
  <v> -o <map.db>` generates a symbols-only map (definitions with parents +
  signatures + per-file content hashes; NO edges — dep internals are not our
  refactoring surface). Maps are cacheable per `module@version` and shareable.
- **Attach**: `codeindex attach <repo> <map.db>` imports map rows into the
  repo index as the dep tier; auto-generation from in-tree vendor metadata for
  Go (`vendor/modules.txt`) and PHP (`composer.lock` + `vendor/`); manual
  command covers everything else (node_modules, site-packages) in v1.
- **Resolution tier priority**: project symbols always beat dep symbols on
  collision (qualified-project > qualified-dep > plain-project > plain-dep,
  deterministic). Dep symbols are resolution TARGETS only — they have no
  outgoing edges, so they can never appear as callers or pollute
  caller-attribution.
- **Change tracking / overlay (the hacked-dep case)**: attached maps record
  per-file hashes; covered in-tree vendor files join change detection. When a
  dev edits `vendor/.../entry.go`, that file re-parses locally and shadows the
  map's rows — `impact`/`callers` reflect the hack immediately; restoring the
  file restores map-equivalent content. Per-query vendor re-check applies
  under a file-count threshold (Go vendor scale); above it (node_modules
  scale), verification runs at attach/build time — documented honestly.
- **Provenance in output**: dep-resolved targets display `[dep <ns>@<ver>]`.
- **Metric**: unresolved-call share before/after on kubernetes (Go vendor) and
  laravel (composer), plus a scripted hacked-dep scenario test.

Non-goals (v1): import-aware resolution of project code (follow-up change);
stdlib maps; node/python auto-generation from lockfiles; namespace-qualified
anchor grammar beyond what exists (`Type.method`); distributing maps over a
network (the artifact format enables it; transport is future).

## Capabilities

### New Capabilities

- `dependency-maps`: depmap generation/attach, namespace metadata, tiered
  resolution with project priority, hash-verified per-file overlay for locally
  modified deps, provenance display, and the measured unresolved-share
  improvement.

### Modified Capabilities

None at requirement level (extends resolution behavior additively; project-
tier behavior and all existing gates unchanged — verified by re-running them).

## Impact

- Schema v4: `symbols.namespace`, `symbols.tier`, a `depfiles` table (per-file
  hash + namespace + version); auto-rebuild on version bump.
- Adapters gain a symbols-only parse mode (skip calls/deps emission).
- CLI: `depmap`, `attach`; walk covers attached in-tree vendor under the
  threshold; resolver tier ordering; impact/callees provenance markers.
- Index grows by attached maps (symbols-only: k8s vendor est. tens of MB);
  recorded against the size bound with the interning caveat already on file.

## Why

`/impact` honestly discloses its gap in every answer: "coverage: call edges
only — import/type dependents not included." Dependents/blast-radius was part
of the originally-validated winner set (branch-out from a known anchor), and
type-level dependencies — who extends this class, who imports this module —
are exactly the edges refactoring impact needs and grep answers poorly at
scale. All four adapters already walk the syntax trees where these facts sit;
the edges table was designed for these kinds from day one.

## What Changes

- **Adapters emit dependency edges** (lexical only, same discipline as calls):
  - Go: `import` specs (path stored verbatim — packages, not symbols) and
    struct embedding (extends-like).
  - TS/JS: named imports (`import {X} from ...` → per-symbol edges, resolvable
    in-repo), `class A extends B`, `implements I`, `interface A extends B`.
  - Python: `import x` / `from x import y`, class bases (`class A(B)`).
  - PHP: `use A\B\C;` statements, `extends`, `implements`, in-class trait `use`.
- **Storage**: new edge kinds `imports`/`extends`/`implements` in the existing
  edges table; file-level import edges carried with `src_symbol_id=0` +
  `src_file` (the schema supported this shape already); extends/implements
  originate from the class symbol and resolve name-based with the existing
  qualifier machinery. Schema version bump (v3) → auto-rebuild.
- **Queries**: `dependents <anchor>` (who imports/extends/implements X) and
  `deps <anchor>` (what X imports/extends/implements) on CLI and as MCP tools;
  Go import paths match exact or by last path segment (documented). `/impact`
  folds dependents in and its coverage line now states call + dependency
  edges.
- **Validation**: per-adapter fixture tests; incremental==full on all six
  pinned repos; per-language dependents spot-checks (who extends a laravel
  class; who imports `internal/graph` in this repo; who imports a flask
  helper; who extends a nest class). **No paid agent A/B**: this extends the
  already-validated impact query shape; engine-level proof suffices (same
  precedent as language-adapters), and surfaces citing measured savings keep
  their existing qualifiers.

Non-goals: Go implicit interface satisfaction (needs type checking), module
path resolution/aliasing (TS path mapping, Python packages), `references`
edges (field/param type usage — future), transitive dependents closure.

## Capabilities

### New Capabilities

- `dependency-edges`: the four adapters' import/extends/implements extraction,
  typed-edge storage with file-level sources, the dependents/deps query
  surface (CLI + MCP), the impact composition update, and the per-language
  validation evidence.

### Modified Capabilities

None at requirement level (`graph-queries`' dependencies/dependents
requirement in `core-indexing-engine` is implemented by this change).

## Impact

- Schema v3 (auto-rebuild on first touch — measured rebuilds 96 ms–31.7 s).
- All four adapters + `ParsedFile` gains a deps list; store PutFile/snapshot
  extended; `internal/query`, CLI, MCP server gain dependents/deps.
- Index grows (import edges are numerous); re-measure sizes during bench.
- Satisfies `core-indexing-engine` task 8.3 and closes `/impact`'s disclosed
  coverage gap; sets up transitive blast-radius and batch-edit work.

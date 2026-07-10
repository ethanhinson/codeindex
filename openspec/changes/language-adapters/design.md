## Context

The adapter seam exists and is proven: `internal/adapter` defines
`Adapter{Extensions() []string; Parse(path, src) (*graph.ParsedFile, error)}`
with an extension→adapter registry; `internal/adapter/golang` (~200 lines) is
the reference implementation; the engine, resolver, store, freshness, plugin,
and MCP layers are all adapter-agnostic **except** two hard-coded `.go`
filters (`merkle.Walk`, `engine.CountLines`) and the Go-only wording in the
consumption surfaces. Grammar support ships inside the already-pinned
`smacker/go-tree-sitter` module as subpackages.

## Goals / Non-Goals

**Goals**

- TS/JS, Python, PHP adapters extracting symbols + name-based call sites,
  each tested against fixtures and proven with the existing
  incremental==full-rebuild bench on a real pinned repo.
- Registry-driven walk so future adapters require zero walk changes.
- Consumption surfaces (hook, note, MCP descriptions, READMEs) truthfully
  updated.

**Non-Goals**

- Precise/scope-aware resolution (deferred; name-based + confidence, as
  measured and shipped for Go).
- .NET/C# (dropped by user). Framework-specific extraction (decorator routes,
  etc.) — that is the future "boundary anchors" work.
- Re-running agent A/B gates per language (mechanics are language-independent;
  measured numbers stay labeled as Go-derived where cited).

## Decisions

**D1 — Same contract as Go: name-based symbols + call edges only.** Each
adapter extracts definition symbols (name, kind, signature line, span) and raw
call sites attributed to the innermost enclosing symbol (byte-range method,
identical to Go). Resolution stays in the store — adapters never resolve.
*Alternative:* per-language cleverness (imports, decorators) — rejected;
that's the deferred resolution change, and mixing it in would make the
per-language correctness proof ambiguous.

**D2 — Symbol kinds mapped per language, minimally.**
- TS/JS: `function_declaration`/`function` → func; `method_definition` →
  method; `class_declaration` → type; `lexical_declaration` binding an
  `arrow_function`/`function` at top level or class-static level → func
  (named by the bound identifier). Anonymous callbacks are NOT symbols.
- Python: `function_definition` → func (→ method when lexically inside a
  `class_definition`); `class_definition` → type. Lambdas are NOT symbols.
- PHP: `function_definition` → func; `method_declaration` → method;
  `class_declaration`/`interface_declaration`/`trait_declaration` → type.
*Rationale:* mirrors what agents anchor on; keeps ambiguity behavior identical
to the validated Go semantics.

**D3 — Callee-name extraction takes the FINAL name, like Go.** `a.b.c(x)` →
callee `c`; `Foo::bar()` (PHP) → `bar`; `self.save()` (Python) → `save`;
`obj.method()` (TS) → `method`. Name-based resolution then applies unchanged,
`[ambiguous]` flags included. *Alternative:* qualified callee names — belongs
to the resolution change (needs `parent_id`).

**D4 — Registry-driven walk.** `merkle.Walk` asks the adapter registry for its
extension set (new `adapter.Extensions()` accessor) instead of testing
`.go`. Skip-list (vendor, node_modules, testdata, .git, .codeindex) unchanged;
`node_modules` exclusion now actually matters for TS repos. `engine.CountLines`
follows the same source. *Alternative:* per-adapter walks — needless.

**D5 — Grammar sourcing: smacker subpackages, pinned by the existing module.**
`smacker/go-tree-sitter/{javascript,typescript/typescript,typescript/tsx,python,php}`.
TSX gets the tsx grammar; `.ts` the typescript grammar; `.js`/`.jsx` the
javascript grammar — one adapter, three grammars selected by extension.
*Alternative:* official tree-sitter bindings per grammar — new modules, no
benefit at our pin.

**D6 — Per-language correctness proof reuses `codeindex bench`.** The
incremental==full-rebuild diff (already language-agnostic: it diffs normalized
symbol/edge content) runs against one pinned real repo per language: nest
(TS, pinned), flask (Python, to pin), laravel/framework (PHP, to pin). Numbers
recorded in `bench/engine/`. *Alternative:* trust the Go proof — no; each
grammar exercises different node shapes and span logic.

**D7 — Truthful surface updates.** Hook extension set and note gating become
registry-consistent (`.go .ts .tsx .js .jsx .py .php`); note/MCP wording says
"Go, TS/JS, Python, PHP"; anywhere the −73%/−62% numbers are cited keeps the
"measured on Go repos" qualifier. *Rationale:* the project's evidence
discipline — claims match measurements.

## Risks / Trade-offs

- **Grammar node-type drift across languages** (e.g. PHP grammar names,
  TS arrow-function shapes) → fixture tests per adapter assert exact expected
  symbols/edges; build fails loudly, not silently.
- **TS/JS anonymous/arrow-heavy codebases under-index** (callbacks aren't
  symbols) → accepted; matches the anchor-based product (agents anchor on
  named things); noted in README.
- **Polyglot repos get bigger indexes/builds** → expected; the ≤2× size and
  per-tier build budgets still apply and get re-measured on nest/flask/laravel.
- **Python decorators / PHP magic methods produce odd signatures** → signature
  is display-only (first line, clipped); no correctness impact.
- **smacker grammar staleness for newest syntax** (e.g. TS 5.x niches) →
  tree-sitter degrades gracefully (ERROR nodes skip extraction for that
  subtree); acceptable for name-based indexing; revisit at the resolution
  change.

## Migration Plan

Additive. Existing Go indexes stay valid; first query on a polyglot repo
patches in the newly-walkable files incrementally (added-files path already
proven). No schema change. Rollback = removing adapter registrations.

## Open Questions

- Whether `.jsx` should use the tsx grammar instead of javascript (tsx is a
  superset; decide at implementation by fixture behavior).
- Whether PHP short-open-tag files matter (default: standard `<?php` only).

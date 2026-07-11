## Context

Resolution today is purely name-based: `resolveName(name)` → 0 hits unresolved,
1 unambiguous, >1 ambiguous (deterministic first + `[ambiguous]`). Measured
consequence: laravel `firstOrCreate` = 4 defs / 54 flagged callers; every flag
invites a verification read (the v3 failure mode). The lexical facts needed to
collapse most of these are already in the syntax tree and were being discarded:
Go declares receiver types; `this`/`self`/`$this` can only mean the enclosing
class; PHP scoped calls name their class. No type inference required.

Constraints carried forward: single binary, no language toolchains, resolution
lives in the store (adapters emit facts, never resolve), incremental==full
equivalence is the gate on every schema/resolver touch, and the index stays a
derived artifact.

## Goals / Non-Goals

**Goals**

- Methods carry their parent type; call sites carry a lexical qualifier hint.
- Qualifier-validated resolution upgrades matches to unambiguous; everything
  else falls back to current behavior (strictly never worse).
- Qualified names in output and accepted as anchors (`Builder.firstOrCreate`).
- Re-test: precision metric (ambiguous-edge reduction per repo), engine proofs
  re-run, agent A/B v5 on laravel.

**Non-Goals**

- Type inference, import/alias resolution, inheritance edges,
  `super()`/`parent::`/`static::`-late-binding semantics (skip qualifier for
  those), variable-type tracking beyond method receivers.

## Decisions

**D1 — Parent stored as TEXT (`symbols.parent`), not a rowid.** Same rationale
as the proven `dst_name` pattern: content keys survive per-file replacement and
keep the incremental==full snapshot comparison id-independent. `parent_id`
rowids would re-introduce exactly the id-coupling the skeleton avoided.
(Deviation from the `symbol-graph` spec's `parent_id` sketch — recorded there.)
Qualified name = `parent + "." + name` when parent is non-empty.

**D2 — Parent attribution per language (all lexical):**
- Go: `method_declaration` receiver's type node, `*` stripped (`func (w *Widget)
  Grow()` → parent `Widget`).
- TS/JS: `method_definition` → innermost enclosing `class_declaration` name.
- Python: `function_definition` inside `class_definition` → that class (already
  tracked for kind; now carries the name).
- PHP: `method_declaration` → enclosing class/interface/trait name.

**D3 — Qualifier is a HINT the resolver validates; extraction only where
lexically meaningful:**
- `this.x()` / `self.x()` / `cls.x()` / `$this->x()` → enclosing class name.
- Go: calls through the receiver variable of the enclosing method
  (`w.scale()` inside `func (w Widget) ...`) → `Widget`.
- PHP `Foo::bar()` (scoped): scope name's final segment; `self::`/`static::`
  map to the enclosing class; `parent::` emits no qualifier.
- TS/Python `Foo.bar()` where the receiver is a bare identifier starting with
  an uppercase letter → candidate qualifier `Foo` (convention-based hint;
  wrong hints cost nothing because of D4's fallback).
- Everything else (arbitrary expressions, lowercase receivers in TS/Py, plain
  selector calls in Go): no qualifier — today's behavior.

**D4 — Resolution: qualified-first with total fallback.**
`resolve(name, qualifier)`: if qualifier ≠ "" and symbols exist with
`name=? AND parent=?` → exactly 1: **unambiguous**; >1 (same class name in
multiple files): deterministic first + ambiguous. If 0 qualified hits, fall
back to plain name-based resolution unchanged. Edges persist `dst_qualifier`
so inbound re-resolution recomputes identically; the affected-names trigger
stays keyed on names (a qualifier is a refinement within a name).

**D5 — Schema versioning by `PRAGMA user_version`; mismatch = rebuild.** The
index is a derived artifact; a migration framework would be machinery for
nothing. `graph.Open` checks the version; on mismatch it deletes the db and
recreates (measured full rebuilds: 96 ms–3.8 s on our pinned repos; kubernetes
31.7 s worst case). ensureFresh then repopulates naturally.

**D6 — Qualified output and anchors.** Caller/callee names display as
`Parent.name` when a parent exists; `def` lines gain the qualified name.
Anchors: `Type.method` / `Type::method` split into (qualifier, name); queries
filter definitions to `parent=qualifier` and callers/callees to edges resolving
into that filtered set. Unresolved edges (name only) cannot match a qualified
anchor — documented, and the unqualified query still shows everything.

**D7 — Equivalence stays the gate.** `parent` joins the symbol snapshot string;
`dst_qualifier` and the resolved target's parent join the edge snapshot string.
`codeindex bench`'s incremental==full check re-runs on all four languages'
pinned repos before the change closes.

**D8 — Re-test design.**
- *Precision metric*: SQL over each pinned repo's index before/after —
  count of `calls` edges by confidence; report absolute + % reduction in
  `ambiguous`, per repo (kubernetes, nest, flask, laravel).
- *Agent A/B v5*: laravel caller-attribution task set generated from the new
  index (unambiguous unique targets AND qualified-anchor tasks), plugin arm,
  reps 2, budget $10, pre-registered expectation: boundary holds off-Go
  (median branch-out savings ≥ 30%) with success parity. Grading reuses the
  callattr grader (names normalized to final segment so qualified output stays
  compatible).

## Risks / Trade-offs

- **Convention-based TS/Py qualifier hints misfire** (uppercase variable
  receivers) → harmless by construction: validation-with-fallback means a bad
  hint degrades to today's behavior, never below it. Verified by tests.
- **Same-named classes in different files** (PHP fixtures galore) → qualified
  set >1 keeps the ambiguous flag; deterministic ordering preserved.
- **Output format change breaks downstream parsers** → plugin hook regexes and
  the A/B grader normalize on the final name segment; both updated + tested in
  this change.
- **Schema rebuild surprises users mid-session** → rebuild is logged to stderr
  by the CLI (`schema v1→v2: rebuilding index`), budget-bounded by measured
  build times.
- **Go receiver-var tracking misses shadowing** (`w := other` inside the
  method) → accepted; shadowed receivers are rare and the failure mode is a
  fallback to name-based, not a wrong answer... verified in tests that
  shadowing does not produce a *wrong unambiguous* edge (hint validation only
  upgrades when the parent actually defines that method — a shadowed variable
  calling a same-named method on another type that ALSO exists on the receiver
  type resolves to the receiver type: wrong-but-flagged risk accepted and
  documented as the known lexical limit).

## Migration Plan

Schema `user_version` 1→2; existing indexes rebuild on first open (automatic,
logged). No user action. Rollback: revert the binary; old binaries rebuild the
v1 schema the same way.

## Open Questions

- Whether `impact`/`callers` should also accept `file.go:Line` anchors —
  deferred (unchanged from skill-and-ide-integration).
- Whether Python `cls.method()` inside `@classmethod` should qualify to the
  class (yes — same enclosing-class rule; noted here for the implementer).

## Why

Ambiguity is the tool's measured quality ceiling. Name-based resolution flags
every same-named match `[ambiguous]` — laravel's `firstOrCreate` resolves to 4
definitions with 54 flagged callers; every flag invites the agent to spend a
verification read (the exact re-verification loop that sank the v3 gate), and
dynamic languages (PHP, Python, TS) collide far more than Go. A large share of
these collisions are resolvable **lexically** — no type inference, no language
toolchains: `$this->scale()` inside class `Widget` can only be `Widget::scale`;
Go declares the receiver type in the method signature; `Foo::bar()` names its
class outright. Pulling this lever cuts `[ambiguous]` noise at the source and
enables qualified anchors (`Builder.firstOrCreate`) so agents can ask precise
questions — then we re-test at both the engine and agent level.

## What Changes

- **Schema (versioned)**: `symbols.parent_id` (methods know their
  class/receiver type), `edges.dst_qualifier` (the lexical qualifier a call
  site carried, retained for correct re-resolution). A `user_version` pragma
  guards the schema: on mismatch the derived index is rebuilt automatically
  (it is a regenerable artifact — no migration machinery).
- **Adapters (all four languages)**: methods report their parent type — Go
  from the receiver declaration (`func (w Widget) Grow()` → parent `Widget`),
  TS/Python/PHP from the lexically enclosing class/interface/trait. Call sites
  carry a qualifier when lexically certain: `this`/`self`/`$this` → the
  enclosing class; PHP `Foo::bar()` scoped calls; TS/Python `Foo.bar()` where
  the receiver identifier names a known type (attempted with fallback).
- **Resolver**: qualifier-aware — a call with qualifier `Q` and name `n`
  resolving to exactly one symbol `n` with parent `Q` is **unambiguous**;
  otherwise resolution falls back to today's plain name-based behavior
  (never worse than current).
- **Query surface**: definitions and callers display qualified names
  (`Widget.grow`); `callers`/`callees`/`impact`/MCP accept qualified anchors
  (`Type.method` or `Type::method`) that filter to the matching parent.
- **Correctness preserved**: parent and qualifier join the normalized snapshot
  so the incremental==full-rebuild equivalence proof still gates everything;
  inbound re-resolution re-applies qualifiers (not just names).
- **Re-test, three levels**:
  1. **Precision metric**: per-repo ambiguous-edge counts before/after on
     kubernetes, nest, flask, laravel — the direct measure of the lever.
  2. **Engine proofs**: `codeindex bench` re-run on all four languages'
     pinned repos (incremental==full must still pass).
  3. **Agent A/B v5** (~$6, plugin arm): caller-attribution tasks on laravel —
     the worst-ambiguity language — validating the v2/v4 boundary holds off-Go
     and that precision shows up in cost/accuracy.

Non-goals: type inference, import/module resolution, cross-file alias
tracking, `extends`/`implements` edges (still the future dependents change),
`super()`/`parent::` resolution (needs inheritance edges).

## Capabilities

### New Capabilities

- `lexical-resolution`: parent attribution in all adapters, the versioned
  schema additions, qualifier-aware resolution with fallback, qualified
  display and qualified anchors across CLI/MCP, snapshot equivalence, and the
  three-level re-test with recorded results.

### Modified Capabilities

None at requirement level. (`symbol-graph`'s "Parent linkage and qualified
names" requirement in `core-indexing-engine` is *implemented* by this change;
its "Name-based resolution with confidence" behavior is strictly refined —
qualified matches upgrade to unambiguous, everything else unchanged.)

## Impact

- Schema bump: existing `.codeindex/graph.db` files rebuild on first touch
  (derived artifact; auto-rebuild measured at 96ms–3.8s on our pinned repos).
- All four adapter packages + `common` helpers; store resolution + snapshot;
  query/MCP output format gains qualified names (consumers see `Widget.grow`
  instead of bare `grow` where a parent exists — plugin/harness parsing
  updated accordingly).
- `bench/agent_ab` gains a laravel caller-attribution task set (v5).
- Satisfies `core-indexing-engine` task 2.1's `parent_id` portion and the
  qualified-names requirement; unblocks the future batch-edit-plan feature
  (which is precision-gated).

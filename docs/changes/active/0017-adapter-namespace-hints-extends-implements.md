---
id: 17
slug: adapter-namespace-hints-extends-implements
title: Attach namespace hints to extends/implements references in the language adapters
status: proposed
priority: high
type: fix
created: 2026-08-22
updated: 2026-08-22
depends_on: []
related: [13, 10]
discovered_from: [16]
adrs: []
spec:
plan:
results:
trivial: false
auto_groomable: false
branch:
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
<!-- docket:artifacts:end -->

## Why

The D7 gate runs that killed change 0016 isolated one structural signal:
on `xsubtypes` tasks ("what extends/implements X across repos") the
workspace index lifted haiku +14pp over the grep control — the only
task shape where it helped — yet scored just 0.43 absolute, capped by a
long-recorded adapter gap: **extends/implements references carry no
namespace hint** (the Go adapter's `addDep` never sets `Source`; other
languages unverified at reference level). Without a hint, the resolution
ladder's import-mediated rung can never fire for subtype edges, so the
index is structurally missing the links its best task shape needs. This
gap also degrades single-repo depmap resolution wherever hints gate
tier-1 attachment.

Fixing it is a prerequisite for the pivot campaign (structural corpus,
change 0010, new gate): without hints, subtype tasks measure the gap,
not the feature. Record in
`bench/engine/FINDINGS-workspace-graph.md` (2026-08-22 entry).

## What changes

- For each language adapter (Go, TS/JS, Python, PHP): where an
  extends/implements/embeds reference is emitted, attach the same
  namespace hint (`Source`/`dst_ns`) the adapter already attaches to
  import and call references, derived from the import binding in scope.
- Verify per language against the bench corpus members (nest for TS
  `extends`/`implements`, symfony/drupal for PHP, prometheus/
  client_golang for Go interface embedding, werkzeug/flask for Python
  subclassing) — the hint must appear on real unresolved subtype edges
  in each member's graph.db.
- Single-repo behavior: hints on unresolved edges are metadata; goldens
  must stay byte-identical (measured, not assumed).

## Out of scope

- Resolver/ladder changes — rung 1 already consumes hints; this change
  only makes subtype edges carry them.
- Workspace query surfaces (killed 0016; revival is the new gate's
  outcome).
- Corpus growth (change 0010).

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

## Auto-groom blocked

**2026-08-22 — `docket-auto-groom` abstained after the adversarial
critic found a structural defect in the central mechanism on re-check.
The permitted revision round was spent, so no spec was emitted.**

### What the groom established (keep this — it is measured, not assumed)

The stub's premise is confirmed for Go and **narrower than assumed
elsewhere**. Verified against the eight built member indexes under
`bench/repos/*/.codeindex/graph.db`:

- `internal/graph/store.go:373` already does `hint := bind[d.Target]`,
  so subtype edges **already** carry `dst_ns` whenever the target name
  equals an imported name. No adapter needs to set `Source` for the
  common case; PHP/TS/Python subtype edges are mostly hinted already
  (symfony 4 679 hinted vs 2 999 not; drupal 8 873 vs 3 440).
- `NO_NS` is not the gap. **2 702 of symfony's 2 999 hintless subtype
  edges resolve correctly anyway** (same-namespace bases needing no
  import). The addressable population is `NO_NS ∧ unresolved` — roughly
  1 050 edges across all eight indexes.
- Within that population the largest slice is **language builtins/SPL**
  (`IteratorAggregate` 45, `Throwable` 42, `Stringable` 35,
  `ArrayAccess` 17, `stdClass` 15 …), which must stay hintless, plus
  dynamic bases (flask's `class X(oldcls)`).
- **Go is a genuine total absence:** `client_golang` has **0** hinted
  subtype edges. Two causes — Go's `KindImports` deps never set
  `Source`, and `embeddedTypeName` discards the package qualifier at
  `case "qualified_type"`. This half of the change is sound and was
  found sound by the critic in both rounds.
- **The PHP/Python/TS defect is aliased imports:** every adapter keys
  the import binding on the *imported* name, never the *local* one
  (`php.go` `break`s before `namespace_aliasing_clause`; `python.go`
  and `tsjs.go` read field `name`, not `alias`). Aliased-`use`
  populations: laravel 839, drupal 314, symfony 289, werkzeug 163,
  flask 92, nest 16. Pre-fix addressable alias class: drupal 202,
  laravel 215, symfony 59, werkzeug 8, flask 4, **nest 0**.
- Two adjacent gaps that are *missing-edge*, not missing-hint, defects
  (verified by parse): `class X(mod.Y)` in Python and `extends ns.Foo`
  in TS emit **no subtype edge at all**; PHP group use (`use A\{…}`)
  emits **no import dep at all** (and occurs zero times in the three
  PHP members). `DumpNormalized` (`store.go:1196`) does not select
  `dst_ns`, so the existing snapshot suites are structurally blind to a
  hint-only change.

### The undecidable decision — why this needs you

**A namespace hint alone cannot fix the aliased case, and the fix that
would is a design call with real cost.**

The resolver keys candidate lookup on the **name**, not just the
namespace: `boundIDs` is `SELECT id, namespace FROM symbols WHERE
name=? AND tier=?` followed by `nsMatch(ns, hint)`, and the name it is
given is the edge's `dst_name` — which, for an aliased subtype, is the
**alias**. For laravel's `use Illuminate\Database\Eloquent\Model as
Eloquent; class X extends Eloquent`, there are zero symbols named
`Eloquent` and one named `Model`. Attaching `dst_ns` makes the edge
*look* fixed while `boundIDs` still returns nothing. The cross-repo path
is the same: rung 1 (`internal/wsresolve/ladder.go:120`) gates on
`lookupDefs(m, e.DstName, …)` — the alias again. So the whole
alias branch of this change would ship **`dst_ns` movement with zero
resolution movement**, which is precisely nothing for the `xsubtypes`
recall the D7 pivot is chasing.

Making it real means carrying the **original symbol name** to the
resolve call, and that is the decision the groom could not safely
default:

- **Option A — rewrite the edge's `dst_name` to the original name.**
  Resolution works everywhere, including rung 1, with no resolver
  change. But the edge then no longer says what the source file
  literally writes; `dst_name` is part of the edge's identity, so this
  is an add+delete in any before/after delta gate, and every consumer
  that displays or matches `dst_name` sees the renamed value.
- **Option B — keep `dst_name` as written and add an "original name"
  alongside it** (a new column plus resolver and `wsresolve` changes to
  try it). Faithful to the source, but it widens the change from an
  adapter fix into a schema + resolver + ladder change, and the stub
  explicitly fences resolver/ladder work out of scope.
- **Option C — ship the Go half only** (which is sound and self-
  contained) and split the alias work into its own change under whichever
  of A/B you pick.

Choosing among these is a schema/altitude call with downstream reach
into the killed-0016 revival path — owner territory, not a conservative
default.

### What a human should supply

1. A ruling between Options A, B and C above (or a fourth).
2. If A or B: whether the resulting `dst_name`/schema change is
   acceptable inside change 0017's stated scope, or wants its own change
   and its own ADR.
3. Confirmation that the acceptance bar should require **resolution
   gain** (`dst_symbol_id: 0 → non-zero` over the computed addressable
   set), not merely `dst_ns` coverage — the groom's draft bar measured
   only the latter and would have certified an inert change.

### Recommendation

**Do not kill this.** The Go half is real, measured, unambiguous, and
already blocks change 0010's subtype-map task shape. The strongest
recommendation this groom may make: **split it** — take Option C, land
the Go qualifier fix as change 0017 (self-contained, no schema
question), and file the alias-resolution work as a separate change
carrying the A-vs-B decision. That unblocks 0010 sooner and keeps the
schema argument out of a fix that does not need it.

Two smaller corrections for whoever picks this up: any future draft's
`NO_NS ∧ unresolved` figure for nest is **17**, not 37; and nest's
addressable alias class is **0**, so TS moves nothing on the current
corpus regardless of the option chosen.

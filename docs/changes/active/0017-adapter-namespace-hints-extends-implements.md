---
id: 17
slug: adapter-namespace-hints-extends-implements
title: Go subtype references carry namespace hints — fix the qualifier discard and KindImports Source
status: proposed
priority: high
type: fix
created: 2026-08-22
updated: 2026-08-22
depends_on: []
related: [13, 10, 18]
discovered_from: [16]
adrs: []
spec:
plan:
results:
trivial: false
auto_groomable: true
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

Owner ruling 2026-08-22 (Option C from the groom's abstain): **this
change is the Go half only** — measured sound in both critic rounds and
self-contained. The aliased-import resolution problem (PHP/Python/TS) is
split out to change 0018, which carries the schema decision.

- Go adapter: set `Source` on `KindImports` deps, and stop
  `embeddedTypeName` discarding the package qualifier
  (`case "qualified_type"`), so Go interface-embedding / subtype
  references carry the namespace hint like calls and imports do.
- **Acceptance bar = resolution gain, not hint coverage** (owner ruling):
  references must move `dst_symbol_id: 0 → non-zero` over the measured
  Go addressable set (client_golang has 0 hinted subtype edges today);
  `dst_ns` movement without resolution movement counts for nothing.
  Verify empirically against the built bench indexes
  (client_golang/prometheus).
- Single-repo goldens byte-identical (measured); note
  `DumpNormalized` does not select `dst_ns`, so snapshot suites are
  structurally blind to hint changes — the resolution-gain check is the
  real tooth.

## Out of scope

- Resolver/ladder changes — rung 1 already consumes hints.
- Aliased-import resolution (PHP/Python/TS) — change 0018 (carries the
  rewrite-dst_name vs add-original-name-column decision).
- The missing-EDGE parse gaps the groom found (Python `class X(mod.Y)`,
  TS `extends ns.Foo` emit no subtype edge; PHP group `use` emits no
  import dep) — recorded on change 0018's territory.
- Workspace query surfaces (killed 0016; revival is the new gate's
  outcome).
- Corpus growth (change 0010).

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

## Groom context (owner rulings 2026-08-22)

The first groom abstained on the alias mechanism (full record in git
history). Owner rulings: **Option C** — Go half here, alias work split
to change 0018; **acceptance bar = resolution gain** over the measured
addressable set, never hint coverage alone. Corrections carried: nest's
`NO_NS ∧ unresolved` figure is 17 (not 37); nest's addressable alias
class is 0.

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


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

## Auto-groom blocked (2026-08-22)

Abstained. The owner's acceptance bar, **as literally written**, is
unsatisfiable by this change as scoped — but the change is **not** worthless,
and the single question below is the only thing standing between this stub and
a spec.

### The undecidable decision

The bar (binding, owner): references must move `dst_symbol_id: 0 → non-zero`
over the measured Go addressable set; `dst_ns` movement without resolution
movement "counts for nothing."

**No hint-only change can ever move `dst_symbol_id` from 0 to non-zero.**
Established three ways, each verified independently by the critic pass:

1. **Analytic.** In `resolve()` (`internal/graph/store.go:1052`) the `nsHint`
   rungs (1068–1073) call `boundIDs` (1101), which runs
   `SELECT id, namespace FROM symbols WHERE name=? AND tier=?` and keeps the
   rows passing `nsMatch`. The ladder's final two rungs (1081–1083) are those
   same two queries **unfiltered**, and they run unconditionally. Every hinted
   rung's candidate set is therefore a strict subset of a rung that runs
   anyway: a hint can change *which* id is picked and can upgrade
   `ambiguous → unambiguous`, but it can never turn no-match into a match. The
   `qualifier` rungs (1064–1066) are strict subsets for the same reason, so an
   alternate design routing the package operand into `Qualifier` instead of
   `Source` cannot escape either (and `store.go:378` passes `""` for qualifier
   on dep edges regardless). The bulk re-resolution pass (`store.go:546`) calls
   the same `resolve()` with the persisted `dst_ns`, so it inherits this.
2. **Empirical.** In the two bench indexes the stub names, **every** unresolved
   subtype edge targets a name that exists as no symbol in the index at all —
   `client_golang` 66 of 66, `prometheus` 30 of 30. Both indexes have zero
   `tier=1` symbols. A hint filters candidates; with zero candidates there is
   nothing to filter.
3. **The workspace path does not rescue it.** `internal/wsresolve/ladder.go`
   exists, is tested, and is wired (`internal/wsfresh/freshen.go:371`); its
   rung 1 (`ladder.go:122–137`, `if e.DstNS != ""` → `memberClaims`) is the one
   place in the codebase where a hint **gates** attachment rather than
   narrowing it — so it is genuinely where a Go hint would become load-bearing.
   But (a) cross edges land in the **overlay**, never in the member's
   `dst_symbol_id`, so a firing rung 1 still does not satisfy the bar as
   written; and (b) measured: **zero** of prometheus's unresolved subtype
   `dst_name`s exist as a tier-0 symbol in `client_golang`, so the bench pair
   yields nothing on this path anyway.

### What the change *does* buy — measured, and it is real

`prometheus` has **25 `ambiguous` subtype edges, all of them Go**, all with an
empty `dst_ns` — precisely the qualified-embedded-type population this change
targets. (`client_golang` has zero ambiguous subtype edges, so the gain is
prometheus-side.) Verified concretely:

- `tsdb/head_read.go:386,512` embed `chunkenc.Chunk`; three tier-0 `Chunk`
  symbols exist (`prompb`, `tsdb/chunkenc`, `tsdb/chunks`), so the edge is
  ambiguous today. A `.../tsdb/chunkenc` hint narrows it to exactly one —
  `ambiguous → unambiguous`.
- `scrape/target.go:320,341,360,400` embed `storage.Appender`; 18 tier-0
  `Appender` symbols exist and the deterministic first pick under
  `ORDER BY file, start_line` is today `cmd/prometheus/main.go` — **the wrong
  symbol**. The hint confines the pick to the `storage` package. This is a
  correctness fix on a currently-wrong `dst_symbol_id`, not merely precision.
- The 5 `Discovery` edges (`discovery/aws/*`, `discovery/ovhcloud/*`) face 22
  same-named candidates; the hint collapses them.

### The one question for the owner

**Does your bar admit `ambiguous → unambiguous` and wrong-pick → right-pick as
"resolution movement," or only literally `dst_symbol_id: 0 → non-zero`?**

The bar's stated contrast is *"`dst_ns` movement without resolution movement
counts for nothing."* An ambiguous edge already has a non-zero
`dst_symbol_id`, so re-pointing it is not literally `0 → non-zero` — but it
changes which symbol the edge resolves to, which is resolution movement and not
bare `dst_ns` movement. Whether the literal phrasing is the whole bar or a
named instance of it is a call only the owner can make: the owner set this bar
explicitly, after a prior abstain, and a groom may not re-bar itself.

**If the answer is "yes, it admits it":** 0017 is buildable today against a
measured 25-edge Go set in `prometheus`, and the spec follows immediately from
the design already worked out below — re-arm and this stub grooms in one pass.
**If the answer is "no, literally 0 → non-zero":** the bar cannot be met by any
hint-only change, and 0017 should be re-scoped (to the overlay-side workspace
rung, where hints do gate) or deferred behind that work.

### Design already established (fold into the spec on re-arm)

The two binding bullets in `## What changes` require **three** edits, not two:

1. `internal/adapter/golang/golang.go`, `import_spec` (~line 108): set
   `RawDep.Source = ipath` on the `KindImports` dep. Requires extending the
   local `addDep` closure (line 59), whose anonymous struct has no source
   field.
2. `internal/adapter/golang/golang.go`, `embeddedTypeName` (line 207) /
   `field_declaration` (line 128): return the `qualified_type` package operand
   instead of discarding it, resolve it through the existing `aliases` map to
   the import path, and set that as the subtype `RawDep.Source`. **`Target`
   must stay the bare type name** (`B`, not `pkg.B`) — it is what the plain
   rungs query and what `DumpNormalized` selects.
3. `internal/graph/store.go:373`: `hint := bind[d.Target]` must become
   `normalizeHint(d.Source, …)` with `bind[d.Target]` as fallback, mirroring
   the calls path at 352–354. **Without this, edits 1 and 2 are inert** —
   `bind` is keyed on `d.Target`, and for Go imports `Target` is the full
   slash-bearing path, which never equals an embedded type name. `Source` is
   the only available channel (`common.DepSite` carries only Kind/Target/
   Source) and line 373 currently discards it.

Edit 3 is a near-no-op for existing edges: for import deps
`normalizeHint(d.Source, d.Target, path)` is by construction the same
expression that populated `bind[d.Target]` at line 343, and for every
non-import dep `Source` is `""` (verified — all five `Source:` assignments in
the four adapters are on `KindImports` DepSites: `python.go:92,97`,
`php.go:102`, `tsjs.go:115,121`). The one behavior delta is two imports in a
file sharing a `Target` with different `Source`s, where `bind`'s
last-write-wins currently gives both edges the second hint.

**Scope note for the owner:** edit 3 brushes the stub's
"Out of scope: Resolver/ladder changes." It is insert-time hint *selection*,
not the ladder — but it is unavoidable, and the owner should confirm it is
admitted.

### Correction to the stub's own `## Why`

The stub asserts the gap "degrades single-repo depmap resolution wherever hints
gate tier-1 attachment." **Hints do not gate tier-1 attachment.** `AttachMap`
(`internal/graph/depmaps.go:81`) attaches by an operator-supplied map path;
hints only narrow *within* already-attached tier-1 rows via `boundIDs`. Both
bench indexes have zero tier-1 symbols. One of the stub's stated motivations is
therefore unfounded and should be struck or restated on re-arm.

Also, for accuracy on re-arm: `client_golang`'s unresolved subtype targets are
stdlib as the prior groom's builtins rule would predict (`io.ReaderFrom`,
`http.Hijacker`/`Flusher`/`CloseNotifier`, `sync.Mutex`, `net.Listener`), but
`prometheus`'s are **not** — `CompleteStrategy`, `PrometheusClient`, `Mock`,
`HTTPClientConfig`, `EC2API` and several TS names are third-party or web-UI,
not stdlib. The conclusion is unaffected (none exist as symbols), but
"it is all stdlib" is not an accurate basis for any decision here.

### Recommendation

**Answer the one question above and re-arm.** Do not kill: the measured 25-edge
prometheus set — including a currently-wrong `storage.Appender` resolution — is
genuine value, and the design is fully worked out. Kill and defer are never
autonomous; this is a recommendation only.

### Groom process note

The adversarial critic pass returned `wrong but fixable from available context`
on the first round; all five required corrections were folded into the record
above and independently re-verified against the code and the bench indexes
before writing. The bounded re-check leg produced no legible verdict on its
return channel (the dispatch completed but surfaced no report), so that leg was
treated as a failed dispatch and the abstain stands on the first-round verdict
plus the applied revision. This note is the re-arm cue for that diagnostic; it
does not affect the substance above.

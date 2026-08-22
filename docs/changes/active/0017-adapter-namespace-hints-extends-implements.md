---
id: 17
slug: adapter-namespace-hints-extends-implements
title: Go subtype references carry namespace hints — fix the qualifier discard and KindImports Source
status: in-progress
priority: high
type: fix
created: 2026-08-22
updated: 2026-08-22
depends_on: []
related: [13, 10, 18]
discovered_from: [16]
adrs: []
spec: docs/superpowers/specs/2026-08-22-adapter-namespace-hints-extends-implements-design.md
plan:
results:
trivial: false
auto_groomable: true
branch: feat/adapter-namespace-hints-extends-implements
pr:
blocked_by:
claimed_at: 2026-08-22T20:07:11Z
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-22-adapter-namespace-hints-extends-implements-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-22-adapter-namespace-hints-extends-implements-design.md) |
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
- **Acceptance bar = disambiguation gain (owner ruling 2026-08-22,
  amending the earlier resolution-gain wording):** `unresolved →
  resolved` is provably impossible for a hint-only change (hints narrow
  candidates; hint-free rungs run last regardless — all 96 unresolved Go
  subtype edges target names absent from the index). The bar: over the
  measured 23 addressable qualified-embed edges, resolution moves
  `ambiguous → unambiguous` with the verified-correct target.
  Pinned exemplars: `chunkenc.Chunk` (wrong-package pick → correct) and
  the 4 `refresh.Discovery` embeds (22 candidates → 1 correct);
  `storage.Appender` is recorded as PARTIAL (right package, still
  ambiguous — 3 same-name symbols in-package) and must not be claimed as
  a win. `dst_ns` movement alone counts for nothing.
- Single-repo goldens byte-identical (measured); note
  `DumpNormalized` does not select `dst_ns`, so snapshot suites are
  structurally blind to hint changes — the resolution-gain check is the
  real tooth.

## Out of scope

- Resolver/ladder changes — rung 1 already consumes hints. **Carve-out
  (owner-admitted):** `store.go:373`'s insert-time hint *selection* IS
  edited here. It is not a ladder change, and without it the two adapter
  edits are inert.
- Go **interface** embedding and `implements` edges. Despite this
  change's title, Go emits no `KindImplements` dep anywhere and
  interface embedding parses as `type_elem` (not `field_declaration`),
  so it emits no dep at all today — coverage there is zero and stays
  zero. The measured 25-edge population is struct embedding only;
  adding a `type_elem` emit site is a missing-EDGE change and would move
  the denominator the acceptance bar is stated over.
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

### 2026-08-22 — reconcile (docket-implement-next)

Re-read the change body and the spec
(`docs/superpowers/specs/2026-08-22-adapter-namespace-hints-extends-implements-design.md`)
against `origin/main` @ `2c8b9c3`, the `related` changes (13 done, 10 build-ready,
18 needs-brainstorm), the ADR ledger, and the learnings index.

**Verdict: no scope change. The spec is accurate as written and stays binding.**

Verified against current code — every line reference in the spec still resolves
exactly, so nothing drifted since the groom measured it:

- `internal/adapter/golang/golang.go:59` — `addDep := func(n, kind, target)`, no
  source channel (edit 1's precondition holds).
- `:108` `case "import_spec"` — emits `addDep(n, graph.KindImports, ipath)` with
  no `Source`; populates `aliases[lastSegment] = ipath`.
- `:125` `case "field_declaration"` — `addDep(n, graph.KindExtends,
  embeddedTypeName(t, src))`, the single struct-embedding emit site.
- `:207` `embeddedTypeName` — `case "qualified_type"` still descends into
  `name`, discarding the package operand.
- `internal/graph/store.go:373` — still literally
  `hint := bind[d.Target] // extends/implements/import targets bind too`; the
  calls path at 352–354 still has the `c.NsHint`-then-`bind` shape edit 3
  mirrors. `bind` is still keyed on the import dep's `Target` (line 343), so it
  remains structurally unable to hint a Go subtype edge.

Cross-change check:

- **13** (`done`) touched workspace resolve stamping, not the adapter or the
  insert-time hint sites — no overlap, nothing to drop.
- **10** (build-ready, not built this run) is corpus growth; it does not move the
  23-edge addressable denominator this change's bar is stated over.
- **18** (needs-brainstorm) still owns the PHP/Python/TS aliased-import work and
  the missing-EDGE parse gaps. Both stay out of scope here.
- No ADR bears on hint selection or adapter dep emission.

Constraints re-affirmed as binding for the build (unchanged, restated because
they are the failure modes):

- Acceptance is the **disambiguation** bar over the 23 addressable
  qualified-embed edges — `ambiguous → unambiguous` with a verified-correct
  target. `unresolved → resolved` is never promised; `dst_ns` movement alone
  counts for nothing; `storage.Appender` is PARTIAL and is not a win.
- The repo has **no committed graph goldens** — re-verified this pass
  (`git ls-files | grep -iE 'golden|snapshot|\.snap'` is empty, and the
  `DumpNormalized` consumers are rebuild-equivalence checks that move
  identically on both sides). Verification item 9's every-diff-line-accounted-for
  rule is the real check; items 6a/6b carry the in-suite no-regression half.
- Suite is `go test -tags nollama -count=1 ./...` (the pinned
  `FINALIZE_TEST_COMMAND`), green on `origin/main`.

No follow-up work surfaced that would warrant a stub; `auto_capture` is disabled
in this repo in any case.

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

## Owner ruling round 2 (2026-08-22) — disambiguation bar

The second abstain's single question is answered: the acceptance bar
admits `ambiguous → unambiguous`-with-correct-target as resolution
movement (see the amended bar in `## What changes`). The abstain's full
measured record is in git history; its binding design content is kept
below.

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

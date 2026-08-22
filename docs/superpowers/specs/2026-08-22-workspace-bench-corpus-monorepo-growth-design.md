<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0010 — Grow the workspace bench corpus — monorepo declaration coverage in every supported language](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0010-workspace-bench-corpus-monorepo-growth.md)**
<!-- docket:backlink:end -->

# Workspace bench corpus — structural growth and a new pre-registered gate

Design for docket change 0010. Bench-only, OSS-pinned. No engine code.

## Problem

The 2026-08-21/22 D7 gate runs answered the frontier hypothesis honestly and
negatively, but on a corpus that could not have answered it any other way: the
frozen 65-task set is **greppable by construction** — 100% rung-1,
import-mediated, the symbol name literally present in the consumer's text. Both
arms tie at every model tier on those shapes because grep is already at ceiling.

The one structural shape present, `xsubtypes` (n=7), is the only place the index
moved: haiku mean cross-recall 0.29 → 0.43, **+14pp**. That is the m5 floor
pattern appearing exactly where the corpus stops being greppable — and n=7 is far
too thin to carry a verdict.

This change grows the corpus so the structural hypothesis is actually testable,
and registers a new gate over it before any scored run.

## Scope decision: the primary goal needs no new repo pins

The re-armed change body carries a measurement that reframes the whole change:
the structural shapes are **already available on the unchanged 10 members**. The
`xsubtypes` n=7 is a miner artifact, not a corpus fact —
`build_tasks_ws.py` mines subtypes only from candidates already picked as a
primary task (`if not c.get("picked"): continue`, line 391) under
`PER_LIB_PRIMARY_CAP = 12`. Re-running the same extraction over *all* candidates
on the *unchanged* members yields **80** subtype-capable tasks (php 56, ts 24) —
reproduced twice, most recently by the critic pass on this spec.

So the work splits, and the split is the central design decision:

- **Phase 1 (primary, gates the verdict): re-mine the existing 10 members for
  structural shapes.** No new pins, no new checkouts, no new licences to clear.
- **Phase 2 (secondary, non-blocking): monorepo declaration-format pins.** The
  original ask (go.work, pnpm-workspace.yaml, npm/yarn `workspaces`, composer
  path repositories, a Python multi-member layout) stands, but it serves
  *member-discovery coverage*, not the structural hypothesis. It must not gate
  the gate.

## Phase 1 — the structural corpus

### What exists today, stated precisely

`build_tasks_ws.py` implements **exactly four** task kinds: `xcallers`,
`ximpact`, `xnew`, `xsubtypes`. **`xcollide`, `xalias`, and `xchain` do not
exist** — no `PROMPTS` entry, no `sub_pattern` entry, no extraction pass. They
appear only in the change stub's prose. This spec therefore describes **one
miner fix plus three new shape implementations**, not "three miner fixes"; the
earlier framing understated the work by roughly two new extraction passes, three
prompt templates, GT computation, and subset plumbing.

### Fix 1 — unblock subtype supply (the measured 80)

Two coupled edits, and **both are required**:

- **Scope the `picked` removal to `xsubtypes` only.** The guard at line 391 sits
  inside `for kind in ("xsubtypes", "xnew"):`. Removing it wholesale also
  unblocks `xnew`, which measurably goes **10 → 126** (122 of them PHP), pushing
  the control subset 58 → 174 and making the corpus 192/256 symfony. The
  registered floors below, and the structural/control balance the gate depends
  on, would both become vacuous. Remove the guard for `xsubtypes` and leave
  `xnew` gated (equivalently, cap `xnew` at its current supply). **Expected
  control subset after this fix: unchanged at 58.**
- **Re-apply `MAX_GT_FILES` inside the subtype loop.** The 40-file GT cap is
  applied only in the primary loop (`if c["cross_files"] > MAX_GT_FILES:
  continue`, line 375); the subtype loop inherited it *transitively* via
  `picked`. Drop `picked` without re-applying the cap and 2 emitted `xsubtypes`
  tasks exceed 40 GT files (max **164**), violating the registered corpus rule
  "GT size is capped at 40 files." The **80** figure holds only *with* the cap
  re-applied.

### New shapes 2–4

- **`xcollide`** — same bare name defined in ≥2 members; grep returns the union,
  and the correct answer needs the import binding. New extraction pass, prompt
  template, and GT.
- **`xalias`** — aliased/renamed imports. New pass. It is a **subset-filter**
  task: the shape's intuitive rationale is backwards, because the aliasing
  statement itself contains the original name, so grep returns a strict
  *superset*. It therefore needs the **proper-subset emit guard at line 406**
  (`if gt == c["gt"]: continue`, applied today to the `xsubtypes`/`xnew` loop and
  documented at lines 29–30) — **not**, as an earlier draft of this spec said, "a
  guard `xcollide` already has"; `xcollide` does not exist. Without the guard the
  shape degenerates into a rephrasing of `xcallers`.
- **`xchain`** — transitive impact A→B→C across members. Buildable with no new
  *machinery*: run the existing extraction a second time with a consumer member
  treated as a lib, using the `namespaces` it already declares in `corpus.json`.
  Its precondition is line 318's `libs = [m for m in members if "shared lib" in
  m["role"]]`, which excludes `nest-core` (corpus.json role: "consumer of
  nest-common (monorepo member)") — the chain pass needs explicit member
  selection rather than the role filter. Measured: `lib_definitions(nest-core)` →
  232 exported symbols; `nest-microservices` imports 30 across 21 files; joining
  on "hop-2 def file also references `nest-common`" gives **22 chain symbols
  across 11 files**. nest-only; the other three clusters are 2 members deep.

### Measured availability

| shape | php | ts | py | go | status |
|---|---|---|---|---|---|
| `xsubtypes` | 56 | 24 | 0 | 0 | implemented; 80 after fix 1 |
| `xalias` | 36 sym / 125 files | 0 | 4 / 5 | 3 pkgs / 17 files | to build |
| `xcollide` | 296 (raw upper bound) | 2 | 18 | 45 | to build |
| `xchain` | no | yes (22/11) | no | no | to build |

Python and Go `xsubtypes` emit **zero** today, confirmed empirically. The
structural verdict will therefore be a **PHP+TS verdict** (56/24), and the
registration must say so plainly rather than imply four-language structural
coverage.

### Registered floors, and how they get their numbers

- `xsubtypes` ≥ 60 — **measured** (80 emitted, guarded and GT-capped).
- `xchain` — pinned at freeze from what the nest cluster supplies (≤22).
- `xcollide` and `xalias` — floors are **registered at build time from the
  guarded, GT-capped emitted counts**, not from the raw upper bounds in the
  table above. Those upper bounds (296 PHP `xcollide`, 42 raw `xalias`) are
  pre-guard, pre-GT-cap counts on shapes with no implementation, so registering
  ≥20 / ≥25 from them today would be registering a floor with no measurement
  behind it. Mine the shapes first, then register. The stub's ≥20 / ≥25 are
  carried as **expectations to test**, not as pre-registered floors.
- greppable control subset ≥ 40 (today's set is 58 and fix 1 leaves it there).

**Aggregate target: structural subset ≥ 105, control ≥ 40, total ≥ 145** (vs 65
today), on the arithmetic 60 (`xsubtypes`) + 20 (`xcollide`) + 25 (`xalias`),
with `xchain` deliberately outside the sum.

**Exclusion-branch arithmetic (written down now, per B5).** If `xalias` is
excluded at freeze (see the 0018 coupling below), the per-shape floor sum falls
to **80**, while the aggregate would still read 105. The **aggregate governs**,
and it remains reachable without `xalias`: measured `xsubtypes` overshoots its
floor by 20 (80 vs 60), plus ≤22 `xchain`, plus `xcollide`. If after mining the
aggregate is not reachable on the remaining shapes, the aggregate is **lowered at
freeze with the arithmetic recorded** — never after the run. Leaving this
unwritten would force either a post-hoc floor edit (exactly what B5 forbids) or a
FAIL caused by corpus construction rather than by the hypothesis.

### Two structural facts to record beside the corpus table

- **Go is structurally under-supplied for subtype tasks and cannot be fixed by
  adding a Go branch to `sub_pattern`.** The entire embedded-`client_golang`-type
  population in the consumer is one line
  (`prometheus/storage/remote/max_timestamp.go:25: prometheus.Gauge`). Go's real
  subtyping is *implicit interface satisfaction* (3 files define
  `Collect(ch chan<- prometheus.Metric)`), which a textual cross-member miner
  cannot compute and which need not name the interface at all.
- **Python's zero is real but small** (~+4–5 tasks): flask subclasses werkzeug
  under aliases (`from werkzeug.wrappers import Request as RequestBase`), which
  the alias-blind pattern misses; the proper-subset guard drops 2 more.

### The structural/control partition must be built, not assumed

B1–B3 below partition the corpus into a **structural subset** and a **greppable
control subset**. **No such partition exists in any artifact today.**
`grade_ws.py` passes `kind` and `rung` through, and since every task is `rung1`
its `rung1_med_cross_recall` (line 110) is a whole-corpus figure, not a
control-subset figure — so B2 as worded would be unevaluable.

Deliverable: a registered **kind → subset** map, emitted into the task file
header and read by the gate script:

- control (greppable): `xcallers`, `ximpact`, `xnew`
- structural: `xsubtypes`, `xcollide`, `xalias`, `xchain`

## Phase 2 — monorepo declaration coverage (secondary, non-blocking)

Member discovery currently has effectively one data point: `nest`, declaring via
`lerna.json` — a source the frozen design did not even list. Formats with zero
organic coverage: `go.work`, `pnpm-workspace.yaml`, npm/yarn `workspaces`,
composer path repositories, Python multi-member layouts.

Specific pins are **not named here** — naming versions I cannot verify would
register a corpus that may not exist. The *selection criteria* are registered
instead, and the resolved pin list is recorded in `corpus.json` and
`bench/workspace/README.md` **before** any mining over grown members:

- OSS, permissive licence, pinned to an exact tag or commit.
- The declaration format appears **organically** — not added by us. A
  synthesised monorepo proves nothing about discovery.
- **Hard bounds, so this cannot expand without limit:** at most **one member per
  format** (5 formats ⇒ ≤5 new members); each checkout **≤ 500 MB**; evaluate at
  most **3 candidate repos per format**. If no candidate qualifies within those
  bounds, the format is recorded as **uncovered** and the search **stops**.
  Uncovered is an honest result; fabricated coverage is not.

Deliverable: a per-format coverage table (formats exercised, per-member quota) in
`bench/workspace/README.md`. Phase 2 changes discovery coverage, not the task
corpus, so it may land after the gate registration and even after the gate run
without invalidating it.

## The new gate registration

Bars go into `bench/workspace/README.md` **before any scored run** (m5/D7 form;
the gate script reads bars from the file, never from its own source).

**Placement — append, do not amend.** The file already carries
`## Bars (copied verbatim from design D7 — all required)`, including an
independent efficiency bar. B1–B5 are **appended as a new dated section, "Bars —
0010 structural gate (registered 2026-08-22)"**, and the D7 block is left intact
as change 0016's historical record. Editing the D7 block in place would destroy
0016's registration and make the file self-contradictory on efficiency.

- **B1 — structural lift (primary).** At **haiku**, B **mean** cross-recall on
  the structural subset ≥ A + 10pp; **OR** B meets the efficiency bar (≥40% fewer
  exploration tokens or tool/shell calls, measured over the cross-repo tasks)
  with recall B ≥ A.
  Stated in the anchor's own quantity and tier on purpose: the
  `xsubtypes 0.29/0.43 (+14pp, n=7)` observation is a **mean** at **haiku**.
  Note explicitly in the registration that this is a **change of quantity** from
  the 2026-08-21 registered follow-up, which stated a *median* — a median on a
  structural subset is likely degenerate (0.0 vs 0.0) and less informative, not
  less gameable. Because a mean over a ~105-task subset can be carried by one
  shape — and ~45% of that subset is shapes with no prior at all — the
  registration **also requires per-shape means and the subset median to be
  reported** alongside the bar, so a single-shape effect is visible rather than
  hidden.
- **B2 — floor competence (restored).** B rung-1 **median** cross-recall
  **≥ 0.9 absolute** on the greppable control subset. This bar **passed** on
  2026-08-21 (B rung-1 med 1.0) and is restored explicitly: dropping a bar the
  previous run passed is silent erosion, and a corpus re-aimed at structural
  shapes could otherwise regress the greppable ones unnoticed. Requires the
  kind→subset partition above.
- **B3 — non-inversion.** B median cross-recall ≥ A on the control subset.
- **B4 — leak audit.** `leak_audit_ws.py` PASS on all four classes (template
  leakage, control contamination via the id-paired join, forced-tool prompt scan,
  grader-codesign ordering-invariance) over the campaign transcripts, as a
  standing **pre-verdict** gate; the gate script refuses a verdict on non-zero
  exit. Isolation unchanged: `--setting-sources project,local` plus the arm-A
  PATH shim and `CODEINDEX_DISABLED`.
  **Index artifacts must be physically absent from the member checkouts**, not
  renamed — the 2026-08-22 quarantine established that renaming is not hiding and
  that observation alone fails the class. This is a **live precondition**: 8
  `.codeindex/graph.db` artifacts sit under `bench/repos/*/` right now. Verify
  absence; do not assume it.
- **B5 — freeze discipline.** Per-shape `n`, per-language `n`, and **any excluded
  shape** are pinned into the registration at freeze, never decided after mining,
  along with the exclusion-branch arithmetic above. The shapes most likely to be
  thin are exactly the ones most likely to falsify, so a post-hoc exclusion
  collapses the verdict onto `xsubtypes` — PHP-dominated and the one shape with a
  favourable prior.
- **Efficiency** is **reported** per run and enters the verdict only through
  B1's OR-clause. This is faithful carry-forward, not a relaxation: the
  **2026-08-21 registered follow-up** ("Bars (all required, per tested model)",
  FINDINGS-workspace-graph.md) already placed efficiency solely inside the
  OR-clause, so it has not been an independent bar since that registration.

**Kill condition** (unchanged in form): if B clears none of the bars, the result
is published as a FINDINGS entry in `bench/engine/FINDINGS-workspace-graph.md`
and the pivot closes. A PASS revives the killed 0016 query surfaces **as a fresh
change**, not as a rescue of the old one.

## Dependency coupling — 0017 and 0018

Encoded as **ordering in the registration text**, not as `depends_on:`. A
`depends_on` would deadlock permanently if 0017 or 0018 is killed or superseded:
`killed` is terminal but never `done`, and dependency satisfaction requires
9 and 17, since the freeze decision below keys on 0018.

The constraint as written is retained, but it is measurably **near-vacuous for
`xsubtypes`**:

- `internal/graph/store.go:373` (`hint := bind[d.Target]`) sits in the
  language-agnostic `PutFile` deps loop, so subtype edges pick up `dst_ns`
  whenever the target name equals an imported name.
- Measured per-member subtype-edge hint rates
  (`sum(dst_ns <> '')` over `bench/repos/*/.codeindex/graph.db`): symfony extends
  3126/4834 and implements 1553/2844; drupal 5516/8010 and 3357/4303; laravel
  1666/2314 and 1230/1533; nest 151/194 and 135/136; flask 25/74; werkzeug
  27/135; **client_golang 0/131**; prometheus 8/128.
- The sufficient claim is therefore the weaker one: **Go subtype edges are
  unhinted and Go supplies zero subtype tasks; PHP/TS edges are substantially
  hinted and are 100% of the subtype supply.** (An earlier draft said "80 of ~85
  subtype-capable tasks are already hinted" — that overstates it; PHP hint rates
  are 55–80%, not ~100%.)
- 0017 abstained on 2026-08-22 (its acceptance bar is unsatisfiable as literally
  written; see its `## Auto-groom blocked` record), and it is Go-scoped. Treating
  it as a hard precondition would stall a 105-task corpus on a coupling worth
  zero emitted tasks.

`xalias` is the shape with a real dependency: mining it is plain-text work
needing nothing from the engine, but the index's ability to *answer* alias tasks
depends on 0018, which has no spec. Per B5 this resolves **at freeze, not after**:
if 0018 has not landed when the corpus is frozen, `xalias` is declared
**excluded at freeze** with its `n` recorded, and the exclusion-branch arithmetic
above governs.

## Out of scope

- Any engine change. Discovery-source list changes belong to change 0009 and its
  successors; adapter hint work is 0017/0018.
- Private-repo material — the corpus stays pinned OSS (owner rule).
- Re-running the D7 evidence gate itself (§5 of the workspace-graph plan); this
  change grows the corpus and registers the new gate, it does not run it.
- Reviving the 0016 query surfaces — that is the new gate's outcome, as a fresh
  change.

## Assumptions

Every decision an interactive brainstorm would have raised, the default taken,
and why. This is the deferred audit trail. Entries marked *(revised)* were
corrected after the adversarial critic pass.

1. **Phase 1 needs no new repo pins.** *Chosen:* re-mine the existing 10 members;
   make monorepo-format pins secondary and non-blocking. *Rejected:* pin new
   monorepos first (couples an available deliverable to an unmeasured
   repo-selection exercise, against the owner's structural-first re-direction);
   pin new repos to raise Go/Python subtype supply (Go's shortfall is
   structural — implicit interface satisfaction — not sampling). *Why:* the 80
   figure reproduces on members already on disk.
2. *(revised)* **Floors are registered from guarded, GT-capped measured counts.**
   *Chosen:* register `xsubtypes ≥ 60` now (measured 80); register `xcollide` and
   `xalias` floors at build time after mining; carry the stub's ≥20/≥25 as
   expectations, not registrations. *Rejected:* registering ≥20/≥25 today from
   the raw upper bounds (296/2/18/45 is pre-guard, pre-GT-cap, on unimplemented
   shapes — a floor with no measurement behind it, which is exactly the
   unfalsifiable registration this gate discipline exists to prevent). *Why:* a
   pre-registered floor must be traceable to a measurement.
3. **No per-language floor on the structural subset; no per-language cap.**
   *Chosen:* record per-shape and per-language `n` at freeze and let the mix
   fall where the corpus supplies it; state plainly that the structural verdict
   is a PHP+TS verdict (py and go emit zero `xsubtypes`). *Rejected:* a
   per-language floor (unmeetable for Go and Python — invites padding); a
   per-language share cap (would require inventing a threshold, and B5 already
   closes the failure mode). *Why:* the real risk is post-hoc shape exclusion.
4. *(revised)* **Primary bar stated as a mean at haiku, with per-shape means and
   the median reported.** *Chosen:* B1 mirrors the anchor's quantity and tier; a
   median on the structural subset would likely be degenerate (0.0 vs 0.0).
   *Rejected:* a median bar (not anchored to a mean observation, and probably
   uninformative here); frontier tier (no structural observation there, and it is
   at ceiling on greppable shapes); an absolute structural target (no prior — B's
   structural absolute today is 0.43). *Fold-ins:* the registration must state
   that this **changes the quantity** from the 2026-08-21 registered median bar,
   and must require per-shape means plus the subset median to be reported, since
   a mean over ~105 tasks can be carried by one shape and ~45% of the subset has
   no prior.
5. **Floor-competence bar restored verbatim.** *Chosen:* B2 = B rung-1 median
   ≥ 0.9 absolute on the greppable control subset, per the owner's correction.
   *Rejected:* omitting it as "already established" — it passed on 2026-08-21,
   and dropping a passing bar is the silent erosion the discipline forbids.
   *Depends on* the kind→subset partition being built (assumption 13).
6. *(revised)* **Efficiency reported, entering only via B1's OR-clause.**
   *Chosen:* carry the 2026-08-21 registered follow-up's form forward and **cite
   it by name** as the authority; specify the efficiency measurement subset
   ("over the cross-repo tasks", as the original said); place B1–B5 in a new
   appended section so the D7 "all required" block does not contradict it.
   *Rejected:* requiring ≥40% savings independently (measured 3.4% at frontier
   and −5.8%/−12.5% at haiku — it would pre-determine a FAIL and make the
   structural hypothesis untestable); deleting it (dropping a registered bar
   without a record). *Why:* efficiency has not been an independent bar since the
   2026-08-21 registration, and the owner's silent-erosion rule is about dropping
   a bar the previous run **passed** — efficiency **failed**. This is faithful
   carry-forward, not the groom re-barring itself.
7. **Coupling encoded as ordering, not `depends_on`.** *Chosen:* README ordering
   `killed` is terminal but never `done`, so a killed dependency deadlocks this
   change permanently. *Why:* the body's own reasoning, made concrete by 0017's
   on it.
8. *(revised)* **The 0017 coupling is near-vacuous for `xsubtypes`.** *Chosen:*
   retain the ordering constraint and record the **measured per-member hint
   rates**, concluding only the sufficient claim: Go subtype edges are unhinted
   and Go supplies zero subtype tasks; PHP/TS are substantially hinted and are
   100% of subtype supply. *Rejected:* the earlier "80 of ~85 already hinted"
   (overstated — PHP is 55–80%); silently dropping the constraint (contradicts
   the re-armed body without a record); treating 0017 as a hard precondition
   (stalls the corpus on a zero-task coupling). *Why:* the FINDINGS attribution of
   the 0.43 cap to the hint gap is broader than the code supports, and the stub's
   own prior-groom section already downgraded the coupling to "binds on the
   scored run, not on this change's deliverable."
9. *(revised)* **`xalias` inclusion resolved at freeze, with the exclusion
   arithmetic written down now.** *Chosen:* mine it; if 0018 has not landed at
   freeze, declare it excluded in the registration with its `n`; the **aggregate
   ≥105 governs** over the per-shape sum, and if unreachable it is lowered **at
   freeze** with the arithmetic recorded. *Rejected:* excluding it now (0018 may
   land first; 33 qualifying PHP symbols measured); deciding after the run
   (precisely the post-hoc exclusion B5 forbids); leaving the branch unwritten
   (forces either a post-hoc floor edit or a construction-caused FAIL). *Why:*
   B5's freeze discipline applied to the one shape with a real dependency.
10. *(revised)* **Phase 2 pins specified by criteria, with hard numeric bounds.**
    *Chosen:* register criteria plus bounds — ≤1 member per format, ≤500 MB per
    checkout, ≤3 candidates evaluated per format, "no qualifying candidate ⇒
    record uncovered and stop." *Rejected:* naming specific repos and versions
    (unverifiable from this context; a registration citing a corpus that does not
    exist is worse than none); synthesising monorepos per format (proves nothing
    about *organic* discovery); unbounded criteria (an autonomous builder told to
    "find permissively-licensed OSS repos" will clone and licence-judge without
    limit). *Why:* the open question's other half — target size — **is** answered
    with a number, and phase 2 gates nothing, so the residual risk is scope, now
    bounded.
11. *(revised)* **`xalias` is a subset-filter task and takes the proper-subset
    guard at line 406.** *Chosen:* apply `if gt == c["gt"]: continue`, the guard
    used today on the `xsubtypes`/`xnew` loop. *Rejected:* keeping the original
    framing (measurably backwards — grep returns a superset); the earlier
    attribution "the guard `xcollide` already has" (**`xcollide` does not
    exist**). *Why:* an unguarded `xalias` is `xcallers` rephrased and would
    inflate the structural count with non-structural tasks.
12. **Leak-audit posture strengthened to "verify absence."** *Chosen:* B4
    requires index artifacts physically absent from member checkouts, flagged as
    a live precondition (8 are on disk now). *Rejected:* the pre-2026-08-22
    posture of renaming/excluding (two blinding attempts failed; observation
    alone fails class 2). *Why:* re-learning the quarantine lesson would cost a
    whole campaign.
13. *(new)* **The structural/control partition is a deliverable, not an
    assumption.** *Chosen:* register an explicit kind→subset map, emit it into
    the task file header, and have the gate script read it. *Rejected:* relying
    on `grade_ws.py`'s existing `rung1_med_cross_recall` (every task is `rung1`,
    so it is a whole-corpus figure — B2 would be unevaluable) or on an implicit
    "structural means xsubtypes" convention (breaks as soon as the three new
    shapes land). *Why:* B1, B2 and B3 are all stated over subsets that do not
    currently exist in any artifact.
14. *(new)* **The `picked` removal is scoped to `xsubtypes`, and `MAX_GT_FILES`
    is re-applied in the subtype loop.** *Chosen:* both, together. *Rejected:*
    removing the guard wholesale (measured: `xnew` 10 → 126, control 58 → 174,
    corpus 192/256 symfony — the floors and the structural/control balance both
    go vacuous); removing it without re-applying the GT cap (2 emitted subtype
    tasks exceed 40 GT files, max 164, violating the registered 40-file rule, and
    the 80 figure depends on the cap). *Why:* the stub's headline number is only
    correct under both constraints.

## Acceptance

- `build_tasks_ws.py` carries fix 1 (scoped `picked` removal **plus** re-applied
  `MAX_GT_FILES`) and implementations of `xcollide`, `xalias` (proper-subset
  guarded) and `xchain` (explicit member selection at line 318).
- Re-mining the unchanged 10 members meets the registered aggregate (structural
  ≥105, control ≥40), or the aggregate is lowered **at freeze** with the
  arithmetic recorded.
- The task file header records the kind→subset partition, per-shape `n`,
  per-language `n`, and any shape excluded at freeze.
- `bench/workspace/README.md` gains a **new appended dated section** carrying
  B1–B5, the kill condition, the phase-2 criteria and bounds, the 0017/0018
  ordering text with the measured hint rates, and the Go/Python structural
  notes — committed **before** any scored run, with the D7 block left intact.
- `leak_audit_ws.py` PASSes all four classes, with index artifacts verified
  absent from the member checkouts.
- No engine code changes; no private-repo material; every member pinned OSS.


---
*Promoted from -DRAFT 2026-08-22 by the driver on the critic's stranded-but-delivered EMIT verdict (sound; three residual clarifications applied: GT-cap reading, related-bullet removed from Acceptance — the metadata edit is done on the groom side, PHP hint range 55–80%). Owner confirmations: efficiency stays reported-only in the OR-clause (it failed both prior runs; not erosion); corpus work proceeds ahead of 0017 (Go emits zero subtype tasks; PHP/TS supply is already substantially hinted).*

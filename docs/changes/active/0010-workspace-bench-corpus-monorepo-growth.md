---
id: 10
slug: workspace-bench-corpus-monorepo-growth
title: Grow the workspace bench corpus — monorepo declaration coverage in every supported language
status: proposed
priority: medium
type: chore
created: 2026-08-18
updated: 2026-08-22
depends_on: []
related: [9, 17, 18]
discovered_from: [9]
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

Grooming change 0009 surfaced that the frozen 65-task workspace bench
corpus contains exactly one monorepo (nest), and it declares members
solely via `lerna.json` — a source the frozen design didn't even list.
Member discovery (`init-workspace --scan`) therefore has effectively one
data point per declaration format, and most formats (go.work,
pnpm-workspace.yaml, composer path repositories, npm/yarn `workspaces`)
have zero organic coverage. Owner decision 2026-08-18: grow the corpus
significantly with monorepo examples in every supported language, then
measure discovery coverage.

## What changes

- Add pinned OSS monorepo members to the workspace bench corpus so every
  supported declaration format has at least one organic example: go.work
  (Go), pnpm-workspace.yaml and npm/yarn `workspaces` (TS/JS, alongside
  the existing lerna case), composer path repositories (PHP), plus a
  Python multi-member layout.
- Re-mine tasks over the grown corpus (extend `build_tasks_ws.py`),
  re-freeze the task set, and re-run the four-class leak audit
  (`leak_audit_ws.py`) as the standing pre-verdict gate.
- Measure and record member-discovery coverage per declaration format
  (which formats are exercised, per-member quota) in
  `bench/workspace/README.md`.

## Owner re-direction (2026-08-22, post-gate pivot)

The D7 gate runs (see `bench/engine/FINDINGS-workspace-graph.md`) showed
the frozen 65-task corpus is greppable by construction — both arms tie
on import-mediated shapes at every model tier — while the one structural
shape (`xsubtypes`, n=7) showed the index lifting haiku +14pp. The
corpus growth this change owns is therefore re-aimed: **structural,
grep-hostile cross-repo tasks are the priority**, monorepo declaration
coverage second.

- Task shapes to mine, with auditable GT: transitive impact chains
  (A→B→C across members), subtype maps (all implementors/extenders of a
  cross-repo interface/class — blocked on change 0017's hint fix landing
  first), cross-repo name collisions (same bare name in ≥2 members,
  where grep returns the union and the correct answer needs the import
  binding), and aliased/renamed imports.
- Grow the subtype set well beyond n=7 so the signal is statistically
  meaningful.
- Register a NEW gate (bars registered before any scored run, m5/D7
  form) on the grown corpus; a pass revives the killed 0016 query
  surfaces as a fresh change.
- Original monorepo-format coverage ask (go.work, pnpm, npm/yarn
  workspaces, composer path repos) stands as the secondary goal.

## Out of scope

- Any engine change — discovery source list changes belong to change
  0009 and its successors.
- Private-repo material — corpus stays pinned OSS (owner rule).
- Re-running the D7 evidence gate itself (§5 of the workspace-graph
  plan); this change only grows the corpus it will run over.

## Open questions

- Which specific OSS repos to pin per format, and target corpus size
  ("significantly" needs a number at brainstorm time).
- Whether grown-corpus task mining changes the per-member quota rules
  recorded in the bench README.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

## Groom corrections applied (2026-08-22, re-arm)

The first groom abstained purely on protocol (revision budget spent);
both named corrections are applied for the next pass, which must honor
them: (1) `related:` now carries 17 — the machine-readable 0017
coupling exists; (2) the new gate registration MUST restore the
2026-08-21 bar the prior run PASSED — "Floor competence: B rung-1
median cross-recall ≥ 0.9 absolute" — as an absolute floor on the
greppable control subset, alongside the relative non-inversion bar;
dropping a passing bar is the exact silent-erosion failure the draft's
own D4 guards against. Cosmetic corrections carried: Go import-alias
population is 3 distinct packages / 17 files (versioncollector, v1,
prom_testutil, client_testutil); `promcli.Collector` was an invented
identifier. Note: 0017 is now Go-scoped (owner Option C); the alias
work lives in change 0018 — subtype-map mining depends on 0017,
alias-task mining depends on 0018.

### Measured findings worth keeping (expensive to re-derive)

**`xsubtypes` n=7 is a miner artifact, not a corpus fact.**
`build_tasks_ws.py` mines subtypes only from candidates already picked as
a primary task (`if not c.get("picked"): continue`, line ~391) under
`PER_LIB_PRIMARY_CAP = 12`. Re-running the same extraction over *all*
candidates on the *unchanged* 10 members yields **80** subtype-capable
tasks with ≤40-file GT — symfony 56, nest-common 24 — reproduced
independently by the critic pass.

**Per-shape availability, measured:**

| | php | ts | py | go |
|---|---|---|---|---|
| xsubtypes | 56 | 24 | ~4 | ~1 |
| xalias (aliased lib symbols / files) | 36 / 125 | **0** | 4 / 5 | 3 pkgs / 17 files |
| xcollide (upper bound) | 296 | 2 | 18 | 45 |
| xchain (3-member cluster) | no | **yes** | no | no |

- **Go is structurally under-supplied for subtype tasks and cannot be
  fixed by a `sub_pattern` Go branch.** The entire embedded-client_golang
  -type population in the consumer is one line
  (`prometheus/storage/remote/max_timestamp.go:25: prometheus.Gauge`) —
  not `prometheus.Collector`, which appears in no embed anywhere. Go's
  real subtyping is implicit interface satisfaction (3 files define
  `Collect(ch chan<- prometheus.Metric)`), which a textual cross-member
  miner cannot compute and which need not name the interface at all.
  Record this beside the corpus table so the next reader does not
  re-derive it.
- **Python's zero is real but small** (~+4–5 tasks): flask subclasses
  werkzeug under aliases (`from werkzeug.wrappers import Request as
  RequestBase`), which the alias-blind pattern misses; the proper-subset
  guard drops 2 more.
- **`xalias`'s intuitive rationale is backwards.** "Grep for the symbol's
  own name misses aliased files" is false — the aliasing statement itself
  contains the original name, so grep returns a strict *superset*. It is
  a **subset-filter** task and needs the same strict-superset emit guard
  `xcollide` has, or it degenerates into a rephrasing of `xcallers`. With
  that guard applied, PHP still yields 33 qualifying symbols.
- **`xchain` is buildable, nest-only, with no new machinery**: run the
  existing extraction a second time with a consumer member treated as a
  lib, using the `namespaces` it already declares in `corpus.json`.
  Measured: `lib_definitions(nest-core)` → 232 exported symbols;
  nest-microservices imports 30 of them across 21 files; joining on
  "hop-2 def file also references nest-common" gives **22 chain symbols
  across 11 files**. Implementation note: `libs = [m for m in members if
  "shared lib" in m["role"]]` (line 318) excludes nest-core, so the
  second pass needs explicit member selection rather than the role
  filter. The other three clusters are only 2 members deep.
- **Reachable floors** (each measured, not aspirational):
  `xsubtypes ≥ 60`, `xcollide ≥ 20`, `xalias ≥ 25` **in total, not per
  language** (raw ceiling 42, ~86% PHP, TS zero), control subset ≥ 40
  (today's greppable set is 58), `xchain` pinned at freeze from what the
  nest cluster supplies. **No per-language floor for the structural
  subset** — unreachable for Go, near-unreachable for Python, and a floor
  a corpus cannot meet only invites padding.
- **Bar 1 must be stated in the anchor's own quantity and tier**: the
  `xsubtypes 0.29/0.43 (+14pp, n=7)` observation is a **mean** at
  **haiku**, so a structural-subset bar stated in medians, or applied at
  a tier where no structural observation exists, is not anchored to it.
- **Per-shape n and any excluded shape must be pinned into the
  registration at freeze**, never decided after mining: the shapes most
  likely to be thin are exactly the ones most likely to falsify, so a
  post-hoc exclusion collapses the verdict onto `xsubtypes` — PHP-
  dominated and the one shape with a favourable prior.
- **The 0017 coupling binds on the scored run, not on this change's
  deliverable.** Phase 1's alias-aware mining is plain-text work needing
  nothing from the engine; only the index's ability to *answer* subtype
  tasks depends on the adapter fix. Hence `related:`, plus the ordering
  constraint written where the gate run will read it
  (`bench/workspace/README.md`), rather than a `depends_on:` that would
  deadlock permanently if 0017 is killed or superseded — `killed` is
  terminal but never `done`.

### Recommendation

Groom this interactively; it is close. Do **not** kill or defer it — the
re-aimed corpus is the pivot's only path back to the killed 0016 query
surfaces, and the measurement above shows the structural shapes are
genuinely available on the existing members without adding a single new
pin. Sequence it after 0017 (in whatever form the owner lands that) so
the gate measures the feature rather than the adapter gap.

## Auto-groom blocked (2026-08-22)

Abstained on a **process** exit, not a design one. A full spec was drafted and
survived an adversarial critic round; the bounded re-check leg that would have
confirmed the revision returned no legible verdict, so the groom cannot certify
its own fixes and must not emit a build-ready spec. The design work is preserved
— re-arming is cheap.

### Where the work is

`docs/superpowers/specs/2026-08-22-workspace-bench-corpus-monorepo-growth-design-DRAFT.md`

Deliberately **not** set as `spec:` and suffixed `-DRAFT` so nothing mistakes it
for a build-ready artifact. To re-arm: read the draft, satisfy yourself on the
two items below, rename off the `-DRAFT` suffix, set `spec:`, flip
`auto_groomable: true`, and delete this section.

### What the groom settled (all critic-verified against code and data)

- **Phase 1 needs no new repo pins.** The `xsubtypes` n=7 is a miner artifact;
  re-mining the unchanged 10 members yields **80** subtype-capable tasks
  (php 56, ts 24) — reproduced independently by the critic, which ran the
  modified miner.
- **Scope split:** phase 1 = structural re-mine (gates the verdict); phase 2 =
  monorepo declaration-format pins (secondary, non-blocking, now numerically
  bounded at ≤1 member per format, ≤500 MB per checkout, ≤3 candidates per
  format, uncovered-and-stop).
- **Target size**, answering the stub's open question: structural ≥105,
  control ≥40, **total ≥145** (vs 65 today), with the xalias-exclusion branch
  arithmetic written down in advance.
- **Bars B1–B5** including the owner-mandated restoration of floor competence
  (B rung-1 median ≥ 0.9 absolute on the greppable control subset), appended to
  `bench/workspace/README.md` as a new dated section leaving the D7 block intact.

### What the critic caught that a naive build would have hit

Recorded because each was expensive to find and would have wrecked the corpus:

1. **`xcollide`, `xalias` and `xchain` do not exist.** `build_tasks_ws.py`
   implements exactly four kinds (`xcallers`, `ximpact`, `xnew`, `xsubtypes`).
   The change is one miner fix plus **three new shape implementations**, not
   "three miner fixes" — a materially larger job than the stub implies.
2. **Removing the `picked` guard wholesale also unblocks `xnew`** (measured
   10 → 126, 122 of them PHP), pushing the control subset 58 → 174 and making
   the corpus 192/256 symfony. The removal must be scoped to `xsubtypes`.
3. **The 40-file GT cap is inherited transitively via `picked`.** Drop the guard
   without re-applying `MAX_GT_FILES` in the subtype loop and 2 emitted tasks
   exceed the cap (max 164 files), violating a registered corpus rule. The 80
   figure holds only with the cap re-applied.
4. **The structural/control partition does not exist in any artifact.**
   `grade_ws.py`'s `rung1_med_cross_recall` is a whole-corpus figure because
   every task is `rung1`, so the floor-competence bar would be unevaluable. A
   registered kind→subset map has to be built and emitted into the task header.
5. **`xcollide ≥ 20` / `xalias ≥ 25` are not measured floors** — they derive from
   raw pre-guard, pre-GT-cap upper bounds on unimplemented shapes. They are
   carried as expectations; the real floors get registered after mining.

### The two things a human should confirm before re-arming

1. **Efficiency's status.** The draft keeps efficiency as *reported*, entering
   the verdict only through B1's OR-clause, on the grounds that the 2026-08-21
   registered follow-up already placed it there and that the silent-erosion rule
   is about dropping a bar the previous run **passed** (efficiency **failed**:
   3.4% at frontier, −5.8%/−12.5% at haiku). The critic examined this
   specifically and judged it faithful carry-forward rather than a groom
   re-barring itself — but it is the one place the new gate is weaker than D7,
   and it is your bar to set.
2. **The 0017 coupling is near-vacuous.** Measured subtype-edge hint rates:
   client_golang **0/131** and prometheus 8/128, versus symfony 3126/4834
   extends and 1553/2844 implements, drupal 5516/8010 and 3357/4303, nest
   151/194 and 135/136. Go is genuinely unhinted, but Go emits **zero** subtype
   tasks, and PHP/TS — substantially hinted already via the language-agnostic
   `internal/graph/store.go:373` — are 100% of the subtype supply. Since 0017
   also abstained today, the draft retains the ordering constraint but records it
   as non-blocking. Confirm you are content for the corpus work to proceed ahead
   of 0017.

`related:` gained **18**: the freeze decision for the `xalias` shape keys on it
(mining is plain-text and needs no engine, but the index's ability to *answer*
alias tasks does). Encoded as ordering rather than `depends_on:`, because
`killed` is terminal but never `done` and a killed dependency would deadlock this
change permanently.

### Groom process note

The critic returned `wrong but fixable from available context` with a
per-assumption table and ten mandatory fold-ins; all ten were applied and the
load-bearing ones re-verified directly against `build_tasks_ws.py` and the member
databases. The bounded re-check dispatch then completed without surfacing a
legible verdict on its return channel — the same return-channel failure recorded
on change 0017 in this run. Per the autonomous-groom protocol that counts as a
failed dispatch, and an author may not be their own adversarial gate, so the
groom abstains rather than emitting. **No design defect is known to remain.**

### Recommendation

Re-arm, or groom interactively — it is one confirmation away. Do not kill or
defer: the re-aimed corpus is the pivot's only path back to the killed 0016 query
surfaces, and the structural shapes are measurably available on the existing
members without adding a single new pin. Kill and defer are never autonomous;
this is a recommendation only.

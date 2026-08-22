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
related: [9]
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

## Auto-groom blocked

**2026-08-22 — `docket-auto-groom` abstained. The design reached
build-ready quality on re-check (4 of 6 re-checked items sound, and the
critic found no missing owner context), but two mechanical defects
survived into the second round and the one permitted revision was spent.
No spec was emitted. Re-arming this stub is cheap: the two corrections
below are one-liners.**

### The two corrections a re-groom must apply

1. **`related:` does not link to 0017.** The design's chosen encoding of
   the 0017 coupling was "`related: [17]` plus the ordering constraint in
   the registration text", asserting the frontmatter link already
   existed. It does not — this change carries `related: [9]`, and only
   0017 carries `related: [13, 10]`. Add `17` to this change's `related:`
   so the coupling exists in a machine-readable field.
2. **A registered bar that the prior run PASSED was dropped.** The
   2026-08-21 registration in `bench/engine/FINDINGS-workspace-graph.md`
   has three required bars; the draft reproduced the two that FAILED
   verbatim and silently dropped **"Floor competence: B rung-1 median
   cross-recall ≥ 0.9 absolute"**. That is the exact failure mode the
   draft's own D4 was written to correct, recurring one level down.
   Restore it as an absolute floor on the greppable control subset,
   alongside the relative `B ≥ A − 5pp` non-inversion bar.

Two cosmetic factual fixes for the same pass: the Go import-alias
population is **3 distinct packages / 17 files** (alias names
`versioncollector`, `v1`, `prom_testutil`, `client_testutil`), not
"2 pkgs / 13 files"; and `promcli.Collector` was an invented identifier —
no `promcli` alias exists in the corpus.

### Why no spec, given nothing needs an owner

The critic returned **no `needs human context` verdict** in either round.
The abstain is purely the protocol's: a spec is emitted only when every
decision in it is safe to auto-commit, because emission means the
autonomous builder will build it — and two load-bearing items were still
wrong when the revision budget ran out. A human groom can close this in
minutes.

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

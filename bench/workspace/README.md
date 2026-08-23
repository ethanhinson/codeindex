# bench/workspace — the workspace-graph evidence gate

Pre-registered evidence gate for `openspec/changes/workspace-graph`
(design D7). Registered 2026-08-17, **before any engine code and before
the first scored run** (tasks.md §2.3; the C2 GO that unblocked this is
recorded in `ROADMAP-DEBATE.md` §6). The gate script must read the bars
from this file, never from its own source (m5 precedent).

## Corpus (open source, four language clusters)

**Amendment 2026-08-17, before any scored run:** the corpus was rebuilt
on open-source members only (owner decision — no private code or private
repo results in the repo) and widened from the registered "2+ languages"
to all four indexer languages. Member count grows accordingly (10 vs the
registered 3–5); the bars below are untouched.

One shared-lib member and its consumer member(s) per language; every lib
pin is the version its consumer actually declares. Machine-readable
pins: `corpus.json`; consumers are existing `bench/repos.json` pins.

| cluster | lib member (pin) | consumers (pin) | evidence of the edge |
|---|---|---|---|
| PHP | `symfony` (v7.2.2) | `drupal` (11.1.0), `laravel` (v11.38.0) | `use Symfony\Component\...` |
| TS | `nest-common` (v10.4.15) | `nest-core`, `nest-microservices` | `import {X} from '@nestjs/common'` — one checkout, the D1 monorepo degenerate case |
| Py | `werkzeug` (3.1.3, flask's floor) | `flask` (3.1.0) | `from werkzeug.x import Y` |
| Go | `client_golang` (v1.20.5, from prometheus go.mod) | `prometheus` (v3.1.0) | `import ".../client_golang/prometheus"` |

Workspace root: `bench/repos/oss-ws/` — a dedicated directory reaching
every member by relative path (design D1's sanctioned shape for
scattered repos). Shared-lib-with-≥2-consumers: `symfony` ← {`drupal`,
`laravel`} and `nest-common` ← {`nest-core`, `nest-microservices`}.

## Tasks

≥30 cross-repo impact/caller tasks mined per M2's discipline
(`build_tasks_ws.py`): ground truth is import-mediated plain-text
extraction across member checkouts — equally available to both arms,
never from codeindex. Tasks are cross-member scoped ("which files in
the OTHER members reference X" — the frontier question). All tasks are
rung-1; this corpus has no organic bare-name (rung-2) cross-edges —
recorded in the task header. GT size is capped at 40 files (larger
answers measure listing stamina, not finding). Per-member and
per-language quotas recorded in the task file header. Plus the existing
single-repo goldens for the non-regression bar.

## Arms

- **A (control):** agent + shell, all member checkouts on disk —
  grep-across-repos. Honest, not blinded.
- **B (treatment):** A + the workspace-graph MCP surface.

Isolation: every headless run uses `--setting-sources project,local`
(the bench-hook-leak rule — the global codeindex plugin contaminates
controls otherwise) AND the run_ab arm-A PATH shim + `CODEINDEX_DISABLED`
(the settings flag alone leaves a globally installed binary reachable —
found and fixed 2026-08-17). Grader-blind formatting. Leak-audit all
four classes before any verdict: `leak_audit_ws.py` (template leakage,
control contamination via the id-paired `bench/agent_ab/leak_audit.py`
join, forced-tool prompt scan, grader-codesign ordering-invariance).
The §5 gate script must run it over the campaign transcripts and refuse
a verdict on non-zero exit.

## Bars (copied verbatim from design D7 — all required)

- Cross-repo caller/blast-radius recall: B ≥ A, and B ≥ 0.9 absolute
  on import-mediated (rung-1) edges.
- Efficiency: B uses ≥40% fewer exploration tokens or ≥40% fewer
  tool/shell calls on the cross-repo tasks (mirrors the M5 gate form).
- Non-regression: single-repo golden suite byte-identical for non-
  workspace roots; workspace-mode answers on a single-member workspace
  match single-repo answers modulo the `repo` field.
- Freshness property: mutate one member, query from another without an
  explicit rebuild → the answer reflects the mutation or its coverage
  clause names the member stale. Silent staleness is a hard fail.
- Discipline rule all four leak classes, including grader-blind
  formatting.

**Kill condition:** if B does not beat A on recall or efficiency, the
result is published as a FINDINGS entry
(`bench/engine/FINDINGS-workspace-graph.md`) and the change closes —
the grep-across control winning is a legitimate answer to the frontier
hypothesis. Iteration happens within the registered budget only.

## Bars — 0010 structural gate (registered 2026-08-22)

**This section is an APPEND, not an amendment.** The D7 block above is change
0016's historical registration and is left byte-identical on purpose; 0016 was
closed on it (`bench/engine/FINDINGS-workspace-graph.md`, 2026-08-22 adoption-
floor verdict). The bars below govern change 0010's corpus and **only** that
corpus. They are registered here **before any scored run on it**, and the gate
script must read them from this file, never from its own source.

### The frozen corpus these bars are stated over

Frozen 2026-08-22 in `tasks/tasks_ws.json` (header `subsets`), seed 1729.
**215 tasks.**

| shape | subset | n | php | ts | go | py |
|---|---|---:|---:|---:|---:|---:|
| `xcallers` | control (greppable) | 24 | 7 | 5 | 6 | 6 |
| `ximpact` | control (greppable) | 24 | 5 | 7 | 6 | 6 |
| `xnew` | control (greppable) | 10 | 9 | 1 | 0 | 0 |
| `xsubtypes` | structural (scored) | 80 | 56 | 24 | 0 | 0 |
| `xcollide` | structural (scored) | 20 | 9 | 6 | 1 | 4 |
| `xchain` | structural (scored) | 22 | 0 | 22 | 0 | 0 |
| `xalias` | structural (**excluded at freeze**) | 35 | 33 | 0 | 1 | 1 |
| **total** | | **215** | **119** | **65** | **14** | **17** |

Subset arithmetic, pinned at freeze:

- scored structural (excludes `xalias`) = 80 + 20 + 22 = **122** ≥ registered 105
- control (greppable) = 24 + 24 + 10 = **58** ≥ registered 40
- total = **215** ≥ registered 145

The kind→subset partition — control = {`xcallers`, `ximpact`, `xnew`};
structural = {`xsubtypes`, `xcollide`, `xalias`, `xchain`} — is emitted into the
task-file header and **read from there** by `grade_ws.py`. It is not hardcoded in
the grader and not restated as prose anywhere that could drift from the header.

### Structural facts recorded beside the corpus table

- **Go's zero `xsubtypes` cannot be fixed by adding a Go branch to `sub_pattern`.**
  The entire embedded-`client_golang`-type population in the consumer is one
  line: `bench/repos/prometheus/storage/remote/max_timestamp.go:25`,
  `prometheus.Gauge`. Go's real subtyping is **implicit interface satisfaction**
  — 3 files in `prometheus` define `Collect(ch chan<- prometheus.Metric)` and so
  satisfy `prometheus.Collector` **without naming it at all**. A textual
  cross-member miner cannot compute that relation, and a Go `sub_pattern` branch
  would not close the gap. Go supplies **zero** `xsubtypes`, permanently, by
  construction.
- **Python's zero is real but small** (~+4–5 tasks if closed). `flask`
  subclasses `werkzeug` under **aliases** —
  `flask/wrappers.py:7` `from werkzeug.wrappers import Request as RequestBase`,
  line 8 the same for `Response as ResponseBase`, then
  `class Request(RequestBase)` / `class Response(ResponseBase)` — which the
  alias-blind subtype pattern misses; the proper-subset guard drops 2 more.
- **`xcollide` collisions are cross-language by construction**, because
  `corpus.json` declares exactly **one** shared lib per language, so two
  same-named declarations almost always sit in different languages (9 of the 10
  emitted collision groups). A text search is language-blind, so the shape stays
  legitimate — but recorded honestly: **file extension is a partial
  disambiguator** for those 9. The `Response` group is the exception and is
  genuinely same-language: it carries a within-`werkzeug` clash
  (`werkzeug.sansio.response.Response` vs `werkzeug.wrappers.Response`) and, in
  the bare-name union, the within-PHP clash `Symfony\Component\BrowserKit\Response`
  vs `Symfony\Component\HttpFoundation\Response`.
- **`xchain` is nest-only.** The php, py and go clusters are two members deep
  (lib + consumer), so no A→B→C chain exists in them. Recorded as a corpus fact;
  no chain was synthesised and no member was added to manufacture one.

### The bars (all required)

**B1 — structural lift (PRIMARY).** At **haiku**
(`claude-haiku-4-5-20251001`), arm B's **mean** cross-recall on the **scored
structural subset** (122 tasks) ≥ arm A's mean **+ 10pp**; **OR** B meets the
efficiency bar (≥40% fewer exploration tokens **or** ≥40% fewer tool/shell
calls, measured over the cross-repo tasks) with recall B ≥ A.

- **This is a change of quantity** from the `2026-08-21: registered follow-up —
  adoption-floor sweep (small models)` entry, which stated a **median** lift.
  The reason is stated up front so it cannot be read as post-hoc: a median over
  a structural subset is likely **degenerate** (0.0 vs 0.0, both arms failing
  most tasks) and therefore *less informative*, not *less gameable*. The mean is
  the quantity in which the observed effect exists.
- Because a mean over 122 tasks **can be carried by a single shape**, this
  registration **also requires**, reported alongside the bar and before any
  verdict: **per-shape means** (`xsubtypes`, `xcollide`, `xchain` separately) and
  the **structural-subset median**. A lift that lives entirely in one shape must
  be visible as such, not hidden inside the subset mean.
- Anchor for both the quantity and the tier: the `xsubtypes` **0.29 / 0.43
  (+14pp, n=7)** observation in the 2026-08-22 adoption-floor verdict is a
  **mean** at **haiku**. The bar is stated in that quantity, at that tier,
  because that is where the effect was seen.

**B2 — floor competence (RESTORED).** Arm B rung-1 **median** cross-recall
**≥ 0.9 absolute** on the **greppable control subset** (58 tasks).

- This bar **PASSED on 2026-08-21** (B rung-1 median 1.0) and it is restored
  explicitly. Dropping a bar that a previous run passed is **silent erosion**;
  keeping it costs nothing and keeps the record honest.
- Evaluating it requires the kind→subset partition above: before it existed,
  "rung-1 median" was a whole-corpus figure (every task is rung-1), so B2 as
  originally worded was not evaluable on this corpus.

**B3 — non-inversion.** Arm B **median** cross-recall ≥ arm A on the **control
subset**. The index must not make the greppable shapes worse.

**B4 — leak audit (standing PRE-VERDICT gate).** `leak_audit_ws.py` PASS on all
**four** classes over the campaign transcripts — template leakage, control
contamination via the id-paired join, forced-tool prompt scan, and
grader-codesign ordering-invariance. The gate script **refuses a verdict on
non-zero exit**; the audit runs before grading, never after.

- Isolation is **unchanged**: `--setting-sources project,local`, the arm-A PATH
  shim, and `CODEINDEX_DISABLED`. The settings flag alone leaves a globally
  installed binary reachable.
- **Index artifacts must be PHYSICALLY ABSENT from the member checkouts, not
  renamed.** The 2026-08-22 quarantine established this the hard way: two arm-A
  haiku runs were quarantined for class-2 contamination after agents noticed
  `.codeindex/` directories — one attempt renamed them in place, which is
  visible, and a depth-limited `find` missed the three nest package roots.
  **Renaming is not hiding, and observation alone fails the class**, whether or
  not any agent read index contents.
- **LIVE PRECONDITION (measured 2026-08-22, not yet satisfied):** index
  artifacts are on disk right now and **must be moved outside `bench/repos/`
  before the scored run**. Measured by `find -L bench/repos -type d -name
  .codeindex`: **16** `.codeindex/` directories under `bench/repos/`, of which
  **12** sit inside workspace member checkouts and **11** contain a `graph.db`
  (`symfony`, `drupal`, `laravel-framework`, `nest`, `nest/packages/common`,
  `nest/packages/core`, `nest/packages/microservices`, `flask`, `werkzeug`,
  `client_golang`, `prometheus`; `oss-ws/.codeindex` holds `workspace.db` +
  `workspace.json`). This supersedes the plan's figure of 13 and agrees with the
  quarantine record's "all 16". Depth matters: a shallow scan misses the three
  nest package roots, and `bench/repos/*` are symlinks, so the scan must follow
  them (`find -L`). These are the owner's local artifacts; change 0010 records
  the requirement and does not delete them.

**B5 — freeze discipline.** Per-shape `n`, per-language `n`, and any excluded
shape are pinned into this registration **at freeze**, together with the
exclusion arithmetic — never decided or adjusted after mining, and never after a
run.

- **`xalias` is excluded from the scored structural subset**: n = **35**
  (php 33 / py 1 / go 1 / ts 0). Reason: change **0018** (aliased-import
  resolution) has **not landed**, so the index cannot answer alias tasks;
  scoring them would measure a known-absent capability. It is mined, counted and
  reported separately — the exclusion is a registration fact, not a reason to
  skip the work.
- The **aggregate floor GOVERNS over the per-shape sum**: the registered
  structural floor of **105** was **MET without `xalias`** (122 ≥ 105), so **no
  floor was lowered** and no shape was reinstated to reach it.

**EFFICIENCY — reported, not an independent bar.** Exploration tokens and
tool/shell calls are **reported per run**, and enter the verdict **only** through
B1's OR-clause. This is **faithful carry-forward, not a relaxation**: the
`2026-08-21: registered follow-up — adoption-floor sweep (small models)` entry
already placed efficiency solely inside its OR-clause, so efficiency has not been
an independent bar since that registration. (The standalone efficiency bar in the
D7 block above belongs to change 0016's corpus and verdict, both closed.)

### Kill condition

If arm B clears **none** of the bars, the result is published as a FINDINGS entry
in `bench/engine/FINDINGS-workspace-graph.md` and **the pivot closes**. The
grep-across control winning on a deliberately grep-hostile corpus is a
legitimate, publishable answer.

A **PASS revives the killed 0016 query surfaces AS A FRESH CHANGE** — a new
change with its own spec, plan and review — **not** as a rescue of the old one.
0016 and PR #14 stay closed on their own registered verdict.

### 0017 / 0018 ordering (coupling, and why it is ordering)

Coupling to changes 0017 and 0018 is encoded as **ordering**, not as
`depends_on`: a **killed** dependency would deadlock permanently, because
`killed` is terminal but never `done`.

- **Change 0017 LANDED** — status `done`, 2026-08-22, merge `ecac858`. The
  ordering constraint is **satisfied, not bypassed**.
- Measured per-member subtype-edge hint rates, as
  `sum(dst_ns <> '') / count(*)` over `bench/repos/*/.codeindex/graph.db`:

  | member | `extends` hinted | `implements` hinted |
  |---|---|---|
  | symfony | 3126 / 4834 | 1553 / 2844 |
  | drupal | 5516 / 8010 | 3357 / 4303 |
  | laravel | 1666 / 2314 | 1230 / 1533 |
  | nest | 151 / 194 | 135 / 136 |
  | flask | 25 / 74 | — |
  | werkzeug | 27 / 135 | — |
  | client_golang | 0 / 131 | — |
  | prometheus | 8 / 128 | 4 / 6 |

  A `—` means the member's index holds **no** edges of that kind at all
  (measured: `flask`, `werkzeug` and `client_golang` each have 0 `implements`
  edges) — not that the rate was unmeasured.

- **The sufficient claim**, stated exactly as measured: Go subtype edges are
  **unhinted** (client_golang 0/131) **and Go supplies zero subtype tasks**, so
  the unhinted Go edges cannot affect this gate; PHP and TS edges are
  **substantially** hinted (PHP 55–80%, **not** ~100%) and PHP+TS are **100% of
  the subtype supply**. The claim is sufficiency, not completeness — the PHP
  shortfall is recorded, not rounded away.
- **0018 has NOT landed** (`proposed`, no spec) — which is exactly why `xalias`
  is excluded above.

### Phase 2 — monorepo declaration-format coverage (criteria registered here, work is later)

Registered now so the criteria cannot be chosen to fit whatever repo happens to
be convenient later. Phase 2 is **secondary and non-blocking**: it must not gate
the verdict and may land after it.

Selection criteria for every new member:

- OSS, **permissive licence**, pinned to an **exact tag or commit**.
- The declaration format must appear **ORGANICALLY** in the repo. A synthesised
  monorepo proves nothing about discovery and is not admissible.

Hard bounds:

- at most **ONE** member per format (5 formats ⇒ **≤5** new members);
- each checkout **≤ 500 MB**;
- at most **THREE** candidate repos evaluated per format.

If no candidate qualifies within those bounds, the format is recorded as
**UNCOVERED** and the search **STOPS**. Uncovered is an honest result;
fabricated coverage is not.

| declaration format | status |
|---|---|
| `go.work` | needs coverage |
| `pnpm-workspace.yaml` | needs coverage |
| npm/yarn `workspaces` | needs coverage |
| composer path repositories | needs coverage |
| Python multi-member | needs coverage |
| `lerna.json` | already covered by `nest` |

#### Phase 2 result — per-format coverage (recorded)

Pins live in `discovery_corpus.json`, which is **discovery-only**:
`build_tasks_ws.py` does not read it, `corpus.json` is unmodified, and
`tasks/tasks_ws.json` stays frozen at 215 tasks (24/24/10/80/20/35/22). Every
member below therefore carries a **task quota of 0** by construction — phase 2
changes discovery coverage, not the task corpus.

Member counts are measured, not asserted: each root was passed to
`internal/workspace.Members` (the code `init-workspace --scan` uses) via a
throwaway probe. `vite` and `babel` were shallow-cloned at the pinned tag,
probed, and discarded — they are **not** vendored under `bench/repos/`; the pin
is the record.

| declaration format | status | member | pin (commit) | licence | members found | task quota |
|---|---|---|---|---|---|---|
| `lerna.json` | **covered** (pre-existing) | `nestjs/nest` | `v10.4.15` (`d0fb875`) | MIT | 9 | 0 |
| composer path repositories | **covered** | `symfony/symfony` | `v7.2.2` (`fca27be`) | MIT | 2 | 0 |
| `pnpm-workspace.yaml` | **covered** | `vitejs/vite` | `v6.0.7` (`a671e58`) | MIT | 73 | 0 |
| npm/yarn `workspaces` | **covered** | `babel/babel` | `v7.26.0` (`63d3038`) | MIT | 173 | 0 |
| `go.work` | **UNCOVERED** | — | — | — | — | 0 |
| Python multi-member | **UNCOVERED** | — | — | — | — | 0 |

Uncovered reasons, recorded rather than rounded away:

- **`go.work`** — the three-candidate bound was spent on `grafana/loki`,
  `open-telemetry/opentelemetry-collector` and `tailscale/tailscale`; none
  declares a root `go.work`. The on-disk Go repos (`gin` v1.10.0, `prometheus`
  v3.1.0, `client_golang` v1.20.5) have none either and discover 0 members.
  Bound reached, search **STOPPED**.
- **Python multi-member** — structural, not a search failure. `members.go` reads
  exactly five declaration sources and **none is Python**; `pyproject.toml`,
  `setup.py` and `setup.cfg` appear only in `memberMarkers`, i.e. as evidence
  that an already-declared candidate is a real member, never as a declaration of
  members. No Python repo can be discovered, so no pin would help — covering
  this format is an **engine** change, not a corpus change. (`flask` 3.1.0 and
  `werkzeug` 3.1.3 were checked and discover 0 members; `flask`'s `examples/`
  subprojects are declared nowhere.)

One incidental finding: `vite`'s organic `pnpm-workspace.yaml` exercises the
recorded `**` limitation — `playground/**` degrades to a single-level glob and
`packages/**/__tests__/**` matches nothing, so 73 members is the truncated
count, not the full declaration.

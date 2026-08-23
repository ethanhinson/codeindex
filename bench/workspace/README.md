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
**207 tasks.**

| shape | subset | n | php | ts | go | py |
|---|---|---:|---:|---:|---:|---:|
| `xcallers` | control (greppable) | 24 | 7 | 5 | 6 | 6 |
| `ximpact` | control (greppable) | 24 | 5 | 7 | 6 | 6 |
| `xnew` | control (greppable) | 10 | 9 | 1 | 0 | 0 |
| `xsubtypes` | structural (scored) | 68 | 55 | 13 | 0 | 0 |
| `xcollide` | structural (scored) | **0** | 0 | 0 | 0 | 0 |
| `xchain` | structural (scored) | 46 | 0 | 46 | 0 | 0 |
| `xalias` | structural (**excluded at freeze**) | 35 | 33 | 0 | 1 | 1 |
| **total** | | **207** | **109** | **72** | **13** | **13** |

`xcollide` stays a **registered** shape at n = 0 — the row is an explicit zero
in the task header (`subsets.per_shape_n`), not an omission. Its zero is a
corpus fact with a stated prerequisite; see the structural facts below.

Subset arithmetic, pinned at freeze:

- scored structural (excludes `xalias`) = 68 + 0 + 46 = **114** ≥ registered 105
- control (greppable) = 24 + 24 + 10 = **58** ≥ registered 40
- total = **207** ≥ registered 145

**Re-freeze 2026-08-22, before any scored run on this corpus** (`xsubtypes`
80 → 68; 215 → 203 total). The first freeze's `xsubtypes` ground truth was
mined with a textual `(?:extends|implements)[^{;]*?\bNAME\b`, which matched
across newlines and required no declaration: it fired on TS **type-parameter
lists** (`explore<T extends HttpServer = any>`, `tryActivate<TContext extends
string = ContextType>(… instance: Controller,`) and on multi-line **PHP
docblock prose** (`An object that implements \Traversable which …`). Measured
against the first freeze: **35 of its 187** `xsubtypes` ground-truth entries
were not declarations, and **13 tasks were wholly wrong** (review named 10; two
more TS tasks and `Type` fell to the same defect and are listed below). On a
wholly-wrong task a correct agent scores 0.0 and a search-everything agent
scores 1.0, which **inverts B1** — the primary bar — on ~11% of its tasks.
The 13, all of which now mine an empty answer and are therefore not emitted:
`Controller`, `Type`, `CanActivate`, `Injectable`, `RouteParamtypes`,
`NestApplicationContextOptions`, `NestApplicationOptions`, `RequestMethod`,
`DynamicModule`, `Provider`, `EnhancerSubtype`, `RouteInfo` (ts) and
`ContainerInterface` (php). One task is **added** — `INestApplication`, whose
corrected answer is now a proper subset of its `xcallers` set instead of equal
to it.
The miner now scans the declaration header structurally (`declared_supertypes`,
dialect-gated: angle-bracket depth for TS, comments/heredocs for PHP), so the
`extends`/`implements` must head a base type of a real
class/interface/trait/enum declaration. Only `xsubtypes` moved — the other six
shapes' ground truth is byte-identical to the first freeze. **No floor was
lowered**: the aggregate rule was re-evaluated at freeze and scored structural
still clears the registered 105 at 110.

**Re-freeze 2026-08-22 (second), before any scored run on this corpus**
(`xchain` 22 → 46; 203 → 227 total). `xchain` is registered **structural**, and
this section defines structural as *not* greppable — "the bare name is the
answer key" is what makes a shape control. The first two freezes' `xchain`
prompt broke that: it named **both** hops, so its operative sentence
("…list every file in the OTHER member projects (not `../nest/packages/core`)
that references `ApplicationConfig, imported from '@nestjs/core'`…") was
`xcallers` character for character, and every one of its **48** ground-truth
entries was reachable by searching the name the prompt handed over. That put
22 greppable tasks — **20%** of the then-110 scored structural tasks — inside
the subset B1 is measured on. It was also the only shape with **no**
non-degeneracy emit guard (`xnew`/`xsubtypes`/`xalias` require a proper subset
of the `xcallers` set, `xcollide` a proper subset of the bare-name union;
`xchain` checked only the 40-file cap).

Two things changed, and nothing else did — the other six shapes' 181 tasks are
**byte-identical** to the previous freeze:

- **The prompt names the hop-1 symbol only.** The task is now keyed on the
  member-A (`nest-common`) symbol; the member-B (`nest-core`) intermediaries
  are the agent's to find. Keying on hop 1 is also the only **well-posed** unit,
  which is why the count moved: several `nest-core` symbols can carry the same
  `nest-common` symbol onward, so naming A alone has a single answer only if
  ground truth is the **union** over all of them. 60 hop-1 symbols qualify. The
  intermediaries are recorded per task (`via_member`, `via_symbols`,
  `via_files`) for audit and are **never rendered into the prompt**.
- **A non-degeneracy guard** (`xchain_is_structural`), the shape's equivalent of
  the proper-subset tests the other shapes carry. A proper-subset test is the
  wrong instrument here — an `xchain` answer is not drawn from the named
  symbol's reference set at all — so the guard requires the strictly stronger
  **disjointness**: not one ground-truth file may itself reference the named
  symbol, and the rendered prompt may not spell an intermediary's name as a
  whole word. It **rejects 14** of the 60 — `CanActivate`, `ContextType`,
  `Controller`, `ExceptionFilter`, `Injectable`, `Logger`, `NestInterceptor`,
  `PARAMTYPES_METADATA`, `PipeTransform`, `Scope`, `WebSocketAdapter`,
  `isEmpty`, `isFunction`, `isUndefined` — each one a symbol some
  `nest-microservices` file both imports directly and reaches through
  `nest-core`. The whole task is rejected, not
  the offending file trimmed: trimming would falsify the prompt, which asks for
  every file reached through the intermediary, and that file is one of them.
  46 emit. Both clauses are asserted under `--selftest`, and the emitted corpus
  is re-checked **from disk** (`xchain_nondegeneracy` re-reads every ground-truth
  file rather than trusting a miner-written number).

**No floor was lowered and nothing was padded** — the aggregate moved **up** at
that freeze: scored structural 110 → **134** (≥ 105), control **58** unchanged
(≥ 40), total 203 → **227** (≥ 145). `xchain` was then **34%** of the scored
structural subset (46/134) and is **40%** after the third re-freeze below
(46/114), which is exactly why B1's standing requirement to report **per-shape
means** alongside the subset mean is load-bearing rather than decorative.

**Re-freeze 2026-08-22 (third), before any scored run on this corpus**
(`xcollide` 20 → 0; 227 → 207 total). `xcollide` groups a bare name across
**languages**, and a cross-language "collision" is not one. The prompt asks the
agent to pick out the files bound to one declaration of `{BARE}`; when the rival
declaration is in another language the **file extension separates them
completely**, and what is left of the task is `xcallers` verbatim — the same
degeneracy the second re-freeze removed from `xchain`, arriving this time
through the GROUPING rather than through a prompt template. **All 10** emitted
collision groups spanned two languages — `ArgumentMetadata`,
`BadRequestException`, `ConsoleLogger`, `Headers`, `HttpException`, `Optional`,
`Range`, `Request`, `Response`, `Type` — and **18 of the 20** tasks had no
same-language rival at all: the only other declaration of the name was in
another language, so the extension did the whole job. Since `xcollide` is
registered **structural**, and this section defines structural as "the bare name
is not the answer key", those 18 were inside the subset B1 is measured on
without meeting its definition.

The **collision key is now language-gated** — `(lang, bare)`, not `bare`
(`xcollide_key`) — so only a genuine same-language clash can group. That is the
`dialect-specific-remedies-need-a-language-gate` rule applied to the key rather
than to a regex: language is part of the identity of a name.

**The gate empties the shape, and the zero is reported rather than engineered
around.** `corpus.json` declares exactly **one** shared lib per language
(`symfony`/php, `nest-common`/ts, `werkzeug`/py, `client_golang`/go), so no bare
name is declared by two members of the **same** language and the shape's ">= 2
declaring members" test can never be met here. The remaining 2 tasks were the py
`Response` pair, whose two declarations (`werkzeug.sansio.response.Response`,
`werkzeug.wrappers.Response`) sit in the **same member** and so pose no
cross-member disambiguation question at all; admitting them would have meant
dropping that test, which is buying a count back by weakening what the shape
asserts. Neither that nor ungating the key was done.

Nothing else changed: the other six shapes' **207** tasks are byte-identical to
the previous freeze field for field, the only difference being the `id` ordinal
of `xalias` and `xchain`, which shift down by 20 because a single counter
numbers every shape in emit order. **No floor was lowered and nothing was
padded** — the aggregate was re-evaluated at freeze and scored structural still
clears the registered 105 at **114** (68 + 0 + 46); control **58** unchanged;
total 203 → 227 → **207** (≥ 145). Both `xcollide` guards keep RED cases under
`--selftest` (`xcollide_guard_cases`) even though no task exercises them, and
the zero itself is asserted (`known_limitations` (4)) so it cannot pass
unnoticed.

The kind→subset partition — control = {`xcallers`, `ximpact`, `xnew`};
structural = {`xsubtypes`, `xcollide`, `xalias`, `xchain`} — is emitted into the
task-file header and **read from there** by `grade_ws.py`. It is not hardcoded in
the grader and not restated as prose anywhere that could drift from the header.

### Structural facts recorded beside the corpus table

- **Go's zero `xsubtypes` cannot be fixed by adding a Go branch to `sub_matcher`.**
  The entire embedded-`client_golang`-type population in the consumer is one
  line: `bench/repos/prometheus/storage/remote/max_timestamp.go:25`,
  `prometheus.Gauge`. Go's real subtyping is **implicit interface satisfaction**
  — 3 files in `prometheus` define `Collect(ch chan<- prometheus.Metric)` and so
  satisfy `prometheus.Collector` **without naming it at all**. A textual
  cross-member miner cannot compute that relation, and a Go `sub_matcher` branch
  would not close the gap. Go supplies **zero** `xsubtypes`, permanently, by
  construction.
- **Python's zero is real but small** (~+4–5 tasks if closed). `flask`
  subclasses `werkzeug` under **aliases** —
  `flask/wrappers.py:7` `from werkzeug.wrappers import Request as RequestBase`,
  line 8 the same for `Response as ResponseBase`, then
  `class Request(RequestBase)` / `class Response(ResponseBase)` — which the
  alias-blind subtype pattern misses; the proper-subset guard drops 2 more.
- **PHP subtype mining is alias-blind for the same reason** — recorded here
  because the 2026-08-22 re-freeze made it visible. `declared_supertypes` reads
  the supertype as it is written in the declaration header, so
  `use Symfony\Component\Validator\Constraint as SymfonyConstraint;` followed by
  `class PluginExistsConstraint extends SymfonyConstraint` is **not** counted as
  a `Constraint` subtype (`drupal`'s `PluginExistsConstraint.php` left the
  `Constraint` ground truth at the re-freeze on exactly this rule). This is the
  same missing capability as python's, and closing it is change **0018**'s
  aliased-import resolution — not a looser pattern in the miner, which is what
  produced the contaminated first freeze.
- **`xcollide` supplies ZERO tasks on this corpus, permanently until the pins
  change.** `corpus.json` declares exactly **one** shared lib per language, so
  no bare name is declared by two members of the **same** language, and the
  language-gated collision key therefore finds no group with two declaring
  members (task header: `xcollide_pass.declaring_members_per_lang` = 1 for all
  four languages, `multi_member_groups` = `[]`). The 10 bare names that do
  occur in more than one language are recorded there too
  (`cross_language_bare_names_separated`) — they are separated, not lost. The
  **prerequisite is new corpus pins** (a second declaring member in some one
  language), not a miner change; the two things that are explicitly **not** the
  fix are ungating the key and dropping the ">= 2 declaring members" test to
  admit within-member clashes such as `werkzeug.sansio.response.Response` vs
  `werkzeug.wrappers.Response`.
- **The scored structural verdict is a PHP + TS verdict — there are no Go or
  Python structural tasks at all.** Stated here rather than left to be inferred
  from the table: `xcollide` was the only shape supplying Go and Py structural
  tasks, so its zero makes the scored subset **php 55 + ts 59 = 114**
  (`xsubtypes` php 55 / ts 13, `xchain` ts 46). This is consistent with what the
  spec already says the structural verdict would be, and with the 0017
  sufficiency claim below ("PHP and TS are 100% of the subtype supply") — it is
  now true of the whole scored structural subset, not just of `xsubtypes`. Go
  and Py remain represented in the **control** subset (`xcallers` 6 + 6,
  `ximpact` 6 + 6) and in the excluded `xalias`, so B2/B3 still range over all
  four languages.
- **`xchain` is nest-only.** The php, py and go clusters are two members deep
  (lib + consumer), so no A→B→C chain exists in them. Recorded as a corpus fact;
  no chain was synthesised and no member was added to manufacture one.
  (`defining_member` for these tasks is `nest-common`, not `nest-core`, since
  the second re-freeze: the named symbol is the hop-1 one. The limitation is
  unchanged and its prerequisite — new corpus pins — is untouched.)

### The bars (all required)

**B1 — structural lift (PRIMARY).** At **haiku**
(`claude-haiku-4-5-20251001`), arm B's **mean** cross-recall on the **scored
structural subset** (114 tasks) ≥ arm A's mean **+ 10pp**; **OR** B meets the
efficiency bar (≥40% fewer exploration tokens **or** ≥40% fewer tool/shell
calls, measured over the cross-repo tasks) with recall B ≥ A.

- **This is a change of quantity** from the `2026-08-21: registered follow-up —
  adoption-floor sweep (small models)` entry, which stated a **median** lift.
  The reason is stated up front so it cannot be read as post-hoc: a median over
  a structural subset is likely **degenerate** (0.0 vs 0.0, both arms failing
  most tasks) and therefore *less informative*, not *less gameable*. The mean is
  the quantity in which the observed effect exists.
- Because a mean over 114 tasks **can be carried by a single shape**, this
  registration **also requires**, reported alongside the bar and before any
  verdict: **per-shape means** (each scored structural shape separately — on
  this corpus `xsubtypes` and `xchain`; `xcollide` is registered but emits 0, so
  it is reported as `n=0`, never dropped) and the **structural-subset median**.
  A lift that lives entirely in one shape must be visible as such, not hidden
  inside the subset mean. With `xchain` at 40% of the subset (46/114) this is
  the operative safeguard, not a formality.
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
  structural floor of **105** was **MET without `xalias`** (114 ≥ 105), so **no
  floor was lowered** and no shape was reinstated to reach it — including at the
  third re-freeze, where `xcollide` fell to 0 and the aggregate was re-evaluated
  rather than the shape or a guard adjusted to hold a number.

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
`tasks/tasks_ws.json` stays frozen at 207 tasks (24/24/10/68/0/35/46). Every
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

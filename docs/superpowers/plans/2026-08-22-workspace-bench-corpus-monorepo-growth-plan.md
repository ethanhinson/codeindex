<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0010 — Grow the workspace bench corpus — monorepo declaration coverage in every supported language](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0010-workspace-bench-corpus-monorepo-growth.md)**
<!-- docket:backlink:end -->

# Plan — workspace bench corpus: structural growth + new pre-registered gate

Change 0010. Spec: `docs/superpowers/specs/2026-08-22-workspace-bench-corpus-monorepo-growth-design.md`
(on `origin/docket`). **Bench-only. No engine code.**

## Ground rules that bind every task

- **No engine code.** Nothing under `internal/`, `cmd/`, or any Go source changes.
  The Go suite is run as a regression gate only.
- **OSS-only ground truth.** Nothing derived from `bench/repos/btt-ws-private/` or
  any private repo may enter tasks, GT, README text, or commit messages.
- **Bars before any scored run.** Task 7 (registration) must be committed before
  anything scored is executed. This change does **not** run the scored campaign.
- **Determinism.** Mining is seeded (`--seed 1729`). Any new candidate ordering
  must be totally ordered — see the determinism learning: a tie broken by
  dict/set iteration order is a latent non-determinism that a re-run in the same
  process will not catch.
- **Prompt vocabulary is constrained by the leak audit.** `leak_audit_ws.py`'s
  `FORCING` list bans the substrings `codeindex`, `mcp`, `grep`, `ripgrep`,
  ` rg `, `glob(`, `use the tool`, `must use`, `call the` in any prompt.
  Every new prompt template must avoid all of them. (`callers` is safe;
  `call the` is not.)

## Baseline, re-measured at reconcile (do not re-derive)

| quantity | value |
|---|---|
| frozen corpus | 65 tasks — xcallers 24, ximpact 24, xnew 10, xsubtypes 7 |
| control subset today | 58 (xcallers + ximpact + xnew) |
| candidates mined | 589 |
| primary picked | 48 (12 per lib × 4 libs, `PER_LIB_PRIMARY_CAP`) |
| xsubtypes, all candidates, **no** cap | 82 — of which **2** exceed 40 GT files (max **164**) |
| xsubtypes, all candidates, **emitted-GT** cap | **80** — php 56 / ts 24; symfony 56, nest-common 24 |
| xsubtypes, all candidates, candidate-level cap | 68 — php 45 / ts 23 (**wrong predicate**, see task 1) |
| xnew with `picked` retained | 10 (php 9, ts 1) — unchanged |

---

## Task 1 — Fix the `picked` artifact, scoped, with the GT cap re-applied on emitted GT

**Files:** `bench/workspace/build_tasks_ws.py`

Two coupled edits; both required, neither alone is correct.

1. **Scope the `picked` guard to `xnew` only.** The guard `if not c.get("picked"): continue`
   sits inside `for kind in ("xsubtypes", "xnew"):`. `xsubtypes` must iterate all
   candidates; `xnew` must keep the guard. Removing it wholesale drives `xnew`
   10 → 126 (122 PHP), control 58 → 174, and the corpus to 192/256 symfony —
   forbidden, and it makes the registered floors vacuous.
2. **Apply `MAX_GT_FILES` to the EMITTED subtype GT**, i.e. `if len(gt) > MAX_GT_FILES: continue`
   evaluated on the sub-kind's own `gt`, **not** by reusing the primary loop's
   candidate-level predicate `c["cross_files"] > MAX_GT_FILES`. This is the
   decisive detail: the candidate-level predicate yields 68 and is a different
   rule; emitted-GT yields exactly 80 and is what the registered corpus rule
   ("GT size is capped at 40 files") actually says, because the cap exists to
   bound the answer the agent must list.

**Invariant hygiene (learning: `one-invariant-many-sites-drifts`).** After this
task the 40-file cap is enforced at the primary loop and at every sub-kind loop,
and tasks 2–4 add three more sites. Do **not** copy the predicate five times.
Introduce one helper — e.g. `def gt_within_cap(gt) -> bool` — and call it at every
emit site, so the sites cannot drift and their comments cannot come to disagree.
The primary loop's candidate-level check may remain as a cheap pre-filter, but the
authoritative gate is the emitted-GT one at emit time.

**Verify (all four must hold):**
- `xsubtypes` == 80, split php 56 / ts 24, members symfony 56 / nest-common 24
- `xnew` == 10
- control subset (`xcallers`+`ximpact`+`xnew`) == 58
- no emitted task anywhere has `len(gt_files) > 40`

---

## Task 2 — Implement `xcollide` (same bare name defined in ≥2 members)

**Files:** `bench/workspace/build_tasks_ws.py`

New extraction pass + `PROMPTS` entry + GT.

- **Shape:** a bare name defined in ≥2 members. Grep on the bare name returns the
  union of references to both definitions; the correct answer is only the files
  bound by import to *one* of them. This is what makes it structural.
- **GT:** the subset of referencing files whose import binding resolves to the
  named defining member. Compute it from the already-extracted per-language
  symbol keys — those keys are fully qualified (PHP FQCN, `pkg:Name`,
  `module.Name`, `importpath.Name`), so the binding is already available; the
  collision is on `bare_name(symbol)`.
- **Emit guard:** emit only when the GT is a **proper subset** of the bare-name
  union across members. If they are equal there is no collision to disambiguate
  and the task is `xcallers` rephrased.
- **Language gating (learning: `dialect-specific-remedies-need-a-language-gate`).**
  Any part of the rule justified by one language's semantics must be gated to that
  language explicitly. In particular Go's `pkg.Name` selector form and PHP's
  leading-backslash FQCN form are *not* interchangeable with the TS/Py forms. Do
  not implement a single regex whose correctness argument cites PHP and then let
  it run for all four languages.
- **Prompt:** must state which defining member is meant, without naming any tool
  or route, and without any banned substring.

**Verify:** emitted n > 0; per-language split recorded; every emitted GT a proper
subset of the union; no GT over 40 files.

---

## Task 3 — Implement `xalias` (aliased/renamed imports), proper-subset guarded

**Files:** `bench/workspace/build_tasks_ws.py`

- **Shape, stated correctly:** this is a **subset-filter** task. The intuitive
  rationale ("grep for the symbol's own name misses aliased files") is
  **backwards** — the aliasing statement itself contains the original name, so a
  text search returns a strict **superset**. The task is to return only the files
  that bind the symbol under an alias.
- **Emit guard is mandatory:** apply the proper-subset guard `if gt == c["gt"]: continue`.
  Without it the shape degenerates into `xcallers` rephrased and would inflate the
  structural count with non-structural tasks.
- **Language gating:** PHP `use X as Y;`, TS `import { X as Y }`, Py
  `from m import X as Y`, Go `alias "path"`. Four distinct forms; four gated
  branches. TS is measured at **0** and that is an expected, recorded outcome, not
  a bug to code around.
- **Freeze posture, already decided at reconcile:** change 0018 has not landed
  (`proposed`, no spec), so `xalias` is **mined, its `n` recorded, and declared
  excluded from the scored structural subset at freeze**. Implement it fully and
  record it; the exclusion is a registration fact, not a reason to skip the work.

**Verify:** emitted n recorded per language; every GT a proper subset; no GT over 40.

---

## Task 4 — Implement `xchain` (transitive A→B→C across members)

**Files:** `bench/workspace/build_tasks_ws.py`

- **No new machinery.** Run the existing extraction a second time with a consumer
  member treated as a lib, using the `namespaces` it already declares in
  `corpus.json`.
- **Precondition:** `libs = [m for m in members if "shared lib" in m["role"]]`
  excludes `nest-core` (role: `consumer of nest-common (monorepo member)`). The
  chain pass needs **explicit member selection**, not the role filter. Do not
  "fix" this by editing the role string in `corpus.json` — that would silently
  change what the primary pass mines and re-open the whole frozen baseline.
- **Chain join:** hop-1 `nest-common` → `nest-core`; hop-2 `nest-core` →
  `nest-microservices`, keeping symbols whose hop-2 definition file also
  references `nest-common`. Measured: 232 exported `nest-core` symbols, 30
  imported by `nest-microservices` across 21 files, joining to **22 chain symbols
  across 11 files**.
- **nest-only, and that is a recorded corpus fact** — the other three clusters are
  2 members deep, so no chain exists in them. Record it; do not synthesise one.

**Verify:** emitted n ≤ 22, recorded; GT correct against a hand-checked sample;
no GT over 40.

---

## Task 5 — Build the structural/control partition artifact

**Files:** `bench/workspace/build_tasks_ws.py` (emit), `bench/workspace/grade_ws.py` (read)

**No such partition exists in any artifact today** — this is a deliverable, not an
assumption. `grade_ws.py`'s `rung1_med_cross_recall` is a whole-corpus figure
because every task is `rung1`, so B2 as worded is currently unevaluable.

- Emit a registered **kind → subset** map into the task-file header:
  - control (greppable): `xcallers`, `ximpact`, `xnew`
  - structural: `xsubtypes`, `xcollide`, `xalias`, `xchain`
- Record alongside it: per-shape `n`, per-language `n`, and any shape excluded at
  freeze (with `xalias`'s exclusion and its `n`).
- `grade_ws.py` reads the map from the header (never hardcodes it) and reports, per
  arm: control-subset median cross-recall (B2/B3), structural-subset **mean**
  cross-recall (B1), the structural-subset **median**, and **per-shape means** —
  the last two are required by the registration so a single-shape effect is
  visible rather than hidden inside a subset mean.

**Verify:** a grade run over the existing `results/runs.jsonl` produces the new
per-subset figures; the map is read from the header, not from source.

---

## Task 6 — Re-mine and re-freeze

- Run the miner; confirm the aggregate: **structural ≥ 105, control ≥ 40, total ≥ 145**.
- `xalias` is excluded at freeze, so the aggregate must be met **without it**:
  `xsubtypes` 80 + `xchain` (≤22) + `xcollide` ≥ 105.
- Pin per-shape `n` and per-language `n` into the header **at freeze**. If the
  aggregate is not reachable, lower it **at freeze with the arithmetic recorded** —
  never after a run.
- Run `--selftest`. Note `MIN_PER_LANG = 4` still applies; structural shapes are
  PHP+TS only by construction, which the control shapes already satisfy for py/go.

---

## Task 7 — Register the gate bars (before any scored run)

**File:** `bench/workspace/README.md`

**Append, do not amend.** Add a new dated section
`## Bars — 0010 structural gate (registered 2026-08-22)`. Leave the existing
`## Bars (copied verbatim from design D7 — all required)` block **intact** as change
0016's historical record; editing it in place would destroy that registration and
make the file self-contradictory on efficiency.

Content: B1–B5 verbatim per the spec, plus the kill condition, the phase-2 criteria
and bounds, the 0017/0018 ordering text with the measured hint rates, and the
Go/Python structural notes.

- **B1 — structural lift (primary).** At **haiku**, B **mean** cross-recall on the
  structural subset ≥ A + 10pp; **OR** B meets the efficiency bar (≥40% fewer
  exploration tokens or tool/shell calls over the cross-repo tasks) with recall
  B ≥ A. Must note this is a **change of quantity** from the 2026-08-21 registered
  median bar, and must require per-shape means + the subset median to be reported.
- **B2 — floor competence (restored).** B rung-1 **median** cross-recall **≥ 0.9
  absolute** on the greppable control subset. Passed 2026-08-21 (B rung-1 med 1.0);
  restored explicitly because dropping a passing bar is silent erosion.
- **B3 — non-inversion.** B median cross-recall ≥ A on the control subset.
- **B4 — leak audit.** All four classes PASS as a standing pre-verdict gate; the
  gate script refuses a verdict on non-zero exit. Isolation unchanged
  (`--setting-sources project,local`, arm-A PATH shim, `CODEINDEX_DISABLED`).
  Index artifacts **physically absent** from member checkouts — renaming is not
  hiding.
- **B5 — freeze discipline.** Per-shape `n`, per-language `n`, and any excluded
  shape pinned at freeze with the exclusion arithmetic.
- **Efficiency** is **reported** per run and enters the verdict only via B1's
  OR-clause — faithful carry-forward of the 2026-08-21 registration, cited by name.

Also record the two structural facts beside the corpus table: **Go cannot be fixed
by adding a Go `sub_pattern` branch** (its real subtyping is implicit interface
satisfaction, which a textual miner cannot compute), and **Python's zero is real
but small** (~+4–5, alias-mediated).

---

## Task 8 — Leak audit PASS + B4 precondition check

- Run `python3 leak_audit_ws.py` over the re-frozen set. **Template leakage** and
  **forced-tool** must PASS — these are fully determined by the new prompts and GT
  and are this change's responsibility. **Grader co-design** must PASS. **Control
  contamination** reads transcripts; with none under `results/transcripts/` it
  reports `NO-TRANSCRIPTS`, which is not a FAIL and is the owner's scored-run
  responsibility.
- Run `python3 leak_audit_ws.py --selftest` (id-pairing proof).
- **B4 live precondition:** 13 `.codeindex/` directories (7+ `graph.db`) currently
  sit under `bench/repos/*/`. Verify and **report** their presence; do not delete
  the owner's local artifacts as part of this change — record the requirement that
  they be physically absent before the scored run.

---

## Task 9 — Characterization tests for the recorded limitations

**Learning: `known-limitations-need-a-characterization-test`.** Three deliberate
gaps must be shipped as assertions of what the code **does**, not as prose that a
later implementer will "fix" into a regression:

1. Go and Python emit **zero** `xsubtypes` — with the prerequisite recorded (Go's
   implicit interface satisfaction is not textually computable; a `sub_pattern` Go
   branch does **not** close it).
2. TS emits **zero** `xalias`.
3. `xchain` is **nest-only**.

Extend `--selftest`, or add a small test entry point, so each asserts today's
behavior with a comment saying so explicitly.

---

## Task 10 — Phase 2: monorepo declaration-format pins (secondary, non-blocking)

Strictly bounded; must not gate the gate and may land after it.

- Criteria: OSS, permissive licence, pinned to an exact tag/commit, declaration
  format present **organically** (never synthesised).
- **Hard bounds:** ≤ **1** member per format (5 formats ⇒ ≤5 new members); each
  checkout ≤ **500 MB**; ≤ **3** candidate repos evaluated per format.
  **No qualifying candidate ⇒ record the format as `uncovered` and STOP.**
- Deliverable: a per-format coverage table in `bench/workspace/README.md`.
- Formats: `go.work`, `pnpm-workspace.yaml`, npm/yarn `workspaces`, composer path
  repositories, Python multi-member. (`lerna.json` already covered by nest.)
- If the bounds cannot be met within a reasonable effort, recording every format
  as `uncovered` with the reason is an **acceptable and honest** outcome. Phase 1
  is what gates the verdict.

---

## Task 11 — Regression gate

`go test -tags nollama -count=1 ./...` on the branch — zero new failures versus
`origin/main`. The bench Python is not in the Go suite; it is gated by tasks 1–9's
own verification. Foreground only.

## Out of scope

Engine code; private-repo material; running the scored D7 campaign; reviving the
0016 query surfaces.

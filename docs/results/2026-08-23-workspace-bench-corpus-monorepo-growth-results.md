<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0010 — Grow the workspace bench corpus — monorepo declaration coverage in every supported language](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0010-workspace-bench-corpus-monorepo-growth.md)**
<!-- docket:backlink:end -->

# Results — workspace bench corpus: structural growth + new pre-registered gate

Change 0010. Bench-only; **zero engine files** in the diff (verified). ADRs produced: **0015**, **0016**.

## Outcome

The corpus is re-aimed from greppable to structural and a new gate (B1–B5) is registered
**before any scored run**, as the discipline requires.

| | before | after |
|---|---|---|
| total tasks | 65 | **207** (floor 145) |
| control (greppable) | 58 | **58** (floor 40) |
| scored structural | 7 (`xsubtypes` only) | **114** (floor 105) |
| shapes | 4 | 7 implemented |

Final per-shape: `xcallers` 24, `ximpact` 24, `xnew` 10 (control 58); `xsubtypes` 68,
`xchain` 46, `xcollide` **0** (structural 114); `xalias` 35 **excluded at freeze**.
Per language: php 109, ts 72, go 13, py 13. **No floor was lowered** — the registered
aggregate was met as mined.

## THE ONE THING TO DO BEFORE MERGING — how this branch merges with your local bench line

`origin/main` does **not** contain `bench/workspace/` at all; the harness lives in **8 unpushed
commits on your local `main`**. Docket cuts feature branches from `origin/main` unconditionally, so
this branch could not inherit it.

What was done: the branch's **first commit (`73b50a9`) seeds exactly 7 harness files
byte-identical to your local main**, and every later commit is the real delta. So the branch's
`bench/workspace/` is a **strict superset** of yours — same base bytes plus this change.

**Resolution: take the feature branch's side for `bench/workspace/`.** No content is lost and no
history is rewritten. Your local bench line stays authoritative; nothing here force-pushes or
rebases it.

Deliberately **not** on the branch, so they merge untouched:
`bench/workspace/results*/` (your campaign archives, ~337 files), `bench/repos/` checkouts
(gitignored), and `bench/repos/btt-ws-private/` (private — owner rule).

Files this branch actually changes: `build_tasks_ws.py`, `grade_ws.py`, `README.md`,
`tasks/tasks_ws.json`, plus new `discovery_corpus.json`. **`corpus.json`, `run_ws.py` and
`leak_audit_ws.py` are byte-identical to your local main** (verified) — they ride only as the seed.

## Manual checks at the merge gate

1. **B4's precondition is UNMET on disk and this change did not fix it** (deliberately — they are
   your local artifacts). **16 `.codeindex/` directories and 11 `graph.db` files** sit under
   `bench/repos/*/`. They must be **physically absent — not renamed** — before any scored run;
   the 2026-08-22 quarantine established that renaming is not hiding.
   *Two traps worth carrying:* `bench/repos/*` entries are symlinks, so a plain `find` sees one;
   and a shallow scan misses `nest/packages/{common,core,microservices}/.codeindex`, which are
   themselves corpus members. Use `find -L bench/repos -type d -name .codeindex`. Two workers on
   this run disagreed here (13 vs 16) — **16 is correct** and is what the registration states.
2. **Prior campaign transcripts can no longer be graded against this corpus.** Task ids embed the
   emission ordinal, so re-freezing renumbered them; **39 of the 65 baseline ids no longer exist**.
   The 2026-08-21 frontier and haiku campaign artifacts are keyed to the old ids. This is inherent
   to re-freezing, not a defect — but it means the new gate starts from a fresh baseline.
3. **Read the registration in `bench/workspace/README.md`** and confirm the bars say what you want
   before anything scored runs. That file is the authority; the gate script reads bars from it.

## Review findings and dispositions

Whole-branch review (deep rung) returned **9 findings: 2 blocker, 3 important, 4 minor**. All were
fixed in-branch; the suite was re-run green afterward.

| # | Sev | Finding | State | Commit |
|---|---|---|---|---|
| 1 | blocker | `xsubtypes` GT contaminated by TS generic constraints and PHP docblocks — 13 tasks wholly wrong | **fixed** | `9fd9d62` |
| 2 | blocker | `xchain` prompt named the hop-2 symbol, making it verbatim an `xcallers` task | **fixed** | `9803a11` |
| 3 | important | `xcollide` grouped bare names across languages — extension alone disambiguated | **fixed** | `aabb15e` |
| 4 | important | Registered floors hardcoded at 4 sites, asserted nowhere | **fixed** | `784f177` |
| 5 | important | No assertion that GT satisfies its shape's semantics | **fixed** | `2206c80` |
| 6 | minor | `xchain` GT scanned only member C while the prompt asked wider | **fixed** | `9aa2347` |
| 7 | minor | Dead no-op `.replace()` | **fixed** (already removed by an earlier fix) | `19b2375` |
| 8 | minor | `grade_ws` coupled import to `sys.argv` | **fixed** | `9aa2347` |
| 9 | minor | Function-local `import os`; unused binding | **fixed** | `19b2375` |

The two blockers were real and consequential — both would have **inverted or trivialised B1, the
primary bar**, on a corpus that was already frozen and registered. Finding this before a scored run
is exactly what the pre-registration discipline is for.

Notable: the blocker-1 worker **rejected the reviewer's suggested fix** (`[^{;\n]*`, forbidding
newlines) as over-broad — it would have dropped legitimate multi-line declarations in both PHP and
TS — and implemented dialect-gated bracket-depth tracking instead. It also found **more** damage
than the review did (13 wholly-wrong tasks and 35 bad GT entries, vs the reported 10).

## Findings worth keeping (expensive to re-derive)

- **`xcollide` emits ZERO on this corpus, and cannot do better.** Once the collision key is
  language-gated, a qualifying task needs the same bare name declared in **≥2 members of the same
  language** — and `corpus.json` declares exactly **one shared lib per language**, so a same-language
  cross-member clash is structurally impossible here. The `Response` near-miss is two declarations
  in the *same* member (`werkzeug.sansio.response` vs `werkzeug.wrappers`), which the ≥2-members
  test correctly rejects. Closing this needs **new corpus pins**, not a miner change.
  *An alternative not taken, for your judgement:* the 20 cross-language tasks could have been kept
  and reclassified as **control** rather than dropped. They were dropped because control already
  clears its floor at 58 and the tasks add little; if you want the go/py coverage back, that is the
  cheap route.
- **The scored structural subset is PHP+TS only** (php 55, ts 59). Go and Python contribute zero
  structural tasks — consistent with what the spec already predicted, now measured and recorded.
- **`xchain` is 46 of the 114 scored structural tasks (40%).** Keying on the hop-1 symbol is the
  only well-posed unit, and it grew the shape from 22 to 46. B1 already requires per-shape means to
  be reported alongside the subset mean; that requirement is now **load-bearing rather than
  belt-and-braces**, because a single shape carries nearly half the subset.
- **The pre-existing frozen corpus violated its own registered rule.** The baseline 65-task set
  contained `ws-ximpact-Reference-001` at **41 GT files**, over the registered 40-file cap — the
  cap was only ever applied at the candidate level. See ADR-0015.
- **Go `xalias` availability was overstated in the spec**: measured 4 symbols across 2 packages /
  13 files, not 3 packages / 17 files.
- **PHP subtype mining is alias-blind**, the same gap already recorded for Python — drupal's
  `PluginExistsConstraint` aliases `Constraint` and is correctly absent from that GT. Change 0018's
  work would close both.
- **Python monorepos can never be discovered by `--scan`.** `internal/workspace/members.go` reads
  five declaration sources and none is Python; `pyproject.toml`/`setup.py`/`setup.cfg` appear only
  as member *confirmation* markers, never as declarations. The phase-2 "UNCOVERED" for Python is an
  **engine limitation, not a search failure**. This is the one item here that likely deserves its
  own change.

## Follow-ups

- **`xalias` (n=35) is excluded from the scored structural subset at freeze** because change 0018
  has not landed, so the index cannot answer alias tasks. Recorded per bar B5 with its arithmetic.
  When 0018 lands, including it is a deliberate re-freeze + re-registration, never a quiet edit.
- **`go.work` is UNCOVERED** after exhausting its registered three-candidate bound (loki,
  opentelemetry-collector, tailscale — all 404 at HEAD).
- **Python discovery coverage needs an engine change** (above).
- Phase-2 pins live in `discovery_corpus.json`, **not** `corpus.json` — see ADR-0016. Promoting one
  into the mined corpus requires a re-freeze and a bar update in the same act.

## Plan deviations

- `superpowers:writing-plans` and `superpowers:finishing-a-development-branch` are unavailable in
  this harness; both degraded to `auto` per the convention's missing-skill rule. The plan was
  authored inline; the branch was pushed and the PR opened directly.
- Task 7 produced **two** commits rather than one: after the registration landed it re-measured and
  found a stated hint rate wrong (`prometheus implements` — `—` where the measured value is 4/6).
  A registration governing a future verdict cannot carry a wrong number, and amending a landed
  commit is forbidden, so the correction is `a0a258d`.
- Three fixes each forced a re-mine and re-freeze, so the corpus moved 215 → 203 → 227 → 207 across
  the run. Every intermediate freeze was internally consistent (task file, header arithmetic and
  README numbers updated together); only the last one matters.
- `--selftest` grew from 3 checks to **32**, including 5 KNOWN LIMITATION assertions, a per-task
  shape-invariant sweep over all 207 tasks, and a binding registered-floor check. Every new guard
  was mutation-tested — the numbers now fail loudly instead of drifting.

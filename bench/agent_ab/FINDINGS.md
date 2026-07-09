# agent-ab-efficacy — findings

**Date:** 2026-07-09
**Verdict: RED** (pre-registered thresholds — see `tasks/tasks.json` header)
**Cost of the experiment:** $7.89 (96 runs), claude 2.1.193, model claude-sonnet-4-6.

## Result

Real Claude Code agents ran 24 tasks (16 comprehension, 8 localization; gin +
prometheus) in two arms — **A** = standard tools, **B** = same tools plus the
`codeindex` CLI (documented in an appended system prompt, called via Bash) — 2
reps each, paired.

| Metric (primary = `total_cost_usd`) | Value |
| ----------------------------------- | ----- |
| Median paired cost reduction (ITT) | **−16.9%** (95% CI −28.9%, −1.6%) |
| Median paired cost reduction (per-protocol, tool actually used) | **−26.3%** (CI −38.1%, −13.2%) |
| Win rate (B cheaper) | 25% |
| Task success | A 89.6% vs B 89.6% (Δ 0.0 pp) |
| codeindex adoption (arm B) | 81.2% |
| Median turns | A 3 → B 3 |

**Negative numbers mean codeindex made runs MORE expensive.** It was adopted, it
did not hurt accuracy — and it cost ~17% more (ITT), ~26% more when actually
used.

## Why (mechanism — unambiguous in the cuts)

- Repo size did **not** flip the sign: gin −16.9%, prometheus −18.1%. The "it
  will shine on bigger repos" hope is falsified for this integration.
- The **more** the tool was used, the **worse** the cost: comprehension
  (100% adoption) −23.9%; localization (38% adoption) −1.5%; the "best case"
  cut (prometheus + comprehension, high adoption) was the **worst** at −26.9%.
- Turns rose A 2.8 → B 3.0; adopting runs made a median of 1 codeindex call.

The codeindex query is an **additive round-trip**. The agent spends a turn to
call it (output tokens to invoke + input/cache tokens to ingest the result), and
it does **not** save enough downstream grep/read to pay for that turn — because
**Claude Code's native Grep is already cheap** for these navigation questions.
Extra tool call, no compensating saving ⇒ net overhead.

## What this does and does not prove

**Proves (with a real baseline, not a strawman):** for common navigation tasks
that an agent already answers cheaply with ripgrep, bolting on a codeindex query
via Bash is net-negative on cost and neutral on accuracy. This directly
contradicts the earlier static studies (median 363× "savings"), which compared
against reading *whole files* — a baseline real Claude does not use. The
ruthless-review warning was correct: the real counterfactual is cheap.

**Does NOT prove the concept is dead — and this is a genuine limitation of the
task design, stated plainly, not as an excuse:** the comprehension tasks ("list
every file that references X") are essentially `rg -l X` — one native grep call.
We tested codeindex on grep's home turf, where its structured output (defs +
ranked callers + signatures) adds little the agent couldn't get cheaply. The
untested frontier is tasks where grep is **expensive or insufficient**:
multi-hop call-graph traversal, cross-repo blast-radius ("what breaks if I change
this signature"), or disambiguating a common name where grep returns hundreds of
unranked hits the agent must open files to sort out. Whether codeindex wins there
is now a **hypothesis, not evidence**.

## Roadmap consequence (pre-registered RED action)

RED means: **stop and rethink the consumption model before building more.** Do
NOT proceed to build the remaining query surface, the 5-language resolver, or the
MCP/plugin on the assumption that agents save tokens — this experiment shows they
do not, for the tasks tested, with the current integration.

Rethink options, in rough priority:
1. **Fix the task design first** — build tasks where the native-grep baseline is
   genuinely expensive (deep call-graph / blast-radius / hot-name
   disambiguation) and re-run. If codeindex still loses there, the concept is in
   serious doubt. If it wins, the product is real but narrow — position it for
   those questions only.
2. **Change the integration** so a query is not a full extra round-trip — e.g. an
   MCP tool whose result the agent trusts enough to *replace* grep+read rather
   than supplement it, or auto-injected context rather than an agent-initiated
   call. Measure whether that flips the sign.
3. **Reconsider the value proposition** — if the honest win is *quality/context*
   (feeding the right 400 tokens improves task success on hard tasks) rather than
   *cost*, design tasks hard enough for a success delta to appear; here success
   was already saturated at ~90% for both arms, so there was no headroom to show
   a quality benefit.

## Value of having run this

Front-loading this experiment cost $8 and ~1 hour and prevented building months
of breadth (query surface, precise resolution across 5 languages, MCP server,
plugin) on a premise the data does not support. That is exactly what the
pre-registration was for.

## Caveats

- 2/96 runs timed out (one per arm — no bias); graded as failures.
- Success was saturated (~90% both arms) — no headroom to detect a quality
  benefit even if one existed on harder tasks.
- Single model (sonnet-4-6), single CLI version, Go-only, two repos, read-only
  tasks. The result is robust for what it tested and should not be extrapolated
  beyond it.

## Reproduce

```
go build -o bench/agent_ab/.bin/codeindex ./cmd/codeindex
python3 bench/agent_ab/build_tasks.py        # regenerates tasks (seeded)
python3 bench/agent_ab/run_ab.py --full --reps 2 --budget-usd 40
python3 bench/agent_ab/grade.py && python3 bench/agent_ab/report.py
```

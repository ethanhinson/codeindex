# agent-ab-efficacy v2 (caller-attribution) — findings

**Date:** 2026-07-09
**Verdict: GREEN** (same pre-registered thresholds as v1)
**Cost:** $8.79 (64 runs), claude 2.1.193, sonnet-4-6.

## Why v2 exists

v1 returned RED: codeindex *increased* agent cost ~17%. Post-mortem: v1's tasks
("which files reference X") are essentially `rg -l X` — one native grep call, so
codeindex was pure overhead. v2 tests codeindex's actual structural advantage:
**"which FUNCTIONS call X."** Grep returns match `file:line`s but not the
enclosing function name, so an agent must open many files to name the callers;
codeindex returns caller names in one query.

## Result

24→16 tasks (caller-attribution, gin + prometheus), 2 arms, 2 reps, paired.

| Metric (primary = `total_cost_usd`) | Value |
| ----------------------------------- | ----- |
| Median paired cost reduction (ITT) | **73.0%** (95% CI 61.6%, 81.5%) |
| Median processed-token reduction | 75.4% |
| Win rate (B cheaper) | 93.8% (15/16) |
| Task success | A 96.9% vs B **100%** (Δ +3.1 pp) |
| codeindex adoption (arm B) | 100% |
| Median turns | **A 13 → B 2** |
| ITT vs per-protocol gap | 0.0 pp (adoption was total) |

Per-task reductions ranged 46–89%; the only non-positive was `gin-Open` at −2%
(neutral). The **largest savings were on the bigger repo** (prometheus:
`AddInterval` 89%, `OpenBlock` $0.69→$0.11 = 85%, `LastCheckpoint` 85%) — the
size-scaling thesis that FAILED in v1 HOLDS here, because these tasks force
multi-file reads that scale with repo size.

## Mechanism (unambiguous)

Median turns tell the whole story: **A 13, B 2.** Arm A grinds through ~13
grep+read cycles to identify and name every calling function; arm B issues one
codeindex query and formats the answer. codeindex did not just cut cost — it also
edged success higher (100% vs 96.9%), because exhaustively naming callers by hand
is error-prone (the one arm-A failure was an unparseable/incomplete answer; arm B
got that task right).

## The complete picture (v1 + v2)

| Task class | grep baseline | Verdict | codeindex effect |
| ---------- | ------------- | ------- | ---------------- |
| "which FILES reference X" (v1) | cheap (`rg -l`) | **RED** | **+17% cost** (overhead) |
| "which FUNCTIONS call X" (v2) | expensive (read N files) | **GREEN** | **−73% cost**, +3pp success |

**codeindex's value is real but bounded.** It loses on locate-questions grep
already answers in one call, and wins decisively on structural call-graph /
impact questions where grep must fan out across many files to attribute results
to functions. This is a genuine, defensible product boundary — not a blanket
"saves tokens."

## What this reshapes in the roadmap

The tool is *for* call-graph / impact questions, not general navigation:
1. **Query surface (core-indexing-engine)** should prioritize the winners —
   callers/callees, and dependents/blast-radius — over "where is X defined"
   (which grep already does cheaply). Fuzzy search and outline are unproven and
   may not earn their keep.
2. **Plugin/skill (change 4)** should trigger codeindex specifically on
   "who calls / what references / what would changing this affect" — NOT on
   "where is X." Mis-triggering on locate-questions reproduces the v1 overhead.
3. **Precise resolution (change 2)** matters most exactly here: caller
   attribution on common/overloaded names is where name-based ambiguity bites,
   and where a receiver-aware/precise resolver would extend the GREEN zone.

## Integrity / caveats (honest)

- **Ground truth is read from the index**, restricted to unambiguously-resolving
  unique-name targets so name-based resolution is exact. Independently
  hand-verified for `IsDebugging` (8/8 callers matched grep+read). Arm A reaching
  ~97% success by independent reading confirms the truth is achievable without
  the tool — the metric is *cost to the same answer*, and both arms could reach
  it.
- **v2 is codeindex's best case by design** — we deliberately selected
  grep-hard tasks, just as v1 (unintentionally) selected grep-easy ones.
  Together they map the boundary; neither alone is the whole truth.
- **Success was near-saturated** (~97–100%), so the headline is cost, not a
  large quality delta — though B's slight success edge is real and directional.
- Single model, Go-only, 2 repos, unique-name targets. Robust for what it tested.
- 1/64 runs unparseable (arm A); 0 timeouts.

## Reproduce

```
python3 bench/agent_ab/build_tasks.py --types caller_attribution \
    --caller-attribution-per-repo 8 --out bench/agent_ab/tasks/tasks_v2.json
python3 bench/agent_ab/run_ab.py --full --tag v2 --reps 2 --budget-usd 30
python3 bench/agent_ab/grade.py --tag v2 && python3 bench/agent_ab/report.py --tag v2
```

# agent-ab v8 — `search` on concept tasks: findings

**Date:** 2026-07-12 · **Verdict: RED** (pre-registered thresholds,
`v8_search/tasks.json` header) · **Cost:** $4.96, 96 runs, claude-sonnet-4-6.

## Setup

24 concept/feature-location tasks (12 gin + 12 laravel-framework, seeded
sample from the frozen curated-x2 fixtures), 2 arms × 2 reps, paired. Arm A =
native tools (Bash/Read/Grep/Glob). Arm B = same + `codeindex search`
documented via appended system prompt. Grader (SYMBOLS-section accept match,
word-boundary) committed before the matrix ran. Registered bars: GREEN needed
success ≥ +15pp AND cost ≤ +10% AND adoption ≥ 70%; ANY success regression =
RED.

## Result

| metric | value |
| --- | --- |
| success | A **100.0%** → B 95.8% (−4.2pp) → **RED** |
| median paired cost | **−8.5%** (B cheaper) |
| adoption (B) | 100% |
| median turns | A 4 → B 3 |

## The two findings that matter

**1. The control was the discovery: sonnet-class agents scored 100% on
concept tasks with grep alone.** The retrieval-level "lexical control = 0%"
measured a single-shot query; the real counterfactual is an agent that
REFORMULATES — concept → identifier guesses → iterative grep → read — plus
non-zero training familiarity with famous OSS. On this population there is no
success headroom for ANY retrieval mechanism, by construction. v1's lesson
("the real baseline is cheap") recurs one level up: the real baseline is
*smart*.

**2. The entire regression is one question, both reps, and it's an
accept-set ambiguity, not a wrong answer.** Asked where JSON body-binding is
implemented, arm B answered `jsonBinding.Bind`/`decodeJSON`
(binding/json.go) — the implementation the accepted `ShouldBindJSON` surface
delegates to. Search took the agent DEEPER than the fixture's
surface-oriented ground truth. Recorded as graded (RED per frozen grader);
the anatomy is stated so nobody mistakes it for a hallucination.

Positive secondary signal, honestly bounded: −8.5% median cost at 100%
adoption is the FIRST agent-level evidence that search's one-call pattern
beats grep loops on cost (v1 measured +17% overhead for the graph tool on
its wrong class). Modest, real, and not worth a success trade on this
population.

## What this changes

- `search` stays exactly where it is: MCP-description-gated, never
  ambient (already true). No promotion claims are available.
- The go/no-go question for the semantic investment moves: public,
  well-named OSS with frontier agents is the WRONG population to show
  agent-level value — agents don't fail there. The populations where arm A
  should stop scoring 100%: private/application code (no training
  familiarity, weaker naming — the field-test loop exists for exactly
  this), bug-symptom tasks (agents + grep at ~20-35% retrieval parity),
  and hook-dispatch corpora with runtime evidence attached.
- Registered follow-up: replicate v8 on a private application codebase via
  the field-measurement loop before any further mechanism work on concept
  retrieval; and a v8b variant on bug-symptom tasks (issues-x fixtures)
  where the 100%-control ceiling cannot exist.

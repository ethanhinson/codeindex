# agent-ab v9 — `search` on private application code: findings

**Date:** 2026-07-13 · **Verdict: YELLOW** (pre-registered thresholds,
`v9_btt/tasks.json` header) · **Cost:** $38.52, 96 runs, claude-sonnet-4-6.

## Setup

24 real JIRA tickets (titles supplied by the repo owner; accept sets mined
from fix-commit hunk headers, name-based, parent-aware locate filter — zero
leaks) on a private production codebase (btt-api: Drupal 7 shell + PSR-4
custom modules, 64,534 symbols, no model training familiarity). Paired
A/B × 2 reps; arm B = native tools + `codeindex search` via system prompt.
Grader frozen (v8 logic). Retrieval context measured first: search 22.5% /
grep-control 7.5% / find-control 0% — the FIRST corpus where the semantic
lane beats its lexical control on fault queries (3×), inverting the OSS
result (business-language tickets give grep nothing to bite).

## Result

| metric | v9 (private) | v8 (OSS concept) | v1 (OSS navigation) |
| --- | --- | --- | --- |
| arm-A success | **37.5%** | 100% | 89.6% |
| success delta (B−A) | **+4.2pp** | −4.2pp | 0.0pp |
| median paired cost | **−23.4%** | −8.5% | **+17%** |
| adoption | 98% | 100% | 81% |

Bars: GREEN needed ≥ +15pp success; YELLOW satisfied via "success parity
(≥ −3pp) with savings ≥ 20%".

## The three findings

1. **The OSS ceiling cracked exactly where predicted.** Native-tool agents
   fell from 100% (famous OSS, v8) to **37.5%** on private code — the first
   quantification of how much of agent competence on public repos is
   familiarity + naming culture rather than transferable navigation skill.
   Private code is the real game, and everyone finds it hard.
2. **Search's value on this population is economic and real: −23.4% median
   cost at 98% adoption with no success trade** (+4.2pp is within noise at
   n=48; the cost delta is the solid result). The win concentrates on
   exploration-heavy tickets (RES-892: 32→9 turns; RES-1031: 47→23), with a
   small additive-round-trip toll on grep-easy ones — v1's mechanism, now
   on the profitable side of the ledger.
3. **The agent loop is worth ~+30pp over raw retrieval** (arm A 37.5% vs
   grep-control 7.5%; arm B 41.7% vs search 22.5%) — reasoning compounds
   whatever retrieval provides, which is why retrieval deltas understate
   agent-level value on hard populations and overstate it on easy ones.

## Trajectory across the A/B series

v1: graph tool on navigation, OSS → RED (+17% cost). v8: search on
concepts, OSS → RED (no headroom, agents at 100%). v9: search on real
tickets, private code → **YELLOW (−23.4% cost, parity success)**. The
pattern: this tool's value is inversely proportional to how well the agent
already knows the terrain. Public benchmarks systematically understate it;
the customer's private repo is where it earns.

## What YELLOW licenses (and doesn't)

- Licenses: keeping `search` exactly as shipped (MCP-gated), documenting
  the private-code economics with these numbers, and running the follow-up
  that could reach GREEN: error_text/description-armed prompts (ticket
  descriptions carried symptom detail the A/B didn't use), and the
  runtime-evidence variant (profile the app, re-run — the Drupal-heat
  mechanism measured +7.2 on core).
- Does not license: ambient promotion, "AI-powered debugging" claims, or
  treating +4.2pp as a success improvement (it is parity).

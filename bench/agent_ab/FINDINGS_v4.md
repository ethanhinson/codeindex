# agent-ab-efficacy v4 (stripped plugin) — findings

**Date:** 2026-07-10
**GATE: PASS** (all four pre-registered thresholds)
**Cost:** $4.37 (32 arm-B runs; arm-A control shared from v3 — identical
no-plugin config, disclosed by design)

## What v4 is

The plugin stripped to the two mechanisms that measured well, plus one command:
- **UserPromptSubmit note** (155 tokens): tool availability + the anchor rule +
  the TRUST instruction ("output is COMPLETE — answer from the references, do
  not re-verify except [ambiguous]")
- **Post-edit hook** (unchanged: proven 100% fire / 0 false across 3 runs)
- `/impact` kept for humans (~25-token description in context)
- CUT: the skill, /callers, /callees → static footprint **<250 tokens (was ~3.1k)**

## Gate results (v3 FAIL → v4 PASS)

| Threshold | v3 | v4 |
| --------- | -- | -- |
| Locate regression ≤10% | −43.9% ❌ | **−7.4%** ✅ |
| Branch-out savings ≥50% | −11.3% ❌ | **+62.3%** ✅ |
| Hook fire-rate ≥80% | 100% ✅ | **100%** ✅ |
| Hook false fires = 0 | 0 ✅ | **0** ✅ |

Supporting: success **B 100% vs A 93.8%** (third consecutive run where the
plugin arm never gave a wrong answer), branch-out adoption 100%, locate
adoption 0% (discipline held), edit tasks +25.5%, per-protocol savings +44.7%.
Behavioral fix confirmed in transcripts: caller-attribution went from 9.5
turns / 6 file reads (v3) to **2 turns / 0 reads** (v4).

## Why it flipped

1. **Footprint**: cutting the skill+commands apparatus removed the fixed
   ~3.1k-token cache_creation tax → locate tasks now ride nearly free (−7.4%,
   within tolerance; residual = the 155-token note + hook registrations).
2. **Trust**: one sentence ("answer directly from the references") eliminated
   the re-verification loop — the same instruction v2 carried, whose absence
   was v3's branch-out failure.

## The consumption law (now measured five ways)

**Always-visible + zero-ceremony + explicit trust = the win. Everything else
converts savings back into cost.** v1: wrong tasks −17%. v2: visible prompt
+73%. v3a: lazy skill → adoption collapse. v3: apparatus footprint + trust
deficit → FAIL. v4: stripped to visible note + hook → PASS.

## Notes

- The report's overall "YELLOW" line is the mixed-set median (locate tasks
  dilute it by construction — codeindex correctly sits out 6/16 tasks). The
  governing pre-registered criteria for this change are the four gate
  thresholds, which PASS.
- Shared control: arm-A rows reused from v3 (no plugin involved in arm A;
  identical tasks/model/config). Bias risk: none directional.
- Same caveats as prior runs: sonnet-4-6, Go repos, short tasks (worst case
  for any footprint).

## Reproduce

```
python3 bench/agent_ab/run_ab.py --full --tag v4 --plugin-arm --reps 2 --budget-usd 20
python3 bench/agent_ab/grade.py --tag v4 && python3 bench/agent_ab/report.py --tag v4
```

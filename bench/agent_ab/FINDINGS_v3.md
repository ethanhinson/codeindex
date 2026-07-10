# agent-ab-efficacy v3 (plugin gate) — findings

**Date:** 2026-07-09
**Gate verdict: FAIL** (pre-registered; one registered iteration used)
**Cost:** v3a partial $2.77 (23 rows, archived) + v3 full $5.58 (64 runs)

## What was tested

The REAL packaged Claude Code plugin (`--plugin-dir`): skill + `/impact`
`/callers` `/callees` commands + post-edit hook + (after iteration) an
always-visible UserPromptSubmit note. Mixed 16-task set: 6 locate (should NOT
use codeindex), 6 branch-out (should), 4 edit-impact (hook territory).

## Gate results

| Threshold | Measured | Verdict |
| --------- | -------- | ------- |
| Locate regression ≤10% | **−43.9%** | ❌ |
| Branch-out savings ≥50% | **−11.3%** | ❌ |
| Hook fire-rate ≥80% on edit tasks | **100%** | ✅ |
| Hook false fires = 0 | **0** | ✅ |

Also: success **B 100% vs A 93.8%** (+6.2 pp — the plugin never gave a wrong
answer; the control did twice), adoption 62.5% overall (100% on branch-out and
edit after the iteration, 0% on locate — trigger discipline held perfectly).

## The two failures, precisely diagnosed

**1. The plugin's static footprint taxes every session (locate −43.9%).**
On locate tasks arm B behaved IDENTICALLY to arm A — same 3 turns, same 2
greps, same ~470 in+out tokens, zero codeindex calls (discipline held). The
entire cost delta is `cache_creation`: **4,060 vs 936 tokens** — a fixed
~3.1k-token boarding fee from the plugin's system-prompt surface (skill
description + three command definitions + hook registrations + the prompt
note). On a $0.03 task that's ~40% inflation. The plugin didn't misbehave — it
is simply too heavy for the sessions it rides in.

**2. Trust deficit: the agent uses codeindex and then re-verifies anyway
(branch-out −11.3% despite 100% adoption).** v2 (system-prompt arm): 2 turns,
~0 file reads, +73%. v3 plugin arm: 9.5 turns, **6 file reads** — MORE than
arm A's 5.5. The agent runs the query, then belt-and-suspenders reads the files
it would have read anyway, plus a Skill round-trip turn. The tool's answer was
treated as a hint to verify, not an answer. v2's prompt explicitly modeled
"answer directly from these references"; the plugin's packaging lost that.

**What worked: the non-round-trip path.** Edit-impact: **+28.9%** savings, 6
turns vs 10, 1 read vs 5, 100% hook fire-rate, zero false fires. The hook
injected caller context right after the edit and the agent used it instead of
re-deriving it.

## The cross-experiment pattern (v1 → v2 → v3)

| Integration | Result |
| ----------- | ------ |
| Agent-initiated on grep-easy tasks (v1) | −17% (overhead) |
| Always-visible prompt doc, grep-hard tasks (v2) | **+73%** |
| Lazy skill packaging (v3a) | adoption collapse (~10%) |
| Full plugin apparatus (v3) | footprint + re-verification eat the win |
| **Hook injection (non-round-trip) (v3 edit tasks)** | **+29%, 100% fire, 0 false, +6pp accuracy** |

The signal is consistent: **visibility and zero-round-trip integrations win;
apparatus and agent-initiated ceremony lose.** Every point of friction (skill
round-trip, unread output trust, static footprint) converts savings back into
cost.

## Recommended v4 (not run — requires approval; iteration budget spent)

Strip the plugin to what measured well:
1. **Keep**: post-edit hook (proven), UserPromptSubmit note (drove 100%
   branch-out adoption) — but the note must carry the trust instruction:
   "the output is complete; answer from it directly, do not re-verify by
   reading files unless a result is flagged [ambiguous]".
2. **Cut**: the skill and two of three commands (fold into one `/impact`), or
   move skill content into the note — target < 500 tokens total static
   footprint (from ~3.1k).
3. Re-run the same gate as v4.

## Caveats

- Single model (sonnet-4-6); footprint economics depend on task size — on
  long sessions the fixed tax amortizes and the picture may invert; our tasks
  are short by design (worst case for footprint).
- The +6.2 pp accuracy gain is consistent across v3a/v3 and echoes v2's edit
  case (A silently missed callers) — on correctness the plugin has never lost.

## Reproduce

```
python3 bench/agent_ab/run_ab.py --full --tag v3 --plugin-arm --reps 2 --budget-usd 30
python3 bench/agent_ab/grade.py --tag v3 && python3 bench/agent_ab/report.py --tag v3
```

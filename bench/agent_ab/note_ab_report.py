#!/usr/bin/env python3
"""Paired report for the prompt-note A/B (note_ab.py runs).

Pairs cur-vs-nav per (task, rep). Gate (pre-registered in note_ab.py):
flip the shipped note only if nav arm success delta >= -5pp AND median
paired cost delta <= +10% AND nav is actually called somewhere.
"""
from __future__ import annotations

import json
import statistics as st
from collections import defaultdict
from pathlib import Path

HERE = Path(__file__).resolve().parent
RUNS = HERE / "results" / "note_ab_runs.jsonl"


def main():
    rows = [json.loads(l) for l in RUNS.read_text().splitlines() if l.strip()]
    by_key = {}
    for r in rows:
        by_key[r["key"]] = r  # last write wins (rerun supersedes)

    pairs = defaultdict(dict)
    for r in by_key.values():
        pairs[(r["task_id"], r["rep"])][r["arm"]] = r
    complete = {k: v for k, v in pairs.items() if "cur" in v and "nav" in v
                and not v["cur"]["timed_out"] and not v["nav"]["timed_out"]}
    print(f"rows={len(by_key)}  pairs complete={len(complete)}/{len(pairs)}")

    def agg(arm):
        rs = [v[arm] for v in complete.values()]
        return {
            "succ": sum(r["success"] for r in rs) / len(rs),
            "f1": st.median(r["f1"] for r in rs),
            "cost": st.median(r["cost_usd"] or 0 for r in rs),
            "turns": st.median(r["num_turns"] or 0 for r in rs),
            "ci_calls": st.median(r["codeindex_calls"] for r in rs),
            "nav_calls": sum(r["nav_calls"] for r in rs),
        }

    a, b = agg("cur"), agg("nav")
    cost_deltas = [(v["nav"]["cost_usd"] or 0) - (v["cur"]["cost_usd"] or 0)
                   for v in complete.values()]
    rel = [d / (v["cur"]["cost_usd"] or 1e-9) for d, v in
           zip(cost_deltas, complete.values())]

    print(f"\n{'':<12}{'succ':>7}{'medF1':>7}{'medCost':>9}{'medTurns':>9}"
          f"{'medCI':>7}{'navSum':>8}")
    for name, m in (("cur", a), ("nav", b)):
        print(f"{name:<12}{m['succ']:>7.0%}{m['f1']:>7.2f}{m['cost']:>9.3f}"
              f"{m['turns']:>9.1f}{m['ci_calls']:>7.1f}{m['nav_calls']:>8}")

    succ_delta = b["succ"] - a["succ"]
    med_rel = st.median(rel)
    print(f"\nsuccess delta (nav-cur): {succ_delta*100:+.1f}pp")
    print(f"median paired cost delta: {med_rel:+.1%}")

    # per-type breakdown
    bt = defaultdict(list)
    for v in complete.values():
        bt[v["cur"]["type"]].append(v)
    print(f"\n{'type':<16}{'n':>3} {'succ cur→nav':>14} {'medcost cur→nav':>18} {'nav used':>9}")
    for tp, vs in sorted(bt.items()):
        sc = sum(v["cur"]["success"] for v in vs) / len(vs)
        sn = sum(v["nav"]["success"] for v in vs) / len(vs)
        cc = st.median(v["cur"]["cost_usd"] or 0 for v in vs)
        cn = st.median(v["nav"]["cost_usd"] or 0 for v in vs)
        nv = sum(v["nav"]["nav_calls"] for v in vs)
        print(f"{tp:<16}{len(vs):>3} {sc:>6.0%} → {sn:<5.0%} {cc:>8.3f} → {cn:<7.3f} {nv:>7}")

    nav_used = b["nav_calls"] > 0
    verdict = (succ_delta >= -0.05 and med_rel <= 0.10 and nav_used)
    print(f"\nGATE: succ_delta>=-5pp: {succ_delta >= -0.05}  "
          f"cost<=+10%: {med_rel <= 0.10}  nav_called: {nav_used}")
    print("VERDICT:", "FLIP the note (nav variant holds)" if verdict
          else "KEEP current note")


if __name__ == "__main__":
    main()

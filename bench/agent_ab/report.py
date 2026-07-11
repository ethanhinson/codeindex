#!/usr/bin/env python3
"""Paired A/B analysis + pre-registered verdict -> results/report.md.

Primary metric: total_cost_usd (cache-aware). Secondary: processed_tokens.
Reports intent-to-treat and per-protocol (codeindex-actually-used) views, with a
seeded bootstrap 95% CI over tasks. Deterministic: re-running on the same
graded.jsonl reproduces byte-identical output.

Usage: python3 report.py
"""

from __future__ import annotations

import json
import random
import statistics as st
from pathlib import Path

HERE = Path(__file__).resolve().parent
GRADED = HERE / "results" / "graded.jsonl"
TASKS = HERE / "tasks" / "tasks.json"
REPORT = HERE / "results" / "report.md"
BOOT_SEED = 20260709
BOOT_N = 5000


def median(xs):
    xs = [x for x in xs if x is not None]
    return st.median(xs) if xs else None


def per_task_arm(rows, metric):
    """metric per (task, arm) = median over reps."""
    buckets = {}
    for r in rows:
        buckets.setdefault((r["task_id"], r["arm"]), []).append(r.get(metric))
    return {k: median(v) for k, v in buckets.items()}


def bootstrap_ci(deltas, seed=BOOT_SEED, n=BOOT_N):
    if not deltas:
        return (None, None)
    rng = random.Random(seed)
    meds = []
    k = len(deltas)
    for _ in range(n):
        sample = [deltas[rng.randrange(k)] for _ in range(k)]
        meds.append(st.median(sample))
    meds.sort()
    return (round(meds[int(0.025 * n)], 1), round(meds[int(0.975 * n)], 1))


def analyze(rows, task_ids):
    cost = per_task_arm(rows, "cost_usd")
    toks = per_task_arm(rows, "processed_tokens")
    succ = per_task_arm(rows, "success")  # median of bool over reps -> 0/0.5/1
    reductions, tok_red, wins, paired = [], [], 0, 0
    table = []
    for tid in task_ids:
        a, b = cost.get((tid, "A")), cost.get((tid, "B"))
        ta, tbk = toks.get((tid, "A")), toks.get((tid, "B"))
        if a is None or b is None:
            continue
        paired += 1
        red = 100 * (a - b) / a if a else 0.0
        reductions.append(red)
        if ta and tbk is not None:
            tok_red.append(100 * (ta - tbk) / ta if ta else 0.0)
        if b < a:
            wins += 1
        table.append((tid, a, b, red, succ.get((tid, "A")), succ.get((tid, "B"))))
    return {
        "n_paired": paired,
        "median_cost_reduction_pct": round(median(reductions), 1) if reductions else None,
        "cost_reduction_ci": bootstrap_ci(reductions),
        "median_token_reduction_pct": round(median(tok_red), 1) if tok_red else None,
        "win_rate_pct": round(100 * wins / paired, 1) if paired else None,
        "table": table,
    }


def success_rate(rows, arm):
    xs = [1 if r["success"] else 0 for r in rows if r["arm"] == arm]
    return round(100 * sum(xs) / len(xs), 1) if xs else None


def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--tag", default="")
    args = ap.parse_args()
    suffix = f"_{args.tag}" if args.tag else ""
    graded_file = HERE / "results" / f"graded{suffix}.jsonl"
    tasks_file = HERE / "tasks" / f"tasks{suffix}.json"
    global REPORT
    REPORT = HERE / "results" / f"report{suffix}.md"

    rows = [json.loads(l) for l in graded_file.read_text().splitlines() if l.strip()]
    header = json.loads(tasks_file.read_text())["header"]
    thresholds = header["thresholds"]
    all_task_ids = sorted({r["task_id"] for r in rows})

    itt = analyze(rows, all_task_ids)

    # Per-protocol: tasks where every arm-B rep used codeindex.
    b_by_task = {}
    for r in rows:
        if r["arm"] == "B":
            b_by_task.setdefault(r["task_id"], []).append(r["codeindex_calls"] >= 1)
    pp_task_ids = [t for t, v in b_by_task.items() if v and all(v)]
    pp = analyze(rows, pp_task_ids)

    b_runs = [r for r in rows if r["arm"] == "B"]
    adoption = round(100 * sum(1 for r in b_runs if r["codeindex_calls"] >= 1)
                     / len(b_runs), 1) if b_runs else 0.0
    sa, sb = success_rate(rows, "A"), success_rate(rows, "B")
    succ_delta = round((sb or 0) - (sa or 0), 1)
    total_cost = round(sum(r.get("cost_usd") or 0 for r in rows), 2)
    n_unp = sum(1 for r in rows if r.get("unparseable"))
    n_to = sum(1 for r in rows if r.get("timed_out"))

    # Verdict (pre-registered thresholds).
    sav = itt["median_cost_reduction_pct"] or 0
    pp_sav = pp["median_cost_reduction_pct"] or 0
    if sav >= 30 and succ_delta >= -5 and adoption >= 70:
        verdict = "GREEN"
    elif (10 <= sav < 30) or (40 <= adoption < 70 and pp_sav >= 30):
        verdict = "YELLOW"
    elif sav < 10 or succ_delta < -5 or (adoption < 40 and pp_sav < 30):
        verdict = "RED"
    else:
        verdict = "YELLOW"

    # Per-type breakdown (and the v3 gate when the task header registers one).
    types = sorted({r["type"] for r in rows})
    per_type = {}
    for ty in types:
        ty_rows = [r for r in rows if r["type"] == ty]
        ty_ids = sorted({r["task_id"] for r in ty_rows})
        a = analyze(ty_rows, ty_ids)
        b_ty = [r for r in ty_rows if r["arm"] == "B"]
        a["adoption"] = round(100 * sum(1 for r in b_ty if r["codeindex_calls"] >= 1)
                              / len(b_ty), 1) if b_ty else 0.0
        a["hook_fire_rate"] = round(100 * sum(1 for r in b_ty if r.get("hook_fires", 0) >= 1)
                                    / len(b_ty), 1) if b_ty else 0.0
        per_type[ty] = a

    v6 = header.get("v6_gate")
    v6_lines, v6_verdict = [], None
    if v6:
        dist = per_type.get("comprehension", {})
        vag = per_type.get("vague_find", {})
        occ = per_type.get("occurrences", {})
        d = dist.get("median_cost_reduction_pct") or 0
        v = vag.get("median_cost_reduction_pct") or 0
        o = occ.get("median_cost_reduction_pct") or 0
        checks = [
            (f"distinctive regression ≤{v6['distinctive_regression_max_pct']}%: measured {d:+.1f}%",
             d >= -v6["distinctive_regression_max_pct"]),
            (f"vague-find savings ≥{v6['vague_savings_min_pct']}%: measured {v:+.1f}%",
             v >= v6["vague_savings_min_pct"]),
            (f"occurrences savings ≥{v6['occurrences_savings_min_pct']}%: measured {o:+.1f}%",
             o >= v6["occurrences_savings_min_pct"]),
        ]
        v6_verdict = "PASS" if all(ok for _, ok in checks) else "FAIL"
        v6_lines = [f"- {'✅' if ok else '❌'} {txt}" for txt, ok in checks]

    gate = header.get("v3_gate")
    gate_lines, gate_verdict = [], None
    if gate:
        loc = per_type.get("comprehension", {})
        br = per_type.get("caller_attribution", {})
        ed = per_type.get("edit_impact", {})
        loc_red = loc.get("median_cost_reduction_pct") or 0
        br_red = br.get("median_cost_reduction_pct") or 0
        hook_rate = ed.get("hook_fire_rate", 0.0)
        false_fires = sum(r.get("hook_fires", 0) for r in rows
                          if r["arm"] == "B" and r["type"] != "edit_impact")
        checks = [
            (f"locate regression ≤{gate['locate_regression_max_pct']}%: "
             f"measured {loc_red:+.1f}%", loc_red >= -gate["locate_regression_max_pct"]),
            (f"branch-out savings ≥{gate['branchout_savings_min_pct']}%: "
             f"measured {br_red:+.1f}%", br_red >= gate["branchout_savings_min_pct"]),
            (f"hook fire-rate ≥{gate['hook_fire_rate_min_pct']}% on edit tasks: "
             f"measured {hook_rate:.0f}%", hook_rate >= gate["hook_fire_rate_min_pct"]),
            (f"hook false fires ≤{gate['hook_false_fires_max']}: "
             f"measured {false_fires}", false_fires <= gate["hook_false_fires_max"]),
        ]
        gate_verdict = "PASS" if all(ok for _, ok in checks) else "FAIL"
        gate_lines = [f"- {'✅' if ok else '❌'} {txt}" for txt, ok in checks]

    L = []
    L.append("# agent-ab-efficacy — report\n")
    if v6_verdict:
        L.append(f"**v6 GATE: {v6_verdict}**\n")
        L.extend(v6_lines)
        L.append("")
    if gate_verdict:
        L.append(f"**v3 GATE: {gate_verdict}**\n")
        L.extend(gate_lines)
        L.append("")
    L.append(f"**Verdict: {verdict}**  (pre-registered thresholds)\n")
    L.append(f"- primary metric: median paired reduction in `total_cost_usd`\n")
    L.append("## Headline (intent-to-treat)\n")
    L.append(f"- paired tasks: {itt['n_paired']}")
    L.append(f"- **median cost reduction: {itt['median_cost_reduction_pct']}%** "
             f"(95% CI {itt['cost_reduction_ci']})")
    L.append(f"- median processed-token reduction: {itt['median_token_reduction_pct']}%")
    L.append(f"- win rate (B cheaper): {itt['win_rate_pct']}%")
    L.append(f"- success: A {sa}% vs B {sb}%  (delta {succ_delta} pp)")
    L.append(f"- codeindex adoption (arm B): {adoption}%")
    L.append(f"- total experiment cost: ${total_cost}   "
             f"unparseable: {n_unp}   timeouts: {n_to}\n")
    L.append("## Per-protocol (codeindex actually used every arm-B rep)\n")
    L.append(f"- paired tasks: {pp['n_paired']}")
    L.append(f"- median cost reduction: {pp['median_cost_reduction_pct']}% "
             f"(95% CI {pp['cost_reduction_ci']})")
    gap = (pp_sav - sav)
    L.append(f"- ITT vs per-protocol gap: {round(gap,1)} pp "
             f"({'discoverability limited — tool helps when used' if gap > 15 else 'consistent'})\n")
    if len(types) > 1:
        L.append("## Per-type breakdown\n")
        L.append("| type | n | median cost Δ | adoption | hook fire-rate |")
        L.append("|------|---|---------------|----------|----------------|")
        for ty in types:
            a = per_type[ty]
            L.append(f"| {ty} | {a['n_paired']} | "
                     f"{(a['median_cost_reduction_pct'] or 0):+.1f}% | "
                     f"{a['adoption']:.0f}% | {a['hook_fire_rate']:.0f}% |")
        L.append("")
    L.append("## Thresholds\n")
    for k in ("green", "yellow", "red"):
        L.append(f"- **{k.upper()}**: {thresholds[k]}")
    L.append("")
    L.append("## Per-task (cost $, primary)\n")
    L.append("| task | cost A | cost B | reduction % | succ A | succ B |")
    L.append("|------|--------|--------|-------------|--------|--------|")
    for tid, a, b, red, sa_, sb_ in sorted(itt["table"]):
        L.append(f"| {tid} | {a:.4f} | {b:.4f} | {red:.0f}% | {sa_} | {sb_} |")
    L.append("\n## Provenance\n")
    L.append(f"- repo pins: {header.get('repo_pins')}")
    L.append(f"- task seed: {header.get('generated_seed')}   bootstrap seed: {BOOT_SEED}")
    models = sorted({r.get('model') for r in rows})
    cliv = sorted({r.get('claude_version') for r in rows})
    L.append(f"- model(s): {models}   claude: {cliv}")
    REPORT.write_text("\n".join(L) + "\n")
    print("\n".join(L))
    print(f"\nwrote {REPORT}")


if __name__ == "__main__":
    main()

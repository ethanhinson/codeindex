#!/usr/bin/env python3
"""Generate a self-contained HTML dashboard for the A/B evaluation results.

Reads the graded runs for each tagged experiment (v1 default + v2) and emits a
single portable file, bench/agent_ab/results/dashboard.html — no server, works
offline, commit-able. Shows the verdict per experiment, headline stats, the
per-task cost table (A vs B, reduction %, success, turns, adoption), and a
click-to-expand drill-down into each run's actual agent answer.

Usage: python3 dashboard.py
"""

from __future__ import annotations

import json
import random
import statistics as st
from html import escape
from pathlib import Path

HERE = Path(__file__).resolve().parent
RESULTS = HERE / "results"
OUT = RESULTS / "dashboard.html"

# (label, graded file, tasks file) for each experiment arm of the study.
EXPERIMENTS = [
    ("v1 · which FILES reference X", "graded.jsonl", "tasks.json",
     "grep-easy (≈ rg -l)"),
    ("v2 · which FUNCTIONS call X", "graded_v2.jsonl", "tasks_v2.json",
     "grep-hard (must read N files)"),
    ("v3 · packaged plugin gate (mixed tasks)", "graded_v3.jsonl", "tasks_v3.json",
     "real plugin via --plugin-dir · GATE FAIL: static footprint (~3.1k tok) + "
     "re-verification ate the win; hook flawless (100% fire, 0 false, +29% on edits)"),
    ("v4 · stripped plugin (note + hook)", "graded_v4.jsonl", "tasks_v4.json",
     "GATE PASS: footprint <250 tok + trust instruction · branch-out +62%, "
     "locate -7.4% (in tolerance), hook 100%/0 false, accuracy 100% vs 93.8%"),
    ("v5 · laravel (PHP) + lexical resolution", "graded_v5.jsonl", "tasks_v5.json",
     "GREEN: boundary holds off-Go — +64% median (CI 57-83), 100% win rate, "
     "success 100% vs 93.8%, qualified anchors live (Type::method)"),
]


def med(xs):
    xs = [x for x in xs if x is not None]
    return st.median(xs) if xs else None


def bootstrap_ci(deltas, seed=20260709, n=5000):
    if not deltas:
        return (None, None)
    rng = random.Random(seed)
    meds = sorted(st.median([deltas[rng.randrange(len(deltas))] for _ in deltas])
                  for _ in range(n))
    return (round(meds[int(0.025 * n)], 1), round(meds[int(0.975 * n)], 1))


def analyze(graded_path: Path):
    rows = [json.loads(l) for l in graded_path.read_text().splitlines() if l.strip()]
    from collections import defaultdict
    cost, turns, succ, tok = (defaultdict(list) for _ in range(4))
    typ, repo = {}, {}
    for r in rows:
        k = (r["task_id"], r["arm"])
        cost[k].append(r.get("cost_usd"))
        turns[k].append(r.get("num_turns"))
        succ[k].append(1 if r.get("success") else 0)
        tok[k].append(r.get("processed_tokens"))
        typ[r["task_id"]] = r["type"]
        repo[r["task_id"]] = r["repo"]
    tasks = sorted(typ)
    per_task, reductions, wins = [], [], 0
    for t in tasks:
        a, b = med(cost[(t, "A")]), med(cost[(t, "B")])
        if a is None or b is None:
            continue
        red = 100 * (a - b) / a if a else 0
        reductions.append(red)
        if b < a:
            wins += 1
        per_task.append({
            "task": t, "repo": repo[t], "type": typ[t],
            "cost_a": round(a, 4), "cost_b": round(b, 4), "reduction": round(red, 0),
            "succ_a": med(succ[(t, "A")]), "succ_b": med(succ[(t, "B")]),
            "turns_a": med(turns[(t, "A")]), "turns_b": med(turns[(t, "B")]),
        })
    b_runs = [r for r in rows if r["arm"] == "B"]
    adoption = round(100 * sum(1 for r in b_runs if r["codeindex_calls"] >= 1)
                     / len(b_runs), 0) if b_runs else 0
    sa = round(100 * sum(1 for r in rows if r["arm"] == "A" and r["success"])
               / max(1, sum(1 for r in rows if r["arm"] == "A")), 1)
    sb = round(100 * sum(1 for r in rows if r["arm"] == "B" and r["success"])
               / max(1, sum(1 for r in rows if r["arm"] == "B")), 1)
    m = med(reductions)
    verdict = ("GREEN" if (m or 0) >= 30 and (sb - sa) >= -5 and adoption >= 70
               else "RED" if (m or 0) < 10 else "YELLOW")
    return {
        "n_tasks": len(per_task), "median_reduction": round(m, 1) if m is not None else 0,
        "ci": bootstrap_ci(reductions), "win_rate": round(100 * wins / max(1, len(per_task)), 0),
        "succ_a": sa, "succ_b": sb, "adoption": adoption,
        "median_turns_a": med([p["turns_a"] for p in per_task]),
        "median_turns_b": med([p["turns_b"] for p in per_task]),
        "total_cost": round(sum(r.get("cost_usd") or 0 for r in rows), 2),
        "verdict": verdict, "per_task": per_task,
        "answers": {f"{r['task_id']}|{r['arm']}|{r['rep']}": {
            "answer": (r.get("answer") or "")[:4000],
            "cost": r.get("cost_usd"), "turns": r.get("num_turns"),
            "ci_calls": r.get("codeindex_calls"), "success": r.get("success"),
            "f1": r.get("f1"), "tools": r.get("tool_calls"),
        } for r in rows},
    }


CSS = """
:root{--green:#1a7f37;--red:#cf222e;--yellow:#9a6700;--bg:#0d1117;--card:#161b22;
--fg:#e6edf3;--mut:#8b949e;--line:#30363d;--accentA:#e08a8a;--accentB:#6fb3ff}
*{box-sizing:border-box}body{margin:0;font:14px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;
background:var(--bg);color:var(--fg)}.wrap{max-width:1000px;margin:0 auto;padding:28px}
h1{font-size:22px;margin:0 0 4px}.sub{color:var(--mut);margin:0 0 20px}
.badge{display:inline-block;padding:2px 10px;border-radius:20px;font-weight:700;font-size:12px}
.GREEN{background:rgba(26,127,55,.18);color:#3fb950;border:1px solid #238636}
.RED{background:rgba(207,34,46,.18);color:#f85149;border:1px solid #da3633}
.YELLOW{background:rgba(154,103,0,.18);color:#d29922;border:1px solid #9e6a03}
.exp{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:18px;margin:16px 0}
.exp h2{margin:0 0 2px;font-size:17px}.exp .tag{color:var(--mut);font-size:12px;margin-bottom:12px}
.cards{display:flex;flex-wrap:wrap;gap:10px;margin:12px 0}
.c{background:#0d1117;border:1px solid var(--line);border-radius:8px;padding:10px 14px;min-width:120px}
.c .v{font-size:20px;font-weight:700}.c .l{color:var(--mut);font-size:11px;text-transform:uppercase;letter-spacing:.04em}
table{width:100%;border-collapse:collapse;margin-top:8px;font-size:13px}
th,td{text-align:left;padding:6px 8px;border-bottom:1px solid var(--line)}
th{color:var(--mut);font-weight:600;font-size:11px;text-transform:uppercase}
td.num{text-align:right;font-variant-numeric:tabular-nums}
.pos{color:#3fb950}.neg{color:#f85149}
tr.task{cursor:pointer}tr.task:hover{background:#1c2330}
.drill{background:#0d1117}.drill td{padding:0}
.dd{padding:12px 16px;display:grid;grid-template-columns:1fr 1fr;gap:16px}
.arm{border:1px solid var(--line);border-radius:8px;padding:10px}
.arm h4{margin:0 0 6px;font-size:12px}.armA h4{color:var(--accentA)}.armB h4{color:var(--accentB)}
.arm .meta{color:var(--mut);font-size:11px;margin-bottom:6px}
pre{white-space:pre-wrap;word-break:break-word;background:#010409;border:1px solid var(--line);
border-radius:6px;padding:8px;margin:0;font-size:12px;max-height:260px;overflow:auto}
.foot{color:var(--mut);font-size:12px;margin-top:24px;border-top:1px solid var(--line);padding-top:12px}
.hl{background:var(--card);border-left:3px solid #3fb950;padding:10px 14px;border-radius:6px;margin:14px 0}
"""

JS = """
function toggle(id){var e=document.getElementById(id);e.style.display=e.style.display==='table-row'?'none':'table-row';}
"""


def stat_cards(a):
    def card(v, l, cls=""):
        return f'<div class="c"><div class="v {cls}">{v}</div><div class="l">{l}</div></div>'
    red = a["median_reduction"]
    rcls = "pos" if red >= 0 else "neg"
    rtxt = f'{red:+.0f}%'
    return "".join([
        card(rtxt, "median cost Δ (B vs A)", rcls),
        card(f'{a["win_rate"]:.0f}%', "win rate (B cheaper)"),
        card(f'{a["succ_a"]:.0f}→{a["succ_b"]:.0f}%', "success A→B"),
        card(f'{a["adoption"]:.0f}%', "adoption"),
        card(f'{a["median_turns_a"]:.0f}→{a["median_turns_b"]:.0f}', "median turns A→B"),
        card(f'${a["total_cost"]}', "run cost"),
    ])


def task_rows(a, label):
    out = []
    for i, p in enumerate(a["per_task"]):
        rid = f"{label}-{i}".replace(" ", "")
        red = p["reduction"]
        rcls = "pos" if red >= 0 else "neg"
        out.append(
            f'<tr class="task" onclick="toggle(\'{rid}\')">'
            f'<td>{escape(p["task"].split("-",1)[1] if "-" in p["task"] else p["task"])}</td>'
            f'<td>{p["repo"]}</td>'
            f'<td class="num">${p["cost_a"]:.3f}</td><td class="num">${p["cost_b"]:.3f}</td>'
            f'<td class="num {rcls}">{red:+.0f}%</td>'
            f'<td class="num">{p["succ_a"]:.1f}/{p["succ_b"]:.1f}</td>'
            f'<td class="num">{p["turns_a"]:.0f}/{p["turns_b"]:.0f}</td></tr>')
        # drill-down (rep 0 answers for each arm)
        ansA = a["answers"].get(f'{p["task"]}|A|0', {})
        ansB = a["answers"].get(f'{p["task"]}|B|0', {})

        def panel(cls, name, d):
            meta = (f'cost ${d.get("cost") or 0:.4f} · turns {d.get("turns")} · '
                    f'codeindex {d.get("ci_calls")} · f1 {d.get("f1")} · tools {escape(str(d.get("tools")))}')
            return (f'<div class="arm {cls}"><h4>{name}</h4>'
                    f'<div class="meta">{meta}</div>'
                    f'<pre>{escape(d.get("answer") or "(none)")}</pre></div>')
        out.append(
            f'<tr id="{rid}" class="drill" style="display:none"><td colspan="7">'
            f'<div class="dd">{panel("armA","Arm A (grep+read)",ansA)}'
            f'{panel("armB","Arm B (codeindex)",ansB)}</div></td></tr>')
    return "".join(out)


def main():
    analyses = []
    for label, gf, tf, cls in EXPERIMENTS:
        p = RESULTS / gf
        if p.exists():
            analyses.append((label, cls, analyze(p)))

    sections = []
    for label, cls, a in analyses:
        sections.append(f"""
        <div class="exp">
          <h2>{escape(label)} <span class="badge {a['verdict']}">{a['verdict']}</span></h2>
          <div class="tag">{escape(cls)} · {a['n_tasks']} tasks · 95% CI {a['ci']}</div>
          <div class="cards">{stat_cards(a)}</div>
          <table><thead><tr><th>task</th><th>repo</th><th>cost A</th><th>cost B</th>
          <th>Δ cost</th><th>succ A/B</th><th>turns A/B</th></tr></thead>
          <tbody>{task_rows(a, label)}</tbody></table>
        </div>""")

    boundary = ""
    if len(analyses) == 2:
        v1, v2 = analyses[0][2], analyses[1][2]
        boundary = (f'<div class="hl"><b>The boundary:</b> codeindex is overhead '
                    f'on questions grep answers cheaply (v1 {v1["median_reduction"]:+.0f}%), '
                    f'and cuts cost decisively on structural call-graph questions grep '
                    f'answers badly (v2 {v2["median_reduction"]:+.0f}%). Its value is real '
                    f'but bounded to callers/callees/impact.</div>')

    html = f"""<!doctype html><html><head><meta charset="utf-8">
<title>codeindex — agent A/B evaluation</title><style>{CSS}</style></head><body>
<div class="wrap">
<h1>codeindex — agent A/B evaluation</h1>
<p class="sub">Real Claude Code agents, with vs without codeindex, on real repos.
Primary metric: total task cost (USD). Click a task to see each arm's actual answer.</p>
{boundary}
{''.join(sections)}
<div class="foot">Generated from graded run data by <code>dashboard.py</code>.
Ground truth is arm-neutral (ripgrep / merged-PR files / index caller lists,
hand-verified). Thresholds pre-registered before the runs. See FINDINGS.md /
FINDINGS_v2.md for full methodology and caveats.</div>
</div><script>{JS}</script></body></html>"""
    OUT.write_text(html)
    print(f"wrote {OUT}  ({len(html)//1024} KB, {len(analyses)} experiments)")


if __name__ == "__main__":
    main()

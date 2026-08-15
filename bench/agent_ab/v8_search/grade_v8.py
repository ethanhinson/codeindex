#!/usr/bin/env python3
"""v8 grading + paired analysis. Success = any accept symbol named in the
answer's SYMBOLS section (word-boundary; bare accepts match a path component
of a dotted name). Committed before the full matrix ran."""
import json, re, statistics
from pathlib import Path

HERE = Path(__file__).resolve().parent
TASKS = {t["id"]: t for t in json.loads((HERE / "tasks.json").read_text())["tasks"]}

def symbols_section(answer: str) -> str:
    m = re.search(r"SYMBOLS:(.*?)(?:LOCATIONS:|$)", answer or "", re.S | re.I)
    return m.group(1) if m else (answer or "")

def hit(answer: str, accepts) -> bool:
    sec = symbols_section(answer)
    for a in accepts:
        name = a.split(".")[-1]
        if re.search(rf"\b{re.escape(name)}\b", sec):
            return True
    return False

runs = [json.loads(l) for l in (HERE / "results/runs.jsonl").read_text().splitlines()]
by = {}
for r in runs:
    r["success"] = hit(r.get("answer", ""), TASKS[r["task_id"]]["ground_truth"]["accept"])
    by[(r["task_id"], r["arm"], r["rep"])] = r

pairs, sa, sb, adopt = [], [], [], []
for (tid, arm, rep), r in by.items():
    if arm != "A": continue
    b = by.get((tid, "B", rep))
    if not b: continue
    sa.append(r["success"]); sb.append(b["success"])
    adopt.append(b.get("codeindex_calls", 0) > 0)
    if r.get("cost_usd") and b.get("cost_usd"):
        pairs.append((b["cost_usd"] - r["cost_usd"]) / r["cost_usd"] * 100)

n = len(sa)
succ_a, succ_b = 100*sum(sa)/n, 100*sum(sb)/n
med = statistics.median(pairs) if pairs else float("nan")
adoption = 100*sum(adopt)/len(adopt)
print(f"pairs={n}  success A={succ_a:.1f}%  B={succ_b:.1f}%  delta={succ_b-succ_a:+.1f}pp")
print(f"median paired cost delta (B vs A): {med:+.1f}%   adoption(B)={adoption:.0f}%")
for repo in ("gin", "laravel-framework"):
    ra = [by[k]["success"] for k in by if by[k]["arm"]=="A" and by[k]["repo"]==repo]
    rb = [by[k]["success"] for k in by if by[k]["arm"]=="B" and by[k]["repo"]==repo]
    if ra: print(f"  {repo}: A {100*sum(ra)/len(ra):.0f}% -> B {100*sum(rb)/len(rb):.0f}%")

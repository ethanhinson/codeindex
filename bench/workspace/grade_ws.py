#!/usr/bin/env python3
"""Grade workspace-graph gate runs: set comparison over file paths.

No LLM grader — grading is mechanical (grader-blind structurally). Answer
lines and ground truth both normalize to paths resolved against the
workspace root, so `../drupal/...`-style relative, workspace-relative, and
absolute forms all compare equal.

Per run: recall / precision over gt_files, cross_recall over gt_cross_files
(the D7 measurement), plus rung/kind passthrough for the gate script.

Usage: python3 grade_ws.py [--runs results/runs.jsonl]
"""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path

HERE = Path(__file__).resolve().parent
TASKS = {t["id"]: t for t in
         json.loads((HERE / "tasks" / "tasks_ws.json").read_text())["tasks"]}
WS_ROOT = (HERE.parent.parent
           / json.loads((HERE / "corpus.json").read_text())["workspace_root"]
           ).resolve()

EXTS = (".php", ".go", ".ts", ".py")
PATH_RE = re.compile(r"[\w./-]+\.(?:php|go|ts|py)\b")


def norm(p: str) -> str | None:
    p = p.strip().strip("`*").rstrip(":,")
    if not p.endswith(EXTS):
        return None
    try:
        if p.startswith("/"):
            return str(Path(p).resolve())
        return str((WS_ROOT / p).resolve())
    except OSError:
        return None


def extract_paths(answer: str) -> set[str]:
    out = set()
    for line in answer.splitlines():
        m = PATH_RE.search(line)
        if m:
            n = norm(m.group(0))
            if n:
                out.add(n)
    return out


def grade_run(r: dict) -> dict | None:
    t = TASKS.get(r["task_id"])
    if t is None:
        return None
    got = extract_paths(r.get("answer") or "")
    gt = {norm(f) for f in t["gt_files"]}
    # tasks are cross-member scoped; for ximpact the def file is the only
    # same-member entry — cross recall excludes it
    gt_cross = gt - {norm(t["def_file"])} if t["kind"] == "ximpact" else gt
    tp = len(got & gt)
    shell_calls = sum(v for k, v in (r.get("tool_calls") or {}).items()
                      if k in ("Bash", "Grep", "Glob", "Read"))
    mcp_calls = sum(v for k, v in (r.get("tool_calls") or {}).items()
                    if k.startswith("mcp__"))
    return {
        "key": r["key"], "task_id": r["task_id"], "arm": r["arm"],
        "rep": r["rep"], "kind": t["kind"], "rung": t["rung"],
        "recall": round(tp / len(gt), 4) if gt else None,
        "precision": round(tp / len(got), 4) if got else 0.0,
        "cross_recall": (round(len(got & gt_cross) / len(gt_cross), 4)
                         if gt_cross else None),
        "n_gt": len(gt), "n_got": len(got),
        "missed": sorted(str(Path(f).relative_to(WS_ROOT.parent))
                         for f in (gt - got) if f),
        "spurious_count": len(got - gt),
        "shell_calls": shell_calls, "mcp_calls": mcp_calls,
        "processed_tokens": r.get("processed_tokens"),
        "cost_usd": r.get("cost_usd"), "timed_out": r.get("timed_out"),
    }


def median(xs):
    xs = sorted(x for x in xs if x is not None)
    if not xs:
        return None
    n = len(xs)
    return xs[n // 2] if n % 2 else (xs[n // 2 - 1] + xs[n // 2]) / 2


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--runs", default="results/runs.jsonl")
    args = ap.parse_args()

    runs = [json.loads(l) for l in
            (HERE / args.runs).read_text().splitlines() if l]
    grades = [g for g in (grade_run(r) for r in runs) if g]
    out = HERE / "results" / "grades.jsonl"
    out.write_text("".join(json.dumps(g) + "\n" for g in grades))

    by_arm = {}
    for g in grades:
        by_arm.setdefault(g["arm"], []).append(g)
    for arm, gs in sorted(by_arm.items()):
        r1 = [g for g in gs if g["rung"] == "rung1"]
        print(f"arm {arm}: n={len(gs)} "
              f"med_recall={median([g['recall'] for g in gs])} "
              f"med_cross_recall={median([g['cross_recall'] for g in gs])} "
              f"rung1_med_cross_recall="
              f"{median([g['cross_recall'] for g in r1])} "
              f"med_precision={median([g['precision'] for g in gs])} "
              f"med_tokens={median([g['processed_tokens'] for g in gs])} "
              f"med_shell_calls={median([g['shell_calls'] for g in gs])}")
    print(f"wrote {out} ({len(grades)} grades)")


if __name__ == "__main__":
    main()

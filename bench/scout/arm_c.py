#!/usr/bin/env python3
"""Arm C: classifier-routed, deterministic navigation — NO agent reasoning.

The real Scout test. For each harness task:
  1. extract the target symbol (from the task id),
  2. classify the task -> tool {callers, grep, find} (embedding + logreg),
  3. run that codeindex tool once,
  4. the tool's raw output IS the answer.
Then grade with the harness's OWN grader and compare to arm B (agent+index) and
arm A (agent, no index) from the prior A/B run.

If arm C's answer quality approaches arm B at ~0 agent cost, a classifier +
deterministic execution can replace the agent for navigation — the whole Scout
thesis, tested end-to-end. All local.
"""
from __future__ import annotations
import json, sys, subprocess, re
from pathlib import Path
import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent / "agent_ab"))
import grade as G  # harness grader
from measure_ceiling import PARA, make_task  # calibrated templates (unused here but keeps parity)
from clf_baseline import embed, build_split_by_template
from sklearn.linear_model import LogisticRegression

# --- train the router classifier on the gin Tier-1 corpus (all of it) ---
def train_router():
    rows = [json.loads(l) for l in open(HERE / "gin_tier1.jsonl")]
    Xtr_t, ytr, _, _ = build_split_by_template(rows, seed=13, holdout_templates=0)
    Xtr = embed(Xtr_t)
    clf = LogisticRegression(max_iter=2000, C=2.0).fit(Xtr, ytr)
    return clf

# --- map harness task -> (symbol, natural task string for the classifier) ---
def parse_task(t):
    tid = t["id"]  # e.g. occur-laravel-framework-throwBadMethodCallException
    # symbol is the last dash-group; repo names contain dashes so split on known prefixes
    m = re.match(r"(occur|vague|comp|callattr|editimp)-(.+)-([A-Za-z_][\w]*)$", tid)
    sym = m.group(3) if m else tid.split("-")[-1]
    prompt = t["prompt"]
    return sym, prompt

# --- run a codeindex tool, return raw output as the "answer" ---
def run_tool(tool, repo, sym, binary):
    args = {"callers": ["callers", repo, sym],
            "grep": ["grep", repo, sym],
            "find": ["find", repo, sym, "--limit", "10"]}[tool]
    return subprocess.run([binary, *args], capture_output=True, text=True).stdout

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--tasks", default="../agent_ab/tasks/tasks_v6.json")
    ap.add_argument("--repos-root", default="../repos")
    ap.add_argument("--binary", default="/opt/homebrew/bin/codeindex")
    ap.add_argument("--out", default="arm_c_results.jsonl")
    args = ap.parse_args()

    print("training router (local embeddings)...", file=sys.stderr)
    clf = train_router()

    data = json.load(open(HERE / args.tasks))
    tasks = data["tasks"]
    rows = []
    # classify all task prompts in one batch
    prompts = [t["prompt"] for t in tasks]
    Xq = embed(prompts)
    routes = clf.predict(Xq)

    for t, tool in zip(tasks, routes):
        sym, prompt = parse_task(t)
        repo = str((HERE / args.repos_root / t["repo"]).resolve())
        # comprehension/vague want a locate/define answer; the router may pick
        # find/grep — we honor the router's choice (that's the test).
        answer = run_tool(tool, repo, sym, args.binary)
        prefix = repo.rstrip("/") + "/"
        gt = t["ground_truth"]
        tp = t["type"]
        if tp == "comprehension":
            score, detail, unp = G.grade_comprehension(answer, gt, prefix)
        elif tp == "vague_find":
            score, detail, unp = G.grade_vague_find(answer, gt, prefix)
        elif tp in ("caller_attribution", "occurrences", "edit_impact"):
            score, detail, unp = G.grade_caller_attribution(answer, gt, prefix)
        elif tp == "localization":
            score, detail, unp = G.grade_localization(answer, gt, prefix)
        else:
            score, detail, unp = 0.0, {}, True
        rows.append({"id": t["id"], "type": tp, "repo": t["repo"], "symbol": sym,
                     "routed_tool": tool, "f1": round(score, 3),
                     "success": score >= 0.5, "unparseable": unp})

    Path(HERE / args.out).write_text("\n".join(json.dumps(r) for r in rows) + "\n")

    # report
    from collections import defaultdict
    bt = defaultdict(lambda: {"n": 0, "f1": 0.0, "succ": 0, "tools": defaultdict(int)})
    for r in rows:
        b = bt[r["type"]]; b["n"] += 1; b["f1"] += r["f1"]; b["succ"] += r["success"]
        b["tools"][r["routed_tool"]] += 1
    print(f"\n=== ARM C (classifier-routed, NO agent) on {args.tasks} ===")
    print(f"{'type':<18} {'n':>3} {'medF1':>6} {'succ%':>6}  routed-tools")
    for tp, b in sorted(bt.items()):
        print(f"{tp:<18} {b['n']:>3} {b['f1']/b['n']:>6.2f} {100*b['succ']/b['n']:>5.0f}%  {dict(b['tools'])}")
    tot = len(rows); f1 = sum(r["f1"] for r in rows)/tot; sc = sum(r["success"] for r in rows)/tot
    print(f"{'OVERALL':<18} {tot:>3} {f1:>6.2f} {100*sc:>5.0f}%")
    print("\nNOTE: arm C cost ~= 1 classifier inference + 1 CLI call (no agent tokens).")
    print("Compare success% to arm B (agent+index) in results/graded_v6.jsonl.")

if __name__ == "__main__":
    main()

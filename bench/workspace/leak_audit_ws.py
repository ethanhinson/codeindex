#!/usr/bin/env python3
"""Four-class leak audit for the workspace-graph gate (tasks.md §2.2).

The discipline rule (ROADMAP M2 + ROADMAP-DEBATE Critic #3) names four leak
classes; this audits all four against the frozen task set, the harness, and
every arm-A transcript on disk. Runs before ANY scored campaign; a scored run
without a clean audit is not admissible evidence (bench/workspace/README.md).

  1. TEMPLATE LEAKAGE   — task prompts must not contain any ground-truth file
                          path (member roots are disclosed by design — both
                          arms see them; the answer files never). GT must not
                          live under the dirs the prompt tells agents to skip
                          (vendor/, node_modules/, dist/, build/) — that would
                          make GT unreachable-by-instruction for both arms.
  2. CONTROL CONTAMINATION — arm-A transcripts show no REAL codeindex output
                          (id-paired tool_use↔tool_result audit, the
                          bench/agent_ab/leak_audit.py join — positional
                          pairing is banned), no mcp__* tool use, and no
                          UserPromptSubmit hook signature (the bench-hook-leak
                          text) injected past --setting-sources.
  3. FORCED-TOOL        — prompts are tool-neutral: no instruction to use
                          codeindex / MCP / grep / any named tool. The verbs
                          "list/find which files reference X" are the task;
                          the route is the arm's own choice.
  4. GRADER CO-DESIGN   — grading must be invariant to answer ordering and
                          decoration (the v10 union-arm leak: output ordering
                          tuned against the grader's scoring regions).
                          Proved by property test against grade_ws.grade_run:
                          reversed line order and backtick/colon decoration
                          grade identically. grade_ws has no scoring regions
                          to co-design against; this pins that property.

Usage:
  python3 leak_audit_ws.py                  # audit tasks + all arm-A transcripts
  python3 leak_audit_ws.py --json
  python3 leak_audit_ws.py --selftest       # id-pairing proof (batched shape)
Exit 1 on any FAIL (use as a campaign pre-gate).
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent / "agent_ab"))
import leak_audit as base  # noqa: E402  (id-paired transcript audit)
import grade_ws  # noqa: E402

TASKS = json.loads((HERE / "tasks" / "tasks_ws.json").read_text())["tasks"]
TRANSCRIPTS = HERE / "results" / "transcripts"

EXCLUDED_DIRS = ("/vendor/", "/node_modules/", "/dist/", "/build/")
# Tool-forcing vocabulary. "grep" alone is fine in *our* audit but banned in
# prompts: naming any route stops the arm contrast from measuring choice.
FORCING = ("codeindex", "mcp", "grep", "ripgrep", " rg ", "glob(", "use the tool",
           "must use", "call the")
HOOK_SIGNATURE = "call-graph index"  # the UserPromptSubmit hook's fingerprint


def audit_template_leakage() -> dict:
    hits = []
    for t in TASKS:
        paths = set(t["gt_files"]) | {t["def_file"]}
        for p in paths:
            if p in t["prompt"]:
                hits.append({"task": t["id"], "leaked_path": p})
        for g in t["gt_files"]:
            if any(d in g for d in EXCLUDED_DIRS):
                hits.append({"task": t["id"], "gt_in_excluded_dir": g})
    return {"class": "template_leakage", "n_tasks": len(TASKS),
            "violations": hits, "verdict": "FAIL" if hits else "PASS"}


def audit_control_contamination() -> dict:
    files = sorted(TRANSCRIPTS.glob("*_A_*.jsonl")) if TRANSCRIPTS.is_dir() else []
    reports, mcp_uses, hook_hits = [], [], []
    for f in files:
        rep = base.audit_file(f)
        reports.append(rep)
        text = f.read_text()
        if HOOK_SIGNATURE in text:
            hook_hits.append(f.name)
        for ln in text.splitlines():
            try:
                ev = json.loads(ln)
            except json.JSONDecodeError:
                continue
            if ev.get("type") != "assistant":
                continue
            for b in ev.get("message", {}).get("content", []) or []:
                if (isinstance(b, dict) and b.get("type") == "tool_use"
                        and str(b.get("name", "")).startswith("mcp__")):
                    mcp_uses.append({"file": f.name, "tool": b["name"]})
    real = [a for r in reports for a in r["real"]]
    attempts = sum(r["attempts"] for r in reports)
    unpaired = sum(r["unpaired"] for r in reports)
    verdict = ("FAIL" if (real or mcp_uses or hook_hits)
               else "NO-TRANSCRIPTS" if not files else "PASS")
    return {"class": "control_contamination", "n_transcripts": len(files),
            "codeindex_attempts": attempts, "real": real, "unpaired": unpaired,
            "mcp_tool_uses": mcp_uses, "hook_signature_hits": hook_hits,
            "verdict": verdict}


def audit_forced_tool() -> dict:
    hits = []
    for t in TASKS:
        low = t["prompt"].lower()
        for w in FORCING:
            if w in low:
                hits.append({"task": t["id"], "phrase": w.strip()})
    return {"class": "forced_tool", "n_tasks": len(TASKS),
            "violations": hits, "verdict": "FAIL" if hits else "PASS"}


def audit_grader_codesign() -> dict:
    """Property test: grade_run is invariant to line order and decoration."""
    t = TASKS[0]
    lines = [g for g in t["gt_files"]][: min(8, len(t["gt_files"]))]
    base_run = {"key": "audit_A_r0", "task_id": t["id"], "arm": "A", "rep": 0}
    variants = {
        "sorted": "\n".join(lines),
        "reversed": "\n".join(reversed(lines)),
        "decorated": "\n".join(f"- `{l}`:" for l in reversed(lines)),
    }
    grades = {}
    for name, answer in variants.items():
        g = grade_ws.grade_run({**base_run, "answer": answer})
        grades[name] = {k: g[k] for k in
                        ("recall", "precision", "cross_recall", "n_got")}
    invariant = len({json.dumps(v, sort_keys=True)
                     for v in grades.values()}) == 1
    return {"class": "grader_codesign", "grades": grades,
            "verdict": "PASS" if invariant else "FAIL"}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--json", action="store_true", dest="as_json")
    ap.add_argument("--selftest", action="store_true",
                    help="run the id-pairing proof from the base audit")
    args = ap.parse_args()
    if args.selftest:
        return base.selftest()

    reports = [audit_template_leakage(), audit_control_contamination(),
               audit_forced_tool(), audit_grader_codesign()]
    failed = [r for r in reports if r["verdict"] == "FAIL"]
    if args.as_json:
        print(json.dumps({"reports": reports,
                          "verdict": "FAIL" if failed else "PASS"}, indent=2))
    else:
        for r in reports:
            print(f"[{r['verdict']:>4}] {r['class']}")
            for v in r.get("violations", []):
                print(f"       {v}")
            for a in r.get("real", []):
                print(f"       REAL {a['tool']} {a['input']}")
            for h in r.get("hook_signature_hits", []):
                print(f"       HOOK {h}")
            for m in r.get("mcp_tool_uses", []):
                print(f"       MCP  {m}")
            if r["class"] == "control_contamination":
                print(f"       transcripts={r['n_transcripts']} "
                      f"attempts={r['codeindex_attempts']} "
                      f"unpaired={r['unpaired']}")
        print(f"\nverdict: {'FAIL' if failed else 'PASS'}"
              f"{' (no transcripts audited)' if any(r['verdict'] == 'NO-TRANSCRIPTS' for r in reports) else ''}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())

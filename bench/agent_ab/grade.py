#!/usr/bin/env python3
"""Arm-blind script grader. Joins results/runs.jsonl with task ground truth and
scores each run's final answer -> results/graded.jsonl.

Comprehension: definitions (path match + line within +/-2) F1 and referencing-
files set F1, averaged; success = F1 >= 0.5.
Localization: file-set F1 vs merged-PR files; success = F1 >= 0.5.
Unparseable answers score 0 and are flagged (distinguished from wrong answers).

Usage: python3 grade.py
"""

from __future__ import annotations

import json
import re
from pathlib import Path

HERE = Path(__file__).resolve().parent
RUNS = HERE / "results" / "runs.jsonl"
GRADED = HERE / "results" / "graded.jsonl"
TASKS = HERE / "tasks" / "tasks.json"
import sys
sys.path.insert(0, str(HERE.parent))
from build_tasks import repo_path  # noqa: E402

GO_FILELINE = re.compile(r"([\w./\-]+\.(?:go|ts|tsx|js|jsx|py|php)):(\d+)")
GO_FILE = re.compile(r"([\w./\-]+\.(?:go|ts|tsx|js|jsx|py|php))\b")


def norm_path(p: str, repo_prefix: str) -> str:
    p = p.strip().strip("`*_ ").lstrip("-* ").strip()
    if p.startswith(repo_prefix):
        p = p[len(repo_prefix):]
    return p.lstrip("/")


def split_sections(answer: str) -> tuple[str, str]:
    """Return (definitions_region, files_region) from the agent answer."""
    a = answer or ""
    up = a.upper()
    di = up.find("DEFINITIONS")
    fi = up.find("FILES")
    if di >= 0 and fi >= 0 and fi > di:
        return a[di:fi], a[fi:]
    if fi >= 0 and di < 0:
        return "", a[fi:]
    if di >= 0 and fi < 0:
        return a[di:], ""
    return a, a  # no headers: search whole answer in both


def f1(claim: set, truth: set) -> tuple[float, float, float]:
    if not claim and not truth:
        return 1.0, 1.0, 1.0
    if not claim or not truth:
        return 0.0, 0.0, 0.0
    tp = len(claim & truth)
    prec = tp / len(claim)
    rec = tp / len(truth)
    return (2 * prec * rec / (prec + rec) if (prec + rec) else 0.0, prec, rec)


def grade_comprehension(answer, gt, repo_prefix):
    defs_region, files_region = split_sections(answer)
    # definitions: match path + line within +/-2
    claimed = [(norm_path(m.group(1), repo_prefix), int(m.group(2)))
               for m in GO_FILELINE.finditer(defs_region)]
    truth_defs = []
    for e in gt["definitions"]:
        p, ln = e.rsplit(":", 1)
        truth_defs.append((p, int(ln)))
    unparseable = not claimed and not GO_FILE.search(files_region or "")
    if claimed:
        correct = sum(1 for (p, l) in claimed
                      if any(p == tp and abs(l - tl) <= 2 for tp, tl in truth_defs))
        dprec = correct / len(claimed)
        drec = (sum(1 for tp, tl in truth_defs
                    if any(p == tp and abs(l - tl) <= 2 for p, l in claimed))
                / len(truth_defs)) if truth_defs else 0.0
        def_f1 = 2 * dprec * drec / (dprec + drec) if (dprec + drec) else 0.0
    else:
        def_f1 = 0.0
    claim_files = {norm_path(m.group(1), repo_prefix)
                   for m in GO_FILE.finditer(files_region)}
    file_f1, _, _ = f1(claim_files, set(gt["files"]))
    task_f1 = (def_f1 + file_f1) / 2
    return task_f1, {"def_f1": round(def_f1, 3), "file_f1": round(file_f1, 3),
                     "n_claim_files": len(claim_files)}, unparseable


def grade_localization(answer, gt, repo_prefix):
    _, files_region = split_sections(answer)
    region = files_region or (answer or "")
    claim = {norm_path(m.group(1), repo_prefix) for m in GO_FILE.finditer(region)}
    unparseable = not claim
    score, prec, rec = f1(claim, set(gt["files"]))
    return score, {"precision": round(prec, 3), "recall": round(rec, 3),
                   "n_claim_files": len(claim)}, unparseable


def grade_caller_attribution(answer, gt, repo_prefix):
    """Grade F1 on (caller_name, file) pairs. Grep gives file:line but not the
    caller function name, so producing the names is the discriminating work."""
    a = answer or ""
    up = a.upper()
    ci = up.find("CALLERS")
    region = a[ci:] if ci >= 0 else a
    truth = set(gt["caller_pairs"])  # "name\tfile"
    truth_files = {p.split("\t", 1)[1] for p in truth}
    claim = set()
    for line in region.splitlines():
        fm = GO_FILE.search(line)
        if not fm:
            continue
        f = norm_path(fm.group(1), repo_prefix)
        # caller name: first identifier token on the line that isn't in the path
        before = line[:fm.start()]
        names = re.findall(r"[A-Za-z_]\w{2,}", before)
        names = [n for n in names if n.lower() not in ("func", "method", "the", "calls", "in")]
        if names:
            claim.add(f"{names[-1]}\t{f}")
    unparseable = not claim
    score, prec, rec = f1(claim, truth)
    return score, {"precision": round(prec, 3), "recall": round(rec, 3),
                   "n_claim": len(claim), "n_truth": len(truth),
                   "n_truth_files": len(truth_files)}, unparseable


def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--tag", default="")
    args = ap.parse_args()
    suffix = f"_{args.tag}" if args.tag else ""
    tasks_file = HERE / "tasks" / f"tasks{suffix}.json"
    runs_file = HERE / "results" / f"runs{suffix}.jsonl"
    graded_file = HERE / "results" / f"graded{suffix}.jsonl"

    tasks = {t["id"]: t for t in json.loads(tasks_file.read_text())["tasks"]}
    prefixes = {}
    rows = []
    for ln in runs_file.read_text().splitlines():
        if not ln.strip():
            continue
        r = json.loads(ln)
        task = tasks.get(r["task_id"])
        if not task:
            continue
        if r["repo"] not in prefixes:
            prefixes[r["repo"]] = str(repo_path(r["repo"]))
        prefix = prefixes[r["repo"]]
        gt = task["ground_truth"]
        if task["type"] == "comprehension":
            score, detail, unp = grade_comprehension(r.get("answer", ""), gt, prefix)
        elif task["type"] in ("caller_attribution", "edit_impact"):
            score, detail, unp = grade_caller_attribution(r.get("answer", ""), gt, prefix)
        else:
            score, detail, unp = grade_localization(r.get("answer", ""), gt, prefix)
        if r.get("timed_out") or not r.get("has_result"):
            score, unp = 0.0, True
        r.update({"f1": round(score, 3), "success": score >= 0.5,
                  "unparseable": unp, "grade_detail": detail})
        rows.append(r)

    graded_file.write_text("\n".join(json.dumps(r) for r in rows))
    n_succ = sum(1 for r in rows if r["success"])
    n_unp = sum(1 for r in rows if r["unparseable"])
    print(f"graded {len(rows)} runs -> {graded_file}")
    print(f"  success: {n_succ}/{len(rows)}   unparseable: {n_unp}")
    for r in rows:
        print(f"  {r['key']:<40} arm={r['arm']} f1={r['f1']:.2f} "
              f"success={r['success']} codeindex={r['codeindex_calls']}"
              f"{' UNPARSEABLE' if r['unparseable'] else ''}")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""M5 arm-blind grader. Joins results/runs_m5.jsonl with task ground truth →
results/graded_m5.jsonl.

Per type:
  grepwin_string    LOCATION: file:line — 1.0 path+line (±1), 0.5 path only.
  grepwin_filename  PATH: exact repo-relative match — 1.0 / 0.0.
  dominate_callers  (caller,file)-pair F1 (agent_ab grader, verbatim).
  dominate_tests    file-set F1.
  dominate_blast    file-set F1; recall recorded separately (this IS the
                    blast-radius-recall metric).
  break_collision   definition-set F1 (path + line ±2).

False confidence (pre-registered in the tasks header): recall < 1.0 AND the
answer's COVERAGE line is not 'incomplete'. A missing COVERAGE line counts as
a completeness claim — the prompt demanded one. Computed for the three
COVERAGE-bearing types (dominate_callers, dominate_blast, break_collision).

Usage: python3 grade_m5.py [--selftest]
"""

from __future__ import annotations

import json
import os
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
BENCH = REPO_ROOT / "bench"
TASKS = HERE / "tasks" / "tasks_m5.json"
RUNS = HERE / "results" / "runs_m5.jsonl"
GRADED = HERE / "results" / "graded_m5.jsonl"

os.environ.setdefault("AB_WORK", str(BENCH / "repos"))
sys.path.insert(0, str(BENCH / "agent_ab"))
sys.path.insert(0, str(BENCH))
from grade import f1, norm_path, GO_FILE, GO_FILELINE, grade_caller_attribution  # noqa: E402
from build_tasks_m5 import repo_path  # noqa: E402

COVERAGE_RE = re.compile(r"COVERAGE:\s*(complete|incomplete)", re.I)


def coverage_claim(answer: str) -> str:
    m = COVERAGE_RE.search(answer or "")
    return m.group(1).lower() if m else "absent"


def grade_grepwin_string(answer, gt, prefix):
    a = answer or ""
    m = GO_FILELINE.search(a)
    if not m:
        return 0.0, {"parsed": None}, True
    p, ln = norm_path(m.group(1), prefix), int(m.group(2))
    tp, tl = gt["location"].rsplit(":", 1)
    if p == tp and abs(ln - int(tl)) <= 1:
        return 1.0, {"parsed": f"{p}:{ln}"}, False
    if p == tp:
        return 0.5, {"parsed": f"{p}:{ln}", "line_wrong": True}, False
    return 0.0, {"parsed": f"{p}:{ln}"}, False


def grade_grepwin_filename(answer, gt, prefix):
    a = answer or ""
    claims = [norm_path(m.group(1), prefix) for m in GO_FILE.finditer(a)]
    if not claims:
        return 0.0, {"parsed": None}, True
    ok = gt["path"] in claims
    return (1.0 if ok else 0.0), {"parsed": claims[:3]}, False


def grade_file_set(answer, gt, prefix):
    a = answer or ""
    up = a.upper()
    fi = up.find("FILES")
    region = a[fi:] if fi >= 0 else a
    claim = {norm_path(m.group(1), prefix) for m in GO_FILE.finditer(region)}
    score, prec, rec = f1(claim, set(gt["files"]))
    return score, {"precision": round(prec, 3), "recall": round(rec, 3),
                   "n_claim": len(claim), "n_truth": len(gt["files"])}, not claim


def grade_collision(answer, gt, prefix):
    a = answer or ""
    up = a.upper()
    di = up.find("DEFINITIONS")
    region = a[di:] if di >= 0 else a
    claimed = [(norm_path(m.group(1), prefix), int(m.group(2)))
               for m in GO_FILELINE.finditer(region)]
    truth = [(e.rsplit(":", 1)[0], int(e.rsplit(":", 1)[1]))
             for e in gt["definitions"]]
    if not claimed:
        return 0.0, {"n_claim": 0, "n_truth": len(truth), "recall": 0.0}, True
    hit = lambda c, t: c[0] == t[0] and abs(c[1] - t[1]) <= 2  # noqa: E731
    tp_c = sum(1 for c in claimed if any(hit(c, t) for t in truth))
    tp_t = sum(1 for t in truth if any(hit(c, t) for c in claimed))
    prec = tp_c / len(claimed)
    rec = tp_t / len(truth)
    score = 2 * prec * rec / (prec + rec) if (prec + rec) else 0.0
    return score, {"precision": round(prec, 3), "recall": round(rec, 3),
                   "n_claim": len(claimed), "n_truth": len(truth)}, False


GRADERS = {
    "grepwin_string": grade_grepwin_string,
    "grepwin_filename": grade_grepwin_filename,
    "dominate_tests": grade_file_set,
    "dominate_blast": grade_file_set,
    "break_collision": grade_collision,
}
COVERAGE_TYPES = {"dominate_callers", "dominate_blast", "break_collision"}


def grade_row(r: dict, task: dict, prefix: str) -> dict:
    answer = r.get("answer", "")
    gt = task["ground_truth"]
    if task["type"] == "dominate_callers":
        score, detail, unp = grade_caller_attribution(answer, gt, prefix)
    else:
        score, detail, unp = GRADERS[task["type"]](answer, gt, prefix)
    if r.get("timed_out") or not r.get("has_result"):
        score, unp = 0.0, True
    out = dict(r)
    out.update({"f1": round(score, 3), "success": score >= 0.5,
                "unparseable": unp, "grade_detail": detail})
    if task["type"] in COVERAGE_TYPES:
        cov = coverage_claim(answer)
        rec = detail.get("recall")
        out["coverage_claim"] = cov
        out["false_confidence"] = (rec is not None and rec < 0.999
                                   and cov != "incomplete")
    return out


def selftest():
    p = "/x/"
    s, d, u = grade_grepwin_string("LOCATION: a/b.go:10", {"location": "a/b.go:11"}, p)
    assert s == 1.0 and not u, (s, d)
    s, _, _ = grade_grepwin_string("LOCATION: a/b.go:99", {"location": "a/b.go:11"}, p)
    assert s == 0.5
    s, _, u = grade_grepwin_string("no idea", {"location": "a/b.go:11"}, p)
    assert s == 0.0 and u
    s, _, _ = grade_grepwin_filename("PATH: pkg/util/helper.go", {"path": "pkg/util/helper.go"}, p)
    assert s == 1.0
    s, d, _ = grade_file_set("FILES:\na.go\nb.go", {"files": ["a.go", "c.go"]}, p)
    assert d["recall"] == 0.5 and 0 < s < 1
    s, d, _ = grade_collision("DEFINITIONS:\na/b.go:10\nc/d.py:5",
                              {"definitions": ["a/b.go:11", "c/d.py:5", "e/f.go:1"]}, p)
    assert d["recall"] == 0.667 and d["precision"] == 1.0
    # false-confidence semantics
    row = {"answer": "DEFINITIONS:\na/b.go:10\nCOVERAGE: complete",
           "has_result": True}
    task = {"type": "break_collision",
            "ground_truth": {"definitions": ["a/b.go:10", "c/d.go:1"]}}
    g = grade_row(row, task, p)
    assert g["false_confidence"] is True and g["coverage_claim"] == "complete"
    row["answer"] = "DEFINITIONS:\na/b.go:10\nCOVERAGE: incomplete"
    assert grade_row(row, task, p)["false_confidence"] is False
    row["answer"] = "DEFINITIONS:\na/b.go:10"  # absent claim = claimed complete
    assert grade_row(row, task, p)["false_confidence"] is True
    row["answer"] = "DEFINITIONS:\na/b.go:10\nc/d.go:1"  # full recall
    assert grade_row(row, task, p)["false_confidence"] is False
    # caller grading is imported verbatim from agent_ab — spot-check the join
    row = {"answer": "CALLERS:\nfoo  a/b.go\nCOVERAGE: complete", "has_result": True}
    task = {"type": "dominate_callers",
            "ground_truth": {"caller_pairs": ["foo\ta/b.go", "bar\tc/d.go"]}}
    g = grade_row(row, task, p)
    assert g["grade_detail"]["recall"] == 0.5 and g["false_confidence"] is True
    print("selftest OK: graders + false-confidence semantics")


def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--selftest", action="store_true")
    ap.add_argument("--family", default="claude",
                    choices=("claude", "fuse", "floor"),
                    help="claude: runs_m5.jsonl; fuse: runs_m5_fuse.jsonl; "
                         "floor: runs_m5_floor.jsonl")
    args = ap.parse_args()
    if args.selftest:
        selftest()
        return
    global RUNS, GRADED
    if args.family != "claude":
        RUNS = HERE / "results" / f"runs_m5_{args.family}.jsonl"
        GRADED = HERE / "results" / f"graded_m5_{args.family}.jsonl"

    tasks = {t["id"]: t for t in json.loads(TASKS.read_text())["tasks"]}
    prefixes = {}
    rows = []
    for ln in RUNS.read_text().splitlines():
        if not ln.strip():
            continue
        r = json.loads(ln)
        task = tasks.get(r["task_id"])
        if not task:
            continue
        if r["repo"] not in prefixes:
            prefixes[r["repo"]] = str(repo_path(r["repo"]))
        rows.append(grade_row(r, task, prefixes[r["repo"]]))

    GRADED.write_text("\n".join(json.dumps(r) for r in rows))
    n_succ = sum(1 for r in rows if r["success"])
    n_fc = sum(1 for r in rows if r.get("false_confidence"))
    print(f"graded {len(rows)} runs -> {GRADED}")
    print(f"  success: {n_succ}/{len(rows)}   false_confidence: {n_fc}")
    for r in rows:
        fc = " FALSE-CONF" if r.get("false_confidence") else ""
        print(f"  {r['key']:<48} arm={r['arm']} f1={r['f1']:.2f} "
              f"success={r['success']} ci={r['codeindex_calls']}{fc}")


if __name__ == "__main__":
    main()

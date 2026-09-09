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
DEFAULT_TASKS = "tasks/tasks_ws.json"

# NO MODULE-LEVEL ARGV PARSING, AND NO IMPORT-TIME FILE READ. This block used
# to resolve the task file out of `sys.argv` at import time, so any script
# that imported grade_ws — a gate driver, leak_audit_ws, a notebook — silently
# handed ITS OWN flags to the grader, or died on a missing default task file
# before its own main() ever ran. The bundle is an explicit argument now,
# built by load(); the module-level fallback below is lazy, so importing the
# module still touches no argv and no task file.
_DEFAULT_BUNDLE = None
WS_ROOT = (HERE.parent.parent
           / json.loads((HERE / "corpus.json").read_text())["workspace_root"]
           ).resolve()

def load(tasks_path: str | Path = DEFAULT_TASKS) -> dict:
    """Read a task file into the bundle `grade_run`/`subset_report` consume.

    `tasks_path` is resolved against this directory when relative, matching
    the old `--tasks` behaviour exactly.
    """
    path = Path(tasks_path)
    if not path.is_absolute():
        path = HERE / path
    raw = json.loads(path.read_text())
    return {
        "path": path,
        "tasks": {t["id"]: t for t in raw["tasks"]},
        # The structural/control partition is READ from the task-file header,
        # never hardcoded here: a hardcoded map rots the moment a shape is
        # added or excluded.
        "subsets": raw["header"].get("subsets") or {},
    }


def default_bundle() -> dict:
    """The lazily-loaded default bundle, for callers that pass none.

    Keeps `grade_ws.grade_run(run)` — the shape leak_audit_ws.py uses —
    working, without reading anything at import time.
    """
    global _DEFAULT_BUNDLE
    if _DEFAULT_BUNDLE is None:
        _DEFAULT_BUNDLE = load()
    return _DEFAULT_BUNDLE


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


def _display(p: str) -> str:
    """Shorten a resolved path for the diagnostic `missed` field.

    Purely cosmetic: `missed` is a diagnostic, never an input to any metric.
    A path can legitimately resolve outside the workspace root's parent (a
    git worktree reaches members through symlinks into the primary tree), so
    fall back to the absolute path rather than raising.
    """
    try:
        return str(Path(p).relative_to(WS_ROOT.parent))
    except ValueError:
        return p


def extract_paths(answer: str) -> set[str]:
    out = set()
    for line in answer.splitlines():
        m = PATH_RE.search(line)
        if m:
            n = norm(m.group(0))
            if n:
                out.add(n)
    return out


def grade_run(r: dict, bundle: dict | None = None) -> dict | None:
    t = (bundle or default_bundle())["tasks"].get(r["task_id"])
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
        "missed": sorted(_display(f) for f in (gt - got) if f),
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


def mean(xs):
    xs = [x for x in xs if x is not None]
    return round(sum(xs) / len(xs), 4) if xs else None


def subset_report(arm, gs, bundle: dict | None = None):
    """Per-arm subset figures, driven entirely by the header's kind -> subset
    map: control median (B2/B3), scored-structural mean (B1) and median, and
    per-shape means. Shapes the header marks excluded are kept out of the
    scored structural subset and reported separately."""
    subsets = (bundle or default_bundle())["subsets"]
    if not subsets:
        print(f"arm {arm}: subsets= (task-file header carries no 'subsets' "
              f"map; re-run build_tasks_ws.py to emit it)")
        return
    kind_subset = subsets["map"]
    excluded = set(subsets.get("excluded_from_scored") or {})
    control = [g for g in gs if kind_subset.get(g["kind"]) == "control"]
    structural = [g for g in gs
                  if kind_subset.get(g["kind"]) == "structural"
                  and g["kind"] not in excluded]
    unmapped = sorted({g["kind"] for g in gs if g["kind"] not in kind_subset})
    cx = [g["cross_recall"] for g in control]
    sx = [g["cross_recall"] for g in structural]
    print(f"arm {arm}: control n={len(control)} "
          f"med_cross_recall={median(cx)} [B2/B3]")
    print(f"arm {arm}: structural(scored) n={len(structural)} "
          f"mean_cross_recall={mean(sx)} [B1] "
          f"med_cross_recall={median(sx)}")
    for kind in sorted({g["kind"] for g in gs}):
        ks = [g for g in gs if g["kind"] == kind]
        tag = kind_subset.get(kind, "UNMAPPED")
        if kind in excluded:
            tag += ",excluded-from-scored"
        print(f"arm {arm}:   shape {kind} ({tag}) n={len(ks)} "
              f"mean_cross_recall={mean([g['cross_recall'] for g in ks])} "
              f"med_cross_recall={median([g['cross_recall'] for g in ks])}")
    for kind in sorted(excluded):
        ks = [g for g in gs if g["kind"] == kind]
        print(f"arm {arm}: excluded {kind} n={len(ks)} "
              f"mean_cross_recall={mean([g['cross_recall'] for g in ks])} "
              f"med_cross_recall={median([g['cross_recall'] for g in ks])} "
              f"(reported, not scored)")
    if unmapped:
        print(f"arm {arm}: WARNING kinds absent from header subset map: "
              f"{unmapped}")


def merge_grades(out, grades):
    """Merge new grades into the archive, keyed by "key".

    grades.jsonl is the curated cross-campaign archive: a run scores only
    the tasks it covers, so a plain overwrite would drop every row from
    every other campaign. Rows carry a unique "key" (task_id + arm + rep),
    so re-grading a run replaces exactly its own rows and leaves the rest.
    """
    existing = {}
    if out.exists():
        for line in out.read_text().splitlines():
            if line:
                row = json.loads(line)
                existing[row["key"]] = row
    for g in grades:
        existing[g["key"]] = g
    out.write_text("".join(json.dumps(r) + "\n" for r in existing.values()))
    return existing


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--runs", default="results/runs.jsonl")
    ap.add_argument("--tasks", default=DEFAULT_TASKS,
                    help="task file whose header carries the subset map")
    args = ap.parse_args()
    bundle = load(args.tasks)

    runs = [json.loads(l) for l in
            (HERE / args.runs).read_text().splitlines() if l]
    grades = [g for g in (grade_run(r, bundle) for r in runs) if g]
    out = HERE / "results" / "grades.jsonl"
    merge_grades(out, grades)

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
        subset_report(arm, gs, bundle)
    print(f"subset map read from {bundle['path'].name} header: "
          f"{bundle['subsets'].get('map') or '(absent)'}")
    print(f"wrote {out} ({len(grades)} grades)")


if __name__ == "__main__":
    main()

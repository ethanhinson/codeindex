#!/usr/bin/env python3
"""Coverage measurement for the multi-hop recipes (recipes.py), against
NON-CIRCULAR ground truth (plain text scan of the repo, never the index).

where-tested: gt = code test-files containing the symbol as a word.
rename-radius: gt = ALL code files containing the symbol as a word (a rename
must touch every one). Both compared at the file level, per symbol, using the
caller-task symbols from a harness task set (real symbols, 5-30 callers).

Usage:
  .venv/bin/python measure_recipes.py --tasks ../agent_ab/tasks/tasks_v6.json \
      --repos-root ../repos --binary /path/to/codeindex
"""
from __future__ import annotations
import argparse, json, re, sys
from pathlib import Path

from recipes import where_tested, rename_radius, is_test_file

CODE_EXT = (".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".php")
SKIP_DIRS = {".git", "node_modules", "vendor", ".codeindex", "dist", "build"}


def scan_repo(root: Path, symbols: list[str]) -> dict[str, set[str]]:
    """One pass over code files -> {symbol: set(relative files containing it
    as a word)}. Plain text truth, independent of the index."""
    pats = {s: re.compile(r"\b" + re.escape(s) + r"\b") for s in symbols}
    hits = {s: set() for s in symbols}
    for p in root.rglob("*"):
        if not p.is_file() or p.suffix not in CODE_EXT:
            continue
        if any(part in SKIP_DIRS for part in p.parts):
            continue
        try:
            text = p.read_text(errors="ignore")
        except OSError:
            continue
        rel = str(p.relative_to(root))
        for s, pat in pats.items():
            if pat.search(text):
                hits[s].add(rel)
    return hits


def prf(claim: set, truth: set):
    if not truth:
        return (1.0, 1.0, 1.0) if not claim else (0.0, 1.0, 0.0)
    if not claim:
        return (0.0, 0.0, 0.0)
    tp = len(claim & truth)
    p, r = tp / len(claim), tp / len(truth)
    return (2 * p * r / (p + r) if p + r else 0.0, p, r)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--tasks", required=True)
    ap.add_argument("--repos-root", default="../repos")
    ap.add_argument("--binary", default="codeindex")
    args = ap.parse_args()

    here = Path(__file__).resolve().parent
    tasks = json.load(open(here / args.tasks))["tasks"]
    by_repo: dict[str, set[str]] = {}
    for t in tasks:
        sym = t.get("meta", {}).get("symbol") or t["id"].split("-")[-1]
        if t["type"] in ("occurrences", "caller_attribution", "edit_impact"):
            by_repo.setdefault(t["repo"], set()).add(sym)

    rows = []
    for repo, syms in sorted(by_repo.items()):
        rp = (here / args.repos_root / repo).resolve()
        truth_all = scan_repo(rp, sorted(syms))
        for sym in sorted(syms):
            wt = where_tested(args.binary, str(rp), sym)
            rr = rename_radius(args.binary, str(rp), sym)
            truth_files = truth_all[sym]
            truth_test = {f for f in truth_files if is_test_file(f)}
            f1_wt, p_wt, r_wt = prf(set(wt["test_files"]), truth_test)
            f1_rr, p_rr, r_rr = prf(set(rr["files_to_touch"]), truth_files)
            rows.append({"repo": repo, "symbol": sym,
                         "wt_f1": round(f1_wt, 3), "wt_p": round(p_wt, 3),
                         "wt_r": round(r_wt, 3), "n_test_truth": len(truth_test),
                         "rr_f1": round(f1_rr, 3), "rr_p": round(p_rr, 3),
                         "rr_r": round(r_rr, 3), "n_truth": len(truth_files),
                         "wt_missed": sorted(truth_test - set(wt["test_files"]))[:5],
                         "rr_missed": sorted(truth_files - set(rr["files_to_touch"]))[:5]})

    out = here / (Path(args.tasks).stem + "_recipes.jsonl")
    out.write_text("\n".join(json.dumps(r) for r in rows) + "\n")
    n = len(rows)
    for k, label in (("wt", "where-tested"), ("rr", "rename-radius")):
        f1 = sum(r[f"{k}_f1"] for r in rows) / n
        p = sum(r[f"{k}_p"] for r in rows) / n
        r_ = sum(r[f"{k}_r"] for r in rows) / n
        print(f"{label:<14} n={n}  F1={f1:.2f}  P={p:.2f}  R={r_:.2f}")
    print(f"rows -> {out.name}")


if __name__ == "__main__":
    main()

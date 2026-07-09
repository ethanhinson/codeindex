#!/usr/bin/env python3
"""Pool per-symbol efficacy rows from all results/efficacy-*.json files into one
distribution. Reports per-repo and overall median/min/max savings vs each
baseline, so the claim rests on the whole sample, not selected cases.

Usage: python3 aggregate_efficacy.py [results_dir]
"""

import glob
import json
import os
import statistics
import sys


def ratio(a, b):
    return (a / b) if b else None


def dist(xs):
    xs = sorted(x for x in xs if x is not None)
    if not xs:
        return {}

    def p(q):
        return round(xs[min(len(xs) - 1, int(q * (len(xs) - 1)))], 1)

    return {"n": len(xs), "min": round(xs[0], 1), "p25": p(0.25),
            "median": round(statistics.median(xs), 1), "p75": p(0.75),
            "max": round(xs[-1], 1)}


def main():
    d = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), "results")
    files = sorted(glob.glob(os.path.join(d, "efficacy-*.json")))
    # Skip aggregate output itself.
    files = [f for f in files if not f.endswith("efficacy-ALL.json")]

    all_rows, per_repo = [], {}
    for f in files:
        data = json.load(open(f))
        slug = data.get("slug", os.path.basename(f))
        # Only batch-format rows (v2) carry the grep-floor baseline.
        rows = [r for r in data.get("rows", []) if "grep_floor_tokens" in r]
        if not rows:
            continue
        per_repo.setdefault(slug, []).extend(rows)
        all_rows.extend(rows)

    def summarize(rows):
        floor = [ratio(r["grep_floor_tokens"], r["with_tokens"]) for r in rows]
        smart = [ratio(r["smart_files_tokens"], r["with_tokens"]) for r in rows]
        naive = [ratio(r["naive_files_tokens"], r["with_tokens"]) for r in rows]
        beats = sum(1 for r in rows if r["with_tokens"] < r["grep_floor_tokens"])
        with_toks = [r["with_tokens"] for r in rows]
        return {
            "n_symbols": len(rows),
            "vs_grep_floor": dist(floor),
            "vs_smart_files": dist(smart),
            "vs_naive_files": dist(naive),
            "pct_index_beats_grep_floor": round(100 * beats / len(rows), 1) if rows else 0,
            "with_answer_tokens": dist(with_toks),
        }

    out = {"overall": summarize(all_rows),
           "per_repo": {k: summarize(v) for k, v in per_repo.items()}}
    outpath = os.path.join(d, "efficacy-ALL.json")
    json.dump(out, open(outpath, "w"), indent=2)

    print(f"pooled from: {[os.path.basename(f) for f in files]}")
    print("\n================ OVERALL ================")
    o = out["overall"]
    print(f"  symbols: {o['n_symbols']}   (index answer median {o['with_answer_tokens']['median']} tokens)")
    print(f"  vs grep -n floor : median {o['vs_grep_floor']['median']}x  min {o['vs_grep_floor']['min']}x  max {o['vs_grep_floor']['max']}x")
    print(f"  vs smart file-read: median {o['vs_smart_files']['median']}x  min {o['vs_smart_files']['min']}x  max {o['vs_smart_files']['max']}x")
    print(f"  vs naive file-read: median {o['vs_naive_files']['median']}x  min {o['vs_naive_files']['min']}x")
    print(f"  index < grep -n output in {o['pct_index_beats_grep_floor']}% of symbols")
    for slug, s in out["per_repo"].items():
        print(f"\n  {slug}: {s['n_symbols']} symbols | "
              f"floor med {s['vs_grep_floor']['median']}x (min {s['vs_grep_floor']['min']}) | "
              f"smart med {s['vs_smart_files']['median']}x (min {s['vs_smart_files']['min']})")
    print(f"\nwrote {outpath}")


if __name__ == "__main__":
    main()

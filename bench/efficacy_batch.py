#!/usr/bin/env python3
"""Batch efficacy test — token cost of navigating real GitHub issues, with vs
without codeindex, across many issues and repos. Built to be hard to dispute:

  * No cherry-picking: a transparent, scripted rule selects the code symbols each
    issue references; every selected symbol is reported, including bad cases.
  * A skeptic-proof baseline: alongside "read files", we measure the CHEAPEST
    possible way to locate a symbol without the index — the raw `grep -n` output.
    If the index answer costs fewer tokens than even grep's output (while being
    resolved and structured), there is little left to argue.
  * Full distribution: we report median / p25 / p75 / min / max and the % of
    symbols where the index beats the grep floor — not just favorable maxima.

Baselines per referenced symbol (to learn "where is X defined + who calls it"):
  with        codeindex query answer (definitions + resolved callers, compact)
  grep_floor  tokens of `rg -n -w X` output — cheapest locate, unresolved/noisy
  smart_files tokens of the top-K files by match count, read whole
  naive_files tokens of all matching files (capped), read whole

Token counting uses Claude's exact tokenizer when bench/.env has a key.

Usage:
  python3 efficacy_batch.py --binary <codeindex> --repo <clone> \
      --slug prometheus/prometheus --issues 11505,16525,11834,14398 \
      --smart-k 8 --naive-cap 30 --out results/efficacy-prometheus.json
  # or auto-pull recent issues:
  python3 efficacy_batch.py ... --recent 25
"""

from __future__ import annotations

import argparse
import json
import re
import statistics
import sys
import urllib.request
from pathlib import Path

import token_bench as tb

# CamelCase identifiers with at least one lowercase (Labels, HasDuplicateLabelNames)
# — excludes ALLCAPS acronyms (URL, DRA, HTTP) and all-lowercase prose.
CAMEL = re.compile(r"\b([A-Z][a-z][A-Za-z0-9]{2,})\b")
BACKTICK = re.compile(r"`([A-Za-z_][A-Za-z0-9_]{2,})`")
# Common CamelCase English/proper words that are not code symbols.
STOP = {
    "This", "That", "The", "When", "Then", "After", "Before", "Give", "Write",
    "Reuse", "Migrate", "Remove", "Reset", "Proposal", "Prometheus", "Kubernetes",
    "Kubelet", "Currently", "Note", "Some", "There", "With", "Should", "Would",
    "Setup", "Also", "Maybe", "Please", "About", "Migrate", "Support",
}


def api_get(url: str):
    req = urllib.request.Request(url, headers={"Accept": "application/vnd.github+json"})
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read())


def fetch_issue(slug: str, num: str):
    d = api_get(f"https://api.github.com/repos/{slug}/issues/{num}")
    return {"number": d["number"], "title": d.get("title", ""), "body": d.get("body") or ""}


def fetch_recent(slug: str, n: int):
    items = api_get(f"https://api.github.com/repos/{slug}/issues?state=all&per_page={n}")
    return [{"number": d["number"], "title": d.get("title", ""), "body": d.get("body") or ""}
            for d in items if "pull_request" not in d]


def extract_candidates(text: str) -> list[str]:
    cands = set()
    cands.update(m.group(1) for m in CAMEL.finditer(text))
    cands.update(m.group(1) for m in BACKTICK.finditer(text))
    return sorted(c for c in cands if c not in STOP)


def query(binary: str, repo: str, sym: str) -> str:
    r = tb.run([binary, "query", repo, sym, "--limit", "1000"], check=False)
    return r.stdout or ""


def query_stats(qout: str):
    """Return (has_def, n_defs, n_callers) parsed from a query answer."""
    lines = qout.splitlines()
    n_defs = sum(1 for l in lines if l.startswith("def  ") and "(not found" not in l)
    n_callers = 0
    m = re.search(r"callers \((\d+)\)", qout)
    if m:
        n_callers = int(m.group(1))
    return n_defs > 0, n_defs, n_callers


def grep_floor(repo: str, sym: str) -> str:
    return "\n".join(tb.rg_lines(["-w", "-F", sym], repo))


def files_mentioning(repo: str, sym: str):
    out = []
    for ln in tb.rg_lines(["-w", "-F", "--count-matches", sym], repo):
        i = ln.rfind(":")
        if i >= 0 and ln[i + 1:].isdigit():
            out.append((ln[:i], int(ln[i + 1:])))
    out.sort(key=lambda x: -x[1])
    return out


def dist(xs: list[float]) -> dict:
    xs = sorted(x for x in xs if x is not None)
    if not xs:
        return {}
    def p(q):
        return round(xs[min(len(xs) - 1, int(q * (len(xs) - 1)))], 1)
    return {"n": len(xs), "min": round(xs[0], 1), "p25": p(0.25),
            "median": round(statistics.median(xs), 1), "p75": p(0.75),
            "max": round(xs[-1], 1)}


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--binary", required=True)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--slug", required=True, help="owner/repo")
    ap.add_argument("--issues", default="", help="comma-separated issue numbers")
    ap.add_argument("--recent", type=int, default=0, help="or auto-pull N recent issues")
    ap.add_argument("--smart-k", type=int, default=8)
    ap.add_argument("--naive-cap", type=int, default=30)
    ap.add_argument("--max-symbols-per-issue", type=int, default=8)
    ap.add_argument("--rg", default=None)
    ap.add_argument("--fast-tokens", action="store_true",
                    help="force tiktoken (ratios are tokenizer-robust; fast on huge repos)")
    ap.add_argument("--out", default=str(Path(__file__).parent / "results" / "efficacy-batch.json"))
    args = ap.parse_args()

    tb.RG = tb.resolve_rg(args.rg)
    if args.fast_tokens:
        tb._tiktoken()
        tb._COUNTER_NAME = "tiktoken-cl100k_base"
    counter = tb._init_counter()
    print(f"ripgrep: {tb.RG}\ntoken counter: {counter}")

    tb.run([args.binary, "build", args.repo], check=False)  # ensure indexed

    if args.issues:
        issues = [fetch_issue(args.slug, n.strip()) for n in args.issues.split(",") if n.strip()]
    elif args.recent:
        issues = fetch_recent(args.slug, args.recent)
    else:
        sys.exit("pass --issues or --recent")

    tok_cache: dict[str, int] = {}
    rows = []
    for iss in issues:
        cands = extract_candidates(iss["title"] + "\n" + iss["body"])
        kept = 0
        for sym in cands:
            if kept >= args.max_symbols_per_issue:
                break
            qout = query(args.binary, args.repo, sym)
            ok, n_defs, n_callers = query_stats(qout)
            if not ok:
                continue
            kept += 1
            files = files_mentioning(args.repo, sym)
            with_tok = tb.count_tokens(qout)
            floor_tok = tb.count_tokens(grep_floor(args.repo, sym))
            smart_tok = sum(tb.file_tokens(Path(args.repo), f, tok_cache) for f, _ in files[:args.smart_k])
            naive_tok = sum(tb.file_tokens(Path(args.repo), f, tok_cache) for f, _ in files[:args.naive_cap])
            rows.append({
                "issue": iss["number"], "symbol": sym, "n_defs": n_defs,
                "n_callers": n_callers, "n_files": len(files),
                "with_tokens": with_tok, "grep_floor_tokens": floor_tok,
                "smart_files_tokens": smart_tok, "naive_files_tokens": naive_tok,
            })
        print(f"  #{iss['number']}: {kept} indexed symbols  ({iss['title'][:56]})")

    ratio = lambda a, b: (a / b) if b else None
    floor_r = [ratio(r["grep_floor_tokens"], r["with_tokens"]) for r in rows]
    smart_r = [ratio(r["smart_files_tokens"], r["with_tokens"]) for r in rows]
    naive_r = [ratio(r["naive_files_tokens"], r["with_tokens"]) for r in rows]
    beats_floor = sum(1 for r in rows if r["with_tokens"] < r["grep_floor_tokens"])

    summary = {
        "slug": args.slug, "token_counter": counter, "n_symbols": len(rows),
        "n_issues": len(issues),
        "ratio_grep_floor": dist(floor_r),
        "ratio_smart_files": dist(smart_r),
        "ratio_naive_files": dist(naive_r),
        "pct_index_beats_grep_floor": round(100 * beats_floor / len(rows), 1) if rows else 0,
        "rows": rows,
    }
    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    Path(args.out).write_text(json.dumps(summary, indent=2))

    print(f"\n=== {args.slug}: {len(rows)} referenced symbols across {len(issues)} issues ===")
    print(f"  savings vs grep -n floor : median {dist(floor_r).get('median')}x  "
          f"(min {dist(floor_r).get('min')}x, max {dist(floor_r).get('max')}x)")
    print(f"  savings vs smart file-read: median {dist(smart_r).get('median')}x  "
          f"(min {dist(smart_r).get('min')}x, max {dist(smart_r).get('max')}x)")
    print(f"  savings vs naive file-read: median {dist(naive_r).get('median')}x")
    print(f"  index answer < grep -n output in {summary['pct_index_beats_grep_floor']}% of symbols")
    print(f"\nwrote {args.out}")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""End-to-end efficacy test: tokens to navigate a real GitHub issue, with vs
without the codeindex plugin.

Premise: to work an issue, an agent must first *understand* the code it touches —
where symbols are defined and who calls them. This harness takes a real issue,
extracts the code symbols it references, and measures the tokens for that
understanding two ways:

  WITHOUT (grep + read): grep each symbol, read the matching files into context.
    naive = read every file mentioning it; smart = read the top-K by match count.
  WITH   (codeindex):    run `codeindex query <symbol>` and read its compact
    `path:line + signature` answer (definitions + callers).

Token counting reuses token_bench (Anthropic count_tokens when bench/.env has a
key, else tiktoken). This measures the navigation/comprehension phase the plugin
targets — not the whole solve (editing/reasoning tokens are unaffected).

Usage:
  python3 efficacy.py --binary <codeindex> --repo <clone> \
      --issue prometheus/prometheus#11505 [--symbols Labels,HasDuplicateLabelNames] \
      --smart-k 8 --out results/efficacy-11505.json
"""

from __future__ import annotations

import argparse
import json
import re
import sys
import urllib.request
from pathlib import Path

import token_bench as tb


def fetch_issue(owner: str, repo: str, num: str) -> tuple[str, str]:
    url = f"https://api.github.com/repos/{owner}/{repo}/issues/{num}"
    req = urllib.request.Request(url, headers={"Accept": "application/vnd.github+json"})
    with urllib.request.urlopen(req, timeout=30) as r:
        d = json.loads(r.read())
    return d.get("title", ""), d.get("body") or ""


# Identifier candidates: CamelCase / func-like tokens, and `backtick` code spans.
CAMEL = re.compile(r"\b([A-Z][A-Za-z0-9]{3,})\b")
FUNCY = re.compile(r"\b([A-Za-z_][A-Za-z0-9_]{3,})\s*\(")
BACKTICK = re.compile(r"`([A-Za-z_][A-Za-z0-9_]{2,})`")


def extract_candidates(text: str) -> list[str]:
    cands = set()
    for pat in (CAMEL, FUNCY, BACKTICK):
        cands.update(m.group(1) for m in pat.finditer(text))
    return sorted(cands)


def query(binary: str, repo: str, sym: str, limit: int = 200) -> str:
    r = tb.run([binary, "query", repo, sym, "--limit", str(limit)], check=False)
    return r.stdout or ""


def has_def(query_out: str) -> bool:
    return "def  " in query_out and "(not found in index)" not in query_out.split("\n")[0]


def files_mentioning(repo: str, sym: str) -> list[tuple[str, int]]:
    lines = tb.rg_lines(["-w", "-F", "--count-matches", sym], repo)
    out = []
    for ln in lines:
        i = ln.rfind(":")
        if i >= 0 and ln[i + 1:].isdigit():
            out.append((ln[:i], int(ln[i + 1:])))
    out.sort(key=lambda x: -x[1])
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--binary", required=True, help="path to the codeindex binary")
    ap.add_argument("--repo", required=True, help="path to the (indexed) repo clone")
    ap.add_argument("--issue", required=True, help="owner/repo#number")
    ap.add_argument("--symbols", default="", help="comma-separated override; else auto-extract")
    ap.add_argument("--smart-k", type=int, default=8)
    ap.add_argument("--rg", default=None)
    ap.add_argument("--out", default=str(Path(__file__).parent / "results" / "efficacy.json"))
    args = ap.parse_args()

    tb.RG = tb.resolve_rg(args.rg)
    counter = tb._init_counter()
    print(f"ripgrep: {tb.RG}\ntoken counter: {counter}")

    m = re.match(r"([^/]+)/([^#]+)#(\d+)", args.issue)
    if not m:
        sys.exit("issue must look like owner/repo#number")
    owner, repo_name, num = m.groups()
    title, body = fetch_issue(owner, repo_name, num)
    print(f"\nIssue {args.issue}: {title}")

    if args.symbols:
        candidates = [s.strip() for s in args.symbols.split(",") if s.strip()]
    else:
        candidates = extract_candidates(title + "\n" + body)

    # Keep only symbols the index actually defines (so both sides are comparable).
    selected = []
    tb.run([args.binary, "build", args.repo], check=False)  # ensure indexed
    for sym in candidates:
        out = query(args.binary, args.repo, sym)
        if has_def(out):
            selected.append((sym, out))
    print(f"symbols referenced & indexed: {[s for s, _ in selected]}")

    tok_cache: dict[str, int] = {}
    rows = []
    tot_with = tot_naive = tot_smart = 0
    for sym, qout in selected:
        with_tok = tb.count_tokens(qout)
        files = files_mentioning(args.repo, sym)
        naive = sum(tb.file_tokens(Path(args.repo), f, tok_cache) for f, _ in files[:100])
        smart = sum(tb.file_tokens(Path(args.repo), f, tok_cache) for f, _ in files[:args.smart_k])
        rows.append({
            "symbol": sym, "with_tokens": with_tok,
            "without_naive_tokens": naive, "without_smart_tokens": smart,
            "n_files": len(files),
        })
        tot_with += with_tok
        tot_naive += naive
        tot_smart += smart
        print(f"  {sym:<28} with={with_tok:>5}  smart={smart:>7}  naive={naive:>7}  "
              f"({len(files)} files)")

    ratio = lambda a, b: round(a / b, 1) if b else None
    summary = {
        "issue": args.issue, "title": title,
        "token_counter": counter,
        "symbols": [s for s, _ in selected],
        "total_with_tokens": tot_with,
        "total_without_smart_tokens": tot_smart,
        "total_without_naive_tokens": tot_naive,
        "savings_smart_x": ratio(tot_smart, tot_with),
        "savings_naive_x": ratio(tot_naive, tot_with),
        "rows": rows,
    }
    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    Path(args.out).write_text(json.dumps(summary, indent=2))

    print(f"\n=== Efficacy: {args.issue} ===")
    print(f"  understanding {len(selected)} referenced symbols:")
    print(f"  WITH codeindex : {tot_with:>8} tokens")
    print(f"  WITHOUT (smart): {tot_smart:>8} tokens  -> {summary['savings_smart_x']}x more")
    print(f"  WITHOUT (naive): {tot_naive:>8} tokens  -> {summary['savings_naive_x']}x more")
    print(f"\nwrote {args.out}")


if __name__ == "__main__":
    main()

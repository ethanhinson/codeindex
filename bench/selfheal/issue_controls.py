#!/usr/bin/env python3
"""Controls for the issues-closed v2 fixtures.

Scores the SAME questions as curated_bench, but through two baselines:
  (a) find-control: `codeindex find <repo> "<title>" --limit 5`
  (b) grep-control: 2 most distinctive content words of the title (longest
      non-stopword tokens), `codeindex grep <repo> "<word>" --limit 15` each,
      attributed symbol names collected in output order, deduped, top 5.

Hit semantics are identical to curated_bench.matches (bare accept name
matches last component or any member of a type of that name).

Usage: python3 bench/selfheal/issue_controls.py
Writes bench/results/issues-v2-controls-<repo>.json
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
BENCH = HERE.parent
sys.path.insert(0, str(BENCH))
from curated_bench import matches  # noqa: E402

BINARY = "/tmp/codeindex-selfheal"

STOPWORDS = {
    "a", "an", "the", "and", "or", "not", "no", "is", "are", "was", "be",
    "been", "being", "will", "would", "should", "can", "cant", "cannot",
    "does", "doesnt", "do", "dont", "did", "didnt", "has", "have", "had",
    "in", "on", "of", "for", "to", "with", "without", "when", "while",
    "after", "before", "from", "into", "onto", "via", "by", "at", "as",
    "if", "it", "its", "this", "that", "these", "those", "there", "than",
    "then", "you", "your", "we", "my", "me", "using", "use", "used",
    "fix", "fixes", "fixed", "fixing", "bug", "bugs", "error", "errors",
    "issue", "issues", "problem", "problems", "wrong", "incorrect",
    "correctly", "properly", "still", "also", "only", "some", "all",
    "any", "new", "old", "more", "even", "very", "make", "makes", "made",
    "add", "adds", "added", "adding", "allow", "allows", "prevent",
    "support", "get", "set", "returns", "return", "returned",
}


def distinctive_words(title: str, k: int = 2) -> list[str]:
    """k longest non-stopword tokens, ties broken by position (stable)."""
    tokens = re.findall(r"[A-Za-z_][A-Za-z0-9_]+", title)
    seen, words = set(), []
    for t in tokens:
        low = t.lower()
        if low in STOPWORDS or len(t) < 3 or low in seen:
            continue
        seen.add(low)
        words.append(t)
    words.sort(key=lambda w: -len(w))  # stable: keeps title order on ties
    return words[:k]


def run(args: list[str]) -> str:
    r = subprocess.run(args, capture_output=True, text=True, timeout=600)
    return r.stdout


def find_names(repo: str, title: str, limit: int = 5) -> list[str]:
    out = run([BINARY, "find", repo, title, "--limit", str(limit)])
    names = []
    for line in out.splitlines()[1:]:
        line = line.strip()
        if not line or line.startswith("("):
            continue
        names.append(line.split()[0])
    return names[:limit]


def grep_names(repo: str, words: list[str], limit: int = 15) -> list[str]:
    """Attributed symbol names across the word greps, output order, deduped."""
    names = []
    for w in words:
        out = run([BINARY, "grep", repo, w, "--limit", str(limit)])
        for line in out.splitlines()[1:]:
            line = line.strip()
            if not line or line.startswith("<outside"):
                continue
            name = line.split()[0]
            if name not in names:
                names.append(name)
    return names[:5]


def score(repo_name: str):
    repo = str(BENCH / "repos" / repo_name)
    fixture = json.loads((HERE / f"issues_{repo_name}.json").read_text())
    rows, find_hits, grep_hits = [], 0, 0
    for case in fixture["questions"]:
        q, accept = case["q"], case["accept"]
        ftop = find_names(repo, q)
        fhit = any(matches(t, a) for t in ftop for a in accept)
        words = distinctive_words(q)
        gtop = grep_names(repo, words)
        ghit = any(matches(t, a) for t in gtop for a in accept)
        find_hits += fhit
        grep_hits += ghit
        rows.append({"q": q, "accept": accept,
                     "find": {"hit": fhit, "top": ftop},
                     "grep": {"hit": ghit, "words": words, "top": gtop}})
    n = len(rows)
    out = {
        "repo": repo_name,
        "fixture": f"bench/selfheal/issues_{repo_name}.json",
        "commit": fixture["commit"],
        "n": n,
        "find_control": {"hits": find_hits,
                         "hit_at_5": round(100 * find_hits / n, 1) if n else 0},
        "grep_control": {"hits": grep_hits,
                         "hit_at_5": round(100 * grep_hits / n, 1) if n else 0},
        "results": rows,
    }
    dest = BENCH / "results" / f"issues-v2-controls-{repo_name}.json"
    dest.write_text(json.dumps(out, indent=1) + "\n")
    print(f"{repo_name}: n={n} find-control hit@5 {find_hits}/{n} "
          f"({out['find_control']['hit_at_5']}%)  grep-control hit@5 "
          f"{grep_hits}/{n} ({out['grep_control']['hit_at_5']}%) -> {dest}")


if __name__ == "__main__":
    for r in ["gin", "flask"]:
        score(r)

#!/usr/bin/env python3
"""Offline recall benchmark for `codeindex find` (locate-and-enriched-grep).

Deterministic, free, pre-registered: sample real symbols, generate vague
queries per class (the ways an agent actually half-remembers a name), and
measure whether `find` puts the target in the top 5.

Pre-registered bar: hit@5 >= 70% on the vague classes (drop/synonym/reorder).
Failing THIS bar — and only this — triggers exploring an optional embeddings
tier as a separate change.

Usage:
  python3 recall_bench.py --binary <codeindex> --repo <clone> [--sample 60] [--seed 99]
"""

from __future__ import annotations

import argparse
import json
import random
import re
import sqlite3
import subprocess
from pathlib import Path

# mirror of the Go tokenizer, close enough for query GENERATION
def tokenize(name: str) -> list[str]:
    parts = re.sub(r"([a-z0-9])([A-Z])", r"\1 \2", name)
    parts = re.sub(r"([A-Z]+)([A-Z][a-z])", r"\1 \2", parts)
    parts = re.sub(r"[_\-.$]", " ", parts)
    parts = re.sub(r"(\d+)", r" \1 ", parts)
    return [t.lower() for t in parts.split() if t]

SYNS = {
    "get": "fetch", "fetch": "get", "load": "read", "read": "load",
    "set": "put", "create": "make", "new": "create", "make": "build",
    "delete": "remove", "remove": "delete", "config": "settings",
    "check": "validate", "validate": "check", "find": "search",
    "start": "run", "run": "start", "stop": "close", "close": "stop",
    "parse": "decode", "update": "modify", "send": "emit", "list": "all",
}

def gen_queries(name: str, rng: random.Random):
    toks = tokenize(name)
    out = [("casefold", name.lower())]
    if len(toks) >= 2:
        out.append(("token-join", " ".join(toks)))
        # drop one token
        drop = list(toks)
        drop.pop(rng.randrange(len(drop)))
        out.append(("token-drop", " ".join(drop)))
        # reorder
        rev = list(reversed(toks))
        if rev != toks:
            out.append(("reorder", " ".join(rev)))
        # synonym swap where possible
        for i, t in enumerate(toks):
            if t in SYNS:
                swapped = list(toks)
                swapped[i] = SYNS[t]
                out.append(("synonym", " ".join(swapped)))
                break
    return out

VAGUE_CLASSES = {"token-drop", "reorder", "synonym"}

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--binary", required=True)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--sample", type=int, default=60)
    ap.add_argument("--seed", type=int, default=99)
    ap.add_argument("--out", default=None)
    args = ap.parse_args()

    db = Path(args.repo) / ".codeindex" / "graph.db"
    con = sqlite3.connect(str(db))
    # sample multi-token, project-tier symbols with some usage (realistic targets)
    rows = con.execute("""
        SELECT s.name, COUNT(e.id) AS callers FROM symbols s
        LEFT JOIN edges e ON e.dst_symbol_id = s.id
        WHERE s.tier = 0 AND length(s.name) >= 6 AND s.file NOT LIKE '%_test%'
        GROUP BY s.name HAVING callers >= 2 ORDER BY s.name""").fetchall()
    con.close()
    multi = [(n, c) for n, c in rows if len(tokenize(n)) >= 2]
    rng = random.Random(args.seed)
    rng.shuffle(multi)
    targets = multi[: args.sample]

    stats = {}
    for name, _ in targets:
        for cls, q in gen_queries(name, rng):
            r = subprocess.run([args.binary, "find", args.repo, q, "--limit", "5"],
                               capture_output=True, text=True, timeout=120)
            # a hit = target name appears as a result symbol name (qualified ok)
            hit5, hit1 = False, False
            lines = [l for l in r.stdout.splitlines()[1:] if l.strip()]
            for rank, line in enumerate(lines[:5]):
                sym = line.split()[0].split(".")[-1]
                if sym == name:
                    hit5 = True
                    hit1 = hit5 and rank == 0
                    break
            s = stats.setdefault(cls, [0, 0, 0])
            s[0] += 1
            s[1] += 1 if hit5 else 0
            s[2] += 1 if hit1 else 0

    print(f"{'class':<12} {'n':>4} {'hit@5':>7} {'hit@1':>7}")
    vague_n = vague_hits = 0
    for cls, (n, h5, h1) in sorted(stats.items()):
        print(f"{cls:<12} {n:>4} {100*h5/n:>6.1f}% {100*h1/n:>6.1f}%")
        if cls in VAGUE_CLASSES:
            vague_n += n
            vague_hits += h5
    bar = 100 * vague_hits / vague_n if vague_n else 0
    verdict = "PASS" if bar >= 70 else "FAIL"
    print(f"\nVAGUE-CLASS hit@5: {bar:.1f}%  (bar: >=70%)  -> {verdict}")
    if args.out:
        Path(args.out).write_text(json.dumps(
            {"stats": stats, "vague_hit5_pct": round(bar, 1), "verdict": verdict,
             "seed": args.seed, "sample": args.sample}, indent=1))


if __name__ == "__main__":
    main()

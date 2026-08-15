#!/usr/bin/env python3
"""Query-formulation gates: is a model needed to turn a VAGUE INTENT into the
right symbol? Run the two cheap gates from NEXT_STEPS step 9 against the
human-curated concept sets (bench/concept_sets/<repo>.json — q = real intent,
accept = any-of-N symbols, same matches() semantics as curated_bench.py):

  Gate A — content-word extraction + `codeindex find` fuzzy matching, hit@5.
  Gate B — bge-base embedding similarity of the query vs tokenized symbol
           names straight out of graph.db, hit@5. No doc context, no graph —
           the cheapest possible semantic lane.

Reference row: `codeindex search` (the shipped hybrid) recorded in
bench/engine/FINDINGS-semantic-search.md. If a gate clears ~80%, distilling a
query-formulation model is dead. If both fail but `search` passes, the answer
is the already-shipped embedding lane — still not a distilled LLM.

Usage:
  .venv/bin/python measure_gates.py --binary <codeindex> [--repos gin,nest,...]
"""
from __future__ import annotations
import argparse, json, re, sqlite3, subprocess
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
SETS = HERE.parent / "concept_sets"
REPOS = HERE.parent / "repos"

STOP = set("""a an the of in on at to for from with by into over under and or
is are was be been do does did how what where which who why when this that
these those it its their his her your my our som all any each every code
repository repo file files function functions method methods class classes
""".split())


def matches(qname: str, accept: str) -> bool:
    if "." in accept:
        return qname == accept
    parts = qname.split(".")
    return parts[-1] == accept or (len(parts) == 2 and parts[0] == accept)


def content_words(q: str) -> str:
    return " ".join(w for w in re.findall(r"[a-zA-Z]+", q.lower())
                    if w not in STOP)


def gate_a_find(binary, repo, q):
    out = subprocess.run(
        [binary, "find", str(repo), content_words(q), "--limit", "5", "--json"],
        capture_output=True, text=True).stdout
    try:
        return [r["qname"] for r in json.loads(out).get("results", [])]
    except json.JSONDecodeError:
        return []


def camel_tokens(name: str) -> str:
    toks = re.findall(r"[A-Z]+(?=[A-Z][a-z])|[A-Z]?[a-z]+|[A-Z]+|\d+",
                      name.replace("_", " ").replace(".", " "))
    return " ".join(t.lower() for t in toks)


def load_symbols(repo: Path):
    con = sqlite3.connect(str(repo / ".codeindex" / "graph.db"))
    rows = con.execute(
        "SELECT name, parent FROM symbols WHERE tier=0 "
        "AND kind IN ('func','method','class')").fetchall()
    con.close()
    qnames = [f"{p}.{n}" if p else n for n, p in rows]
    return qnames, [camel_tokens(qn) for qn in qnames]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--binary", required=True)
    ap.add_argument("--repos", default="gin,flask,nest,laravel-framework")
    args = ap.parse_args()

    from clf_baseline import embed  # bge-base, local MPS

    print(f"{'repo':<18} {'n':>3} {'gateA find@5':>13} {'gateB bge@5':>12}")
    for name in args.repos.split(","):
        fixture = json.loads((SETS / f"{name}.json").read_text())
        repo = (REPOS / name).resolve()
        cases = fixture["questions"]

        a_hits = 0
        for c in cases:
            top = gate_a_find(args.binary, repo, c["q"])
            a_hits += any(matches(t, a) for t in top for a in c["accept"])

        qnames, tokenized = load_symbols(repo)
        sym_vecs = embed(tokenized)
        q_vecs = embed([c["q"] for c in cases])
        sims = q_vecs @ sym_vecs.T
        b_hits = 0
        for i, c in enumerate(cases):
            top = [qnames[j] for j in np.argsort(-sims[i])[:5]]
            b_hits += any(matches(t, a) for t in top for a in c["accept"])

        n = len(cases)
        print(f"{name:<18} {n:>3} {100*a_hits/n:>12.1f}% {100*b_hits/n:>11.1f}%")


if __name__ == "__main__":
    main()

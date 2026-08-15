#!/usr/bin/env python3
"""Offline recall benchmark for `codeindex find` (locate-and-enriched-grep).

Deterministic, free, pre-registered: sample real symbols, generate vague
queries per class (the ways an agent actually half-remembers a name), and
measure whether `find` puts the target in the top 5.

Pre-registered bar: hit@5 >= 70% on the vague classes (drop/synonym/reorder).
Failing THIS bar — and only this — triggers exploring an optional embeddings
tier as a separate change.

--concept mode (semantic-code-search change, pre-registered 2026-07-11):
CONCEPT class = the target's doc comment with EVERY name-derived token
stripped — a description of the symbol that never names it. Measured against
`search --flat` (hybrid) with `find` as the lexical control. Bars, registered
before first measurement:
  - concept hit@5 >= 60% for `search` on documented symbols
  - `find` classes above must not regress (run both modes on a bump)
  - full-build embed overhead within budget (reported per run; ceiling
    2 min added at kubernetes scale remains the registered target)
No hints are supplied (clients add them; this is the conservative floor).

Usage:
  python3 recall_bench.py --binary <codeindex> --repo <clone> [--sample 60] [--seed 99]
  python3 recall_bench.py --binary <codeindex> --repo <clone> --concept
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

COMMENT_PREFIXES = ("//", "#", "*", "/*", "/**")

def doc_comment(repo: Path, file: str, start_line: int) -> str:
    """Contiguous comment block above the definition (mirror of the Go
    extractor in internal/engine/embedpass.go)."""
    try:
        lines = (repo / file).read_text(errors="replace").splitlines()
    except OSError:
        return ""
    def is_comment(t: str) -> bool:
        return (t.startswith("//") or (t.startswith("#") and not t.startswith("#!"))
                or t.startswith("*") or t.startswith("/*"))

    def strip_marker(t: str) -> str:
        for p in ("/**", "/*", "//", "#", "*"):
            if t.startswith(p):
                t = t[len(p):]
                break
        return t.strip().removesuffix("*/").strip()

    # Find the top of the comment block, then read the description top-down,
    # stopping at the first @tag (mirror of internal/engine/embedpass.go).
    end = start_line - 2
    first = -1
    i = end
    while i >= 0 and i > end - 40:
        t = lines[i].strip()
        if not is_comment(t):
            break
        first = i
        if t.startswith("/*"):
            break
        i -= 1
    parts = []
    if first >= 0:
        for i in range(first, end + 1):
            body = strip_marker(lines[i].strip())
            if body.startswith("@"):
                break
            if body:
                parts.append(body)
            if len(parts) >= 4:
                break
    doc = " ".join(parts)[:240]
    if not doc and file.endswith(".py"):
        doc = py_docstring(lines, start_line)
    return doc

def py_docstring(lines: list[str], start_line: int) -> str:
    """Mirror of pyDocstring in internal/engine/embedpass.go."""
    for i in range(start_line, min(start_line + 3, len(lines))):
        t = lines[i].strip()
        for q in ('"""', "'''"):
            if not t.startswith(q):
                continue
            body = t[len(q):]
            if q in body:
                return body[:body.index(q)].strip()
            parts = [body.strip()]
            for k in range(i + 1, len(lines)):
                if len(parts) >= 4:
                    break
                t2 = lines[k].strip()
                if q in t2:
                    parts.append(t2[:t2.index(q)].strip())
                    break
                parts.append(t2)
            return " ".join(p for p in parts if p)[:240]
    return ""

# Small closed-class list for the measurability guard — enough to detect
# stopword-soup queries, deliberately not a full NLP stopword inventory.
STOPWORDS = {
    "the", "a", "an", "of", "for", "and", "or", "to", "in", "on", "at", "by",
    "with", "from", "as", "is", "are", "was", "be", "been", "this", "that",
    "these", "those", "it", "its", "if", "not", "no", "all", "any", "can",
    "will", "should", "may", "when", "which", "who", "what", "how", "into",
    "out", "up", "down", "over", "under", "than", "then", "them", "they",
    "there", "here", "also", "such", "some", "same", "other", "given",
    "returns", "return", "gets", "sets", "value", "values", "instance",
    "new", "given", "specified", "using", "used", "use",
}

def informative(word: str) -> bool:
    return len(word) >= 3 and word.lower() not in STOPWORDS

def concept_query(name: str, doc: str) -> str | None:
    """The doc phrase with every name-derived token removed — a concept
    description that never names its target. None if too little remains."""
    name_toks = set(tokenize(name))
    words = [w for w in re.findall(r"[A-Za-z]+", doc)
             if w.lower() not in name_toks and not set(tokenize(w)) & name_toks]
    if len(words) < 3:
        return None
    return " ".join(words[:10]).lower()

def concept_query_guarded(name: str, doc: str) -> tuple[str | None, bool]:
    """(query, had_candidate): measurability guard — a case is emitted only
    if the residual keeps >=2 informative content words. had_candidate is
    True when there was a doc at all (denominator for the discard rate)."""
    q = concept_query(name, doc)
    if q is None:
        return None, bool(doc.strip())
    if sum(1 for w in q.split() if informative(w)) < 2:
        return None, True
    return q, True

def run_concept(args):
    repo = Path(args.repo)
    db = repo / ".codeindex" / "graph.db"
    con = sqlite3.connect(str(db))
    rows = con.execute("""
        SELECT s.name, s.file, s.start_line, COUNT(e.id) AS callers FROM symbols s
        LEFT JOIN edges e ON e.dst_symbol_id = s.id
        WHERE s.tier = 0 AND length(s.name) >= 6 AND s.file NOT LIKE '%_test%'
        GROUP BY s.name, s.file, s.start_line HAVING callers >= 2
        ORDER BY s.name, s.file, s.start_line""").fetchall()
    con.close()

    rng = random.Random(args.seed)
    rng.shuffle(rows)
    cases = []
    candidates = emitted = 0
    for name, file, line, _ in rows:
        q, had = concept_query_guarded(name, doc_comment(repo, file, line))
        candidates += 1 if had else 0
        if q:
            emitted += 1
            cases.append((name, q))
        if len(cases) >= args.sample:
            break
    discard_pct = 100 * (candidates - emitted) / candidates if candidates else 0.0
    valid = candidates > 0 and emitted >= candidates * 0.5
    print(f"measurability guard: {emitted}/{candidates} candidates admitted "
          f"(discard {discard_pct:.0f}%) -> mechanical class "
          f"{'VALID' if valid else 'NOT VALID'} for this repo")
    if not cases:
        print("no measurable documented symbols — cannot run concept class")
        return

    def hit5(cmd_out: str, name: str) -> bool:
        for line in cmd_out.splitlines()[1:6]:
            if not line.strip():
                continue
            sym = line.split()[0].split(".")[-1]
            if sym == name:
                return True
        return False

    n = s_hits = f_hits = 0
    for name, q in cases:
        rs = subprocess.run([args.binary, "search", args.repo, q, "--flat", "--limit", "5"],
                            capture_output=True, text=True, timeout=300)
        rf = subprocess.run([args.binary, "find", args.repo, q, "--limit", "5"],
                            capture_output=True, text=True, timeout=120)
        n += 1
        s_hits += 1 if hit5(rs.stdout, name) else 0
        f_hits += 1 if hit5(rf.stdout, name) else 0

    s_pct, f_pct = 100 * s_hits / n, 100 * f_hits / n
    verdict = "PASS" if s_pct >= 60 else "FAIL"
    print(f"CONCEPT class (doc-phrase, name tokens stripped), n={n}")
    print(f"  search (hybrid) hit@5: {s_pct:.1f}%   (bar: >=60%)  -> {verdict}")
    print(f"  find (lexical control) hit@5: {f_pct:.1f}%   (delta: {s_pct - f_pct:+.1f} pts)")
    if args.out:
        Path(args.out).write_text(json.dumps(
            {"class": "concept", "n": n, "search_hit5_pct": round(s_pct, 1),
             "find_hit5_pct": round(f_pct, 1), "verdict": verdict,
             "seed": args.seed, "sample": args.sample}, indent=1))

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--binary", required=True)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--sample", type=int, default=60)
    ap.add_argument("--seed", type=int, default=99)
    ap.add_argument("--out", default=None)
    ap.add_argument("--concept", action="store_true",
                    help="run the concept (doc-phrase) class against `search`")
    args = ap.parse_args()

    if args.concept:
        run_concept(args)
        return

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

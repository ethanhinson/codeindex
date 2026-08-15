#!/usr/bin/env python3
"""Evidence collector for STEP 3 miss analysis of the issues-v2 fixtures.

For every search miss it gathers, per question:
  - ranks of accept symbols in search --limit 15 (bucket D signal)
  - string literals containing distinctive title words within/adjacent
    (+/-5 lines) to the accept symbols' current spans (bucket B signal)
  - whether any accept bare name appears in the fix commit's diff text
    (bucket A signal when absent) and the diff's changed-function context
  - lexical overlap between title words and accept names (bucket C signal)

Output: bench/selfheal/.miss_evidence.json (working artifact reviewed by
hand to produce issues_miss_analysis.md).
"""

from __future__ import annotations

import json
import re
import sqlite3
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
BENCH = HERE.parent
sys.path.insert(0, str(BENCH))
from curated_bench import matches, result_names  # noqa: E402
from issue_controls import distinctive_words     # noqa: E402

BINARY = "/tmp/codeindex-selfheal"


def git(repo, *args):
    return subprocess.run(["git", "-C", str(repo), *args],
                          capture_output=True, text=True).stdout


def accept_spans(db, accept):
    con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    spans = []
    for a in accept:
        if "." in a:
            parent, name = a.split(".", 1)
            q = ("SELECT file,start_line,end_line FROM symbols "
                 "WHERE tier=0 AND name=? AND parent=?", (name, parent))
        else:
            q = ("SELECT file,start_line,end_line FROM symbols "
                 "WHERE tier=0 AND (name=? OR parent=?)", (a, a))
        spans += [(f, s, e) for f, s, e in con.execute(*q)]
    con.close()
    return spans


LIT_RE = re.compile(r'"((?:[^"\\]|\\.)*)"|\'((?:[^\'\\]|\\.)*)\'|`([^`]*)`')


def literals_near(repo: Path, spans, words):
    """(file:line, literal, word) where a distinctive word occurs inside a
    string literal within span +/- 5 lines."""
    found = []
    lw = [w.lower() for w in words]
    for file, s, e in spans:
        p = repo / file
        if not p.exists() or s is None:
            continue
        lines = p.read_text(errors="replace").splitlines()
        lo, hi = max(1, s - 5), min(len(lines), (e or s) + 5)
        for i in range(lo, hi + 1):
            for m in LIT_RE.finditer(lines[i - 1]):
                lit = next(g for g in m.groups() if g is not None)
                for w in lw:
                    if w in lit.lower():
                        found.append((f"{file}:{i}", lit[:90], w))
    # dedupe on literal text
    seen, out = set(), []
    for loc, lit, w in found:
        if lit not in seen:
            seen.add(lit)
            out.append({"loc": loc, "literal": lit, "word": w})
    return out


def main():
    report = {}
    for rname in ["gin", "flask"]:
        repo = BENCH / "repos" / rname
        db = repo / ".codeindex" / "graph.db"
        res = json.loads((BENCH / "results" / f"issues-v2-{rname}.json").read_text())
        meta = json.loads((HERE / f"issues_{rname}.meta.json").read_text())
        fixture = json.loads((HERE / f"issues_{rname}.json").read_text())
        assert len(meta) == len(fixture["questions"])
        sha_by_q = {q["q"]: m for q, m in zip(fixture["questions"], meta)}
        misses = []
        for case in res["results"]:
            if case["hit"]:
                continue
            q, accept = case["q"], case["accept"]
            m = sha_by_q[q]
            # D signal: rank in top-15
            r = subprocess.run([BINARY, "search", str(repo), q, "--flat",
                                "--limit", "15"],
                               capture_output=True, text=True, timeout=600)
            top15 = result_names(r.stdout)[:15]
            ranks = [i + 1 for i, t in enumerate(top15)
                     if any(matches(t, a) for a in accept)]
            # B signal
            words = distinctive_words(q, k=4)
            spans = accept_spans(db, accept)
            lits = literals_near(repo, spans, [w for w in words if len(w) >= 4])
            # A signal: accept names present in the fix diff?
            diff = git(repo, "diff", f"{m['sha']}^", m["sha"])
            if not diff:
                diff = git(repo, "show", m["sha"])
            bare = {a.split(".")[-1] for a in accept} | {a.split(".")[0] for a in accept}
            in_diff = sorted(b for b in bare if b and b in diff)
            # C signal: lexical overlap title-words vs accept names
            overlap = sorted({w for w in words for a in accept
                              if w.lower() in a.lower()})
            misses.append({
                "q": q, "accept": accept, "sha": m["sha"], "issue": m["issue"],
                "top5": case["top"], "accept_ranks_top15": ranks,
                "literals": lits, "accept_names_in_diff": in_diff,
                "title_overlap_with_accept": overlap,
            })
        report[rname] = misses
        print(f"{rname}: {len(misses)} misses collected")
    (HERE / ".miss_evidence.json").write_text(json.dumps(report, indent=1) + "\n")


if __name__ == "__main__":
    main()

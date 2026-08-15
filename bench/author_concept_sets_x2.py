#!/usr/bin/env python3
"""Author the x2 EXPANDED concept-set fixtures (expansion of the frozen curated sets).

Expanded set = every question from the frozen fixture bench/concept_sets/<repo>.json
(copied verbatim, never re-verified or edited) + new questions drafted from framework
documentation knowledge, verified the same way as the originals: direct symbol-table
existence lookups against the pinned index (SELECT ... FROM symbols WHERE tier=0 AND
name=? [AND parent=?]). NO search/retrieval runs are made — fixtures are frozen
before any measurement.

Output goes to NEW files bench/concept_sets/x2/<repo>.json; the original frozen
fixtures are never modified.

Usage:
  python3 author_concept_sets_x2.py --name gin --drafts concept_sets/x2/drafts_x2.json
Repo path defaults to bench/repos/<name>; split/commit are inherited from the
original frozen fixture (commit is cross-checked against git HEAD).
"""

from __future__ import annotations

import argparse
import json
import sqlite3
import subprocess
from datetime import date
from pathlib import Path

BENCH = Path(__file__).parent


def verify(repo: Path, accepts: list[str]) -> list[str]:
    """Keep accepts that exist in the symbol table (direct lookup only)."""
    con = sqlite3.connect(str(repo / ".codeindex" / "graph.db"))
    kept = []
    for a in accepts:
        if "." in a:
            parent, name = a.rsplit(".", 1)
            n = con.execute(
                "SELECT COUNT(*) FROM symbols WHERE tier=0 AND name=? AND parent=?",
                (name, parent)).fetchone()[0]
        else:
            n = con.execute(
                "SELECT COUNT(*) FROM symbols WHERE tier=0 AND name=?", (a,)).fetchone()[0]
        if n > 0:
            kept.append(a)
    con.close()
    return kept


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--name", required=True)
    ap.add_argument("--repo", help="defaults to bench/repos/<name>")
    ap.add_argument("--drafts", default=str(BENCH / "concept_sets" / "x2" / "drafts_x2.json"))
    args = ap.parse_args()

    repo = Path(args.repo) if args.repo else BENCH / "repos" / args.name
    original = json.loads((BENCH / "concept_sets" / f"{args.name}.json").read_text())
    drafts = json.loads(Path(args.drafts).read_text())[args.name]

    commit = subprocess.run(["git", "-C", str(repo), "rev-parse", "HEAD"],
                            capture_output=True, text=True).stdout.strip()
    if commit != original["commit"]:
        raise SystemExit(
            f"{args.name}: repo HEAD {commit} != frozen fixture commit {original['commit']}")

    seen = {q["q"] for q in original["questions"]}
    kept, dropped = [], []
    for d in drafts:
        if d["q"] in seen:
            raise SystemExit(f"{args.name}: draft duplicates a frozen question: {d['q']!r}")
        acc = verify(repo, d["accept"])
        if acc:
            kept.append({"q": d["q"], "accept": acc})
        else:
            dropped.append(d["q"])

    out = {
        "repo": original["repo"],
        "commit": original["commit"],
        "split": original["split"],
        "provenance": (
            original["provenance"]
            + " | expansion x2, drafted from framework documentation knowledge, "
            "verified by symbol-table lookup only, frozen before any measurement, "
            f"{date.today().isoformat()}. Original questions copied verbatim."
        ),
        "questions": original["questions"] + kept,
    }
    dst = BENCH / "concept_sets" / "x2" / f"{args.name}.json"
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.write_text(json.dumps(out, indent=1))
    print(f"{args.name}: {len(original['questions'])} original + {len(drafts)} drafted "
          f"-> kept {len(kept)} new, dropped {len(dropped)}, total {len(out['questions'])}")
    for q in dropped:
        print(f"  dropped: {q}")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Curated concept-set evaluation (concept-eval capability).

Runs a frozen any-of-N question fixture against `codeindex search` and
scores hit@5. Refuses fixtures whose pinned commit does not match the
indexed clone. Records binary identity and frozen parameters so a held-out
run is a durable, auditable artifact.

Usage:
  python3 curated_bench.py --binary <codeindex> --repo <clone> \
      --fixture concept_sets/<name>.json [--out results.json] \
      [--params '{"lambda":0.4}'] [--flags "--hints ..."]
"""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
from pathlib import Path


def repo_head(repo: str) -> str:
    return subprocess.run(["git", "-C", repo, "rev-parse", "HEAD"],
                          capture_output=True, text=True).stdout.strip()


def result_names(stdout: str) -> list[str]:
    """Qualified names of the top results from `search --flat` output."""
    names = []
    for line in stdout.splitlines()[1:]:
        line = line.strip()
        if not line:
            continue
        names.append(line.split()[0])
    return names


def matches(qname: str, accept: str) -> bool:
    """accept "Parent.Name" matches that exact qualified symbol; accept
    "Name" matches the bare symbol OR any member of a type named Name
    (a method of the accepted class answers a question about the class)."""
    if "." in accept:
        return qname == accept
    parts = qname.split(".")
    return parts[-1] == accept or (len(parts) == 2 and parts[0] == accept)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--binary", required=True)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--fixture", required=True)
    ap.add_argument("--out", default=None)
    ap.add_argument("--params", default=None,
                    help="frozen parameter JSON recorded with the run")
    ap.add_argument("--limit", type=int, default=5)
    ap.add_argument("--as-error-text", action="store_true",
                    help="also pass each question via --error-text (simulates "
                         "an agent routing symptom text explicitly)")
    args = ap.parse_args()

    fixture = json.loads(Path(args.fixture).read_text())
    head = repo_head(args.repo)
    if fixture["commit"] != head:
        raise SystemExit(
            f"fixture pin mismatch: fixture={fixture['commit']} repo={head} — refusing to run")

    binary_id = hashlib.sha256(Path(args.binary).read_bytes()).hexdigest()[:16]

    results, hits = [], 0
    for case in fixture["questions"]:
        cmd = [args.binary, "search", args.repo, case["q"], "--flat", "--limit", str(args.limit)]
        if args.as_error_text:
            cmd += ["--error-text", case["q"]]
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=600)
        top = result_names(r.stdout)[: args.limit]
        hit = any(matches(t, a) for t in top for a in case["accept"])
        hits += 1 if hit else 0
        results.append({"q": case["q"], "hit": hit, "top": top, "accept": case["accept"]})

    n = len(results)
    pct = 100 * hits / n if n else 0.0
    print(f"{fixture['repo']} [{fixture['split']}] curated any-of-N hit@5: "
          f"{hits}/{n} = {pct:.1f}%")

    if args.out:
        Path(args.out).write_text(json.dumps({
            "repo": fixture["repo"],
            "split": fixture["split"],
            "fixture_commit": fixture["commit"],
            "binary_sha256_16": binary_id,
            "params": json.loads(args.params) if args.params else None,
            "n": n, "hits": hits, "hit5_pct": round(pct, 1),
            "results": results,
        }, indent=1))


if __name__ == "__main__":
    main()

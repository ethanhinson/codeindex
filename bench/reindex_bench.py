#!/usr/bin/env python3
"""codeindex — re-index / incremental-update validation spike.

The token spike (`token_bench.py`) validated the *output* token savings but
could NOT touch the re-index path (no persistent index, no Merkle tree). This
spike tests the incremental-update assumptions that are measurable without the
engine:

  1. Change-detection walk cost — the lazy per-query re-check overhead.
     fast-path (stat only) vs full content hashing, on real repos. Validates
     whether the query-latency budget's re-check component is realistic.

  2. Edge blast-radius — when one file changes, how many graph edges resolve
     INTO the symbols it defines (name-based), i.e. how far a one-file change
     ripples. Tests the claim "incremental work is proportional to changed
     files": true only if edits hit rarely-referenced symbols.

  3. (optional) Real commit churn — files changed per commit from git history,
     so we know the typical changed-file count incremental update must handle.

NOT covered (engine-only): parse+patch throughput — re-parsing a changed file
with tree-sitter and writing SQLite. Measure that once the Go engine exists.

Usage:
  python3 reindex_bench.py --only gin,prometheus,kubernetes --work <dir> \
      --sample 30 --churn prometheus --out results/reindex.json
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import random
import statistics
import time
from pathlib import Path

import token_bench as tb  # reuse rg resolution, run(), extract_symbols(), LANG_DEFS


# --------------------------------------------------------------------------- #
# 1. Change-detection walk cost
# --------------------------------------------------------------------------- #


def list_files(repo: Path) -> list[str]:
    """The file set the indexer would walk (rg --files respects .gitignore)."""
    r = tb.run([tb.RG, "--files"], cwd=repo, check=False)
    return [l for l in r.stdout.splitlines() if l]


def walk_cost(repo: Path, hash_cap: int) -> dict:
    files = list_files(repo)

    # fast-path: stat every file (what the size+mtime shortcut costs).
    t0 = time.perf_counter()
    total_bytes = 0
    ok = 0
    for rel in files:
        try:
            st = os.stat(repo / rel)
            total_bytes += st.st_size
            ok += 1
        except OSError:
            pass
    t_stat = time.perf_counter() - t0

    # full content hashing (cold-build cost / worst-case re-check with no fast
    # path). Capped to bound runtime; rate is reported so it extrapolates.
    sample = files[:hash_cap]
    t1 = time.perf_counter()
    hashed_bytes = 0
    for rel in sample:
        try:
            with open(repo / rel, "rb") as fh:
                data = fh.read()
            hashlib.blake2b(data).digest()
            hashed_bytes += len(data)
        except OSError:
            pass
    t_hash = time.perf_counter() - t1

    stat_rate = ok / t_stat if t_stat else 0
    hash_mb_s = (hashed_bytes / 1e6) / t_hash if t_hash else 0
    # Extrapolate full-hash time to the whole tree from the measured rate.
    full_hash_est = (total_bytes / 1e6) / hash_mb_s if hash_mb_s else 0

    return {
        "n_files": ok,
        "total_mb": round(total_bytes / 1e6, 1),
        "stat_walk_ms": round(t_stat * 1000, 1),
        "stat_files_per_s": int(stat_rate),
        "hash_sampled_files": len(sample),
        "hash_mb_per_s": int(hash_mb_s),
        "full_hash_est_ms": round(full_hash_est * 1000, 1),
    }


# --------------------------------------------------------------------------- #
# 2. Edge blast-radius: inbound references per defined symbol
# --------------------------------------------------------------------------- #


def inbound_count(repo: Path, name: str) -> int:
    """Total occurrences of `name` across the repo ~= edges resolving to it."""
    r = tb.run([tb.RG, "-w", "-F", "--count-matches", name], cwd=repo, check=False)
    if r.returncode not in (0, 1):
        return 0
    total = 0
    for ln in r.stdout.splitlines():
        i = ln.rfind(":")
        if i >= 0 and ln[i + 1:].isdigit():
            total += int(ln[i + 1:])
    return total


def blast_radius(repo: Path, lang: str, sample: int, seed: int) -> dict:
    symbols = tb.extract_symbols(repo, lang)
    rng = random.Random(seed)
    unique = {}
    for s in symbols:
        if len(s.name) >= 4 and s.name not in unique:
            unique[s.name] = s
    pool = list(unique.values())
    rng.shuffle(pool)
    picked = pool[:sample]

    # Per-file local re-parse scope: how many symbols each file defines.
    by_file: dict[str, int] = {}
    for s in symbols:
        by_file[s.file] = by_file.get(s.file, 0) + 1

    counts = [inbound_count(repo, s.name) for s in picked]
    counts.sort()
    n = len(counts)

    def pct(p):
        return counts[min(n - 1, int(p * (n - 1)))] if n else 0

    defs_per_file = sorted(by_file.values())
    m = len(defs_per_file)
    return {
        "n_symbols_sampled": n,
        "inbound_median": pct(0.5),
        "inbound_p90": pct(0.9),
        "inbound_max": counts[-1] if counts else 0,
        "pct_hot_gt_100": round(100 * sum(1 for c in counts if c > 100) / n, 1) if n else 0,
        "pct_cold_le_10": round(100 * sum(1 for c in counts if c <= 10) / n, 1) if n else 0,
        "defs_per_file_median": defs_per_file[m // 2] if m else 0,
        "defs_per_file_p90": defs_per_file[min(m - 1, int(0.9 * (m - 1)))] if m else 0,
    }


# --------------------------------------------------------------------------- #
# 3. Real commit churn (needs history)
# --------------------------------------------------------------------------- #

CODE_EXT = (".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".php", ".cs")


def commit_churn(repo: Path, deepen: int) -> dict | None:
    # Deepen the shallow clone to get some history.
    tb.run(["git", "fetch", "-q", "--deepen", str(deepen)], cwd=repo, check=False)
    r = tb.run(["git", "log", f"-{deepen}", "--pretty=format:@", "--name-only"],
               cwd=repo, check=False)
    if r.returncode != 0 or not r.stdout:
        return None
    per_commit = []
    cur = 0
    for line in r.stdout.splitlines():
        if line == "@":
            per_commit.append(cur)
            cur = 0
        elif line.endswith(CODE_EXT):
            cur += 1
    per_commit = [c for c in per_commit if c > 0]
    if not per_commit:
        return None
    per_commit.sort()
    n = len(per_commit)
    return {
        "commits_analyzed": n,
        "files_per_commit_median": per_commit[n // 2],
        "files_per_commit_p90": per_commit[min(n - 1, int(0.9 * (n - 1)))],
        "files_per_commit_max": per_commit[-1],
    }


# --------------------------------------------------------------------------- #
# Main
# --------------------------------------------------------------------------- #


def bench_repo(spec, work, sample, seed, hash_cap, churn_repos, deepen):
    repo = tb.ensure_repo(spec, work)
    if not repo:
        return None
    print(f"\n=== {spec['name']} ({spec['lang']}, {spec.get('tier','?')}) ===")

    t = time.time()
    walk = walk_cost(repo, hash_cap)
    print(f"  walk: {walk['n_files']} files, {walk['total_mb']} MB | "
          f"stat {walk['stat_walk_ms']}ms | full-hash est {walk['full_hash_est_ms']}ms "
          f"({time.time()-t:.1f}s)")

    t = time.time()
    blast = blast_radius(repo, spec["lang"], sample, seed)
    print(f"  blast-radius: inbound median {blast['inbound_median']}, "
          f"p90 {blast['inbound_p90']}, max {blast['inbound_max']} | "
          f"hot(>100) {blast['pct_hot_gt_100']}% cold(<=10) {blast['pct_cold_le_10']}% "
          f"({time.time()-t:.1f}s)")

    churn = None
    if spec["name"] in churn_repos:
        churn = commit_churn(repo, deepen)
        if churn:
            print(f"  churn: median {churn['files_per_commit_median']} files/commit, "
                  f"p90 {churn['files_per_commit_p90']}, max {churn['files_per_commit_max']} "
                  f"({churn['commits_analyzed']} commits)")

    return {"repo": spec["name"], "lang": spec["lang"], "tier": spec.get("tier"),
            "walk_cost": walk, "blast_radius": blast, "commit_churn": churn}


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repos", default=str(Path(__file__).parent / "repos.json"))
    ap.add_argument("--only", default="")
    ap.add_argument("--sample", type=int, default=30)
    ap.add_argument("--seed", type=int, default=7)
    ap.add_argument("--hash-cap", type=int, default=20000)
    ap.add_argument("--churn", default="", help="comma-separated repos to analyze history for")
    ap.add_argument("--deepen", type=int, default=300)
    ap.add_argument("--work", default=None)
    ap.add_argument("--rg", default=None)
    ap.add_argument("--out", default=str(Path(__file__).parent / "results" / "reindex.json"))
    args = ap.parse_args()

    tb.RG = tb.resolve_rg(args.rg)
    print(f"ripgrep: {tb.RG}")

    cfg = json.loads(Path(args.repos).read_text())
    only = {x for x in args.only.split(",") if x}
    repos = [r for r in cfg["repos"] if not only or r["name"] in only]
    churn_repos = {x for x in args.churn.split(",") if x}

    work = Path(args.work) if args.work else Path(
        os.environ.get("TMPDIR", "/tmp")) / "codeindex-bench-repos"
    work.mkdir(parents=True, exist_ok=True)

    results = []
    for spec in repos:
        try:
            r = bench_repo(spec, work, args.sample, args.seed, args.hash_cap,
                           churn_repos, args.deepen)
            if r:
                results.append(r)
        except Exception as e:
            print(f"  ! error on {spec['name']}: {e}")

    out = {"results": results}
    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    Path(args.out).write_text(json.dumps(out, indent=2))
    print(f"\nwrote {args.out}")


if __name__ == "__main__":
    main()

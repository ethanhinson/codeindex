#!/usr/bin/env python3
"""Offline diversity re-ranking experiment for codeindex search output.

Re-ranks the top-40 candidates from `codeindex search --flat` using card
vectors read from the repo's index database (sqlite3, read-only). Never
modifies the search engine. Measures whether MMR, family round-robin, and
family-relative discount can fix "clone wall" misses without regressions.

Usage:
  python3 rerank_dryrun.py --binary B --repo R --fixture F --frozen FR \
      --out OUT.json [--limit40 40]
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sqlite3
import struct
import subprocess
import sys
from pathlib import Path


# ---------------------------------------------------------------------------
# Grading — exact copy of curated_bench.py semantics
# ---------------------------------------------------------------------------

def matches(qname: str, accept: str) -> bool:
    """accept "Parent.Name" matches that exact qualified symbol; accept
    "Name" matches the bare symbol OR any member of a type named Name
    (a method of the accepted class answers a question about the class)."""
    if "." in accept:
        return qname == accept
    parts = qname.split(".")
    return parts[-1] == accept or (len(parts) == 2 and parts[0] == accept)


def grade(top5: list[str], accept: list[str]) -> bool:
    return any(matches(t, a) for t in top5 for a in accept)


# ---------------------------------------------------------------------------
# Step 1 — candidate parsing
# ---------------------------------------------------------------------------

# Flat output header: "search "<q>" (N symbols, K clusters):"
# Each result line: "  <qname>  <kind>  <file>:<line>  [callers=N]  [sig...]  [<tag>]"
# Fields separated by 2+ spaces; lanes tag at end wrapped in [].
_LANE_RE = re.compile(r"\[([^\]]+)\]$")
_CALLER_RE = re.compile(r"^callers=\d+$")


def parse_flat_output(stdout: str) -> list[dict]:
    """Parse `--flat` output into a list of candidate dicts.

    Each dict has: qname, file (repo-relative), line (int), lanes (str).
    Order matches the engine's ranking (rank 0 = best).
    """
    candidates = []
    lines = stdout.splitlines()
    if not lines:
        return candidates
    # Skip header line (first non-empty line)
    for raw in lines[1:]:
        if not raw.startswith("  "):
            continue
        line = raw.strip()
        if not line:
            continue
        # Split on runs of 2+ spaces
        parts = re.split(r"  +", line)
        if len(parts) < 3:
            continue
        qname = parts[0]
        # kind = parts[1]  (not needed)
        loc = parts[2]  # file:line
        # Extract optional [lanes] tag — last field
        lanes = ""
        m = _LANE_RE.search(loc if len(parts) == 3 else parts[-1])
        if m:
            lanes = m.group(1)
        # Parse file:line from loc field
        if ":" not in loc:
            continue
        colon = loc.rfind(":")
        fpath = loc[:colon]
        try:
            lineno = int(loc[colon + 1:])
        except ValueError:
            continue
        candidates.append({
            "qname": qname,
            "file": fpath,
            "line": lineno,
            "lanes": lanes,
        })
    return candidates


def run_search(binary: str, repo: str, query: str, limit: int) -> list[dict]:
    """Run codeindex search and return parsed candidates (unset ROLE_BOOST)."""
    env = {k: v for k, v in __import__("os").environ.items()
           if k != "CODEINDEX_ROLE_BOOST"}
    env["CODEINDEX_ROLE_BOOST"] = "0"
    r = subprocess.run(
        [binary, "search", repo, query, "--flat", "--limit", str(limit)],
        capture_output=True, text=True, timeout=600, env=env,
    )
    return parse_flat_output(r.stdout)


# ---------------------------------------------------------------------------
# Step 2 — reproduction gate
# ---------------------------------------------------------------------------

def check_reproduction(
    per_question: list[dict],
    frozen: dict,
    questions: list[dict],
) -> None:
    """Hard gate: per-question hit booleans must match frozen artifact exactly.

    per_question is indexed by question order; frozen["results"] is a list
    keyed by "q" text. Exits with code 2 on any mismatch.
    """
    frozen_by_q = {r["q"]: r for r in frozen["results"]}
    mismatches = []
    for pq in per_question:
        q = pq["q"]
        if q not in frozen_by_q:
            continue
        fr = frozen_by_q[q]
        if pq["base_hit"] != fr["hit"]:
            mismatches.append({
                "q": q,
                "computed_hit": pq["base_hit"],
                "frozen_hit": fr["hit"],
                "computed_top5": pq["base_top5"],
                "frozen_top5": fr["top"],
            })
    if mismatches:
        print(
            f"\nREPRODUCTION GATE FAILED: {len(mismatches)} question(s) differ "
            "from curated-FROZEN artifact:\n",
            file=sys.stderr,
        )
        for m in mismatches:
            print(f"  Q: {m['q']!r}", file=sys.stderr)
            print(f"    computed hit={m['computed_hit']}  frozen hit={m['frozen_hit']}",
                  file=sys.stderr)
            print(f"    computed top5: {m['computed_top5']}", file=sys.stderr)
            print(f"    frozen  top5 : {m['frozen_top5']}", file=sys.stderr)
        sys.exit(2)
    print(f"[gate] reproduction OK — {len(per_question)} questions match frozen artifact")


# ---------------------------------------------------------------------------
# Step 3 — vector loading
# ---------------------------------------------------------------------------

def _l2_norm(v: list[float]) -> list[float]:
    import math
    n = math.sqrt(sum(x * x for x in v))
    if n == 0:
        return v
    return [x / n for x in v]


def load_vectors(
    db_path: str,
    candidates_by_query: list[list[dict]],
) -> tuple[dict[tuple[str, str, str], list[float]], str, int, int]:
    """Load and L2-normalize int8 vectors for all candidates.

    Returns (vec_map, model, total_candidates, vectorized_count).
    vec_map key = (name, parent, file_repo_relative).
    Symbols without a vector get an empty list [].
    """
    uri = f"file:{db_path}?mode=ro"
    conn = sqlite3.connect(uri, uri=True)
    cur = conn.cursor()

    # Discover model from vecmeta
    cur.execute("SELECT value FROM vecmeta WHERE key='model'")
    row = cur.fetchone()
    model = row[0] if row else None
    if not model:
        # Fall back to majority in vecs
        cur.execute("SELECT model, COUNT(*) AS c FROM vecs GROUP BY model ORDER BY c DESC LIMIT 1")
        row = cur.fetchone()
        model = row[0] if row else ""

    # Collect all (name, parent, file) combos needed
    needed: set[tuple[str, str, str]] = set()
    for cands in candidates_by_query:
        for c in cands:
            qname = c["qname"]
            parts = qname.split(".", 1)
            name = parts[-1]
            parent = parts[0] if len(parts) == 2 else ""
            needed.add((name, parent, c["file"]))

    # Batch lookup: join symbols -> symvec -> vecs
    # symbols view: name, parent, file columns; id links to symvec
    vec_map: dict[tuple[str, str, str], list[float]] = {}
    dim = None

    for (name, parent, fpath) in needed:
        cur.execute(
            """SELECT s.id
               FROM symbols s
               WHERE s.name=? AND s.parent=? AND s.file=?
               LIMIT 1""",
            (name, parent, fpath),
        )
        sym_row = cur.fetchone()
        if sym_row is None:
            # Try without parent restriction (some symbols may have no parent in db)
            cur.execute(
                """SELECT s.id FROM symbols s
                   WHERE s.name=? AND s.file=? LIMIT 1""",
                (name, fpath),
            )
            sym_row = cur.fetchone()
        if sym_row is None:
            vec_map[(name, parent, fpath)] = []
            continue
        symbol_id = sym_row[0]
        cur.execute(
            """SELECT v.vec FROM symvec sv
               JOIN vecs v ON v.hash=sv.hash AND v.model=?
               WHERE sv.symbol_id=?""",
            (model, symbol_id),
        )
        vec_row = cur.fetchone()
        if vec_row is None:
            vec_map[(name, parent, fpath)] = []
            continue
        blob = vec_row[0]
        if len(blob) % 4 != 0:
            # q8_0 = int8 per dimension; 384-byte blob = 384 int8 values
            vals = list(struct.unpack(f"{len(blob)}b", blob))
        else:
            # Could be float32 (4 bytes each) or int8 (1 byte each)
            # Heuristic: if blob length == expected float32 size and blob length >> 4,
            # check whether interpreting as float32 gives sane values.
            # We know from schema inspection: 384-byte blobs = int8 quantized
            vals = list(struct.unpack(f"{len(blob)}b", blob))
        if dim is None:
            dim = len(vals)
        vec_map[(name, parent, fpath)] = _l2_norm([float(x) for x in vals])

    conn.close()
    total = sum(len(cands) for cands in candidates_by_query)
    vectorized = sum(
        1
        for cands in candidates_by_query
        for c in cands
        if _cand_key(c) in vec_map and vec_map[_cand_key(c)]
    )
    return vec_map, model, total, vectorized


def _cand_key(c: dict) -> tuple[str, str, str]:
    qname = c["qname"]
    parts = qname.split(".", 1)
    name = parts[-1]
    parent = parts[0] if len(parts) == 2 else ""
    return (name, parent, c["file"])


def get_vec(c: dict, vec_map: dict) -> list[float]:
    return vec_map.get(_cand_key(c), [])


# ---------------------------------------------------------------------------
# Cosine similarity
# ---------------------------------------------------------------------------

def cosine(a: list[float], b: list[float]) -> float:
    if not a or not b:
        return 0.0
    return sum(x * y for x, y in zip(a, b))  # already L2-normalized


# ---------------------------------------------------------------------------
# Step 4 — re-rankers
# ---------------------------------------------------------------------------

def rerank_baseline(cands: list[dict], _vec_map: dict, **_kw) -> list[str]:
    return [c["qname"] for c in cands[:5]]


def rerank_mmr(
    cands: list[dict], vec_map: dict, lam: float
) -> list[str]:
    """Greedy MMR: lam * s_norm(i) - (1-lam) * max_cos(i, selected)."""
    n = len(cands)
    if n == 0:
        return []
    # rank-proxy scores: s_i = 1/(60+i), scaled to [0,1] within query
    raw = [1.0 / (60.0 + i) for i in range(n)]
    s_min, s_max = raw[-1], raw[0]
    span = s_max - s_min if s_max > s_min else 1.0
    snorm = [(s - s_min) / span for s in raw]

    vecs = [get_vec(c, vec_map) for c in cands]
    selected_idx: list[int] = []
    selected_vecs: list[list[float]] = []

    # First pick = original rank 0 (best)
    selected_idx.append(0)
    selected_vecs.append(vecs[0])
    remaining = list(range(1, n))

    while len(selected_idx) < 5 and remaining:
        best_score = -1e9
        best_i = remaining[0]
        for i in remaining:
            sim_to_selected = max(
                (cosine(vecs[i], sv) for sv in selected_vecs if sv),
                default=0.0,
            )
            score = lam * snorm[i] - (1 - lam) * sim_to_selected
            if score > best_score:
                best_score = score
                best_i = i
        selected_idx.append(best_i)
        selected_vecs.append(vecs[best_i])
        remaining.remove(best_i)

    return [cands[i]["qname"] for i in selected_idx]


def _build_families(cands: list[dict], vec_map: dict, tau: float) -> list[list[int]]:
    """Connected components of graph {(i,j): cos(i,j) >= tau} over candidates."""
    n = len(cands)
    vecs = [get_vec(c, vec_map) for c in cands]
    parent = list(range(n))

    def find(x: int) -> int:
        while parent[x] != x:
            parent[x] = parent[parent[x]]
            x = parent[x]
        return x

    def union(x: int, y: int) -> None:
        rx, ry = find(x), find(y)
        if rx != ry:
            parent[rx] = ry

    for i in range(n):
        for j in range(i + 1, n):
            if vecs[i] and vecs[j] and cosine(vecs[i], vecs[j]) >= tau:
                union(i, j)

    fam_map: dict[int, list[int]] = {}
    for i in range(n):
        root = find(i)
        fam_map.setdefault(root, []).append(i)

    # Sort families by their best (lowest) original rank
    families = list(fam_map.values())
    families.sort(key=lambda f: min(f))
    return families


def rerank_rr(cands: list[dict], vec_map: dict, tau: float) -> list[str]:
    """Round-robin across families ordered by best original rank."""
    families = _build_families(cands, vec_map, tau)
    # Per family: keep a pointer to iterate best-remaining member
    pointers = [list(sorted(f)) for f in families]  # sorted by original rank
    result: list[str] = []
    fam_idx = 0
    num_fam = len(pointers)
    # Emit round-robin until 5 collected
    while len(result) < 5:
        n_left = sum(len(p) for p in pointers)
        if n_left == 0:
            break
        for fi in range(num_fam):
            if pointers[fi]:
                idx = pointers[fi].pop(0)
                result.append(cands[idx]["qname"])
                if len(result) == 5:
                    break
    return result


def rerank_famz(
    cands: list[dict], vec_map: dict, tau: float, beta: float
) -> list[str]:
    """Family-relative discount: s'_i = s_i * (1 - beta * maxcos_higher)."""
    n = len(cands)
    if n == 0:
        return []
    raw = [1.0 / (60.0 + i) for i in range(n)]
    vecs = [get_vec(c, vec_map) for c in cands]

    # For each i, find max cosine to any same-family candidate with lower index
    families = _build_families(cands, vec_map, tau)
    fam_of = {}
    for fi, fam in enumerate(families):
        for idx in fam:
            fam_of[idx] = fi

    adj: list[float] = []
    for i in range(n):
        fam_i = fam_of.get(i, i)
        # candidates in same family with better (lower) original rank
        higher = [
            j for j in range(i)
            if fam_of.get(j, j) == fam_i and vecs[j]
        ]
        maxcos = max((cosine(vecs[i], vecs[j]) for j in higher), default=0.0)
        adj.append(maxcos)

    scored = [(raw[i] * (1.0 - beta * adj[i]), i) for i in range(n)]
    scored.sort(key=lambda x: -x[0])
    return [cands[i]["qname"] for _, i in scored[:5]]


# ---------------------------------------------------------------------------
# Step 5 — grade
# ---------------------------------------------------------------------------

def find_best_accept_rank(cands: list[dict], accept: list[str]) -> int | None:
    """0-based rank of first accepted answer in top-40, or None."""
    for i, c in enumerate(cands):
        if any(matches(c["qname"], a) for a in accept):
            return i
    return None


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def build_configs() -> list[tuple[str, callable]]:
    configs = [("baseline", lambda c, vm: rerank_baseline(c, vm))]
    for lam in [0.9, 0.8, 0.7, 0.6, 0.5]:
        lam_c = lam
        configs.append((f"mmr_{lam_c}", lambda c, vm, l=lam_c: rerank_mmr(c, vm, l)))
    for tau in [0.995, 0.99, 0.98, 0.97, 0.95]:
        tau_c = tau
        configs.append((f"rr_{tau_c}", lambda c, vm, t=tau_c: rerank_rr(c, vm, t)))
    for tau in [0.99, 0.97]:
        for beta in [0.5, 1.0]:
            tau_c, beta_c = tau, beta
            configs.append(
                (f"famz_{tau_c}_{beta_c}",
                 lambda c, vm, t=tau_c, b=beta_c: rerank_famz(c, vm, t, b))
            )
    return configs


def main() -> None:
    ap = argparse.ArgumentParser(
        description="Offline diversity re-ranking experiment for codeindex search."
    )
    ap.add_argument("--binary", required=True, help="codeindex binary path")
    ap.add_argument("--repo", required=True, help="cloned repo directory")
    ap.add_argument("--fixture", required=True, help="concept_sets/<name>.json")
    ap.add_argument("--frozen", required=True,
                    help="curated-FROZEN-<name>.json baseline artifact")
    ap.add_argument("--out", required=True, help="output artifact JSON path")
    ap.add_argument("--limit40", type=int, default=40,
                    help="candidate pool size (default 40)")
    args = ap.parse_args()

    fixture = json.loads(Path(args.fixture).read_text())
    frozen = json.loads(Path(args.frozen).read_text())
    binary_sha = hashlib.sha256(Path(args.binary).read_bytes()).hexdigest()[:16]
    db_path = str(Path(args.repo) / ".codeindex" / "graph.db")

    questions = fixture["questions"]
    n_q = len(questions)

    # Step 1 — gather candidates
    print(f"[step1] running {n_q} searches (limit={args.limit40}) …")
    all_cands: list[list[dict]] = []
    for qi, case in enumerate(questions):
        cands = run_search(args.binary, args.repo, case["q"], args.limit40)
        all_cands.append(cands)
        print(f"  [{qi+1:02d}/{n_q}] {len(cands):3d} candidates — {case['q'][:60]!r}")

    # Build per-question baseline top-5 and initial hit
    per_question: list[dict] = []
    for qi, case in enumerate(questions):
        cands = all_cands[qi]
        base_top5 = [c["qname"] for c in cands[:5]]
        base_hit = grade(base_top5, case["accept"])
        recall40 = any(
            any(matches(c["qname"], a) for a in case["accept"])
            for c in cands
        )
        best_rank = find_best_accept_rank(cands, case["accept"])
        per_question.append({
            "q": case["q"],
            "accept": case["accept"],
            "base_top5": base_top5,
            "base_hit": base_hit,
            "recall40": recall40,
            "best_accept_rank": best_rank,
        })

    # Step 2 — reproduction gate
    print("\n[step2] checking reproduction gate …")
    check_reproduction(per_question, frozen, questions)

    # Step 3 — load vectors
    print("\n[step3] loading vectors from db …")
    vec_map, model, total_cands, vec_count = load_vectors(db_path, all_cands)
    cov_pct = 100.0 * vec_count / total_cands if total_cands else 0.0
    print(f"  model: {model}")
    print(f"  coverage: {vec_count}/{total_cands} = {cov_pct:.1f}%")

    # Warn per-query if <90% vectorized
    for qi, cands in enumerate(all_cands):
        n_vec = sum(1 for c in cands if get_vec(c, vec_map))
        if cands and n_vec < 0.9 * len(cands):
            misses = [c["qname"] for c in cands if not get_vec(c, vec_map)]
            print(
                f"  WARNING query {qi+1} coverage {n_vec}/{len(cands)}: "
                f"missing vectors for {misses[:5]}"
            )

    # Step 4+5 — re-rank and grade all configs
    print("\n[step4+5] re-ranking and grading …")
    configs = build_configs()
    config_results: dict[str, dict] = {}

    baseline_hits = sum(1 for pq in per_question if pq["base_hit"])
    baseline_pct = 100.0 * baseline_hits / n_q if n_q else 0.0
    recall40_hits = sum(1 for pq in per_question if pq["recall40"])
    recall40_pct = 100.0 * recall40_hits / n_q if n_q else 0.0

    for cfg_name, cfg_fn in configs:
        hits = 0
        flips: list[dict] = []
        for qi, case in enumerate(questions):
            cands = all_cands[qi]
            pq = per_question[qi]
            new_top5 = cfg_fn(cands, vec_map)
            new_hit = grade(new_top5, case["accept"])
            hits += 1 if new_hit else 0
            if new_hit != pq["base_hit"]:
                direction = "gained" if new_hit else "lost"
                flips.append({
                    "q": case["q"],
                    "direction": direction,
                    "base_top5": pq["base_top5"],
                    "new_top5": new_top5,
                })
        pct = 100.0 * hits / n_q if n_q else 0.0
        config_results[cfg_name] = {
            "pct": round(pct, 1),
            "hits": hits,
            "flips": flips,
        }

    # Step 6 — artifact + stdout summary
    artifact = {
        "repo": fixture["repo"],
        "fixture_commit": fixture["commit"],
        "binary_sha256_16": binary_sha,
        "baseline_pct": round(baseline_pct, 1),
        "recall40_pct": round(recall40_pct, 1),
        "configs": config_results,
        "per_question": per_question,
        "limitations": [
            "rank-proxy scores (s_i = 1/(60+i)); true blend scores not exposed by --flat",
        ],
    }
    Path(args.out).write_text(json.dumps(artifact, indent=1))
    print(f"\n[done] artifact written to {args.out}")

    # Summary table
    gained_total = sum(
        len([f for f in v["flips"] if f["direction"] == "gained"])
        for v in config_results.values()
    )
    lost_total = sum(
        len([f for f in v["flips"] if f["direction"] == "lost"])
        for v in config_results.values()
    )

    print(f"\nrecall@40: {recall40_hits}/{n_q} = {recall40_pct:.1f}%")
    print(f"\n{'config':<22}  {'hit@5 %':>8}  {'hits':>4}  {'gained':>6}  {'lost':>6}")
    print("-" * 58)
    for cfg_name, res in config_results.items():
        gained = len([f for f in res["flips"] if f["direction"] == "gained"])
        lost = len([f for f in res["flips"] if f["direction"] == "lost"])
        marker = ""
        if res["hits"] > baseline_hits:
            marker = " +"
        elif res["hits"] < baseline_hits:
            marker = " -"
        print(
            f"{cfg_name:<22}  {res['pct']:>7.1f}%  {res['hits']:>4}  "
            f"{gained:>6}  {lost:>6}{marker}"
        )


if __name__ == "__main__":
    main()

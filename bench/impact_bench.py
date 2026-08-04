#!/usr/bin/env python3
"""codeindex — blast-radius accuracy benchmark.

Scores codeindex's impact/blast-radius answers against a hybrid ground-truth
oracle: recall (did it find every real dependent?) and precision (noise),
per-language and aggregate, with an [ambiguous]-flagged subset broken out.

Deterministic, seeded, pre-registered (mirrors token_bench.py / recall_bench.py):
  aggregate recall >= 0.95, per-language recall >= 0.90; precision reported, ungated.

Usage:
  python3 impact_bench.py --binary <codeindex> [--repo <clone>] [--sample N]
      [--seed S] [--lang go,ts,js,py,php] [--out results/impact.json]
"""

from __future__ import annotations

import argparse
import json
import os
import random
import re
import shutil
import sqlite3
import subprocess
import tempfile
from dataclasses import dataclass, field
from pathlib import Path

# (file, enclosing-symbol) — the identity every set comparison uses.
Edge = tuple  # tuple[str, str] at runtime


def normalize_file(path: str, repo_root: str | None = None) -> str:
    """Normalize a file path to a repo-relative, forward-slash form."""
    p = path.replace("\\", "/").strip()
    if repo_root:
        root = repo_root.replace("\\", "/").rstrip("/") + "/"
        if p.startswith(root):
            p = p[len(root):]
    # drop a leading "./"
    if p.startswith("./"):
        p = p[2:]
    return p


@dataclass
class Score:
    tp: int = 0
    fn: int = 0
    fp: int = 0

    @property
    def recall(self) -> float:
        denom = self.tp + self.fn
        return 1.0 if denom == 0 else self.tp / denom

    @property
    def precision(self) -> float:
        denom = self.tp + self.fp
        return 1.0 if denom == 0 else self.tp / denom


def score_sets(truth: set, predicted: set) -> Score:
    tp = len(truth & predicted)
    fn = len(truth - predicted)
    fp = len(predicted - truth)
    return Score(tp=tp, fn=fn, fp=fp)


def score_with_ambiguous(truth: set, predicted: set, ambiguous: set):
    """Return (overall, ambiguous_subset) scores."""
    overall = score_sets(truth, predicted)
    amb_predicted = predicted & ambiguous
    amb_truth = truth & ambiguous
    amb = score_sets(amb_truth, amb_predicted)
    return overall, amb


# --- Impact runner -----------------------------------------------------------

_CALLER_LINE = re.compile(
    r"^\s+(?P<file>[^\s].*?):(?P<line>\d+)\s+(?P<qname>\S.*?)(?P<amb>\s+\[ambiguous\])?\s*$"
)


def parse_callers_output(text: str, repo_root: str | None = None):
    """Return (impact_edges, ambiguous_edges) from `codeindex callers` stdout."""
    edges: set = set()
    ambiguous: set = set()
    for line in text.splitlines():
        if line.startswith("def ") or line.startswith("callers ") or \
           line.startswith("referenced in") or line.strip().startswith("..."):
            continue
        m = _CALLER_LINE.match(line)
        if not m:
            continue
        f = normalize_file(m.group("file"), repo_root)
        qname = m.group("qname").strip()
        edge = (f, qname)
        edges.add(edge)
        if m.group("amb"):
            ambiguous.add(edge)
    return edges, ambiguous


def run_impact(binary: str, repo: str, symbol: str, limit: int = 500):
    """Query codeindex for callers of `symbol`; return (impact, ambiguous) edge sets."""
    try:
        r = subprocess.run(
            [binary, "callers", repo, symbol, "--limit", str(limit)],
            capture_output=True, text=True, timeout=120,
        )
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return set(), set()
    if r.returncode != 0 or not r.stdout.strip():
        return set(), set()
    return parse_callers_output(r.stdout, repo)


# --- FixtureOracle -----------------------------------------------------------

class FixtureOracle:
    """Ground truth for authored fixtures: G = the authored (file, symbol) edges."""

    def __init__(self, fixtures_dir: str, langs: list | None = None):
        self.root = Path(fixtures_dir)
        self.langs = langs

    def repo_dir(self, lang: str) -> str:
        return str(self.root / lang)

    def symbols(self, lang: str) -> list:
        manifest = self.root / lang / "manifest.json"
        if not manifest.exists():
            return []
        data = json.loads(manifest.read_text())
        out = []
        for entry in data.get("symbols", []):
            truth = {(normalize_file(f), s) for f, s in entry["dependents"]}
            out.append({
                "symbol": entry["symbol"],
                "ambiguous": bool(entry.get("ambiguous", False)),
                "truth": truth,
            })
        return out


# --- CompileOracle -----------------------------------------------------------

def toolchain_available(lang: str) -> bool:
    return bool(shutil.which({"go": "go", "ts": "tsc"}.get(lang, "")))


_GO_DIAG = re.compile(r"^\.?/?(?P<file>[^\s:]+\.go):(?P<line>\d+):\d+:\s+undefined:")
_TS_DIAG = re.compile(r"^(?P<file>[^\s(]+\.ts)\((?P<line>\d+),\d+\):\s+error TS2304:")


def parse_go_diagnostics(stderr: str, repo_root: str):
    out = []
    for line in stderr.splitlines():
        m = _GO_DIAG.match(line.strip())
        if m:
            out.append((normalize_file(m.group("file"), repo_root), int(m.group("line"))))
    return out


def parse_tsc_diagnostics(stdout: str, repo_root: str):
    out = []
    for line in stdout.splitlines():
        m = _TS_DIAG.match(line.strip())
        if m:
            out.append((normalize_file(m.group("file"), repo_root), int(m.group("line"))))
    return out


_GO_METHOD = re.compile(r"^\s*func\s+\((?:\w+\s+)?\*?(?P<recv>\w+)\)\s+(?P<name>\w+)\s*\(")
_GO_FUNC = re.compile(r"^\s*func\s+(?P<name>\w+)\s*\(")
_TS_FUNC = re.compile(r"^\s*(?:export\s+)?(?:async\s+)?function\s+(?P<name>\w+)\s*\(")
_TS_METHOD = re.compile(r"^\s*(?:public|private|protected\s+)?(?P<name>\w+)\s*\([^)]*\)\s*[:{]")


def map_site_to_enclosing(file: str, line: int, repo_root: str):
    try:
        lines = Path(file).read_text().splitlines()
    except OSError:
        return None
    for i in range(min(line, len(lines)) - 1, -1, -1):
        text = lines[i]
        m = _GO_METHOD.match(text)
        if m:
            return f"{m.group('recv')}.{m.group('name')}"
        for pat in (_GO_FUNC, _TS_FUNC):
            m = pat.match(text)
            if m:
                return m.group("name")
        m = _TS_METHOD.match(text)
        if m and m.group("name") not in ("if", "for", "while", "switch", "catch"):
            return m.group("name")
    return None


class CompileOracle:
    def __init__(self, lang: str):
        self.lang = lang

    def truth_for(self, repo_root: str, symbol: str, decl_file: str, decl_line: int):
        if not toolchain_available(self.lang):
            return None
        work = tempfile.mkdtemp(prefix="impactoracle_")
        try:
            dst = Path(work) / "repo"
            shutil.copytree(repo_root, dst)
            target = dst / decl_file
            src_lines = target.read_text().splitlines()
            nonce = symbol + "_zzq"
            # rename only the declaration line's identifier occurrence
            src_lines[decl_line - 1] = re.sub(
                rf"\b{re.escape(symbol)}\b", nonce, src_lines[decl_line - 1], count=1
            )
            target.write_text("\n".join(src_lines) + "\n")
            if self.lang == "go":
                r = subprocess.run(["go", "build", "./..."], cwd=dst,
                                   capture_output=True, text=True, timeout=180)
                sites = parse_go_diagnostics(r.stderr, str(dst))
            else:
                r = subprocess.run(["tsc", "--noEmit"], cwd=dst,
                                   capture_output=True, text=True, timeout=180)
                sites = parse_tsc_diagnostics(r.stdout, str(dst))
            edges = set()
            for f, ln in sites:
                enc = map_site_to_enclosing(str(dst / f), ln, str(dst))
                if enc:
                    edges.add((normalize_file(f, str(dst)), enc))
            return edges or None
        finally:
            shutil.rmtree(work, ignore_errors=True)


# --- Real-repo sampler -------------------------------------------------------

def sample_unique_symbols(db_path: str, lang: str, n: int, seed: int) -> list:
    """Select unique-named function/method symbols with ≥1 caller from the graph DB.

    'Unique-named' means exactly one symbol row has that name in the entire index.
    Uses the real codeindex schema: symbols(id, name, file, start_line, kind, tier)
    and edges(dst_symbol_id).  tier=0 = project-tier (own code).

    The `lang` parameter is accepted for signature compatibility; it does not filter
    (the DB is one repo of one language).

    Returns up to `n` dicts {"symbol": name, "file": decl_file, "line": decl_line},
    deterministically shuffled with random.Random(seed).
    """
    con = sqlite3.connect(db_path)
    try:
        rows = con.execute(
            """
            SELECT s.name, s.file, s.start_line
            FROM symbols s
            JOIN (
                SELECT name FROM symbols
                GROUP BY name HAVING COUNT(*) = 1
            ) u ON u.name = s.name
            WHERE s.kind IN ('func', 'function', 'method')
              AND s.tier = 0
              AND s.id IN (SELECT dst_symbol_id FROM edges)
            """
        ).fetchall()
    finally:
        con.close()
    rng = random.Random(seed)
    rng.shuffle(rows)
    return [{"symbol": r[0], "file": r[1], "line": r[2]} for r in rows[:n]]

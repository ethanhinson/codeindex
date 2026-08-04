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
import re
import shutil
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

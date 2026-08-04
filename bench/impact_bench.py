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

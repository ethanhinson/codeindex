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


# --- Aggregation + reporting -------------------------------------------------

AGG_BAR = 0.95
LANG_BAR = 0.90


def aggregate(per_symbol: list) -> dict:
    langs = {}
    for rec in per_symbol:
        L = langs.setdefault(rec["lang"], {"tp": 0, "fn": 0, "fp": 0,
                                           "atp": 0, "afn": 0, "afp": 0,
                                           "graded": 0, "not_run_any": False})
        if rec.get("status") == "not_run":
            L["not_run_any"] = True
            continue
        if not rec.get("graded"):
            continue
        s, a = rec["score"], rec["amb_score"]
        L["tp"] += s.tp; L["fn"] += s.fn; L["fp"] += s.fp
        L["atp"] += a.tp; L["afn"] += a.fn; L["afp"] += a.fp
        L["graded"] += 1

    def _r(tp, fn): return 1.0 if tp + fn == 0 else tp / (tp + fn)
    def _p(tp, fp): return 1.0 if tp + fp == 0 else tp / (tp + fp)

    per_language, gtp = {}, {"tp": 0, "fn": 0, "fp": 0, "graded": 0}
    per_lang_pass = {}
    for lang, L in sorted(langs.items()):
        graded_any = L["graded"] > 0
        rec = {
            "recall": _r(L["tp"], L["fn"]) if graded_any else None,
            "precision": _p(L["tp"], L["fp"]) if graded_any else None,
            "amb_recall": _r(L["atp"], L["afn"]) if graded_any else None,
            "amb_precision": _p(L["atp"], L["afp"]) if graded_any else None,
            "graded": L["graded"],
            "not_run": L["not_run_any"] and not graded_any,
        }
        per_language[lang] = rec
        if graded_any:
            gtp["tp"] += L["tp"]; gtp["fn"] += L["fn"]; gtp["fp"] += L["fp"]
            gtp["graded"] += L["graded"]
            per_lang_pass[lang] = rec["recall"] >= LANG_BAR
    agg_recall = _r(gtp["tp"], gtp["fn"])
    return {
        "per_language": per_language,
        "aggregate": {"recall": agg_recall, "precision": _p(gtp["tp"], gtp["fp"]),
                      "graded": gtp["graded"]},
        "bar": {"AGG_BAR": AGG_BAR, "LANG_BAR": LANG_BAR,
                "agg_pass": agg_recall >= AGG_BAR,
                "per_lang_pass": per_lang_pass},
    }


def write_findings(report: dict, path: str) -> None:
    lines = ["# Blast-radius accuracy — FINDINGS", "",
             f"Pre-registered bar: aggregate recall >= {AGG_BAR}, "
             f"per-language recall >= {LANG_BAR}. Precision reported, not gated (v1).", "",
             "| lang | recall | precision | amb recall | amb precision | graded | status |",
             "|---|---|---|---|---|---|---|"]
    for lang, r in report["per_language"].items():
        if r["not_run"]:
            lines.append(f"| {lang} | — | — | — | — | 0 | not run (toolchain absent) |")
        else:
            lines.append(
                f"| {lang} | {r['recall']:.3f} | {r['precision']:.3f} | "
                f"{r['amb_recall']:.3f} | {r['amb_precision']:.3f} | {r['graded']} | "
                f"{'PASS' if report['bar']['per_lang_pass'].get(lang) else 'MISS'} |")
    agg = report["aggregate"]
    lines += ["",
              f"**Aggregate recall = {agg['recall']:.3f}** "
              f"(precision {agg['precision']:.3f}, {agg['graded']} graded) — "
              f"{'PASS' if report['bar']['agg_pass'] else 'MISS'} vs bar {AGG_BAR}."]
    Path(path).write_text("\n".join(lines) + "\n")


# --- CLI ---------------------------------------------------------------------

def _run_symbol(binary, repo, sym, truth, is_ambiguous):
    impact, ambiguous = run_impact(binary, repo, sym)
    overall, amb = score_with_ambiguous(truth, impact, ambiguous)
    return {"score": overall, "amb_score": amb, "graded": True, "status": "graded"}


def main():
    ap = argparse.ArgumentParser(description="codeindex blast-radius accuracy benchmark")
    ap.add_argument("--binary", required=True)
    ap.add_argument("--repo", default=None, help="real repo clone (Go/TS) for CompileOracle")
    ap.add_argument("--repo-lang", default=None, choices=["go", "ts"])
    ap.add_argument("--sample", type=int, default=30)
    ap.add_argument("--seed", type=int, default=99)
    ap.add_argument("--lang", default="go,ts,js,py,php")
    ap.add_argument("--fixtures", default=str(Path(__file__).parent / "impact_fixtures"))
    ap.add_argument("--out", default=str(Path(__file__).parent / "results" / "impact.json"))
    args = ap.parse_args()
    langs = args.lang.split(",")

    per_symbol = []
    fix = FixtureOracle(args.fixtures)
    for lang in langs:
        for entry in fix.symbols(lang):
            rec = _run_symbol(args.binary, fix.repo_dir(lang), entry["symbol"],
                              entry["truth"], entry["ambiguous"])
            rec.update(lang=lang, symbol=entry["symbol"])
            per_symbol.append(rec)

    # Real-repo CompileOracle pass (Go/TS), only if a repo + toolchain are present.
    if args.repo and args.repo_lang:
        lang = args.repo_lang
        if not toolchain_available(lang):
            per_symbol.append({"lang": lang, "symbol": "(real-repo)", "graded": False,
                               "status": "not_run", "score": Score(), "amb_score": Score()})
        else:
            db = str(Path(args.repo) / ".codeindex" / "graph.db")
            oracle = CompileOracle(lang)
            for s in sample_unique_symbols(db, lang, args.sample, args.seed):
                G = oracle.truth_for(args.repo, s["symbol"], s["file"], s["line"])
                if G is None:
                    per_symbol.append({"lang": lang, "symbol": s["symbol"],
                                       "graded": False, "status": "excluded_empty_truth",
                                       "score": Score(), "amb_score": Score()})
                    continue
                rec = _run_symbol(args.binary, args.repo, s["symbol"], G, False)
                rec.update(lang=lang, symbol=s["symbol"])
                per_symbol.append(rec)

    report = aggregate(per_symbol)
    report["symbols"] = [
        {"lang": r["lang"], "symbol": r.get("symbol"), "status": r["status"],
         "recall": (r["score"].recall if r["graded"] else None),
         "precision": (r["score"].precision if r["graded"] else None)}
        for r in per_symbol
    ]
    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(report, indent=2, default=lambda o: o.__dict__))
    write_findings(report, str(Path(__file__).parent / "impact-FINDINGS.md"))
    print(f"aggregate recall={report['aggregate']['recall']:.3f} "
          f"agg_pass={report['bar']['agg_pass']}")


if __name__ == "__main__":
    main()

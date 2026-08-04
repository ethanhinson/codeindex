# Blast-radius accuracy benchmark Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `bench/impact_bench.py` — a deterministic, pre-registered accuracy benchmark that scores codeindex's impact/blast-radius answers (recall + precision) against a hybrid ground-truth oracle, across all five supported languages.

**Architecture:** A single Python harness (mirroring `bench/token_bench.py` / `bench/recall_bench.py`) with three cooperating pieces behind clean seams: (1) an **impact runner** that shells out to `codeindex callers <repo> <symbol>` and parses the reference lines into an impact set `I` normalized to `(file, enclosing-symbol)`; (2) a **hybrid oracle** — `FixtureOracle` reads authored `manifest.json` edges for JS/Python/PHP (and Go/TS validation fixtures), `CompileOracle` renames a declaration and parses `go build` / `tsc --noEmit` diagnostics for Go/TS real repos; (3) a **scorer** computing recall, precision, and an `[ambiguous]`-subset breakdown, aggregated per-language and overall, emitting JSON to `bench/results/` and a human-readable `bench/impact-FINDINGS.md`. All pieces are pure-functionally separable so the scorer has a golden-fixture unit test.

**Tech Stack:** Python 3 stdlib only (`argparse`, `json`, `subprocess`, `random`, `re`, `pathlib`, `shutil`, `tempfile`) — matching the existing benches, which take no third-party dependency for the harness core. The codeindex binary under test is a prebuilt Go binary passed via `--binary`. Real-repo oracles shell to `go` and `tsc` when present.

## Global Constraints

- **Additive only — never alter the resolver.** All new files live under `bench/`. The deterministic resolver in `internal/` is *measured*, not changed (spec Out of scope).
- **Deterministic + seeded.** Real-repo sampling uses a seeded RNG following the `recall_bench.py --seed` / `--sample` convention; a fixed seed yields a fixed run.
- **Pre-registered bar, fixed before running:** aggregate recall ≥ 0.95, per-language recall ≥ 0.90. Precision is **reported but NOT gated** in v1. Missing the bar is a logged finding, not a hard failure of the harness.
- **Identity for set comparison is `(file, enclosing-symbol)`.** codeindex's `callers` output line is `  <file>:<line>  <QName>[  [ambiguous]]` where `QName()` = `Parent.Name` (or `Name`) — the enclosing symbol. Compiler diagnostics report a call-*site* `file:line`, which must be mapped up to its enclosing symbol before comparison. Fixture manifests are authored already in `(file, enclosing-symbol)` identity.
- **Impact query surface:** `codeindex callers <repo-root> <symbol> [--limit N]` prints a `def ...` line(s), then `callers (N):`, then one indented `  <file>:<line>  <QName>[  [ambiguous]]` per caller, then a `referenced in N file(s): ...` line. The index must exist first: `codeindex build <repo-root>` writes `<repo-root>/.codeindex/graph.db`.
- **Missing toolchain is "not run", never a silent pass.** If `go` or `tsc` is absent, that language records status `not_run`; aggregate output states which languages actually ran.
- **CLI shape (from spec):** `python3 impact_bench.py --binary <codeindex> [--repo <clone>] [--sample N] [--seed S] [--lang go,ts,js,py,php] [--out <path>]`.
- **Follow bench conventions:** JSON results to `bench/results/`, human-readable findings to `bench/impact-FINDINGS.md` (matching `FINDINGS.md` / `efficacy-FINDINGS.md`).

---

### Task 1: Impact-set data model, normalization, and the scorer

**Files:**
- Create: `bench/impact_bench.py`
- Test: `bench/test_impact_bench.py`

**Interfaces:**
- Consumes: nothing (first task).
- Produces:
  - `Edge = tuple[str, str]` — the canonical `(file, enclosing_symbol)` identity used everywhere.
  - `normalize_file(path: str) -> str` — strips a leading repo-root prefix and normalizes separators so oracle paths and codeindex paths compare equal. Signature: `normalize_file(path: str, repo_root: str | None = None) -> str`.
  - `class Score` — a `dataclass` with fields `tp: int`, `fn: int`, `fp: int`, and properties `recall: float` (`tp/(tp+fn)` or `1.0` when `tp+fn==0`), `precision: float` (`tp/(tp+fp)` or `1.0` when `tp+fp==0`).
  - `score_sets(truth: set[Edge], predicted: set[Edge]) -> Score` — pure set comparison.
  - `score_with_ambiguous(truth: set[Edge], predicted: set[Edge], ambiguous: set[Edge]) -> tuple[Score, Score]` — returns `(overall_score, ambiguous_subset_score)` where the ambiguous subset scores only predicted edges flagged `[ambiguous]` against the truth restricted to those files/symbols. The ambiguous-subset truth is `truth ∩ (edges whose predicted counterpart was flagged)`; precisely: `amb_predicted = predicted ∩ ambiguous`, `amb_truth = truth ∩ ambiguous`, then `score_sets(amb_truth, amb_predicted)`.

- [ ] **Step 1: Write the failing test**

```python
# bench/test_impact_bench.py
import importlib.util
from pathlib import Path

_spec = importlib.util.spec_from_file_location(
    "impact_bench", Path(__file__).parent / "impact_bench.py"
)
impact_bench = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(impact_bench)


def test_score_sets_perfect_recall_and_precision():
    truth = {("a.go", "A.f"), ("b.go", "B.g")}
    pred = {("a.go", "A.f"), ("b.go", "B.g")}
    s = impact_bench.score_sets(truth, pred)
    assert s.tp == 2 and s.fn == 0 and s.fp == 0
    assert s.recall == 1.0 and s.precision == 1.0


def test_score_sets_missed_dependent_lowers_recall():
    truth = {("a.go", "A.f"), ("b.go", "B.g")}
    pred = {("a.go", "A.f")}  # missed B.g
    s = impact_bench.score_sets(truth, pred)
    assert s.tp == 1 and s.fn == 1 and s.fp == 0
    assert s.recall == 0.5 and s.precision == 1.0


def test_score_sets_false_positive_lowers_precision():
    truth = {("a.go", "A.f")}
    pred = {("a.go", "A.f"), ("noise.go", "N.x")}
    s = impact_bench.score_sets(truth, pred)
    assert s.tp == 1 and s.fn == 0 and s.fp == 1
    assert s.recall == 1.0 and s.precision == 0.5


def test_score_sets_empty_truth_is_recall_one():
    # a symbol with zero authored dependents: correct answer is empty impact set
    s = impact_bench.score_sets(set(), set())
    assert s.recall == 1.0 and s.precision == 1.0
    s2 = impact_bench.score_sets(set(), {("x.go", "X.y")})
    assert s2.recall == 1.0 and s2.precision == 0.5  # spurious dependents still hurt precision


def test_score_with_ambiguous_breaks_out_flagged_subset():
    truth = {("a.go", "A.f"), ("b.go", "B.g"), ("c.go", "C.h")}
    pred = {("a.go", "A.f"), ("b.go", "B.g"), ("c.go", "C.h")}
    ambiguous = {("b.go", "B.g"), ("c.go", "C.h")}  # two flagged [ambiguous]
    overall, amb = impact_bench.score_with_ambiguous(truth, pred, ambiguous)
    assert overall.recall == 1.0 and overall.precision == 1.0
    # ambiguous subset: 2 flagged predictions, both correct
    assert amb.tp == 2 and amb.fn == 0 and amb.fp == 0
    assert amb.recall == 1.0 and amb.precision == 1.0
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd bench && python3 -m pytest test_impact_bench.py -v` (or `python3 test_impact_bench.py` if pytest is unavailable — see Step 3 note)
Expected: FAIL — `impact_bench.py` does not exist / has no `score_sets`.

- [ ] **Step 3: Write minimal implementation**

Create `bench/impact_bench.py` with the model + scorer. (Later tasks append to this same file.)

```python
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
```

Note on the test runner: if `pytest` is not installed, add a `if __name__ == "__main__":` block at the very bottom of `test_impact_bench.py` that calls each `test_*` function and prints `OK`, so the suite runs under bare `python3`. Add it now:

```python
# bench/test_impact_bench.py  (append at end)
if __name__ == "__main__":
    import sys
    fns = [v for k, v in sorted(globals().items())
           if k.startswith("test_") and callable(v)]
    for fn in fns:
        fn()
        print(f"ok {fn.__name__}")
    print(f"\n{len(fns)} passed")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd bench && python3 -m pytest test_impact_bench.py -v || python3 test_impact_bench.py`
Expected: PASS — all five scorer tests green.

- [ ] **Step 5: Commit**

```bash
git add bench/impact_bench.py bench/test_impact_bench.py
git commit -m "feat(bench): impact-set data model, normalization, and scorer with golden tests"
```

---

### Task 2: Impact runner — parse codeindex `callers` output into an impact set

**Files:**
- Modify: `bench/impact_bench.py` (append)
- Test: `bench/test_impact_bench.py` (append)

**Interfaces:**
- Consumes: `normalize_file`, `Edge` from Task 1.
- Produces:
  - `parse_callers_output(text: str, repo_root: str | None = None) -> tuple[set[Edge], set[Edge]]` — parses the raw stdout of `codeindex callers` and returns `(impact_edges, ambiguous_edges)`. `impact_edges` are all caller edges; `ambiguous_edges` are the subset whose line ended with `[ambiguous]`. Ignores the `def ...`, `callers (N):`, and `referenced in ...` lines. A caller line matches `^\s+(?P<file>...):(?P<line>\d+)\s+(?P<qname>...?)(?:\s+\[ambiguous\])?\s*$`.
  - `run_impact(binary: str, repo: str, symbol: str, limit: int = 500) -> tuple[set[Edge], set[Edge]]` — shells out `[binary, "callers", repo, symbol, "--limit", str(limit)]`, returns `parse_callers_output(stdout, repo)`. On non-zero exit or empty output, returns `(set(), set())` (a symbol codeindex cannot resolve yields an empty impact set — the scorer grades that against truth).

- [ ] **Step 1: Write the failing test**

```python
# bench/test_impact_bench.py  (append above the __main__ block)
def test_parse_callers_output_extracts_edges_and_ambiguous():
    raw = (
        "def  pkg.Foo  a.go:10  func Foo()\n"
        "callers (3):\n"
        "  b.go:20  B.callSite\n"
        "  c.go:30  C.other  [ambiguous]\n"
        "  d.go:40  topLevelFn\n"
        "referenced in 3 file(s): b.go c.go d.go\n"
    )
    edges, ambiguous = impact_bench.parse_callers_output(raw)
    assert edges == {("b.go", "B.callSite"), ("c.go", "C.other"), ("d.go", "topLevelFn")}
    assert ambiguous == {("c.go", "C.other")}


def test_parse_callers_output_empty_callers():
    raw = "def  pkg.Foo  a.go:10  func Foo()\ncallers (0):\nreferenced in 0 file(s):\n"
    edges, ambiguous = impact_bench.parse_callers_output(raw)
    assert edges == set()
    assert ambiguous == set()


def test_parse_callers_output_strips_repo_root():
    raw = "callers (1):\n  /tmp/repo/b.go:20  B.callSite\n"
    edges, _ = impact_bench.parse_callers_output(raw, repo_root="/tmp/repo")
    assert edges == {("b.go", "B.callSite")}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k parse_callers -v || python3 test_impact_bench.py`
Expected: FAIL — `parse_callers_output` not defined.

- [ ] **Step 3: Write minimal implementation**

Append to `bench/impact_bench.py`:

```python
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k parse_callers -v || python3 test_impact_bench.py`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add bench/impact_bench.py bench/test_impact_bench.py
git commit -m "feat(bench): impact runner parses codeindex callers output into edge sets"
```

---

### Task 3: FixtureOracle + authored fixtures for all five languages (ambiguity home)

**Files:**
- Modify: `bench/impact_bench.py` (append)
- Create: `bench/impact_fixtures/js/manifest.json` + source files
- Create: `bench/impact_fixtures/py/manifest.json` + source files
- Create: `bench/impact_fixtures/php/manifest.json` + source files
- Create: `bench/impact_fixtures/go/manifest.json` + source files
- Create: `bench/impact_fixtures/ts/manifest.json` + source files
- Test: `bench/test_impact_bench.py` (append)

**Interfaces:**
- Consumes: `Edge`, `normalize_file` from Task 1.
- Produces:
  - Fixture manifest schema: a JSON object `{"lang": "<lang>", "symbols": [{"symbol": "<name>", "ambiguous": <bool>, "dependents": [["<file>", "<enclosing-symbol>"], ...]}, ...]}`. `dependents` is authored in `(file, enclosing-symbol)` identity. `ambiguous: true` marks a same-name/shadowing/re-export case whose dependents codeindex is expected to flag `[ambiguous]`.
  - `class FixtureOracle` with `__init__(self, fixtures_dir: str, langs: list[str] | None = None)`, and `symbols(self, lang: str) -> list[dict]` (the manifest's symbol entries for that lang, each dict with `symbol`, `ambiguous`, and `truth: set[Edge]`), and `repo_dir(self, lang: str) -> str` (path to that fixture's source root, for handing to the impact runner).

- [ ] **Step 1: Write the failing test**

Author the smallest real fixture first (Python), then the test. Create `bench/impact_fixtures/py/manifest.json`:

```json
{
  "lang": "py",
  "symbols": [
    {
      "symbol": "shared_helper",
      "ambiguous": false,
      "dependents": [["app.py", "run"], ["worker.py", "process"]]
    },
    {
      "symbol": "collide",
      "ambiguous": true,
      "dependents": [["mod_a.py", "use_a"], ["mod_b.py", "use_b"]]
    }
  ]
}
```

Create `bench/impact_fixtures/py/util.py`:

```python
def shared_helper(x):
    return x + 1
```

Create `bench/impact_fixtures/py/app.py`:

```python
from util import shared_helper

def run():
    return shared_helper(1)
```

Create `bench/impact_fixtures/py/worker.py`:

```python
from util import shared_helper

def process():
    return shared_helper(2)
```

Create `bench/impact_fixtures/py/mod_a.py`:

```python
def collide():
    return "a"

def use_a():
    return collide()
```

Create `bench/impact_fixtures/py/mod_b.py`:

```python
def collide():
    return "b"

def use_b():
    return collide()
```

Now the test:

```python
# bench/test_impact_bench.py  (append above __main__)
from pathlib import Path as _Path
_FIX = _Path(__file__).parent / "impact_fixtures"

def test_fixture_oracle_reads_python_manifest():
    oracle = impact_bench.FixtureOracle(str(_FIX), langs=["py"])
    syms = oracle.symbols("py")
    by_name = {s["symbol"]: s for s in syms}
    assert by_name["shared_helper"]["truth"] == {("app.py", "run"), ("worker.py", "process")}
    assert by_name["shared_helper"]["ambiguous"] is False
    assert by_name["collide"]["ambiguous"] is True
    assert by_name["collide"]["truth"] == {("mod_a.py", "use_a"), ("mod_b.py", "use_b")}
    assert oracle.repo_dir("py").endswith("impact_fixtures/py")
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k fixture_oracle -v || python3 test_impact_bench.py`
Expected: FAIL — `FixtureOracle` not defined.

- [ ] **Step 3: Write minimal implementation**

Append to `bench/impact_bench.py`:

```python
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k fixture_oracle -v || python3 test_impact_bench.py`
Expected: PASS.

- [ ] **Step 5: Author the remaining four fixtures (js, php, go, ts)**

Author each as a tiny repo with a `manifest.json` in the same schema. Keep each minimal: one clean multi-dependent symbol and one `ambiguous: true` collision case per language. Use idiomatic dependents so codeindex's resolver actually produces edges.

`bench/impact_fixtures/js/manifest.json` + `util.js` (exports `sharedHelper`), `app.js` / `worker.js` (each defines a function that calls `sharedHelper`), `mod_a.js` / `mod_b.js` (each defines a local `collide` + a `useA`/`useB` caller). Manifest mirrors the Python one with `.js` files and JS enclosing-symbol names.

`bench/impact_fixtures/php/manifest.json` + `Util.php` (function `shared_helper`), `App.php` / `Worker.php`, `ModA.php` / `ModB.php`. Use plain functions (not namespaced classes) for the clean case; use two same-named functions in different files for the collision.

`bench/impact_fixtures/go/manifest.json` + a single-package or two-package tiny module: `util.go` (`func SharedHelper()`), `app.go` (`func Run()` calls it), `worker.go` (`func Process()` calls it). For the collision, two files each with a package-local `Collide` and a caller. Add a minimal `go.mod` (`module impactfix` + a `go 1.x` line) so `go build` works in Task 4's validation.

`bench/impact_fixtures/ts/manifest.json` + `util.ts` (`export function sharedHelper()`), `app.ts` / `worker.ts` importing it, `modA.ts` / `modB.ts` for the collision. Add a minimal `tsconfig.json` (`{"compilerOptions": {"noEmit": true, "moduleResolution": "node", "strict": false}, "include": ["*.ts"]}`) for Task 4's validation.

For each fixture, set `dependents` in the manifest to the exact `(file, enclosing-symbol)` edges codeindex should return — where `enclosing-symbol` is `QName()` = the enclosing function/method name (for a method, `Type.method`; for a package function, just the function name).

- [ ] **Step 6: Commit**

```bash
git add bench/impact_bench.py bench/test_impact_bench.py bench/impact_fixtures/
git commit -m "feat(bench): FixtureOracle + authored five-language fixtures (ambiguity cases)"
```

---

### Task 4: CompileOracle — Go/TS compile-break ground truth + fixture validation

**Files:**
- Modify: `bench/impact_bench.py` (append)
- Test: `bench/test_impact_bench.py` (append)

**Interfaces:**
- Consumes: `Edge`, `normalize_file` from Task 1.
- Produces:
  - `toolchain_available(lang: str) -> bool` — `shutil.which("go")` for `go`, `shutil.which("tsc")` for `ts`.
  - `map_site_to_enclosing(file: str, line: int, repo_root: str) -> str | None` — reads `file` and walks *upward* from `line` to the nearest enclosing function/method declaration, returning its `QName`-style name (best-effort regex: Go `func (recv T) Name(` → `T.Name`, `func Name(` → `Name`; TS `function Name(` / `Name(...) {` method → `Name`). Returns `None` if no enclosing decl found (caller logs + drops that site).
  - `class CompileOracle` with `__init__(self, lang: str)`, and `truth_for(self, repo_root: str, symbol: str, decl_file: str, decl_line: int) -> set[Edge] | None` — renames the declaration of `symbol` at `decl_file:decl_line` to a nonce (`symbol + "_zzq"`) in a throwaway copy of `repo_root`, runs `go build ./...` (Go) or `tsc --noEmit` (TS), parses diagnostics for sites referencing the now-undefined nonce/symbol, maps each site to its enclosing symbol, and returns the edge set `G`. Returns `None` (excluded from scoring) when the toolchain is absent or `G` is empty (unused symbol / dynamic dispatch — cannot grade an empty compiler truth).
  - `parse_go_diagnostics(stderr: str, repo_root: str) -> list[tuple[str, int]]` and `parse_tsc_diagnostics(stdout: str, repo_root: str) -> list[tuple[str, int]]` — return `(file, line)` site lists from compiler output (Go: `file:line:col: undefined: name`; TS: `file(line,col): error TS2304: Cannot find name`).

- [ ] **Step 1: Write the failing test (pure parsers + mapping, no toolchain needed)**

```python
# bench/test_impact_bench.py  (append above __main__)
def test_parse_go_diagnostics_extracts_sites():
    stderr = (
        "# impactfix\n"
        "./app.go:6:9: undefined: SharedHelper\n"
        "./worker.go:5:9: undefined: SharedHelper\n"
    )
    sites = impact_bench.parse_go_diagnostics(stderr, "/tmp/repo")
    assert ("app.go", 6) in sites
    assert ("worker.go", 5) in sites


def test_parse_tsc_diagnostics_extracts_sites():
    stdout = (
        "app.ts(4,10): error TS2304: Cannot find name 'sharedHelper'.\n"
        "worker.ts(3,10): error TS2304: Cannot find name 'sharedHelper'.\n"
    )
    sites = impact_bench.parse_tsc_diagnostics(stdout, "/tmp/repo")
    assert ("app.ts", 4) in sites
    assert ("worker.ts", 3) in sites


def test_map_site_to_enclosing_go(tmp_path):
    f = tmp_path / "app.go"
    f.write_text("package main\n\nfunc Run() int {\n    return SharedHelper()\n}\n")
    name = impact_bench.map_site_to_enclosing(str(f), 4, str(tmp_path))
    assert name == "Run"
```

(If `pytest`/`tmp_path` is unavailable, the `__main__` fallback should create a temp dir via `tempfile` for the `tmp_path` test — add a small shim in the fallback runner that passes a fresh `tempfile.mkdtemp()` Path to any test whose name ends in a `tmp_path` param. Simpler: in the `__main__` block, wrap each call in `try/except TypeError` and skip param-requiring tests, and additionally run `test_map_site_to_enclosing_go` with an explicit `_Path(tempfile.mkdtemp())`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k "diagnostics or enclosing" -v || python3 test_impact_bench.py`
Expected: FAIL — parsers/mapper not defined.

- [ ] **Step 3: Write minimal implementation**

Append to `bench/impact_bench.py`:

```python
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k "diagnostics or enclosing" -v || python3 test_impact_bench.py`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add bench/impact_bench.py bench/test_impact_bench.py
git commit -m "feat(bench): CompileOracle — Go/TS compile-break ground truth + diagnostic parsers"
```

---

### Task 5: Real-repo sampler — unique-name selection with declaration locations

**Files:**
- Modify: `bench/impact_bench.py` (append)
- Test: `bench/test_impact_bench.py` (append)

**Interfaces:**
- Consumes: nothing new (reads `<repo>/.codeindex/graph.db` like `recall_bench.py`).
- Produces:
  - `sample_unique_symbols(db_path: str, lang: str, n: int, seed: int) -> list[dict]` — opens the codeindex SQLite graph (`<repo>/.codeindex/graph.db`), selects function/method symbols whose `name` is **unique** in the index (exactly one definition row) and that have ≥1 caller, deterministically shuffles with `random.Random(seed)`, and returns up to `n` dicts `{"symbol": name, "file": decl_file, "line": decl_line}`. Mirror `recall_bench.py`'s `sqlite3` access pattern (read the `symbols` table; join callers count). If the schema column names differ, discover them from `recall_bench.py`'s query verbatim.

- [ ] **Step 1: Write the failing test (build a tiny SQLite that matches the schema recall_bench uses)**

First read `bench/recall_bench.py`'s SQL to copy the exact table/column names, then write a test that seeds an in-file SQLite with that schema and asserts uniqueness filtering + determinism:

```python
# bench/test_impact_bench.py  (append above __main__)
import sqlite3 as _sqlite3

def _make_db(path):
    con = _sqlite3.connect(path)
    # NOTE: mirror the exact schema recall_bench.py queries — adjust column
    # names here to match what Step-1 reading of recall_bench.py reveals.
    con.executescript(
        """
        CREATE TABLE symbols (id INTEGER PRIMARY KEY, name TEXT, file TEXT,
                              start_line INTEGER, kind TEXT);
        CREATE TABLE edges (id INTEGER PRIMARY KEY, callee_id INTEGER);
        INSERT INTO symbols (id,name,file,start_line,kind) VALUES
          (1,'UniqueA','a.go',10,'func'),
          (2,'UniqueB','b.go',20,'func'),
          (3,'Dup','c.go',30,'func'),
          (4,'Dup','d.go',40,'func');
        INSERT INTO edges (callee_id) VALUES (1),(1),(2),(3);
        """
    )
    con.commit(); con.close()

def test_sample_unique_symbols_excludes_duplicates_and_is_deterministic(tmp_path):
    db = str(tmp_path / "graph.db")
    _make_db(db)
    got1 = impact_bench.sample_unique_symbols(db, "go", n=10, seed=7)
    names = {s["symbol"] for s in got1}
    assert "Dup" not in names           # duplicate name excluded
    assert names == {"UniqueA", "UniqueB"}
    got2 = impact_bench.sample_unique_symbols(db, "go", n=10, seed=7)
    assert [s["symbol"] for s in got1] == [s["symbol"] for s in got2]  # deterministic
```

**Adjust the schema in `_make_db` and the query in Step 3 to match the real column names discovered by reading `bench/recall_bench.py` (it already queries `symbols` and an edges/callers table).**

- [ ] **Step 2: Run test to verify it fails**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k sample_unique -v || python3 test_impact_bench.py`
Expected: FAIL — `sample_unique_symbols` not defined.

- [ ] **Step 3: Write minimal implementation**

Append to `bench/impact_bench.py` (import `random` and `sqlite3` at top of file):

```python
# --- Real-repo sampler -------------------------------------------------------

def sample_unique_symbols(db_path: str, lang: str, n: int, seed: int):
    con = sqlite3.connect(db_path)
    try:
        # unique-named function/method symbols with at least one caller.
        # Column names mirror recall_bench.py's schema access.
        rows = con.execute(
            """
            SELECT s.name, s.file, s.start_line
            FROM symbols s
            JOIN (SELECT name FROM symbols GROUP BY name HAVING COUNT(*) = 1) u
              ON u.name = s.name
            WHERE s.kind IN ('func','function','method')
              AND s.id IN (SELECT callee_id FROM edges)
            """
        ).fetchall()
    finally:
        con.close()
    rng = random.Random(seed)
    rng.shuffle(rows)
    return [{"symbol": r[0], "file": r[1], "line": r[2]} for r in rows[:n]]
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k sample_unique -v || python3 test_impact_bench.py`
Expected: PASS. If it fails on schema, correct the SQL to the real columns from `recall_bench.py` and re-run.

- [ ] **Step 5: Commit**

```bash
git add bench/impact_bench.py bench/test_impact_bench.py
git commit -m "feat(bench): real-repo sampler selects unique-named symbols deterministically"
```

---

### Task 6: Orchestration, aggregation, CLI, and outputs (JSON + FINDINGS)

**Files:**
- Modify: `bench/impact_bench.py` (append `main()` + aggregation + reporting)
- Test: `bench/test_impact_bench.py` (append aggregation test)

**Interfaces:**
- Consumes: everything above — `FixtureOracle`, `CompileOracle`, `run_impact`, `sample_unique_symbols`, `score_with_ambiguous`, `Score`.
- Produces:
  - `aggregate(per_symbol: list[dict]) -> dict` — folds per-symbol `{lang, symbol, score, amb_score, graded(bool), status}` records into `{per_language: {lang: {recall, precision, graded, not_run, amb_recall, amb_precision}}, aggregate: {recall, precision, graded}, bar: {agg_pass, per_lang_pass, AGG_BAR: 0.95, LANG_BAR: 0.90}}`. Recall/precision are micro-averaged over graded symbols (sum tp/fn/fp), NOT a mean of ratios.
  - `write_findings(report: dict, path: str) -> None` — writes `bench/impact-FINDINGS.md` in the existing FINDINGS style: a title, the pre-registered bar, a per-language table, the aggregate line, the ambiguous-subset line, and a "languages not run" note.
  - `main()` — argparse CLI per the Global Constraints shape; drives fixtures (always) + real repos (when `--repo` given and toolchain present); writes JSON to `--out` (default `bench/results/impact.json`) and findings to `bench/impact-FINDINGS.md`.

- [ ] **Step 1: Write the failing test**

```python
# bench/test_impact_bench.py  (append above __main__)
def test_aggregate_micro_averages_and_applies_bar():
    per_symbol = [
        {"lang": "py", "symbol": "a", "graded": True, "status": "graded",
         "score": impact_bench.Score(tp=9, fn=1, fp=0),
         "amb_score": impact_bench.Score(tp=0, fn=0, fp=0)},
        {"lang": "py", "symbol": "b", "graded": True, "status": "graded",
         "score": impact_bench.Score(tp=10, fn=0, fp=1),
         "amb_score": impact_bench.Score(tp=2, fn=0, fp=0)},
        {"lang": "go", "symbol": "c", "graded": False, "status": "not_run",
         "score": impact_bench.Score(), "amb_score": impact_bench.Score()},
    ]
    rep = impact_bench.aggregate(per_symbol)
    # py micro recall = (9+10)/(9+1+10+0) = 19/20 = 0.95
    assert abs(rep["per_language"]["py"]["recall"] - 0.95) < 1e-9
    assert rep["per_language"]["go"]["not_run"] is True
    # aggregate recall over graded only = 19/20 = 0.95 -> passes agg bar
    assert abs(rep["aggregate"]["recall"] - 0.95) < 1e-9
    assert rep["bar"]["agg_pass"] is True
    assert rep["bar"]["per_lang_pass"]["py"] is True  # 0.95 >= 0.90
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k aggregate -v || python3 test_impact_bench.py`
Expected: FAIL — `aggregate` not defined.

- [ ] **Step 3: Write minimal implementation**

Append to `bench/impact_bench.py`:

```python
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd bench && python3 -m pytest test_impact_bench.py -k aggregate -v || python3 test_impact_bench.py`
Expected: PASS. Then run the **full** suite: `python3 -m pytest test_impact_bench.py -v || python3 test_impact_bench.py` — all green.

- [ ] **Step 5: End-to-end smoke on fixtures (no real repo, no toolchain needed)**

Build the codeindex binary and run the harness against fixtures only, to prove it wires together. (If building the binary is heavy, this smoke is best-effort — the unit suite is the gate.)

Run:
```bash
go build -o /tmp/codeindex ./cmd/codeindex
cd bench && python3 impact_bench.py --binary /tmp/codeindex --lang py,js,php --out results/impact.json
cat impact-FINDINGS.md
```
Expected: prints an aggregate recall line; `impact-FINDINGS.md` and `results/impact.json` are written. Recall numbers reflect the resolver's real accuracy on the fixtures (they are evidence, not required to be 1.0).

- [ ] **Step 6: Commit**

```bash
git add bench/impact_bench.py bench/test_impact_bench.py bench/impact-FINDINGS.md bench/results/impact.json
git commit -m "feat(bench): orchestration, aggregation, CLI, and JSON/FINDINGS outputs"
```

---

### Task 7: Docs — wire impact_bench into bench/README

**Files:**
- Modify: `bench/README.md`

**Interfaces:**
- Consumes: the finished `impact_bench.py` CLI.
- Produces: nothing code-facing.

- [ ] **Step 1: Read the current bench/README.md**

Read `bench/README.md` to match its section style and heading depth.

- [ ] **Step 2: Add an `impact_bench.py` section**

Add a section describing: what it measures (impact-set recall vs. false positives), the hybrid oracle (compile-break for Go/TS real repos, authored fixtures for JS/Py/PHP + the ambiguity cases), the pre-registered bar (agg recall ≥ 0.95, per-lang ≥ 0.90, precision ungated), the CLI invocation examples (fixtures-only and with `--repo`/`--repo-lang`), and where outputs land (`bench/results/impact.json`, `bench/impact-FINDINGS.md`). Note the "missing toolchain → not run" behavior.

- [ ] **Step 3: Commit**

```bash
git add bench/README.md
git commit -m "docs(bench): document impact_bench accuracy benchmark"
```

---

## Self-Review

**1. Spec coverage:**
- "What it measures" (recall/precision, per-lang + aggregate, ambiguous subset) → Tasks 1, 6.
- CompileOracle (rename decl, `go build`/`tsc --noEmit`, parse diagnostics, restore via throwaway copy) → Task 4.
- FixtureOracle + `bench/impact_fixtures/<lang>/manifest.json` for all five, ambiguity only in fixtures → Task 3.
- Go/TS validation fixtures (assert the fixture breaks) → Task 3 (fixtures) + Task 4 (`CompileOracle` runs on them).
- Sampling: unique-named real-repo symbols, seeded → Task 5.
- Scoring normalization to `(file, enclosing-symbol)`; map compile site → enclosing symbol → Tasks 1 (`normalize_file`), 4 (`map_site_to_enclosing`).
- Pre-registered bar (0.95 / 0.90, precision ungated) → Task 6 (`aggregate`, `AGG_BAR`/`LANG_BAR`).
- Components (harness, oracle backends, impact runner, scorer, outputs JSON + FINDINGS) → Tasks 1–6.
- Error handling: missing toolchain → "not run" (Task 4/6); empty compiler truth → excluded (Task 6, `status: excluded_empty_truth`); rename collision → unique-name sampling prevents it (Task 5), residual skipped.
- Testing: golden-fixture scorer test with an ambiguous case → Task 1.
- CLI shape → Task 6.

**2. Placeholder scan:** No "TBD"/"add error handling"/"similar to Task N". Two intentional pointers require live inspection during build: (a) the exact SQLite schema column names in Task 5 — the plan says explicitly to copy them from `recall_bench.py`; (b) the four non-Python fixtures' concrete source in Task 3 — the schema + one fully-worked Python fixture are given, the rest replicate that shape. Both are called out, not hidden.

**3. Type consistency:** `Edge = (file, enclosing_symbol)` used uniformly; `Score` fields `tp/fn/fp` + `recall`/`precision` props consistent Tasks 1→6; `run_impact` returns `(impact, ambiguous)` consumed by `score_with_ambiguous` in `_run_symbol`; `FixtureOracle.symbols()` returns dicts with `symbol`/`ambiguous`/`truth` consumed in `main()`; `CompileOracle.truth_for(...)` returns `set|None` handled in `main()`; `aggregate` consumes the per-symbol record shape produced by `_run_symbol` + the real-repo/`not_run`/`excluded` records.

Plan complete.

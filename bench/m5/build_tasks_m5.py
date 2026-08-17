#!/usr/bin/env python3
"""M5 task generator — the go/no-go benchmark's three adversarial suites.

Suites (see README.md):
  grepwin   — grep-should-win: exact unique strings, filename lookup.
  dominate  — codeindex-should-dominate: widely-called symbols, test-surface
              discovery, two-level blast radius.
  break     — designed-to-break: same-name collisions (measures the
              false-confidence rate via a mandatory COVERAGE line).

Ground-truth provenance (recorded per task as gt_source):
  ripgrep / filesystem  — arm-neutral (grepwin_*, dominate_tests,
                          break_collision).
  graph.db              — dominate_callers (unambiguous name edges; same
                          hand-check-verified pattern as agent_ab
                          caller_attribution) and dominate_blast (level 1 =
                          unambiguous edges, level 2 = dst_symbol_id-resolved
                          edges). NOT arm-neutral; flagged in the header and
                          discounted accordingly in gate_m5.py's notes.

Usage:
  python3 build_tasks_m5.py [--repos gin,flask,nest,laravel-framework]
                            [--seed 20260816] [--per-repo-scale 1.0]
  python3 build_tasks_m5.py --selftest
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import random
import re
import sqlite3
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
BENCH = REPO_ROOT / "bench"
BIN = BENCH / "agent_ab" / ".bin" / "codeindex"
TASKS_OUT = HERE / "tasks" / "tasks_m5.json"

os.environ.setdefault("AB_WORK", str(BENCH / "repos"))
sys.path.insert(0, str(BENCH))
import token_bench as tb  # noqa: E402

LANG_MAP = {"py": "python", "go": "go", "ts": "ts", "php": "php"}

# Pre-registered M5 gates. Written verbatim into the tasks header; gate_m5.py
# reads them from there, never from its own source.
M5_GATES = {
    "go_gate": {
        "dominate_success_delta_min_pp": -5,
        "dominate_median_cost_savings_min_pct": 30,
        "dominate_median_processed_token_savings_min_pct": 50,
        "dominate_savings_rule": "cost>=30% OR processed_tokens>=50%",
        "dominate_recall_delta_min": 0.0,
        "grepwin_median_cost_regression_max_pct": 10,
        "break_false_confidence_delta_max_pp": 10,
        "note": "GO iff on the dominate suite arm B keeps success within 5pp "
                "of arm A, clears the savings rule, and does not lose recall; "
                "AND on grepwin arm B's median paired cost regresses <=10%; "
                "AND on break arm B's false-confidence rate exceeds arm A's "
                "by <=10pp.",
    },
    "kill_gate": {
        "c_success_delta_min_pp": -5,
        "c_cost_ratio_max": 1.10,
        "note": "KILL iff on the dominate suite arm C (cheap explorer + "
                "shell) reaches success within 5pp of arm B at <=1.10x arm "
                "B's median cost — the structural index then fails to justify "
                "its maintenance burden.",
    },
    "compound_gate": {
        "sx_vs_large_shell_success_delta_min_pp": -5,
        "sx_vs_small_shell_success_delta_min_pp": 10,
        "note": "Fuse-family (model-scale) gate. COMPOUND PASS iff on the "
                "dominate suite, within the fuse harness: small+index (SX) "
                "success >= large+shell (L) success - 5pp — the index "
                "substitutes for model scale — AND SX success >= small+shell "
                "(S) success + 10pp — the index, not the small model alone, "
                "closes the gap. Fuse-family arms (L/LX/S/SX) are NEVER "
                "compared to claude-CLI arms (A/B/C): different harness, "
                "different system prompts and tools. Token medians are the "
                "fuse-family cost proxy (gateway reports no $ cost).",
    },
    "min_pairs_per_verdict": 10,
    "false_confidence_def": "recall < 1.0 AND the answer's COVERAGE line is "
                            "not 'incomplete' (a missing COVERAGE line counts "
                            "as a completeness claim).",
}

COVERAGE_INSTR = (
    " End your answer with a line 'COVERAGE: complete' if you are confident "
    "you found every one, or 'COVERAGE: incomplete' if there may be more."
)

STRING_PROMPT = (
    "In the repository at {REPO_PATH}, exactly one line of code contains this "
    "exact string:\n\n  {literal}\n\nFind it. Answer with a single line "
    "'LOCATION: <repo-relative-file>:<line>', nothing else. Base it on the "
    "repository, not memory."
)

FILENAME_PROMPT = (
    "In the repository at {REPO_PATH} there is exactly one file named "
    "'{basename}'. Give its location. Answer with a single line "
    "'PATH: <repo-relative-path>', nothing else."
)

CALLERS_PROMPT = (
    "In the repository at {REPO_PATH}, find every function or method that "
    "CALLS '{symbol}' (exact name). Answer as a 'CALLERS:' section, one entry "
    "per line, exact form '<functionName>  <file>' (repo-relative), nothing "
    "else before the coverage line. Base every entry on evidence from the "
    "code." + COVERAGE_INSTR
)

TESTS_PROMPT = (
    "In the repository at {REPO_PATH}, which TEST files exercise the symbol "
    "'{symbol}' (reference it by name)? Answer as a 'FILES:' section listing "
    "repo-relative test file paths, nothing else. Base it on the repository."
)

BLAST_PROMPT = (
    "You are planning a change to '{symbol}' in the repository at {REPO_PATH}. "
    "Determine its blast radius: every file containing a function that calls "
    "'{symbol}' directly, plus every file containing a function that calls "
    "one of THOSE direct callers (two levels total). Answer as a 'FILES:' "
    "section of repo-relative paths, nothing else before the coverage line."
    + COVERAGE_INSTR
)

COLLISION_PROMPT = (
    "In the repository at {REPO_PATH}, the name '{symbol}' is defined in "
    "MORE THAN ONE place. Find every definition. Answer as a 'DEFINITIONS:' "
    "section, one '<repo-relative-file>:<line>' per line, nothing else before "
    "the coverage line." + COVERAGE_INSTR
)


def repo_path(name: str) -> Path:
    return Path(os.environ["AB_WORK"]) / name


def load_repos() -> dict:
    cfg = json.loads((BENCH / "repos.json").read_text())
    return {r["name"]: r for r in cfg["repos"]}


def _is_test(path: str, lang: str) -> bool:
    if lang == "go":
        return path.endswith("_test.go")
    if lang == "ts":
        return any(s in path for s in (".test.", ".spec.", "__tests__/",
                                       "/test/", "/tests/"))
    if lang == "php":
        return "/tests/" in path or "/Tests/" in path or path.endswith("Test.php")
    if lang == "python":
        base = path.rsplit("/", 1)[-1]
        return ("/tests/" in path or "/test/" in path
                or base.startswith("test_") or path.endswith("_test.py"))
    return False


def git_files(rp: Path) -> list[str]:
    out = subprocess.run(["git", "-C", str(rp), "ls-files"],
                         capture_output=True, text=True, check=True)
    return out.stdout.splitlines()


# --------------------------------------------------------------------------- #
# grepwin suite (arm-neutral gt: ripgrep / filesystem)
# --------------------------------------------------------------------------- #

# message-like literal: starts with a letter, has spaces, safe charset for
# embedding in a prompt and in `rg -F`
LITERAL_RE = re.compile(r'"([A-Za-z][A-Za-z0-9 _.,:%()/-]{24,70})"')


def grepwin_string_tasks(name: str, rp: Path, n: int, rng: random.Random,
                         lang: str) -> list[dict]:
    types = tb.LANG_DEFS[lang]["types"]
    lines = tb.rg_lines(types + ["-n", "-o", LITERAL_RE.pattern], rp)
    cands = {}
    for ln in lines:
        try:
            f, lno, m = ln.split(":", 2)
        except ValueError:
            continue
        lit = m.strip().strip('"')
        if " " not in lit or _is_test(f, lang):
            continue
        cands.setdefault(lit, []).append((f, int(lno)))
    uniq = [(lit, occ[0]) for lit, occ in cands.items() if len(occ) == 1]
    rng.shuffle(uniq)
    tasks = []
    for lit, (f, lno) in uniq:
        # verify uniqueness with a plain -F pass (the regex pass can miss
        # occurrences inside longer strings)
        hits = tb.rg_lines(["-n", "-F", f'"{lit}"'], rp)
        if len(hits) != 1:
            continue
        tasks.append({
            "id": f"m5-gwstr-{name}-{hashlib.sha1(lit.encode()).hexdigest()[:8]}",
            "type": "grepwin_string", "suite": "grepwin", "repo": name,
            "gt_source": "ripgrep",
            "prompt": STRING_PROMPT.format(REPO_PATH="{REPO_PATH}", literal=f'"{lit}"'),
            "ground_truth": {"location": f"{f}:{lno}", "literal": lit},
            "meta": {"file": f},
        })
        if len(tasks) >= n:
            break
    return tasks


def grepwin_filename_tasks(name: str, rp: Path, n: int,
                           rng: random.Random) -> list[dict]:
    files = git_files(rp)
    by_base: dict[str, list[str]] = {}
    for f in files:
        by_base.setdefault(f.rsplit("/", 1)[-1], []).append(f)
    uniq = [(b, ps[0]) for b, ps in by_base.items()
            if len(ps) == 1 and len(b) >= 10 and "/" in ps[0]
            and re.search(r"\.(go|ts|tsx|js|py|php)$", b)]
    rng.shuffle(uniq)
    tasks = []
    for base, path in uniq[:n]:
        tasks.append({
            "id": f"m5-gwfile-{name}-{hashlib.sha1(path.encode()).hexdigest()[:8]}",
            "type": "grepwin_filename", "suite": "grepwin", "repo": name,
            "gt_source": "filesystem",
            "prompt": FILENAME_PROMPT.format(REPO_PATH="{REPO_PATH}", basename=base),
            "ground_truth": {"path": path},
            "meta": {"basename": base},
        })
    return tasks


# --------------------------------------------------------------------------- #
# dominate suite
# --------------------------------------------------------------------------- #

def _graph(rp: Path) -> sqlite3.Connection:
    db = rp / ".codeindex" / "graph.db"
    if not db.exists():
        subprocess.run([str(BIN), "build", str(rp)], capture_output=True,
                       check=True)
    return sqlite3.connect(str(db))


def _unique_symbol_caller_pairs(con: sqlite3.Connection) -> dict[str, list]:
    """dst_name -> [(caller_name, caller_file)] over unambiguous edges,
    restricted to unique-name func/method targets."""
    rows = con.execute("""
        WITH uniq AS (
          SELECT name FROM symbols WHERE kind IN ('func','method')
          AND length(name) >= 8 GROUP BY name HAVING COUNT(*) = 1)
        SELECT e.dst_name, sc.name, sc.file
        FROM edges e JOIN symbols sc ON sc.id = e.src_symbol_id
        WHERE e.confidence = 'unambiguous'
          AND e.dst_name IN (SELECT name FROM uniq)
        GROUP BY e.dst_name, sc.name, sc.file""").fetchall()
    out: dict[str, list] = {}
    for dst, nm, f in rows:
        out.setdefault(dst, []).append((nm, f))
    return out


def dominate_callers_tasks(name: str, rp: Path, n: int,
                           rng: random.Random) -> list[dict]:
    con = _graph(rp)
    pairs = _unique_symbol_caller_pairs(con)
    con.close()
    cand = []
    for sym, lst in pairs.items():
        files = {f for _, f in lst}
        prod = [1 for _, f in lst if "_test" not in f and "/test" not in f]
        if 12 <= len(lst) <= 120 and len(files) >= 5 and sum(prod) >= 3:
            cand.append((sym, sorted(f"{nm}\t{f}" for nm, f in lst)))
    rng.shuffle(cand)
    return [{
        "id": f"m5-domcall-{name}-{sym}",
        "type": "dominate_callers", "suite": "dominate", "repo": name,
        "gt_source": "graph.db",
        "prompt": CALLERS_PROMPT.format(REPO_PATH="{REPO_PATH}", symbol=sym),
        "ground_truth": {"caller_pairs": lst},
        "meta": {"symbol": sym, "n_callers": len(lst)},
    } for sym, lst in cand[:n]]


def dominate_tests_tasks(name: str, rp: Path, n: int, rng: random.Random,
                         lang: str) -> list[dict]:
    con = _graph(rp)
    syms = [r[0] for r in con.execute(
        "SELECT name FROM symbols WHERE kind IN ('func','method') AND "
        "length(name) >= 6 AND file NOT LIKE '%test%' "
        "GROUP BY name HAVING COUNT(*) = 1").fetchall()]
    con.close()
    rng.shuffle(syms)
    tasks = []
    for sym in syms:
        refs = tb.rg_lines(["-w", "-l", "-F", sym], rp)
        tests = sorted({f for f in refs if _is_test(f, lang)})
        if not (3 <= len(tests) <= 25):
            continue
        tasks.append({
            "id": f"m5-domtest-{name}-{sym}",
            "type": "dominate_tests", "suite": "dominate", "repo": name,
            "gt_source": "ripgrep",
            "prompt": TESTS_PROMPT.format(REPO_PATH="{REPO_PATH}", symbol=sym),
            "ground_truth": {"files": tests},
            "meta": {"symbol": sym, "n_tests": len(tests)},
        })
        if len(tasks) >= n:
            break
    return tasks


def dominate_blast_tasks(name: str, rp: Path, n: int,
                         rng: random.Random) -> list[dict]:
    con = _graph(rp)
    uniq = [r for r in con.execute(
        "SELECT name FROM symbols WHERE kind IN ('func','method') AND "
        "length(name) >= 8 GROUP BY name HAVING COUNT(*) = 1").fetchall()]
    rng.shuffle(uniq)
    tasks = []
    for (sym,) in uniq:
        l1 = con.execute(
            "SELECT DISTINCT sc.id, sc.file FROM edges e "
            "JOIN symbols sc ON sc.id = e.src_symbol_id "
            "WHERE e.dst_name = ? AND e.confidence = 'unambiguous'",
            (sym,)).fetchall()
        if not (3 <= len(l1) <= 40):
            continue
        l1_ids = [r[0] for r in l1]
        q = ",".join("?" * len(l1_ids))
        l2 = con.execute(
            f"SELECT DISTINCT sc.file FROM edges e "
            f"JOIN symbols sc ON sc.id = e.src_symbol_id "
            f"WHERE e.dst_symbol_id IN ({q})", l1_ids).fetchall()
        files = sorted({f for _, f in l1} | {f for (f,) in l2})
        if not (6 <= len(files) <= 40):
            continue
        tasks.append({
            "id": f"m5-domblast-{name}-{sym}",
            "type": "dominate_blast", "suite": "dominate", "repo": name,
            "gt_source": "graph.db",
            "prompt": BLAST_PROMPT.format(REPO_PATH="{REPO_PATH}", symbol=sym),
            "ground_truth": {"files": files},
            "meta": {"symbol": sym, "n_l1": len(l1), "n_files": len(files)},
        })
        if len(tasks) >= n:
            break
    con.close()
    return tasks


# --------------------------------------------------------------------------- #
# break suite (arm-neutral gt: regex definition extraction, same source as the
# shipped comprehension tasks)
# --------------------------------------------------------------------------- #

def break_collision_tasks(name: str, rp: Path, n: int, rng: random.Random,
                          lang: str) -> list[dict]:
    symbols = tb.extract_symbols(rp, lang, cap=100000)
    by_name: dict[str, list] = {}
    for s in symbols:
        by_name.setdefault(s.name, []).append(s)
    cand = []
    for sym, defs in by_name.items():
        if len(sym) < 5 or not (2 <= len(defs) <= 6):
            continue
        nontest = [d for d in defs if not _is_test(d.file, lang)]
        if len(nontest) < 2 or len({d.file for d in defs}) < 2:
            continue
        cand.append((sym, defs))
    rng.shuffle(cand)
    return [{
        "id": f"m5-brkcol-{name}-{sym}",
        "type": "break_collision", "suite": "break", "repo": name,
        "gt_source": "ripgrep",
        "prompt": COLLISION_PROMPT.format(REPO_PATH="{REPO_PATH}", symbol=sym),
        "ground_truth": {"definitions": sorted(f"{d.file}:{d.line}" for d in defs)},
        "meta": {"symbol": sym, "n_defs": len(defs)},
    } for sym, defs in cand[:n]]


# --------------------------------------------------------------------------- #
# selftest
# --------------------------------------------------------------------------- #

def selftest():
    tb.RG = tb.resolve_rg(None)
    rng = random.Random(1)
    rp = repo_path("gin")
    assert rp.exists(), f"{rp} missing"
    t = grepwin_string_tasks("gin", rp, 2, rng, "go")
    assert t, "no unique string literals found in gin"
    for x in t:
        f, ln = x["ground_truth"]["location"].rsplit(":", 1)
        line = (rp / f).read_text().splitlines()[int(ln) - 1]
        assert x["ground_truth"]["literal"] in line, (x["id"], line)
    t = grepwin_filename_tasks("gin", rp, 2, rng)
    assert t and all((rp / x["ground_truth"]["path"]).exists() for x in t)
    t = dominate_callers_tasks("gin", rp, 2, rng)
    assert t and all(x["meta"]["n_callers"] >= 12 for x in t)
    t = dominate_tests_tasks("gin", rp, 2, rng, "go")
    assert t and all(f.endswith("_test.go")
                     for x in t for f in x["ground_truth"]["files"])
    t = dominate_blast_tasks("gin", rp, 2, rng)
    assert t and all(6 <= len(x["ground_truth"]["files"]) <= 40 for x in t)
    t = break_collision_tasks("gin", rp, 2, rng, "go")
    assert t
    for x in t:
        for d in x["ground_truth"]["definitions"]:
            f, ln = d.rsplit(":", 1)
            line = (rp / f).read_text().splitlines()[int(ln) - 1]
            assert x["meta"]["symbol"] in line, (x["id"], d, line)
    print("selftest OK: all six generators produce verifiable tasks on gin")


# --------------------------------------------------------------------------- #
# main
# --------------------------------------------------------------------------- #

# per-repo counts at scale 1.0 → 17/repo, 68 total over four repos
COUNTS = {"grepwin_string": 3, "grepwin_filename": 2, "dominate_callers": 4,
          "dominate_tests": 3, "dominate_blast": 2, "break_collision": 3}


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repos", default="gin,flask,nest,laravel-framework")
    ap.add_argument("--seed", type=int, default=20260816)
    ap.add_argument("--per-repo-scale", type=float, default=1.0)
    ap.add_argument("--out", default=str(TASKS_OUT))
    ap.add_argument("--selftest", action="store_true")
    args = ap.parse_args()

    tb.RG = tb.resolve_rg(None)
    if args.selftest:
        selftest()
        return

    repos = load_repos()
    rng = random.Random(args.seed)
    names = [n.strip() for n in args.repos.split(",") if n.strip()]
    scale = args.per_repo_scale

    all_tasks, pins = [], {}
    for name in names:
        info = repos[name]
        lang = LANG_MAP[info["lang"]]
        rp = repo_path(name)
        if not rp.exists():
            print(f"  ! {rp} missing; skipping {name}", file=sys.stderr)
            continue
        pins[name] = {"url": info["url"], "commit": info["commit"], "lang": lang}
        c = {k: max(1, round(v * scale)) for k, v in COUNTS.items()}
        got = {}
        for fn, key, extra in [
            (grepwin_string_tasks, "grepwin_string", (lang,)),
            (grepwin_filename_tasks, "grepwin_filename", ()),
            (dominate_callers_tasks, "dominate_callers", ()),
            (dominate_tests_tasks, "dominate_tests", (lang,)),
            (dominate_blast_tasks, "dominate_blast", ()),
            (break_collision_tasks, "break_collision", (lang,)),
        ]:
            t = fn(name, rp, c[key], rng, *extra)
            all_tasks.extend(t)
            got[key] = len(t)
        print(f"{name}: {got}")

    header = {
        "generated_seed": args.seed,
        "repo_pins": pins,
        "m5_gates": M5_GATES,
        "n_tasks": len(all_tasks),
        "by_suite": {s: sum(1 for t in all_tasks if t["suite"] == s)
                     for s in ("grepwin", "dominate", "break")},
        "by_type": {k: sum(1 for t in all_tasks if t["type"] == k)
                    for k in COUNTS},
        "gt_note": "gt_source=ripgrep/filesystem tasks are arm-neutral; "
                   "gt_source=graph.db tasks (dominate_callers, "
                   "dominate_blast) reuse the hand-check-verified "
                   "unambiguous-edge pattern from agent_ab and are NOT "
                   "arm-neutral — a codeindex recall bug would understate "
                   "arm A there. gate_m5.py surfaces this split.",
        "note": "Prompts contain the literal {REPO_PATH}; the runner "
                "substitutes the clone path at run time.",
    }
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps({"header": header, "tasks": all_tasks}, indent=2))
    sha = hashlib.sha1(out.read_bytes()).hexdigest()[:12]
    print(f"\nwrote {out}  ({len(all_tasks)} tasks, sha1 {sha})")


if __name__ == "__main__":
    main()

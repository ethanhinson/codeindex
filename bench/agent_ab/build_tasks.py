#!/usr/bin/env python3
"""Generate the frozen A/B task set (comprehension + localization) with
arm-neutral, script-computable ground truth. See the agent-ab-efficacy spec.

Comprehension ground truth comes from ripgrep (equally available to both arms),
never from codeindex. Localization ground truth comes from the merged PR that
closed a real issue (external to both arms).

Outputs bench/agent_ab/tasks/tasks.json with a header carrying seed, repo pins,
and the pre-registered GREEN/YELLOW/RED thresholds.

Prompts embed the literal token {REPO_PATH}; the runner substitutes the resolved
absolute clone path at run time (keeps tasks.json machine-independent).

Usage:
  python3 build_tasks.py [--comprehension-per-repo 8] [--localization-per-repo 4]
      [--seed 1729] [--repos gin,prometheus]
  python3 build_tasks.py --selftest        # unit-check ground-truth extraction
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
import urllib.request
from pathlib import Path

BIN = Path(__file__).resolve().parent / ".bin" / "codeindex"

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))  # import token_bench
import token_bench as tb  # noqa: E402

HERE = Path(__file__).resolve().parent
REPOS_JSON = HERE.parent / "repos.json"
CACHE = HERE / "cache"
TASKS_DIR = HERE / "tasks"

# Pre-registered decision thresholds — copied verbatim from design.md D7.
# Savings = median paired reduction in total_cost_usd (primary metric).
THRESHOLDS = {
    "metric": "median paired reduction in total_cost_usd",
    "green": "savings >= 30% AND success_delta >= -5pp AND adoption >= 70%",
    "yellow": "savings 10-30%, OR adoption 40-70% with per-protocol savings >= 30%",
    "red": "savings < 10% (ITT), OR success_delta < -5pp, OR adoption < 40% with per-protocol savings < 30%",
}

WORK_DEFAULT = os.environ.get(
    "AB_WORK",
    "/private/tmp/claude-501/-Users-ethanhinson-dev-code-indexer/"
    "b10fc4ca-af21-47ad-9b20-9ea114b75e7c/scratchpad/bench-repos",
)


# --------------------------------------------------------------------------- #
# Repo pins
# --------------------------------------------------------------------------- #

def load_repos():
    cfg = json.loads(REPOS_JSON.read_text())
    out = {}
    for r in cfg["repos"]:
        slug = re.sub(r"^https://github\.com/|\.git$", "", r["url"])
        out[r["name"]] = {"slug": slug, "commit": r["commit"], "lang": r["lang"]}
    return out


def repo_path(name: str) -> Path:
    return Path(WORK_DEFAULT) / name


# --------------------------------------------------------------------------- #
# Comprehension tasks (ripgrep ground truth)
# --------------------------------------------------------------------------- #

COMPREHENSION_PROMPT = (
    "In the repository at {REPO_PATH}, where is '{symbol}' defined? Give every "
    "definition as file:line. Then list every file (path relative to the repo "
    "root) that references '{symbol}'. Answer with a 'DEFINITIONS:' section of "
    "file:line entries and a 'FILES:' section of repo-relative paths, nothing "
    "else. Base every claim on evidence from the repository — do not answer "
    "from memory."
)


def referencing_files(rp: Path, name: str) -> list[str]:
    return sorted(set(tb.rg_lines(["-w", "-l", "-F", name], rp)))


# test-file heuristics per language — a def in one of these is excluded from
# comprehension targets (we want production symbols, not fixtures)
def _is_test_file(path: str, lang: str) -> bool:
    if lang == "go":
        return path.endswith("_test.go")
    if lang == "ts":
        return any(s in path for s in (".test.", ".spec.", "__tests__/", "/test/"))
    if lang == "php":
        return "/tests/" in path or "/Tests/" in path or path.endswith("Test.php")
    if lang == "python":
        return "/tests/" in path or "/test/" in path or "test_" in path.rsplit("/", 1)[-1] or path.endswith("_test.py")
    return False


def comprehension_tasks(name: str, rp: Path, n: int, rng: random.Random,
                        lang: str = "go") -> list[dict]:
    symbols = tb.extract_symbols(rp, lang)
    by_name: dict[str, list] = {}
    for s in symbols:
        by_name.setdefault(s.name, []).append(s)

    clean, hot = [], []
    for sym, defs in by_name.items():
        if len(sym) < 4:
            continue
        if not any(not _is_test_file(d.file, lang) for d in defs):
            continue  # must be defined in non-test code
        files = referencing_files(rp, sym)
        if not files:
            continue
        rec = (sym, defs, files)
        if len(defs) == 1 and 2 <= len(files) <= 20:
            clean.append(rec)
        elif len(defs) > 1 or len(files) > 20:
            hot.append(rec)

    rng.shuffle(clean)
    rng.shuffle(hot)
    n_hot = round(n * 0.3)
    picked = hot[:n_hot] + clean[: n - n_hot]
    rng.shuffle(picked)

    tasks = []
    for sym, defs, files in picked[:n]:
        tasks.append({
            "id": f"comp-{name}-{sym}",
            "type": "comprehension",
            "repo": name,
            "prompt": COMPREHENSION_PROMPT.format(REPO_PATH="{REPO_PATH}", symbol=sym),
            "ground_truth": {
                "definitions": sorted(f"{d.file}:{d.line}" for d in defs),
                "files": files,
            },
            "meta": {"symbol": sym, "n_defs": len(defs), "n_files": len(files),
                     "bucket": "hot" if (len(defs) > 1 or len(files) > 20) else "clean"},
        })
    return tasks


# --------------------------------------------------------------------------- #
# Caller-attribution tasks (v2) — the discriminating task type.
#
# "List the FUNCTIONS that call X." Grep gives file:line match locations but NOT
# the enclosing function name, so an agent must open many files to name callers;
# codeindex returns caller names directly. Ground truth is read straight from the
# index (graph.db), restricted to unambiguously-resolving unique-name targets so
# name-based resolution is exact (verified by hand-check on a sample).
# --------------------------------------------------------------------------- #

CALLER_PROMPT = (
    "In the repository at {REPO_PATH}, find every function or method that CALLS "
    "'{symbol}'. For each calling function, give its name and the file it is in. "
    "Answer as a 'CALLERS:' section with one entry per line in the exact form "
    "'<functionName>  <file>' (repo-relative path), nothing else. Base every "
    "entry on evidence from the code, not memory."
)


def ensure_index(rp: Path):
    if not (rp / ".codeindex" / "graph.db").exists():
        subprocess.run([str(BIN), "build", str(rp)], capture_output=True, check=True)


def caller_attribution_tasks(name: str, rp: Path, n: int, rng: random.Random) -> list[dict]:
    ensure_index(rp)
    con = sqlite3.connect(str(rp / ".codeindex" / "graph.db"))
    uniq = [r[0] for r in con.execute(
        "SELECT name FROM symbols WHERE kind IN('func','method') AND length(name)>=4 "
        "GROUP BY name HAVING COUNT(*)=1").fetchall()]
    cand = []
    for sym in uniq:
        rows = con.execute(
            "SELECT DISTINCT sc.name, sc.file FROM edges e "
            "JOIN symbols sc ON sc.id=e.src_symbol_id "
            "WHERE e.dst_name=? AND e.confidence='unambiguous'", (sym,)).fetchall()
        files = {f for _, f in rows}
        prod = [(nm, f) for nm, f in rows if not f.endswith("_test.go")]
        # 5-30 distinct (caller,file) pairs across >=3 files, with real prod callers
        if 5 <= len(rows) <= 30 and len(files) >= 3 and len(prod) >= 3:
            cand.append((sym, sorted(f"{nm}\t{f}" for nm, f in rows)))
    con.close()
    rng.shuffle(cand)
    tasks = []
    for sym, pairs in cand[:n]:
        tasks.append({
            "id": f"callattr-{name}-{sym}",
            "type": "caller_attribution",
            "repo": name,
            "prompt": CALLER_PROMPT.format(REPO_PATH="{REPO_PATH}", symbol=sym),
            "ground_truth": {"caller_pairs": pairs},
            "meta": {"symbol": sym, "n_callers": len(pairs)},
        })
    return tasks


# --------------------------------------------------------------------------- #
# Edit-impact tasks (v3) — refactor-flavored: modify a function, then report
# what's affected. Exercises the plugin's post-edit hook (arm B) and grades the
# affected-callers answer exactly like caller_attribution.
# --------------------------------------------------------------------------- #

EDIT_PROMPT = (
    "In the repository at {REPO_PATH}, add this comment line as the FIRST line "
    "inside the body of function '{symbol}': '// audited for error handling'. "
    "Make that one edit. Then determine which functions are affected by changes "
    "to '{symbol}' (i.e. every function or method that CALLS it) so a reviewer "
    "knows what to re-test. Answer as a 'CALLERS:' section, one entry per line, "
    "exact form '<functionName>  <file>' (repo-relative), nothing else. Base "
    "every entry on evidence from the code."
)


def edit_impact_tasks(name: str, rp: Path, n: int, rng: random.Random) -> list[dict]:
    base = caller_attribution_tasks(name, rp, n * 3, rng)  # reuse candidates
    tasks = []
    for t in base[:n]:
        sym = t["meta"]["symbol"]
        tasks.append({
            "id": f"editimp-{name}-{sym}",
            "type": "edit_impact",
            "repo": name,
            "prompt": EDIT_PROMPT.format(REPO_PATH="{REPO_PATH}", symbol=sym),
            "ground_truth": t["ground_truth"],
            "meta": t["meta"],
        })
    return tasks


# --------------------------------------------------------------------------- #
# v6 locate tasks
# --------------------------------------------------------------------------- #

import re as _re


def _tok(name):
    parts = _re.sub(r"([a-z0-9])([A-Z])", r"\1 \2", name)
    parts = _re.sub(r"([A-Z]+)([A-Z][a-z])", r"\1 \2", parts)
    parts = _re.sub(r"[_\-.$]", " ", parts)
    return [t.lower() for t in parts.split() if t]


VAGUE_PROMPT = (
    "In the repository at {REPO_PATH} there is a symbol whose name means "
    "roughly: \"{hint}\" (tokens may be reordered, incomplete, or use a "
    "synonym; naming convention unknown). Identify the exact symbol. Answer "
    "with 'NAME: <exactSymbolName>' and 'DEFINITION: <file>:<line>' on two "
    "lines, nothing else. Base it on the repository."
)


def vague_find_tasks(name: str, rp: Path, n: int, rng: random.Random) -> list[dict]:
    ensure_index(rp)
    con = sqlite3.connect(str(rp / ".codeindex" / "graph.db"))
    rows = con.execute("""
        SELECT s.name, s.file, s.start_line, COUNT(e.id) AS c FROM symbols s
        LEFT JOIN edges e ON e.dst_symbol_id = s.id
        WHERE s.tier=0 AND length(s.name) >= 8 AND s.file NOT LIKE '%_test%'
        GROUP BY s.id HAVING c >= 3""").fetchall()
    con.close()
    cands = [(nm, f, l) for nm, f, l, _c in rows if len(_tok(nm)) >= 3]
    # unique names only (unambiguous grading)
    seen = {}
    for nm, f, l in cands:
        seen.setdefault(nm, []).append((f, l))
    cands = [(nm, v[0][0], v[0][1]) for nm, v in seen.items() if len(v) == 1]
    # token-sets of ALL symbols (any tier), to reject ill-posed hints: a
    # dropped-token hint that exactly equals another real symbol's token set
    # makes that other symbol a legitimate answer.
    con = sqlite3.connect(str(rp / ".codeindex" / "graph.db"))
    all_sets = {}
    for (anm,) in con.execute("SELECT DISTINCT name FROM symbols"):
        all_sets.setdefault(frozenset(_tok(anm)), set()).add(anm)
    con.close()
    rng.shuffle(cands)
    tasks = []
    for nm, f, l in cands:
        if len(tasks) >= n:
            break
        toks = _tok(nm)
        drop = list(toks)
        drop.pop(rng.randrange(len(drop)))
        rng.shuffle(drop)
        hint = " ".join(drop)
        owners = all_sets.get(frozenset(drop), set())
        if owners - {nm}:
            continue  # ill-posed: hint exactly matches a different symbol
        tasks.append({
            "id": f"vague-{name}-{nm}",
            "type": "vague_find",
            "repo": name,
            "prompt": VAGUE_PROMPT.format(REPO_PATH="{REPO_PATH}", hint=hint),
            "ground_truth": {"name": nm, "definition": f"{f}:{l}"},
            "meta": {"symbol": nm, "hint": hint},
        })
    return tasks


OCCUR_PROMPT = (
    "In the repository at {REPO_PATH}, find every function or method that "
    "CALLS '{symbol}' (exact name; not similarly-named functions, not the "
    "definition itself). Answer as a 'CALLERS:' section, one entry per line, "
    "exact form '<functionName>  <file>' (repo-relative), nothing else. Base "
    "every entry on the code."
)


def occurrences_tasks(name: str, rp: Path, n: int, rng: random.Random) -> list[dict]:
    # Taxonomy note: despite the name, this type is semantically
    # CALLER-ATTRIBUTION — the prompt asks for "functions that CALL X" and gt
    # is caller_pairs (it reuses caller_attribution candidates verbatim). The
    # name is frozen because recorded runs (tasks_v6/lphp/nest, FINDINGS v10,
    # bench/scout) reference it. Literal-token-reference tasks live in
    # bench/scout/gen_tier1.py under the class "token_refs".
    base = caller_attribution_tasks(name, rp, n * 2, rng)
    out = []
    for t in base[:n]:
        sym = t["meta"]["symbol"]
        out.append({
            "id": f"occur-{name}-{sym}",
            "type": "occurrences",
            "repo": name,
            "prompt": OCCUR_PROMPT.format(REPO_PATH="{REPO_PATH}", symbol=sym),
            "ground_truth": t["ground_truth"],
            "meta": t["meta"],
        })
    return out


# --------------------------------------------------------------------------- #
# Localization tasks (merged-PR ground truth)
# --------------------------------------------------------------------------- #

LOCALIZATION_PROMPT = (
    "You are triaging a GitHub issue in the repository at {REPO_PATH}.\n\n"
    "ISSUE TITLE: {title}\n\nISSUE BODY:\n{body}\n\n"
    "Identify which files in the repository would need to change to fix this "
    "issue. Answer with a 'FILES:' section listing repo-relative paths, nothing "
    "else. Base your answer on evidence from the repository."
)

ISSUE_REF = re.compile(r"\b(?:fix(?:es|ed)?|close[sd]?|resolve[sd]?)\s+#(\d+)", re.I)


def api_get(url: str):
    CACHE.mkdir(parents=True, exist_ok=True)
    key = hashlib.sha1(url.encode()).hexdigest()
    cf = CACHE / f"{key}.json"
    if cf.exists():
        return json.loads(cf.read_text())
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "codeindex-ab"}
    tb.load_dotenv()
    tok = os.environ.get("GITHUB_TOKEN")
    if tok:
        headers["Authorization"] = f"Bearer {tok}"
    req = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(req, timeout=30) as r:
        data = json.loads(r.read())
    cf.write_text(json.dumps(data))
    return data


def localization_tasks(name: str, slug: str, rp: Path, want: int) -> list[dict]:
    tasks: list[dict] = []
    for page in range(1, 8):
        try:
            prs = api_get(f"https://api.github.com/repos/{slug}/pulls"
                          f"?state=closed&per_page=50&page={page}&sort=updated&direction=desc")
        except Exception as e:
            print(f"  ! PR fetch failed ({e})", file=sys.stderr)
            break
        if not prs:
            break
        for pr in prs:
            if not pr.get("merged_at"):
                continue
            refs = ISSUE_REF.findall((pr.get("title") or "") + "\n" + (pr.get("body") or ""))
            if not refs:
                continue
            try:
                files = api_get(f"https://api.github.com/repos/{slug}/pulls/{pr['number']}/files?per_page=100")
            except Exception:
                continue
            gofiles = [f["filename"] for f in files
                       if f["filename"].endswith(".go") and "vendor/" not in f["filename"]]
            if not (1 <= len(gofiles) <= 10):
                continue
            existing = [f for f in gofiles if (rp / f).exists()]
            if not existing or len(existing) < 0.6 * len(gofiles):
                continue
            try:
                issue = api_get(f"https://api.github.com/repos/{slug}/issues/{refs[0]}")
            except Exception:
                continue
            if "pull_request" in issue or not issue.get("title"):
                continue
            body = (issue.get("body") or "")[:2000]
            tasks.append({
                "id": f"loc-{name}-{issue['number']}",
                "type": "localization",
                "repo": name,
                "prompt": LOCALIZATION_PROMPT.format(
                    REPO_PATH="{REPO_PATH}", title=issue["title"], body=body),
                "ground_truth": {"files": sorted(existing)},
                "meta": {"issue": issue["number"], "pr": pr["number"],
                         "pr_go_files": len(gofiles), "existing": len(existing)},
            })
            if len(tasks) >= want:
                return tasks
    return tasks


# --------------------------------------------------------------------------- #
# Self-test (task 2.4)
# --------------------------------------------------------------------------- #

def selftest(repos):
    tb.RG = tb.resolve_rg(None)
    name = "gin"
    rp = repo_path(name)
    assert rp.exists(), f"{rp} missing — clone repos first"
    syms = tb.extract_symbols(rp, "go")
    by_name = {}
    for s in syms:
        by_name.setdefault(s.name, []).append(s)
    # RouterGroup is a known gin type.
    assert "RouterGroup" in by_name, "expected RouterGroup symbol"
    defs = by_name["RouterGroup"]
    assert any(d.file == "routergroup.go" for d in defs), defs
    files = referencing_files(rp, "RouterGroup")
    assert "routergroup.go" in files and len(files) >= 3, files
    print(f"selftest OK: RouterGroup {len(defs)} def(s), {len(files)} referencing files")
    print("  defs:", [f"{d.file}:{d.line}" for d in defs][:3])


# --------------------------------------------------------------------------- #
# Main
# --------------------------------------------------------------------------- #

def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--comprehension-per-repo", type=int, default=8)
    ap.add_argument("--localization-per-repo", type=int, default=4)
    ap.add_argument("--caller-attribution-per-repo", type=int, default=8)
    ap.add_argument("--edit-impact-per-repo", type=int, default=2)
    ap.add_argument("--vague-find-per-repo", type=int, default=4)
    ap.add_argument("--occurrences-per-repo", type=int, default=3)
    ap.add_argument("--v6-gate", action="store_true")
    ap.add_argument("--v3-gate", action="store_true",
                    help="stamp the v3 gate thresholds into the header")
    ap.add_argument("--types", default="comprehension,localization",
                    help="comma list of: comprehension,localization,caller_attribution")
    ap.add_argument("--seed", type=int, default=1729)
    ap.add_argument("--repos", default="gin,prometheus")
    ap.add_argument("--out", default=str(TASKS_DIR / "tasks.json"))
    ap.add_argument("--selftest", action="store_true")
    args = ap.parse_args()
    types = [t.strip() for t in args.types.split(",") if t.strip()]

    repos = load_repos()
    if args.selftest:
        selftest(repos)
        return

    tb.RG = tb.resolve_rg(None)
    rng = random.Random(args.seed)
    names = [n.strip() for n in args.repos.split(",") if n.strip()]

    all_tasks, pins = [], {}
    for name in names:
        info = repos[name]
        rp = repo_path(name)
        if not rp.exists():
            print(f"  ! {rp} missing; skipping {name}", file=sys.stderr)
            continue
        pins[name] = {"slug": info["slug"], "commit": info["commit"]}
        counts = {}
        if "comprehension" in types:
            t = comprehension_tasks(name, rp, args.comprehension_per_repo, rng,
                                    lang=info["lang"])
            all_tasks.extend(t); counts["comprehension"] = len(t)
        if "caller_attribution" in types:
            t = caller_attribution_tasks(name, rp, args.caller_attribution_per_repo, rng)
            all_tasks.extend(t); counts["caller_attribution"] = len(t)
        if "vague_find" in types:
            t = vague_find_tasks(name, rp, args.vague_find_per_repo, rng)
            all_tasks.extend(t); counts["vague_find"] = len(t)
        if "occurrences" in types:
            t = occurrences_tasks(name, rp, args.occurrences_per_repo, rng)
            all_tasks.extend(t); counts["occurrences"] = len(t)
        if "edit_impact" in types:
            t = edit_impact_tasks(name, rp, args.edit_impact_per_repo, rng)
            all_tasks.extend(t); counts["edit_impact"] = len(t)
        if "localization" in types:
            t = localization_tasks(name, info["slug"], rp, args.localization_per_repo)
            all_tasks.extend(t); counts["localization"] = len(t)
        print(f"{name}: {counts}")

    v3_gate = {
        "locate_regression_max_pct": 10,
        "branchout_savings_min_pct": 50,
        "hook_fire_rate_min_pct": 80,
        "hook_false_fires_max": 0,
        "note": "Pre-registered v3 gate (skill-and-ide-integration): locate tasks "
                "(comprehension type) must not regress >10% median paired cost; "
                "branch-out (caller_attribution) must retain >=50% median savings; "
                "hook must fire on >=80% of edit_impact arm-B runs and never on "
                "non-symbol edits.",
    } if args.v3_gate else None

    v6_gate = {
        "distinctive_regression_max_pct": 10,
        "vague_savings_min_pct": 30,
        "occurrences_savings_min_pct": 30,
        "note": "Pre-registered v6 gate (locate-and-enriched-grep): "
                "comprehension (distinctive) class must not regress >10%; "
                "vague_find and occurrences classes must save >=30% median "
                "paired cost; success delta >= -5pp.",
    } if args.v6_gate else None

    header = {
        "generated_seed": args.seed,
        "repo_pins": pins,
        "thresholds": THRESHOLDS,
        "v3_gate": v3_gate,
        "v6_gate": v6_gate,
        "n_tasks": len(all_tasks),
        "by_type": {t: sum(1 for x in all_tasks if x["type"] == t)
                    for t in ("comprehension", "localization", "caller_attribution",
                              "edit_impact", "vague_find", "occurrences")},
        "note": "Prompts contain the literal {REPO_PATH}; the runner substitutes "
                "the resolved absolute clone path. Ground truth is arm-neutral "
                "(ripgrep / merged-PR files).",
    }
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps({"header": header, "tasks": all_tasks}, indent=2))
    th = hashlib.sha1(out.read_bytes()).hexdigest()[:12]
    print(f"\nwrote {out}  ({len(all_tasks)} tasks, sha1 {th})")


if __name__ == "__main__":
    main()

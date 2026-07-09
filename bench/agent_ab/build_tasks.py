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
import sys
import urllib.request
from pathlib import Path

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


def comprehension_tasks(name: str, rp: Path, n: int, rng: random.Random) -> list[dict]:
    symbols = tb.extract_symbols(rp, "go")
    by_name: dict[str, list] = {}
    for s in symbols:
        by_name.setdefault(s.name, []).append(s)

    clean, hot = [], []
    for sym, defs in by_name.items():
        if len(sym) < 4:
            continue
        if not any(not d.file.endswith("_test.go") for d in defs):
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
    ap.add_argument("--seed", type=int, default=1729)
    ap.add_argument("--repos", default="gin,prometheus")
    ap.add_argument("--selftest", action="store_true")
    args = ap.parse_args()

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
        comp = comprehension_tasks(name, rp, args.comprehension_per_repo, rng)
        loc = localization_tasks(name, info["slug"], rp, args.localization_per_repo)
        print(f"{name}: {len(comp)} comprehension, {len(loc)} localization")
        all_tasks.extend(comp)
        all_tasks.extend(loc)

    header = {
        "generated_seed": args.seed,
        "repo_pins": pins,
        "thresholds": THRESHOLDS,
        "n_tasks": len(all_tasks),
        "by_type": {t: sum(1 for x in all_tasks if x["type"] == t)
                    for t in ("comprehension", "localization")},
        "note": "Prompts contain the literal {REPO_PATH}; the runner substitutes "
                "the resolved absolute clone path. Ground truth is arm-neutral "
                "(ripgrep / merged-PR files).",
    }
    TASKS_DIR.mkdir(parents=True, exist_ok=True)
    out = TASKS_DIR / "tasks.json"
    out.write_text(json.dumps({"header": header, "tasks": all_tasks}, indent=2))
    th = hashlib.sha1(out.read_bytes()).hexdigest()[:12]
    print(f"\nwrote {out}  ({len(all_tasks)} tasks, sha1 {th})")


if __name__ == "__main__":
    main()

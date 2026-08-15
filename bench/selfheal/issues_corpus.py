#!/usr/bin/env python3
"""Mine closed GitHub issues with fix commits into curated concept fixtures.

Closed issues whose fix commit we can locate are naturally-occurring concept
queries with ground truth: the issue TITLE is what a real user asked, and the
symbols touched by the fix commit are the acceptable answers.

Pipeline (deterministic, cached):
  1. Scan git log of the pinned bench clone for fix-ish subjects referencing
     an issue/PR number: fix/close/resolve ... (#N).
  2. Diff each candidate against its first parent; keep commits touching
     1-4 source files (docs/tests-only commits are dropped).
  3. MAPPING v2 (function-name-based): extract enclosing function/method
     names from git hunk-header context (xfuncname) and from added
     definition lines, then resolve name -> tier=0 symbols in the SAME
     changed file via the index db. Line-span overlap is used only as a
     fallback for commits whose hunks carry no function context, and only
     when the file is byte-identical between the commit and HEAD (so line
     numbers cannot have drifted); otherwise the question is dropped.
     Symbols become the accept set ("Name" or "Parent.Name").
  4. Fetch issue titles from the GitHub REST API (unauthenticated, budgeted,
     cached in .issue_cache.json). Titles that literally contain a mapped
     symbol name are skipped (locate-class, not concept-class).
  5. Emit fixtures in the curated format (issues_<repo>.json).

Usage: python3 bench/selfheal/issues_corpus.py
"""

from __future__ import annotations

import json
import os
import re
import sqlite3
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
BENCH = HERE.parent
CACHE_PATH = HERE / ".issue_cache.json"

TOTAL_REQUEST_BUDGET = 45  # unauthenticated (GitHub caps 60/hr/IP)
AUTHENTICATED_BUDGET = 2000  # with a PAT (GitHub caps 5000/hr)


def github_token() -> str:
    """GITHUB_TOKEN from the environment, falling back to the repo-root
    .env (KEY=VALUE lines; never committed)."""
    tok = os.environ.get("GITHUB_TOKEN", "").strip()
    if tok:
        return tok
    env_path = Path(__file__).resolve().parents[2] / ".env"
    try:
        for line in env_path.read_text().splitlines():
            line = line.strip()
            if line.startswith("GITHUB_TOKEN="):
                return line.split("=", 1)[1].strip().strip('"').strip("'")
    except OSError:
        pass
    return ""
SLEEP_BETWEEN_CALLS = 1.0
MAX_SOURCE_FILES = 4
MAX_ACCEPT_SYMBOLS = 10  # broader commits are too diffuse to be one question

REPOS = [
    {
        "name": "gin",
        "path": BENCH / "repos" / "gin",
        "github": "gin-gonic/gin",
        "lang": "go",
        "is_source": lambda p: p.endswith(".go") and not p.endswith("_test.go"),
    },
    {
        "name": "flask",
        "path": BENCH / "repos" / "flask",
        "github": "pallets/flask",
        "lang": "py",
        "is_source": lambda p: p.startswith("src/flask/") and p.endswith(".py"),
        # pre-1.1 layout used flask/ instead of src/flask/
        "normalize": lambda p: "src/" + p if p.startswith("flask/") else p,
    },
]

# fix-ish subject referencing an issue/PR number
FIXISH_RE = re.compile(r"\b(fix(e[sd])?|close[sd]?|resolve[sd]?)\b", re.I)
ISSUE_REF_RE = re.compile(r"#(\d+)")
# subjects that are clearly not behavior fixes (issue titles would be junk)
DENY_SUBJECT_RE = re.compile(
    r"\b(typo|docs?|comment|readme|changelog|spelling|grammar|lint|"
    r"format|chore|ci|codecov|badge|license|revert)\b", re.I)
# fetched titles that are not behavior questions (comment/style-only fixes)
DENY_TITLE_RE = re.compile(
    r"\b(typos?|docstrings?|annotations?|type hints?|style|spelling|"
    r"grammar|description error)\b", re.I)


def git(repo: Path, *args: str) -> str:
    r = subprocess.run(["git", "-C", str(repo), *args],
                       capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"git {args} failed: {r.stderr.strip()}")
    return r.stdout


def mine_candidates(repo: Path):
    """(sha, issue_number, subject) for fix-ish commits referencing #N."""
    out = []
    total = 0
    for line in git(repo, "log", "--format=%H\x01%s").splitlines():
        total += 1
        sha, _, subject = line.partition("\x01")
        if not FIXISH_RE.search(subject):
            continue
        m = ISSUE_REF_RE.search(subject)
        if not m:
            continue
        if DENY_SUBJECT_RE.search(subject):
            continue
        out.append((sha, int(m.group(1)), subject))
    return out, total


HUNK_RE = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@ ?(.*)")

# mapping v2: candidate function/method names come from git's hunk-header
# function context (xfuncname) and from added definition lines, not from
# line-number overlap against the (drifted) current checkout.
GO_DEF_RES = [
    re.compile(r"func\s*\(\s*\w+\s+\*?(\w+)\s*\)\s*(\w+)\s*[([]"),   # method
    re.compile(r"func\s+()(\w+)\s*[([]"),                            # func
    re.compile(r"type\s+()(\w+)\s+(?:struct|interface)\b"),          # type
]
PY_DEF_RES = [
    re.compile(r"(?:^|\s)def\s+()(\w+)\s*\("),
    re.compile(r"(?:^|\s)class\s+()(\w+)\b"),
]


def _def_names(text: str, lang: str):
    """(receiver_or_'', name) candidates found in a context/added line."""
    out = []
    for rx in (GO_DEF_RES if lang == "go" else PY_DEF_RES):
        for m in rx.finditer(text):
            out.append((m.group(1) or "", m.group(2)))
    return out


def changed_files(repo: Path, sha: str, is_source, normalize, lang):
    """Per source file: candidate def names + new-side changed line ranges.
    {file: {"names": [(recv,name)...], "ranges": [(lo,hi)...]}} or None if
    the commit touches no source file or more than MAX_SOURCE_FILES.

    Diffs against the first parent so merge commits (the common fix shape in
    flask: "Merge pull request #N ...") yield the PR's full diff instead of
    an empty combined diff."""
    try:
        show = git(repo, "diff", "--unified=0", f"{sha}^", sha)
    except RuntimeError:
        # shallow-history boundary commit: parent unavailable
        try:
            show = git(repo, "show", "--unified=0", "--pretty=format:", sha)
        except RuntimeError:
            return None
    files: dict[str, dict] = {}
    cur = None
    n_source = 0
    for line in show.splitlines():
        if line.startswith("+++ "):
            path = line[4:].strip()
            if path == "/dev/null":
                cur = None
                continue
            if path.startswith("b/"):
                path = path[2:]
            path = normalize(path)
            if is_source(path):
                cur = files.setdefault(path, {"names": [], "ranges": []})
                n_source += 1
            else:
                cur = None
        elif line.startswith("@@") and cur is not None:
            m = HUNK_RE.match(line)
            if not m:
                continue
            start = int(m.group(1))
            count = int(m.group(2)) if m.group(2) is not None else 1
            if count == 0:  # pure deletion: touch the surrounding line
                cur["ranges"].append((max(1, start), start + 1))
            else:
                cur["ranges"].append((start, start + count - 1))
            cur["names"].extend(_def_names(m.group(3), lang))
        elif cur is not None and line.startswith("+") and not line.startswith("+++"):
            cur["names"].extend(_def_names(line[1:], lang))
    if n_source == 0 or n_source > MAX_SOURCE_FILES:
        return None
    return files


def load_symbols(repo: Path):
    """tier=0 symbols grouped by file: {file: [(name,parent,start,end)]}"""
    db = repo / ".codeindex" / "graph.db"
    con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    by_file: dict[str, list] = {}
    for name, parent, file, s, e in con.execute(
            "SELECT name, parent, file, start_line, end_line "
            "FROM symbols WHERE tier=0"):
        by_file.setdefault(file, []).append((name, parent or "", s, e))
    con.close()
    return by_file


def _file_unchanged_since(repo: Path, sha: str, file: str, orig_path: str) -> bool:
    """True when the file content is byte-identical between <sha> and HEAD,
    so commit-time line numbers are exact against the current index."""
    for p in {file, orig_path}:
        try:
            if git(repo, "diff", "--name-only", sha, "HEAD", "--", p).strip() == "":
                # path may not have existed at sha (renamed layout) — require
                # it existed there, else the empty diff is vacuous
                git(repo, "cat-file", "-e", f"{sha}:{p}")
                return True
        except RuntimeError:
            continue
    return False


def map_symbols(repo: Path, sha: str, per_file: dict, sym_by_file: dict):
    """MAPPING v2. Accept entries + bare names.

    Primary: candidate def names (hunk-header xfuncname context + added
    definition lines) resolved against symbols in the SAME changed file.
    A (receiver, name) candidate prefers parent==receiver matches; a bare
    name matches any symbol of that name in the file.

    Fallback (only when a file yields no name candidates): line-span overlap
    as in v1, but only if the file is unchanged between the commit and HEAD —
    otherwise line numbers may have drifted and the file contributes nothing
    (precision over recall)."""
    accept, bare = [], set()

    def add(name, parent):
        entry = f"{parent}.{name}" if parent else name
        if entry not in accept:
            accept.append(entry)
            bare.add(name)

    for file, info in per_file.items():
        if not (repo / file).exists() or file not in sym_by_file:
            continue
        syms = sym_by_file[file]
        if info["names"]:
            for recv, cname in info["names"]:
                hits = [(n, p) for n, p, s, e in syms if n == cname]
                if recv:
                    recv_hits = [(n, p) for n, p in hits if p == recv]
                    hits = recv_hits or hits
                for n, p in hits:
                    add(n, p)
        elif info["ranges"]:
            orig = "flask/" + file[len("src/flask/"):] if file.startswith("src/flask/") else file
            if not _file_unchanged_since(repo, sha, file, orig):
                continue
            for n, p, s, e in syms:
                if s is None or e is None:
                    continue
                if any(s <= hi and e >= lo for lo, hi in info["ranges"]):
                    add(n, p)
    return accept, bare


class TitleFetcher:
    """Budget is LIFETIME across all runs: a persistent counter in the cache
    file ensures the corpus never issues more than TOTAL_REQUEST_BUDGET
    unauthenticated GitHub API calls, no matter how often it is rerun."""

    def __init__(self):
        self.cache = {}
        if CACHE_PATH.exists():
            self.cache = json.loads(CACHE_PATH.read_text())
        self.lifetime_requests = self.cache.get("__meta__", {}).get(
            "requests_made", 0)
        self.requests_made = 0  # this run, for reporting
        self.rate_limited = False

    def save(self):
        self.cache["__meta__"] = {"requests_made": self.lifetime_requests}
        CACHE_PATH.write_text(json.dumps(self.cache, indent=1, sort_keys=True))

    def fetch(self, gh_repo: str, number: int):
        """{'title':..., 'is_pr':..., 'state':...} or {'error':...} or None
        (budget exhausted / rate limited)."""
        key = f"{gh_repo}#{number}"
        if key in self.cache:
            return self.cache[key]
        budget = AUTHENTICATED_BUDGET if github_token() else TOTAL_REQUEST_BUDGET
        if self.rate_limited or self.lifetime_requests >= budget:
            return None
        url = f"https://api.github.com/repos/{gh_repo}/issues/{number}"
        headers = {
            "User-Agent": "codeindex-bench-issues-corpus",
            "Accept": "application/vnd.github+json",
        }
        token = github_token()
        if token:
            headers["Authorization"] = f"Bearer {token}"
        req = urllib.request.Request(url, headers=headers)
        self.requests_made += 1
        self.lifetime_requests += 1
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                data = json.loads(resp.read())
            entry = {
                "title": data.get("title", ""),
                "is_pr": "pull_request" in data,
                "state": data.get("state", ""),
            }
        except urllib.error.HTTPError as e:
            if e.code == 403:
                self.rate_limited = True
                print(f"  RATE LIMITED (403) after {self.requests_made} requests",
                      file=sys.stderr)
                self.save()
                return None
            entry = {"error": e.code}
        except Exception as e:  # network etc — cache nothing, skip
            print(f"  fetch error {key}: {e}", file=sys.stderr)
            self.save()
            return None
        self.cache[key] = entry
        self.save()
        time.sleep(SLEEP_BETWEEN_CALLS)
        return entry


def title_contains_symbol(title: str, bare_names: set[str]) -> bool:
    for name in bare_names:
        if re.search(r"(?<![A-Za-z0-9_])" + re.escape(name) + r"(?![A-Za-z0-9_])",
                     title, re.I):
            return True
    return False


def build_repo(cfg, fetcher: TitleFetcher, per_repo_fetch_budget: int):
    repo = cfg["path"]
    head = git(repo, "rev-parse", "HEAD").strip()
    sym_by_file = load_symbols(repo)

    candidates, total_commits = mine_candidates(repo)
    funnel = {
        "commits_scanned": total_commits,
        "fixish_with_issue_ref": len(candidates),
        "source_file_filter_pass": 0,
        "with_mapped_symbols": 0,
        "title_fetch_attempted": 0,
        "title_fetch_ok": 0,
        "final_questions": 0,
    }

    seen_issues = set()
    questions = []
    fetched_this_repo = 0
    for sha, number, subject in candidates:  # newest first
        if number in seen_issues:
            continue
        per_file = changed_files(repo, sha, cfg["is_source"],
                                 cfg.get("normalize", lambda p: p),
                                 cfg["lang"])
        if per_file is None:
            continue
        funnel["source_file_filter_pass"] += 1
        accept, bare = map_symbols(repo, sha, per_file, sym_by_file)
        if not accept or len(accept) > MAX_ACCEPT_SYMBOLS:
            continue
        funnel["with_mapped_symbols"] += 1
        seen_issues.add(number)

        cached = f"{cfg['github']}#{number}" in fetcher.cache
        if not cached and fetched_this_repo >= per_repo_fetch_budget:
            continue
        funnel["title_fetch_attempted"] += 1
        info = fetcher.fetch(cfg["github"], number)
        if not cached:
            fetched_this_repo += 1
        if info is None:
            # budget exhausted or rate limited: keep scanning so cached
            # titles are still used and the funnel stays deterministic
            continue
        if "error" in info or not info.get("title"):
            continue
        funnel["title_fetch_ok"] += 1
        title = info["title"].strip()
        if title_contains_symbol(title, bare):
            continue
        if DENY_TITLE_RE.search(title):
            continue
        questions.append({
            "q": title,
            "accept": accept,
            "_meta": {"sha": sha[:10], "issue": number,
                      "is_pr": info.get("is_pr"), "subject": subject},
        })

    funnel["final_questions"] = len(questions)
    fixture = {
        "repo": cfg["name"],
        "commit": head,
        "split": "issues-closed",
        "provenance": (
            "Mined from closed GitHub issues/PRs by bench/selfheal/issues_corpus.py "
            "(mapping v2) on 2026-07-12: fix-ish commit subjects referencing (#N) in "
            f"the deepened pinned clone (git log, {total_commits} commits, 1-4 source "
            "files, <=10 symbols); accept sets are tier=0 symbols resolved BY NAME "
            "from git hunk-header function context (xfuncname) and added definition "
            "lines, within the same changed file; line-span overlap used only for "
            "commits with no function context AND files byte-identical to HEAD "
            "(drift-proof). Questions are the verbatim GitHub issue/PR titles "
            "(REST API, cached in .issue_cache.json, zero new network calls); "
            "titles literally containing a mapped symbol name were dropped as "
            "locate-class."
        ),
        "questions": questions,
    }
    return fixture, funnel


def main():
    fetcher = TitleFetcher()
    budgets = {"gin": 22, "flask": 23}
    for cfg in REPOS:
        fixture, funnel = build_repo(cfg, fetcher, budgets[cfg["name"]])
        out = HERE / f"issues_{cfg['name']}.json"
        # strip _meta from the emitted fixture but keep a sidecar for audit
        meta = [q.pop("_meta") for q in fixture["questions"]]
        out.write_text(json.dumps(fixture, indent=1) + "\n")
        (HERE / f"issues_{cfg['name']}.meta.json").write_text(
            json.dumps(meta, indent=1) + "\n")
        print(f"== {cfg['name']} -> {out}")
        for k, v in funnel.items():
            print(f"   {k}: {v}")
    print(f"network requests made this run: {fetcher.requests_made}"
          f"{' (rate limited)' if fetcher.rate_limited else ''}")


if __name__ == "__main__":
    main()

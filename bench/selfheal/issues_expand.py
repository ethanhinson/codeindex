#!/usr/bin/env python3
"""Expanded issue-fix corpus miner (7 repos, mapping v2 + hygiene fixes).

Extends bench/selfheal/issues_corpus.py (mapping v2: git hunk-header
xfuncname names resolved against the index db) to seven pinned repos:

  tuning:   gin (go), flask (py), nest (ts), laravel-framework (php)
  held-out: prometheus (go), symfony (php), vscode (ts)   [split issues-heldout]

Three corpus-hygiene fixes over v2 (from issues_miss_analysis.md):
  (a) comment-only hunks do not attribute: a hunk whose added/removed lines
      are all comments/whitespace contributes neither names nor ranges;
  (b) title (and subject, pre-fetch) deny-filter with SUBSTRING matching for
      refactor/lint/typo/docstring/style/fmt/gofmt/golint/cleanup/annotation/
      spelling;
  (c) titles literally containing a mapped accept symbol name are skipped
      (locate-class, existing rule).

TS and PHP get language def regexes plus git diff drivers: builtin golang/
python/php drivers, and a custom xfuncname for *.ts, injected per-invocation
via core.attributesFile (never touching the pinned clones).

Frozen fixtures (issues_gin.json, issues_flask.json) are NOT touched; output
goes to bench/selfheal/issues_x/<repo>.json (+ .meta.json sidecar).

Usage:
  python3 bench/selfheal/issues_expand.py --dry-run [repo ...]  # map only
  python3 bench/selfheal/issues_expand.py [repo ...]            # full build
"""

from __future__ import annotations

import json
import re
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import issues_corpus as ic  # noqa: E402
from issues_corpus import (  # noqa: E402
    git, load_symbols, title_contains_symbol, TitleFetcher,
    HUNK_RE, FIXISH_RE, ISSUE_REF_RE, DENY_SUBJECT_RE, DENY_TITLE_RE,
    MAX_SOURCE_FILES, MAX_ACCEPT_SYMBOLS, _file_unchanged_since,
)

ic.SLEEP_BETWEEN_CALLS = 0.35  # PAT budget: 5000/hr; 0.35s is ~2800/hr max

BENCH = HERE.parent
OUT_DIR = HERE / "issues_x"
RUN_REQUEST_CAP = 1400  # hard cap on NEW api calls this run (limit: 1500)

# hygiene fix (b): substring deny list applied to commit subjects (pre-fetch,
# saves budget) and fetched titles
DENY_SUBSTRINGS = [
    "refactor", "lint", "typo", "docstring", "style", "fmt", "gofmt",
    "golint", "cleanup", "annotation", "spelling",
]


def deny_text(text: str) -> bool:
    tl = text.lower()
    return any(s in tl for s in DENY_SUBSTRINGS)


# ---------------------------------------------------------------- languages

TS_KEYWORDS = {
    "if", "for", "while", "switch", "catch", "return", "new", "typeof",
    "await", "super", "this", "else", "do", "throw", "delete", "void",
    "yield", "in", "of", "instanceof", "case", "break", "continue",
    "default", "export", "import", "function", "assert", "expect", "it",
    "describe", "require", "declare",
}

# strict: applied to added lines and hunk context
TS_STRICT_RES = [
    re.compile(r"\b(?:class|interface|enum|namespace)\s+([A-Za-z_$][\w$]*)"),
    re.compile(r"\bfunction\s*\*?\s+([A-Za-z_$][\w$]*)\s*[(<]"),
    re.compile(r"^\s*(?:(?:public|private|protected|static|readonly|abstract"
               r"|override|async|get|set)\s+)+([A-Za-z_$][\w$]*)\s*[(<]"),
    re.compile(r"^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*"
               r"=\s*(?:async\s*)?(?:\([^)]*\)\s*(?::[^=;]{0,80})?=>|function\b)"),
    re.compile(r"^\s*(constructor)\s*\("),
]
# loose: hunk-context lines only (git already chose them as enclosing);
# same-file symbol resolution is the final filter
TS_LOOSE_RES = [
    re.compile(r"^\s*([A-Za-z_$][\w$]*)\s*(?:<[^>]*>)?\([^;]*$"),
    re.compile(r"^\s*([A-Za-z_$][\w$]*)\s*\([^;)]*\)[^;]*\{\s*$"),
]

PHP_RES = [
    re.compile(r"\bfunction\s+&?([A-Za-z_]\w*)\s*\("),
    re.compile(r"\b(?:class|interface|trait|enum)\s+([A-Za-z_]\w*)"),
]

# custom xfuncname for *.ts (POSIX ERE; git has no builtin typescript driver)
TS_XFUNCNAME = (
    r"^[ \t]*(((export|default|abstract|declare)[ \t]+)*"
    r"(class|interface|namespace|enum)[ \t]+[A-Za-z_$].*"
    r"|((export|default|declare)[ \t]+)*(async[ \t]+)?function[ \t*]+"
    r"[A-Za-z_$][A-Za-z0-9_$]*.*"
    r"|((public|private|protected|static|readonly|abstract|override|async"
    r"|get|set)[ \t]+)+[A-Za-z_$][A-Za-z0-9_$]*[ \t]*[(<].*"
    r"|constructor[ \t]*\(.*"
    r"|[A-Za-z_$][A-Za-z0-9_$]*[ \t]*\([^;]*)$"
)

_ATTRS_FILE = None


def driver_args():
    """git -c args mapping languages to diff drivers (builtin golang/python/
    php; custom ts), via a temp attributes file so clones stay untouched."""
    global _ATTRS_FILE
    if _ATTRS_FILE is None:
        f = tempfile.NamedTemporaryFile(
            "w", suffix=".gitattributes", delete=False)
        f.write("*.go diff=golang\n*.py diff=python\n"
                "*.php diff=php\n*.ts diff=tsdrv\n")
        f.close()
        _ATTRS_FILE = f.name
    return ["-c", f"core.attributesFile={_ATTRS_FILE}",
            "-c", f"diff.tsdrv.xfuncname={TS_XFUNCNAME}"]


def def_names(text: str, lang: str, context: bool = False):
    """(receiver_or_'', name) candidates in a context/added line."""
    out = []
    if lang in ("go", "py"):
        rxs = ic.GO_DEF_RES if lang == "go" else ic.PY_DEF_RES
        for rx in rxs:
            for m in rx.finditer(text):
                out.append((m.group(1) or "", m.group(2)))
        return out
    if lang == "ts":
        rxs = TS_STRICT_RES + (TS_LOOSE_RES if context else [])
        for rx in rxs:
            for m in rx.finditer(text):
                name = m.group(1)
                if name not in TS_KEYWORDS:
                    out.append(("", name))
        return out
    if lang == "php":
        for rx in PHP_RES:
            for m in rx.finditer(text):
                out.append(("", m.group(1)))
        return out
    raise ValueError(lang)


def is_comment_line(s: str, lang: str) -> bool:
    s = s.strip()
    if not s:
        return True
    if lang == "py":
        return s.startswith("#") or s.startswith('"""') or s.startswith("'''")
    # go / ts / php
    if s.startswith("//") or s.startswith("/*") or s.startswith("*"):
        return True
    if lang == "php" and s.startswith("#"):
        return True
    return False


# ------------------------------------------------------------------- repos

def _mk(name, github, lang, is_source, split, normalize=None, denorm=None):
    return {
        "name": name, "path": BENCH / "repos" / name, "github": github,
        "lang": lang, "is_source": is_source, "split": split,
        "normalize": normalize or (lambda p: p),
        "denorm": denorm or (lambda p: p),
    }


XREPOS = [
    _mk("gin", "gin-gonic/gin", "go",
        lambda p: p.endswith(".go") and not p.endswith("_test.go"),
        "issues-closed"),
    _mk("flask", "pallets/flask", "py",
        lambda p: p.startswith("src/flask/") and p.endswith(".py"),
        "issues-closed",
        normalize=lambda p: "src/" + p if p.startswith("flask/") else p,
        denorm=lambda p: p[len("src/"):] if p.startswith("src/flask/") else p),
    _mk("nest", "nestjs/nest", "ts",
        lambda p: (p.startswith("packages/") and p.endswith(".ts")
                   and not p.endswith(".spec.ts") and not p.endswith(".d.ts")),
        "issues-closed"),
    _mk("laravel-framework", "laravel/framework", "php",
        lambda p: p.startswith("src/Illuminate/") and p.endswith(".php"),
        "issues-closed"),
    _mk("prometheus", "prometheus/prometheus", "go",
        lambda p: (p.endswith(".go") and not p.endswith("_test.go")
                   and not p.endswith(".pb.go")
                   and not p.startswith("vendor/")),
        "issues-heldout"),
    _mk("symfony", "symfony/symfony", "php",
        lambda p: (p.startswith("src/Symfony/") and p.endswith(".php")
                   and "/Tests/" not in p),
        "issues-heldout"),
    _mk("vscode", "microsoft/vscode", "ts",
        lambda p: (p.startswith("src/vs/") and p.endswith(".ts")
                   and not p.endswith(".test.ts") and not p.endswith(".d.ts")
                   and "/test/" not in p),
        "issues-heldout"),
]

# (min_target, stop_at, per-repo new-fetch cap)
TARGETS = {"issues-closed": (40, 60, 220), "issues-heldout": (30, 45, 180)}


def mine_candidates(repo: Path):
    """(sha, issue_number, subject) newest-first for fix-ish commits
    referencing #N; substring deny applied to subjects up front."""
    out, total = [], 0
    for line in git(repo, "log", "--format=%H\x01%s").splitlines():
        total += 1
        sha, _, subject = line.partition("\x01")
        if not FIXISH_RE.search(subject):
            continue
        m = ISSUE_REF_RE.search(subject)
        if not m:
            continue
        if DENY_SUBJECT_RE.search(subject) or deny_text(subject):
            continue
        out.append((sha, int(m.group(1)), subject))
    return out, total


def changed_files(repo: Path, sha: str, cfg):
    """Like issues_corpus.changed_files but: language diff drivers injected,
    per-language def regexes, and hygiene fix (a): comment-only hunks are
    dropped before they can attribute names or ranges."""
    lang = cfg["lang"]
    try:
        show = git(repo, *driver_args(), "diff", "--unified=0",
                   f"{sha}^", sha)
    except RuntimeError:
        try:
            show = git(repo, *driver_args(), "show", "--unified=0",
                       "--pretty=format:", sha)
        except RuntimeError:
            return None
    files: dict[str, dict] = {}
    cur = None
    n_source = 0
    hunk = None

    def flush():
        nonlocal hunk
        if hunk is not None and cur is not None:
            if not (hunk["lines"]
                    and all(is_comment_line(l, lang) for l in hunk["lines"])):
                cur["names"].extend(hunk["names"])
                cur["ranges"].append(hunk["range"])
        hunk = None

    for line in show.splitlines():
        if line.startswith("+++ "):
            flush()
            path = line[4:].strip()
            if path == "/dev/null":
                cur = None
                continue
            if path.startswith("b/"):
                path = path[2:]
            path = cfg["normalize"](path)
            if cfg["is_source"](path):
                cur = files.setdefault(path, {"names": [], "ranges": []})
                n_source += 1
            else:
                cur = None
        elif line.startswith("--- "):
            flush()
        elif line.startswith("@@") and cur is not None:
            flush()
            m = HUNK_RE.match(line)
            if not m:
                continue
            start = int(m.group(1))
            count = int(m.group(2)) if m.group(2) is not None else 1
            rng = ((max(1, start), start + 1) if count == 0
                   else (start, start + count - 1))
            hunk = {"range": rng, "lines": [],
                    "names": def_names(m.group(3), lang, context=True)}
        elif hunk is not None and cur is not None:
            if line.startswith("+") and not line.startswith("+++"):
                hunk["lines"].append(line[1:])
                hunk["names"].extend(def_names(line[1:], lang))
            elif line.startswith("-") and not line.startswith("---"):
                hunk["lines"].append(line[1:])
    flush()
    if n_source == 0 or n_source > MAX_SOURCE_FILES:
        return None
    return files


def map_symbols(repo: Path, sha: str, per_file: dict, sym_by_file: dict,
                denorm):
    """MAPPING v2 (as issues_corpus.map_symbols, denorm generalized)."""
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
            if not _file_unchanged_since(repo, sha, file, denorm(file)):
                continue
            for n, p, s, e in syms:
                if s is None or e is None:
                    continue
                if any(s <= hi and e >= lo for lo, hi in info["ranges"]):
                    add(n, p)
    return accept, bare


def build_repo(cfg, fetcher: TitleFetcher, dry_run=False):
    repo = cfg["path"]
    head = git(repo, "rev-parse", "HEAD").strip()
    sym_by_file = load_symbols(repo)
    min_target, stop_at, fetch_cap = TARGETS[cfg["split"]]

    candidates, total_commits = mine_candidates(repo)
    funnel = {
        "commits_scanned": total_commits,
        "fixish_with_issue_ref": len(candidates),
        "source_file_filter_pass": 0,
        "with_mapped_symbols": 0,
        "title_fetch_attempted": 0,
        "title_fetch_ok": 0,
        "denied_or_locate_class": 0,
        "final_questions": 0,
    }

    seen_issues, seen_titles = set(), set()
    questions = []
    samples = []  # dry-run: (file, accept) examples
    fetched_this_repo = 0
    for sha, number, subject in candidates:  # newest first
        if len(questions) >= stop_at:
            break
        if number in seen_issues:
            continue
        per_file = changed_files(repo, sha, cfg)
        if per_file is None:
            continue
        funnel["source_file_filter_pass"] += 1
        accept, bare = map_symbols(repo, sha, per_file, sym_by_file,
                                   cfg["denorm"])
        if not accept or len(accept) > MAX_ACCEPT_SYMBOLS:
            continue
        funnel["with_mapped_symbols"] += 1
        seen_issues.add(number)

        if dry_run:
            if len(samples) < 12:
                samples.append((sha[:10], subject[:70],
                                sorted(per_file), accept))
            if funnel["with_mapped_symbols"] >= 30:
                break
            continue

        cached = f"{cfg['github']}#{number}" in fetcher.cache
        if not cached and (fetched_this_repo >= fetch_cap
                           or fetcher.requests_made >= RUN_REQUEST_CAP):
            continue
        funnel["title_fetch_attempted"] += 1
        info = fetcher.fetch(cfg["github"], number)
        if not cached:
            fetched_this_repo += 1
        if info is None:
            continue
        if "error" in info or not info.get("title"):
            continue
        funnel["title_fetch_ok"] += 1
        title = info["title"].strip()
        if (title_contains_symbol(title, bare) or DENY_TITLE_RE.search(title)
                or deny_text(title) or title.lower() in seen_titles):
            funnel["denied_or_locate_class"] += 1
            continue
        seen_titles.add(title.lower())
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
        "split": cfg["split"],
        "provenance": (
            "Mined from closed GitHub issues/PRs by bench/selfheal/issues_expand.py "
            "(mapping v2 + hygiene fixes a/b/c) on 2026-07-12: fix-ish commit "
            f"subjects referencing (#N) in the deepened pinned clone ({total_commits} "
            "commits, newest-first, 1-4 source files, <=10 symbols); accept sets are "
            "tier=0 symbols resolved BY NAME from git hunk-header function context "
            "(builtin golang/python/php drivers, custom ts xfuncname) and added "
            "definition lines, within the same changed file; comment-only hunks do "
            "not attribute; line-span overlap only for hunks with no name context "
            "AND files byte-identical to HEAD. Questions are verbatim GitHub "
            "issue/PR titles (REST API, cached in .issue_cache.json); titles "
            "containing a mapped symbol name (locate-class) or matching the "
            "refactor/lint/typo/docstring/style/fmt/cleanup/annotation/spelling "
            "substring deny-list were dropped."
        ),
        "questions": questions,
    }
    return fixture, funnel, samples, min_target


def main():
    args = sys.argv[1:]
    dry_run = "--dry-run" in args
    names = [a for a in args if not a.startswith("--")]
    repos = [c for c in XREPOS if not names or c["name"] in names]

    fetcher = TitleFetcher()
    start_lifetime = fetcher.lifetime_requests
    OUT_DIR.mkdir(exist_ok=True)
    for cfg in repos:
        fixture, funnel, samples, min_target = build_repo(
            cfg, fetcher, dry_run=dry_run)
        print(f"== {cfg['name']} [{cfg['split']}]"
              f"{' (dry run)' if dry_run else ''}")
        for k, v in funnel.items():
            print(f"   {k}: {v}")
        if dry_run:
            for sha, subj, fs, acc in samples:
                print(f"   {sha} {subj}\n      files={fs}\n      accept={acc}")
            continue
        if funnel["final_questions"] < min_target:
            print(f"   WARNING: below target ({funnel['final_questions']}"
                  f" < {min_target})")
        out = OUT_DIR / f"{cfg['name']}.json"
        meta = [dict(q.pop("_meta"), q=q["q"]) for q in fixture["questions"]]
        out.write_text(json.dumps(fixture, indent=1) + "\n")
        (OUT_DIR / f"{cfg['name']}.meta.json").write_text(
            json.dumps(meta, indent=1) + "\n")
        print(f"   -> {out}")
    print(f"requests this run: {fetcher.requests_made}"
          f"{' (rate limited)' if fetcher.rate_limited else ''}; "
          f"lifetime: {start_lifetime} -> {fetcher.lifetime_requests}")


if __name__ == "__main__":
    main()

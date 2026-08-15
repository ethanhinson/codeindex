#!/usr/bin/env python3
"""Multi-hop navigation as DETERMINISTIC recipes — fixed compositions of
codeindex graph ops over the --json contract, no model, no policy.

The Scout thesis, hop two: the common multi-hop trajectories an agent runs
(impact, where-tested, rename-radius, dead-code) are not open-ended tool
policies — they are fixed recipes. `codeindex impact` and the
codeindex:impact skill already prove the pattern for one of them. Only the
residual (trajectories these recipes cannot express) can justify a learned
policy.

Each recipe returns a JSON-able dict; `python3 recipes.py <recipe> <repo>
<symbol>` prints it. Coverage measurement lives in measure_recipes.py.
"""
from __future__ import annotations
import json, re, subprocess, sys
from pathlib import Path

DEFAULT_BINARY = "codeindex"


def _run(binary, *args):
    # Recipes want the COMPLETE answer set; the CLI's display-oriented default
    # limits (30-50) silently truncate high-fanout symbols.
    out = subprocess.run([binary, *args, "--limit", "2000", "--json"],
                         capture_output=True, text=True).stdout
    try:
        return json.loads(out)
    except json.JSONDecodeError:
        return {}


def is_test_file(path: str) -> bool:
    """Language-agnostic test-file heuristics (superset of build_tasks.py's)."""
    p = path.lower()
    return (p.endswith("_test.go") or ".test." in p or ".spec." in p
            or "__tests__/" in p or "/test/" in p or "/tests/" in p
            or p.startswith(("test/", "tests/"))
            or p.rsplit("/", 1)[-1].startswith("test_")
            or p.endswith("_test.py") or p.endswith("test.php"))


def _word_sites(repo, symbol, groups):
    """Keep only grep groups whose file really contains `symbol` as a word.
    codeindex grep is substring-based ('Decode' hits 'NewDecoder'); rename and
    where-tested semantics are word-based. One cheap file read per candidate —
    deterministic glue, no model. Call-edge results are symbol-resolved
    upstream and never need this."""
    pat = re.compile(r"\b" + re.escape(symbol) + r"\b")
    checked: dict[str, bool] = {}
    out = []
    for g in groups:
        f = g["file"]
        if f not in checked:
            try:
                checked[f] = bool(pat.search((Path(repo) / f).read_text(errors="ignore")))
            except OSError:
                checked[f] = False
        if checked[f]:
            out.append(g)
    return out


def impact(binary, repo, symbol):
    """Refactor blast radius: defs + callers + dependents + callees.
    Single graph op — codeindex impact IS the recipe."""
    return _run(binary, "impact", repo, symbol)


def where_tested(binary, repo, symbol):
    """find impl -> its callers -> keep the ones in test files; grep fills in
    test files that exercise the symbol without a resolvable call edge
    (fixtures, dynamic dispatch, string-based registration)."""
    callers = _run(binary, "callers", repo, symbol)
    grep = _run(binary, "grep", repo, symbol)
    test_callers = [c for c in callers.get("callers", [])
                    if is_test_file(c["file"])]
    test_sites = _word_sites(repo, symbol,
                             [g for g in grep.get("groups", [])
                              if is_test_file(g["file"])])
    files = list(dict.fromkeys([c["file"] for c in test_callers]
                               + [g["file"] for g in test_sites]))
    return {"symbol": symbol,
            "definitions": callers.get("definitions", []),
            "test_callers": test_callers,     # graph-resolved call edges
            "test_sites": test_sites,         # grep-attributed occurrences
            "test_files": files}


def rename_radius(binary, repo, symbol):
    """Everything a rename of `symbol` must touch: every definition (all
    exact-name owners), every caller call-site, every literal token site
    (comments, strings, reflection), plus dep provenance flags — union of
    three ops the same way fmt_union builds the navigation answer."""
    callers = _run(binary, "callers", repo, symbol)
    find = _run(binary, "find", repo, symbol)
    grep = _run(binary, "grep", repo, symbol)
    exact = [r for r in find.get("results", []) if r.get("match") == "exact"]
    token_sites = _word_sites(repo, symbol, grep.get("groups", []))
    files = list(dict.fromkeys(
        [d["file"] for d in callers.get("definitions", [])]
        + [c["file"] for c in callers.get("callers", [])]
        + [g["file"] for g in token_sites]))
    return {"symbol": symbol,
            "definitions": callers.get("definitions", []) or exact,
            "exact_owners": exact,            # same bare name, other parents
            "call_sites": callers.get("callers", []),
            "token_sites": token_sites,
            "raw_token_hits": grep.get("raw_hits", 0),
            "files_to_touch": files,
            "dep_flagged": [r for r in find.get("results", []) if r.get("dep")]}


def dead_code(binary, repo, symbol):
    """Is `symbol` dead? No graph callers AND no token references outside its
    own definition file(s). Structural verdict only — exported/public symbols
    and dynamic dispatch need a human/agent confirmation pass (the
    trust-vs-verify residual), so the recipe reports evidence, not certainty."""
    callers = _run(binary, "callers", repo, symbol)
    grep = _run(binary, "grep", repo, symbol)
    def_files = {d["file"] for d in callers.get("definitions", [])}
    outside = _word_sites(repo, symbol,
                          [g for g in grep.get("groups", [])
                           if g["file"] not in def_files])
    verdict = (len(callers.get("callers", [])) == 0 and not outside)
    return {"symbol": symbol,
            "definitions": callers.get("definitions", []),
            "callers_total": callers.get("callers_total", 0),
            "token_sites_outside_def": outside,
            "structurally_dead": verdict,
            "caveat": "exported symbols / dynamic dispatch require verification"}


RECIPES = {"impact": impact, "where-tested": where_tested,
           "rename-radius": rename_radius, "dead-code": dead_code}


def main():
    if len(sys.argv) < 4 or sys.argv[1] not in RECIPES:
        sys.exit(f"usage: recipes.py <{'|'.join(RECIPES)}> <repo> <symbol> [binary]")
    recipe, repo, symbol = sys.argv[1:4]
    binary = sys.argv[4] if len(sys.argv) > 4 else DEFAULT_BINARY
    print(json.dumps(RECIPES[recipe](binary, repo, symbol), indent=2))


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""codeindex — token-savings validation harness (pre-implementation spike).

Goal: empirically test the core assumption behind codeindex *before* building the
engine — that answering navigation questions from a compact symbol index costs
far fewer tokens than the grep-and-read approach an agent uses today.

Why this is valid without the engine: the codeindex MVP resolves edges by symbol
*name*. So `ripgrep` grep-by-name is a faithful proxy for the index's edges, and
language-aware regexes are a faithful proxy for its symbol definitions. We are
measuring the token delta of the *output contract*, which is what actually saves
tokens.

For each sampled symbol we compare two strategies per question type:

  BASELINE (grep + read) — what a naive agent does: grep for the symbol, then
    read the matching files into context. Token cost = tokens of those files.
      * naive: read every file that mentions the symbol
      * smart: read only the top-K files by match count
  INDEX (codeindex) — compact `path:line  signature` references. Token cost =
    tokens of that reference list.

Question types (mirror the MVP query surface):
  def(X)      where X is defined + its signature   (baseline: read the file)
  callers(X)  who references X                       (baseline: read referencing files)
  outline(F)  all symbols in a file                  (baseline: read the whole file)

Token counting is pluggable (see count_tokens): Anthropic count_tokens API if
ANTHROPIC_API_KEY is set (accurate for Claude), else tiktoken cl100k_base, else a
char/4 heuristic. Ratios are robust to the counter; absolute counts are most
accurate via the API.

Usage:
  python3 token_bench.py --repos repos.json --only gin,prometheus,nest \
      --sample 40 --seed 7 --work <dir> --out results/summary.json
"""

from __future__ import annotations

import argparse
import glob
import hashlib
import json
import os
import random
import re
import shutil
import statistics
import subprocess
import sys
import time
from dataclasses import dataclass, field, asdict
from pathlib import Path

# --------------------------------------------------------------------------- #
# ripgrep resolution — must be a real binary (shells alias `rg` to a wrapper)
# --------------------------------------------------------------------------- #

RG = "rg"  # overwritten by resolve_rg()


def resolve_rg(override: str | None) -> str:
    if override:
        return override
    found = shutil.which("rg")
    if found:
        return found
    # Editors bundle a real ripgrep; find one for this arch.
    import platform

    arch = "arm64" if platform.machine() == "arm64" else "x64"
    patterns = [
        os.path.expanduser(f"~/.nvm/**/vendor/ripgrep/{arch}-darwin/rg"),
        os.path.expanduser("~/.vscode/**/@vscode/ripgrep/bin/rg"),
        f"/Applications/*.app/Contents/Resources/app/node_modules/@vscode/ripgrep/bin/rg",
    ]
    for pat in patterns:
        hits = glob.glob(pat, recursive=True)
        if hits:
            return hits[0]
    raise SystemExit("Could not find a real ripgrep binary; pass --rg /path/to/rg")

# --------------------------------------------------------------------------- #
# Token counting (pluggable)
# --------------------------------------------------------------------------- #

_ENC = None
_COUNTER_NAME = None
_COUNT_CACHE: dict[str, int] = {}
_ANTHROPIC_WARNED = False


def load_dotenv() -> None:
    """Load KEY=VALUE lines from bench/.env or the repo-root .env into the env.

    Existing environment variables win (setdefault), so an exported key is not
    overridden by the file.
    """
    here = Path(__file__).resolve().parent
    for p in (here / ".env", here.parent / ".env"):
        if not p.exists():
            continue
        for line in p.read_text().splitlines():
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, _, v = line.partition("=")
            os.environ.setdefault(k.strip(), v.strip().strip('"').strip("'"))


def _tiktoken():
    global _ENC
    if _ENC is None:
        import tiktoken

        _ENC = tiktoken.get_encoding("cl100k_base")
    return _ENC


def _init_counter() -> str:
    """Select the best available token counter, once."""
    global _COUNTER_NAME
    if _COUNTER_NAME is not None:
        return _COUNTER_NAME
    load_dotenv()
    # 1) Anthropic count_tokens (exact for Claude) — if a key is present.
    if os.environ.get("ANTHROPIC_API_KEY"):
        _COUNTER_NAME = "anthropic-count_tokens"
        return _COUNTER_NAME
    # 2) tiktoken cl100k_base — good proxy; ratios are tokenizer-robust.
    try:
        _tiktoken()
        _COUNTER_NAME = "tiktoken-cl100k_base"
        return _COUNTER_NAME
    except Exception:
        pass
    # 3) Heuristic fallback.
    _COUNTER_NAME = "heuristic-chars/4"
    return _COUNTER_NAME


def _anthropic_count(text: str) -> int:
    """Exact Claude token count via the REST count_tokens endpoint (no SDK)."""
    import urllib.request

    body = json.dumps({
        "model": os.environ.get("ANTHROPIC_COUNT_MODEL", "claude-sonnet-4-6"),
        "messages": [{"role": "user", "content": text}],
    }).encode("utf-8")
    req = urllib.request.Request(
        "https://api.anthropic.com/v1/messages/count_tokens",
        data=body, method="POST")
    req.add_header("x-api-key", os.environ["ANTHROPIC_API_KEY"])
    req.add_header("anthropic-version", "2023-06-01")
    req.add_header("content-type", "application/json")
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.loads(resp.read())["input_tokens"]


def count_tokens(text: str) -> int:
    name = _init_counter()
    if not text:
        return 0
    if name == "anthropic-count_tokens":
        key = hashlib.blake2b(text.encode("utf-8", "replace")).hexdigest()
        if key in _COUNT_CACHE:
            return _COUNT_CACHE[key]
        try:
            n = _anthropic_count(text)
        except Exception as e:  # fall back per-call, warn once
            global _ANTHROPIC_WARNED
            if not _ANTHROPIC_WARNED:
                print(f"  ! anthropic count_tokens failed ({e}); "
                      f"falling back to tiktoken for remaining counts",
                      file=sys.stderr)
                _ANTHROPIC_WARNED = True
            n = len(_tiktoken().encode(text, disallowed_special=()))
        _COUNT_CACHE[key] = n
        return n
    if name == "tiktoken-cl100k_base":
        return len(_tiktoken().encode(text, disallowed_special=()))
    return max(1, round(len(text) / 4))


# --------------------------------------------------------------------------- #
# Repo acquisition
# --------------------------------------------------------------------------- #


def run(cmd, cwd=None, check=True, capture=True, timeout=180):
    return subprocess.run(
        cmd, cwd=cwd, check=check, timeout=timeout,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
        text=True, errors="replace",  # tolerate non-UTF-8 bytes in scanned files
    )


def ensure_repo(spec: dict, work: Path) -> Path | None:
    """Shallow-clone the repo at its pinned commit/tag into work/<name>."""
    dest = work / spec["name"]
    if (dest / ".codeindex_cloned").exists():
        return dest
    dest.mkdir(parents=True, exist_ok=True)
    url, ref = spec["url"], spec["commit"]
    try:
        run(["git", "init", "-q"], cwd=dest)
        run(["git", "remote", "add", "origin", url], cwd=dest, check=False)
        # Try fetching the exact ref (tag or sha) shallowly.
        r = run(["git", "fetch", "--depth", "1", "-q", "origin", ref],
                cwd=dest, check=False)
        if r.returncode != 0:
            # Fall back to fetching the tag ref explicitly.
            run(["git", "fetch", "--depth", "1", "-q", "origin",
                 f"refs/tags/{ref}:refs/tags/{ref}"], cwd=dest, check=False)
        run(["git", "checkout", "-q", "FETCH_HEAD"], cwd=dest, check=False)
        (dest / ".codeindex_cloned").write_text("ok\n")
        return dest
    except subprocess.CalledProcessError as e:
        print(f"  ! clone failed for {spec['name']}: {e.stderr}", file=sys.stderr)
        return None


# --------------------------------------------------------------------------- #
# Symbol extraction (ripgrep + language regexes) — proxy for the index
# --------------------------------------------------------------------------- #

# Each entry: (rg type flags, python regex with a 'name' group) for definitions.
LANG_DEFS = {
    "go": {
        "types": ["-tgo"],
        "patterns": [
            re.compile(r"^\s*func\s+(?:\([^)]*\)\s*)?(?P<name>[A-Za-z_]\w*)\s*\("),
            re.compile(r"^\s*type\s+(?P<name>[A-Za-z_]\w*)\s"),
        ],
        "rg_def": r"^\s*(func|type)\s",
    },
    "ts": {
        "types": ["-tts", "-tjs"],
        "patterns": [
            re.compile(r"^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+(?P<name>[A-Za-z_$][\w$]*)\s*\("),
            re.compile(r"^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+(?P<name>[A-Za-z_$][\w$]*)"),
        ],
        "rg_def": r"^\s*(export\s+)?(default\s+)?(async\s+)?(abstract\s+)?(function|class)\s",
    },
}


@dataclass
class Symbol:
    name: str
    file: str      # repo-relative
    line: int
    signature: str
    kind: str


def rg_lines(args, cwd) -> list[str]:
    # timeout guards against a hung rg stalling a whole pipeline (observed once)
    try:
        r = run([RG, "--no-heading", "-n", *args], cwd=cwd, check=False)
    except Exception:
        return []
    if r.returncode not in (0, 1):  # 1 == no matches
        return []
    return r.stdout.splitlines()


def extract_symbols(repo: Path, lang: str, cap: int = 4000) -> list[Symbol]:
    cfg = LANG_DEFS[lang]
    out: list[Symbol] = []
    lines = rg_lines([*cfg["types"], "-e", cfg["rg_def"]], repo)
    for ln in lines:
        # format: path:line:content
        parts = ln.split(":", 2)
        if len(parts) < 3:
            continue
        path, lineno, content = parts
        for pat in cfg["patterns"]:
            m = pat.match(content)
            if m:
                sig = content.strip().rstrip("{").strip()
                out.append(Symbol(m.group("name"), path, int(lineno),
                                   sig[:200], "def"))
                break
        if len(out) >= cap:
            break
    return out


# --------------------------------------------------------------------------- #
# Measurement
# --------------------------------------------------------------------------- #


def file_tokens(repo: Path, rel: str, cache: dict) -> int:
    if rel in cache:
        return cache[rel]
    try:
        text = (repo / rel).read_text(errors="replace")
    except Exception:
        cache[rel] = 0
        return 0
    t = count_tokens(text)
    cache[rel] = t
    return t


def ref_files_for(repo: Path, name: str) -> list[tuple[str, int]]:
    """Files referencing `name`, with match counts (grep-by-name = MVP edges)."""
    lines = rg_lines(["-w", "-F", "--count", name], repo)
    res = []
    for ln in lines:
        # format: path:count
        i = ln.rfind(":")
        if i < 0:
            continue
        res.append((ln[:i], int(ln[i + 1:])))
    res.sort(key=lambda x: -x[1])
    return res


def _ref_to_obj(ref: str) -> dict:
    """`path:line  source` -> {path, line, src} for fair JSON token comparison."""
    head, _, src = ref.partition("  ")
    path, _, line = head.rpartition(":")
    return {"path": path, "line": int(line) if line.isdigit() else line, "src": src}


def caller_refs(repo: Path, name: str, limit: int) -> list[str]:
    """Compact `path:line  trimmed-source` reference list (index output)."""
    lines = rg_lines(["-w", "-F", "-n", name], repo)
    refs = []
    for ln in lines[:limit]:
        parts = ln.split(":", 2)
        if len(parts) < 3:
            continue
        path, lineno, content = parts
        refs.append(f"{path}:{lineno}  {content.strip()[:120]}")
    return refs


@dataclass
class QResult:
    qtype: str
    baseline_naive: int
    baseline_smart: int
    index_tokens: int
    index_json_tokens: int
    n_ref_files: int


def measure_symbol(repo: Path, sym: Symbol, limit: int, smart_k: int,
                   tok_cache: dict, syms_by_file: dict) -> list[QResult]:
    results = []

    # --- def(X): baseline reads the defining file; index = one signature line.
    def_index = f"{sym.file}:{sym.line}  {sym.signature}"
    def_baseline = file_tokens(repo, sym.file, tok_cache)
    results.append(QResult(
        "def", def_baseline, def_baseline,
        count_tokens(def_index),
        count_tokens(json.dumps({"path": sym.file, "line": sym.line,
                                 "signature": sym.signature})),
        1,
    ))

    # --- callers(X): baseline reads referencing files; index = ref list.
    ref_files = ref_files_for(repo, sym.name)
    if ref_files:
        naive = sum(file_tokens(repo, f, tok_cache) for f, _ in ref_files[:100])
        smart = sum(file_tokens(repo, f, tok_cache) for f, _ in ref_files[:smart_k])
        refs = caller_refs(repo, sym.name, limit)
        idx_text = "\n".join(refs)
        # JSON carries the SAME info as the text form, to fairly measure the
        # structured-output token overhead a MCP/IDE consumer would pay.
        idx_json = json.dumps([_ref_to_obj(r) for r in refs])
        results.append(QResult(
            "callers", naive, smart,
            count_tokens(idx_text), count_tokens(idx_json), len(ref_files),
        ))

    # --- outline(F): baseline reads the whole file; index = its def lines.
    file_syms = syms_by_file.get(sym.file, [])
    if file_syms:
        outline = "\n".join(f"{s.line}  {s.signature}" for s in file_syms)
        whole = file_tokens(repo, sym.file, tok_cache)
        results.append(QResult(
            "outline", whole, whole,
            count_tokens(outline),
            count_tokens(json.dumps([{"line": s.line, "sig": s.signature}
                                     for s in file_syms])),
            1,
        ))

    return results


# --------------------------------------------------------------------------- #
# Aggregation
# --------------------------------------------------------------------------- #


def ratio(a: int, b: int) -> float:
    return (a / b) if b else float("inf")


def summarize(qresults: list[QResult]) -> dict:
    by_type: dict[str, list[QResult]] = {}
    for q in qresults:
        by_type.setdefault(q.qtype, []).append(q)

    out = {}
    for qtype, items in by_type.items():
        naive_ratios = [ratio(q.baseline_naive, q.index_tokens) for q in items]
        smart_ratios = [ratio(q.baseline_smart, q.index_tokens) for q in items]
        idx_tokens = [q.index_tokens for q in items]
        finite_naive = [r for r in naive_ratios if r != float("inf")]
        finite_smart = [r for r in smart_ratios if r != float("inf")]
        out[qtype] = {
            "n": len(items),
            "median_ratio_naive": round(statistics.median(finite_naive), 1) if finite_naive else None,
            "median_ratio_smart": round(statistics.median(finite_smart), 1) if finite_smart else None,
            "pct_meeting_10x_smart": round(
                100 * sum(1 for r in smart_ratios if r >= 10) / len(items), 1),
            "median_index_tokens": int(statistics.median(idx_tokens)),
            "p90_index_tokens": int(sorted(idx_tokens)[int(0.9 * (len(idx_tokens) - 1))]),
            "median_index_json_tokens": int(statistics.median([q.index_json_tokens for q in items])),
            "median_baseline_smart_tokens": int(statistics.median([q.baseline_smart for q in items])),
        }
    return out


# --------------------------------------------------------------------------- #
# Main
# --------------------------------------------------------------------------- #


def bench_repo(spec: dict, work: Path, sample: int, seed: int, limit: int,
               smart_k: int) -> dict | None:
    print(f"\n=== {spec['name']} ({spec['lang']}, {spec.get('tier','?')}) ===")
    t0 = time.time()
    repo = ensure_repo(spec, work)
    if not repo:
        return None
    print(f"  cloned in {time.time()-t0:.1f}s -> {repo}")

    t1 = time.time()
    symbols = extract_symbols(repo, spec["lang"])
    print(f"  extracted {len(symbols)} symbol defs in {time.time()-t1:.1f}s")
    if not symbols:
        return None

    syms_by_file: dict[str, list[Symbol]] = {}
    for s in symbols:
        syms_by_file.setdefault(s.file, []).append(s)

    # Sample interesting symbols: dedupe by name, favor names >= 4 chars.
    rng = random.Random(seed)
    unique = {}
    for s in symbols:
        if len(s.name) >= 4 and s.name not in unique:
            unique[s.name] = s
    pool = list(unique.values())
    rng.shuffle(pool)
    picked = pool[:sample]
    print(f"  sampling {len(picked)} symbols")

    tok_cache: dict[str, int] = {}
    qresults: list[QResult] = []
    t2 = time.time()
    for i, sym in enumerate(picked):
        qresults.extend(measure_symbol(repo, sym, limit, smart_k,
                                       tok_cache, syms_by_file))
    print(f"  measured in {time.time()-t2:.1f}s")

    return {
        "repo": spec["name"],
        "lang": spec["lang"],
        "tier": spec.get("tier"),
        "commit": spec["commit"],
        "n_symbols_total": len(symbols),
        "n_sampled": len(picked),
        "summary": summarize(qresults),
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repos", default=str(Path(__file__).parent / "repos.json"))
    ap.add_argument("--only", default="", help="comma-separated repo names")
    ap.add_argument("--sample", type=int, default=40)
    ap.add_argument("--seed", type=int, default=7)
    ap.add_argument("--limit", type=int, default=50, help="max index refs per query")
    ap.add_argument("--smart-k", type=int, default=5, help="files a smart agent reads")
    ap.add_argument("--work", default=None, help="clone dir")
    ap.add_argument("--rg", default=None, help="path to a real ripgrep binary")
    ap.add_argument("--out", default=str(Path(__file__).parent / "results" / "summary.json"))
    args = ap.parse_args()

    global RG
    RG = resolve_rg(args.rg)
    print(f"ripgrep: {RG}")
    counter = _init_counter()
    print(f"token counter: {counter}")

    cfg = json.loads(Path(args.repos).read_text())
    only = {x for x in args.only.split(",") if x}
    repos = [r for r in cfg["repos"] if not only or r["name"] in only]

    work = Path(args.work) if args.work else Path(
        os.environ.get("TMPDIR", "/tmp")) / "codeindex-bench-repos"
    work.mkdir(parents=True, exist_ok=True)

    all_results = []
    for spec in repos:
        try:
            res = bench_repo(spec, work, args.sample, args.seed,
                             args.limit, args.smart_k)
            if res:
                all_results.append(res)
        except Exception as e:  # keep going across repos
            print(f"  ! error on {spec['name']}: {e}", file=sys.stderr)

    output = {"token_counter": counter, "config": {
        "sample": args.sample, "seed": args.seed, "limit": args.limit,
        "smart_k": args.smart_k}, "results": all_results}
    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    Path(args.out).write_text(json.dumps(output, indent=2))
    print(f"\nwrote {args.out}")
    print_report(output)


def print_report(output: dict):
    print("\n" + "=" * 78)
    print(f"TOKEN-SAVINGS REPORT  (counter: {output['token_counter']})")
    print("=" * 78)
    for res in output["results"]:
        print(f"\n{res['repo']}  [{res['lang']}, {res['tier']}]  "
              f"{res['n_symbols_total']} defs, {res['n_sampled']} sampled")
        print(f"  {'query':<9} {'nx':>7} {'smart×':>7} {'%≥10×':>6} "
              f"{'idx tok':>8} {'json tok':>9} {'baseline tok':>13}")
        for qtype, s in res["summary"].items():
            print(f"  {qtype:<9} {str(s['median_ratio_naive']):>7} "
                  f"{str(s['median_ratio_smart']):>7} "
                  f"{str(s['pct_meeting_10x_smart']):>6} "
                  f"{s['median_index_tokens']:>8} "
                  f"{s['median_index_json_tokens']:>9} "
                  f"{s['median_baseline_smart_tokens']:>13}")
    print("\nLegend: nx = median baseline(naive: read all referencing files) / index.")
    print("        smart× = median baseline(read top-K files) / index.")
    print("        idx tok = median tokens of the compact index answer (text).")


if __name__ == "__main__":
    main()

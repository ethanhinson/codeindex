#!/usr/bin/env python3
"""Mine the workspace-graph gate task set (openspec workspace-graph, design D7).

Open-source corpus, four language clusters (see corpus.json): a shared-lib
member and its consumer member(s) per language, every lib pin taken from what
its consumer declares. M2 miner discipline: ground truth is script-computable
from plain text search across member checkouts — equally available to both
arms, never from codeindex. Grading is set comparison over file paths (no
LLM grader → grader-blind structurally).

Mining is import-mediated per language (all tasks are rung-1; this corpus
has no organic rung-2 bare-name edges — recorded in the header):

  PHP  use Lib\\Ns\\Class;  or inline FQCN
  TS   import { Name } from '@lib/pkg'
  Py   from lib.module import Name
  Go   import "module/path/pkg" + pkg.Name usages

Tasks are scoped CROSS-MEMBER — "which files in the OTHER member projects
reference X" — the exact frontier question, and the scope keeps ground truth
honest without resolving each language's internal-reference idioms
(relative imports, same-package calls).

  xcallers    — files in other members that reference <symbol>
  ximpact     — cross-member direct blast radius (+ definition file)
  xnew        — files in other members that instantiate it (php/ts)
  xsubtypes   — files in other members that extend/implement it (php/ts/py)

xnew/xsubtypes are emitted only when their answer is a proper subset of the
xcallers set (a genuinely different answer, not a rephrasing).

Prompts embed {WS_ROOT}; the runner substitutes the workspace root path.

Usage:
  python3 build_tasks_ws.py [--seed 1729] [--min-tasks 30] [--out tasks/tasks_ws.json]
  python3 build_tasks_ws.py --selftest
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
CORPUS = HERE / "corpus.json"

EXCLUDE_DIRS = {"vendor", "node_modules", ".git", "dist", "build",
                ".venv", "venv", "__pycache__", "testdata", "fixtures",
                "Fixtures", "fixture"}
EXT = {"php": [".php"], "ts": [".ts"], "py": [".py"], "go": [".go"]}
PER_LIB_PRIMARY_CAP = 12
MIN_PER_LANG = 4
MAX_GT_FILES = 40  # bigger answers measure listing stamina, not finding


def load_members():
    cfg = json.loads(CORPUS.read_text())
    ws_root = (HERE.parent.parent / cfg["workspace_root"]).resolve()
    ws_root.mkdir(parents=True, exist_ok=True)
    members = []
    for m in cfg["members"]:
        root = (ws_root / m["root"]).resolve()
        if not root.is_dir():
            sys.exit(f"member {m['id']}: missing checkout {root} "
                     f"(clone per corpus.json)")
        members.append({**m, "root": root})
    return ws_root, members


def member_files(member):
    """[(member-relative path, text)] for the member's own sources."""
    exts = {e for lang in member["lang"] for e in EXT[lang]}
    out = []
    for p in member["root"].rglob("*"):
        if p.suffix not in exts or not p.is_file():
            continue
        rel = p.relative_to(member["root"])
        if set(rel.parts[:-1]) & EXCLUDE_DIRS:
            continue
        if p.suffix == ".ts" and rel.name.endswith(".d.ts"):
            continue
        try:
            out.append((rel.as_posix(), p.read_text(errors="replace")))
        except OSError:
            continue
    out.sort()
    return out


# --------------------------------------------------------------------------- #
# Reference extraction (consumer side) — symbol keys are per-language:
#   php: FQCN            ts: pkg:Name          py: module.Name
#   go:  importpath.Name
# --------------------------------------------------------------------------- #

PHP_USE = re.compile(r"^use\s+([\w\\]+?)(?:\s+as\s+\w+)?\s*;", re.M)
TS_IMPORT = re.compile(
    r"import\s+(?:type\s+)?\{([^}]+)\}\s+from\s+['\"]([^'\"]+)['\"]", re.S)
PY_FROM = re.compile(
    r"^from\s+([\w.]+)\s+import\s+(\([^)]*\)|[^\n]+)", re.M)
GO_IMPORT_BLOCK = re.compile(r"import\s*\(([^)]*)\)", re.S)
GO_IMPORT_ONE = re.compile(r'^import\s+(?:(\w+)\s+)?"([^"]+)"', re.M)
GO_IMPORT_LINE = re.compile(r'^\s*(?:(\w+)\s+)?"([^"]+)"', re.M)


def php_refs(text, prefixes):
    out = set()
    for fq in PHP_USE.findall(text):
        if any(fq.startswith(p) for p in prefixes):
            out.add(fq)
    for p in prefixes:
        for m in re.finditer(re.escape(p) + r"(?:\w+\\)*\w+", text):
            out.add(m.group(0))
    return out


def ts_refs(text, pkgs):
    out = set()
    for names, src in TS_IMPORT.findall(text):
        if src not in pkgs and not any(src.startswith(p + "/") for p in pkgs):
            continue
        pkg = src if src in pkgs else next(
            p for p in pkgs if src.startswith(p + "/"))
        for raw in names.split(","):
            n = raw.strip().removeprefix("type ").split(" as ")[0].strip()
            if n.isidentifier():
                out.add(f"{pkg}:{n}")
    return out


def py_refs(text, pkgs):
    out = set()
    for mod, names in PY_FROM.findall(text):
        if mod.split(".")[0] not in pkgs:
            continue
        for raw in names.strip("()").replace("\\\n", ",").split(","):
            n = raw.strip().split(" as ")[0].strip()
            if n.isidentifier() and not n.startswith("_"):
                out.add(f"{mod}.{n}")
    return out


def go_refs(text, modpaths):
    imports = []  # (alias, path)
    for block in GO_IMPORT_BLOCK.findall(text):
        imports += GO_IMPORT_LINE.findall(block)
    imports += GO_IMPORT_ONE.findall(text)
    out = set()
    for alias, path in imports:
        if not any(path == mp or path.startswith(mp + "/")
                   for mp in modpaths):
            continue
        name = alias or path.rsplit("/", 1)[-1]
        for m in re.finditer(r"\b" + re.escape(name) + r"\.([A-Z]\w*)", text):
            out.add(f"{path}.{m.group(1)}")
    return out


def extract_refs(text, lang, lib_namespaces):
    if lang == "php":
        return php_refs(text, lib_namespaces)
    if lang == "ts":
        return ts_refs(text, lib_namespaces)
    if lang == "py":
        return py_refs(text, lib_namespaces)
    if lang == "go":
        return go_refs(text, lib_namespaces)
    return set()


# --------------------------------------------------------------------------- #
# Definition lookup (lib side)
# --------------------------------------------------------------------------- #

PHP_NS = re.compile(r"^namespace\s+([\w\\]+)\s*;", re.M)
PHP_DECL = re.compile(
    r"^\s*(?:final\s+|abstract\s+|readonly\s+)*(?:class|interface|trait|enum)"
    r"\s+(\w+)", re.M)
TS_EXPORT = re.compile(
    r"^export\s+(?:declare\s+)?(?:abstract\s+)?"
    r"(?:class|interface|function|const|enum|type)\s+(\w+)", re.M)
PY_DEF = re.compile(r"^(?:class|def)\s+(\w+)", re.M)
GO_DEF = re.compile(r"^(?:func|type|var)\s+(\w+)", re.M)


def lib_definitions(member, texts):
    """symbol-key -> member-relative def file."""
    lang = member["lang"][0]
    defs = {}
    if lang == "php":
        for rel, text in texts:
            ns = PHP_NS.search(text)
            prefix = ns.group(1) + "\\" if ns else ""
            for name in PHP_DECL.findall(text):
                defs.setdefault(prefix + name, rel)
    elif lang == "ts":
        pkg = member["namespaces"][0]
        for rel, text in texts:
            for name in TS_EXPORT.findall(text):
                defs.setdefault(f"{pkg}:{name}", rel)
    elif lang == "py":
        pkg = member["namespaces"][0]
        # prefer real definition files over __init__ re-exports
        for init_last in (False, True):
            for rel, text in texts:
                if rel.endswith("__init__.py") != init_last:
                    continue
                for name in PY_DEF.findall(text):
                    defs.setdefault(name, rel)  # bare name; modules re-export
    elif lang == "go":
        mod = member["namespaces"][0]
        for rel, text in texts:
            pkg_dir = str(Path(rel).parent)
            path = mod if pkg_dir == "." else f"{mod}/{pkg_dir}"
            for name in GO_DEF.findall(text):
                defs.setdefault(f"{path}.{name}", rel)
    return defs


def find_def(symbol, lang, defs):
    if lang == "py":
        return defs.get(symbol.rsplit(".", 1)[-1])
    return defs.get(symbol)


# --------------------------------------------------------------------------- #
# Sub-kind criteria (within an already-referencing file)
# --------------------------------------------------------------------------- #

def sub_pattern(kind, lang, bare):
    if kind == "xnew" and lang in ("php", "ts"):
        return re.compile(r"new\s+\\?(?:[\w\\.]+[\\.])?" + re.escape(bare)
                          + r"\s*[(<]")
    if kind == "xsubtypes":
        if lang in ("php", "ts"):
            return re.compile(
                r"(?:extends|implements)[^{;]*?\b" + re.escape(bare) + r"\b")
        if lang == "py":
            return re.compile(r"^class\s+\w+\([^)]*\b" + re.escape(bare)
                              + r"\b[^)]*\)", re.M)
    return None


PROMPT_TAIL = (
    " Search each project's own source only (ignore any vendor/, "
    "node_modules/, dist/ or build/ directories). Output ONLY the file "
    "paths relative to {WS_ROOT}, one per line, nothing else."
)

PROMPTS = {
    "xcallers": (
        "You are working in a multi-repo workspace rooted at {WS_ROOT}. "
        "Member projects (relative to that root): {MEMBERS}. "
        "{SYMDESC} is defined in the {LIB} project. List every file in the "
        "OTHER member projects (not {LIB}) that references it." + PROMPT_TAIL),
    "ximpact": (
        "You are working in a multi-repo workspace rooted at {WS_ROOT}. "
        "Member projects (relative to that root): {MEMBERS}. "
        "{SYMDESC} (defined in the {LIB} project) is being changed "
        "incompatibly. List every file that would be directly affected "
        "OUTSIDE {LIB}: its definition file plus every file in the other "
        "member projects that references it." + PROMPT_TAIL),
    "xnew": (
        "You are working in a multi-repo workspace rooted at {WS_ROOT}. "
        "Member projects (relative to that root): {MEMBERS}. "
        "{SYMDESC} is defined in the {LIB} project. List every file in the "
        "OTHER member projects (not {LIB}) that instantiates it with "
        "`new`." + PROMPT_TAIL),
    "xsubtypes": (
        "You are working in a multi-repo workspace rooted at {WS_ROOT}. "
        "Member projects (relative to that root): {MEMBERS}. "
        "{SYMDESC} is defined in the {LIB} project. List every file in the "
        "OTHER member projects (not {LIB}) that declares a class extending "
        "or implementing it." + PROMPT_TAIL),
}

SYMDESC = {
    "php": "The PHP class/interface `{sym}`",
    "ts": "The TypeScript export `{name}` from `{pkg}`",
    "py": "The Python name `{name}` (importable as `{sym}`)",
    "go": "The Go identifier `{name}` from package `{pkg}`",
}


def describe(symbol, lang):
    if lang == "php":
        return SYMDESC["php"].format(sym=symbol)
    if lang == "ts":
        pkg, name = symbol.split(":", 1)
        return SYMDESC["ts"].format(name=name, pkg=pkg)
    if lang == "py":
        mod, name = symbol.rsplit(".", 1)
        return SYMDESC["py"].format(name=name, sym=symbol)
    pkg, name = symbol.rsplit(".", 1)
    return SYMDESC["go"].format(name=name, pkg=pkg)


def bare_name(symbol, lang):
    if lang == "php":
        return symbol.rsplit("\\", 1)[-1]
    if lang == "ts":
        return symbol.split(":", 1)[-1]
    return symbol.rsplit(".", 1)[-1]


def ws_rel(ws_root, member, rel):
    import os
    base = os.path.relpath(member["root"], ws_root)
    return f"{base}/{rel}"


def mine(seed, min_tasks):
    import random
    ws_root, members = load_members()
    libs = [m for m in members if "shared lib" in m["role"]]
    texts = {m["id"]: member_files(m) for m in members}
    member_rels = {m["id"]: ws_rel(ws_root, m, "").rstrip("/")
                   for m in members}

    # one pass over every non-lib-member file per lib: symbol -> refs
    candidates = []
    for lib in libs:
        lang = lib["lang"][0]
        defs = lib_definitions(lib, texts[lib["id"]])
        refs = {}  # symbol -> {member-id: [files]}
        for m in members:
            if m["id"] == lib["id"] or lang not in m["lang"]:
                continue
            for rel, text in texts[m["id"]]:
                for sym in extract_refs(text, lang, lib["namespaces"]):
                    refs.setdefault(sym, {}).setdefault(
                        m["id"], []).append(rel)
        for sym, by_member in sorted(refs.items()):
            def_rel = find_def(sym, lang, defs)
            if def_rel is None:
                continue  # not defined in the lib (transitive re-export etc.)
            gt = sorted({ws_rel(ws_root, next(
                x for x in members if x["id"] == mid), f)
                for mid, fs in by_member.items() for f in fs})
            candidates.append({
                "symbol": sym, "lang": lang, "lib": lib["id"],
                "def_file": ws_rel(ws_root, lib, def_rel),
                "consumers": sorted(by_member),
                "cross_files": len(gt), "gt": gt,
                "by_member": by_member,
            })

    rng = random.Random(seed)
    rng.shuffle(candidates)
    candidates.sort(key=lambda c: -c["cross_files"])

    tasks = []

    def emit(kind, c, gt, idx):
        tasks.append({
            "id": f"ws-{kind}-{bare_name(c['symbol'], c['lang'])}-{idx:03d}",
            "kind": kind, "rung": "rung1", "lang": c["lang"],
            "symbol": c["symbol"], "defining_member": c["lib"],
            "def_file": c["def_file"], "consumers": c["consumers"],
            "prompt": PROMPTS[kind].format(
                WS_ROOT="{WS_ROOT}",
                MEMBERS=", ".join(sorted(member_rels.values())),
                SYMDESC=describe(c["symbol"], c["lang"]),
                LIB=member_rels[c["lib"]]) .replace(
                    "{WS_ROOT}", "{WS_ROOT}"),
            "gt_files": gt,
        })

    idx = 0
    picked = {}
    for c in candidates:
        if c["cross_files"] > MAX_GT_FILES:
            continue
        if picked.get(c["lib"], 0) >= PER_LIB_PRIMARY_CAP:
            continue
        kind = "xcallers" if idx % 2 == 0 else "ximpact"
        gt = list(c["gt"])
        if kind == "ximpact":
            gt = sorted(set(gt) | {c["def_file"]})
        emit(kind, c, gt, idx)
        picked[c["lib"]] = picked.get(c["lib"], 0) + 1
        c["picked"] = True
        idx += 1

    texts_by_id = {mid: dict(t) for mid, t in texts.items()}
    for kind in ("xsubtypes", "xnew"):
        for c in candidates:
            if not c.get("picked"):
                continue
            pat = sub_pattern(kind, c["lang"], bare_name(c["symbol"], c["lang"]))
            if pat is None:
                continue
            hits = {}
            for mid, fs in c["by_member"].items():
                sel = [f for f in fs if pat.search(texts_by_id[mid][f])]
                if sel:
                    hits[mid] = sel
            if not hits:
                continue
            gt = sorted({ws_rel(ws_root, next(
                x for x in members if x["id"] == mid), f)
                for mid, fs in hits.items() for f in fs})
            if gt == c["gt"]:
                continue
            emit(kind, c, gt, idx)
            idx += 1

    quota = {}
    lang_quota = {}
    for t in tasks:
        quota[t["defining_member"]] = quota.get(t["defining_member"], 0) + 1
        lang_quota[t["lang"]] = lang_quota.get(t["lang"], 0) + 1

    header = {
        "generator": "build_tasks_ws.py",
        "seed": seed,
        "corpus": json.loads(CORPUS.read_text()),
        "bars": "bench/workspace/README.md (registered 2026-08-17; the gate "
                "script reads bars from there, never from its own source)",
        "gt_discipline": "import-mediated plain-text extraction across "
                         "member checkouts; cross-member scoped; arm-neutral",
        "per_member_quota": quota,
        "per_lang_quota": lang_quota,
        "rung_counts": {"rung1": len(tasks), "rung2": 0},
        "rung2_note": "no organic bare-name cross-edges in this OSS corpus",
        "n_tasks": len(tasks),
    }
    if len(tasks) < min_tasks:
        print(f"WARNING: only {len(tasks)} tasks mined (< {min_tasks})",
              file=sys.stderr)
    return {"header": header, "tasks": tasks}, ws_root


def selftest(bundle, ws_root):
    ok = True
    h = bundle["header"]
    for t in bundle["tasks"]:
        for f in t["gt_files"]:
            if not (ws_root / f).is_file():
                print(f"MISSING GT FILE {t['id']}: {f}"); ok = False
        if not t["gt_files"]:
            print(f"EMPTY GT {t['id']}"); ok = False
        if "codeindex" in t["prompt"].lower():
            print(f"ARM LEAK {t['id']}"); ok = False
    if h["n_tasks"] < 30:
        print(f"FEWER THAN 30 TASKS: {h['n_tasks']}"); ok = False
    for lang in ("php", "ts", "py", "go"):
        if h["per_lang_quota"].get(lang, 0) < MIN_PER_LANG:
            print(f"lang {lang} thin: {h['per_lang_quota'].get(lang, 0)}")
            ok = False
    print(f"selftest {'PASS' if ok else 'FAIL'}: {h['n_tasks']} tasks, "
          f"langs={h['per_lang_quota']}, quota={h['per_member_quota']}")
    return ok


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--seed", type=int, default=1729)
    ap.add_argument("--min-tasks", type=int, default=30)
    ap.add_argument("--out", default="tasks/tasks_ws.json")
    ap.add_argument("--selftest", action="store_true")
    args = ap.parse_args()

    bundle, ws_root = mine(args.seed, args.min_tasks)
    if args.selftest:
        sys.exit(0 if selftest(bundle, ws_root) else 1)
    out = HERE / args.out
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(bundle, indent=1))
    print(f"wrote {out}: {bundle['header']['n_tasks']} tasks, "
          f"langs={bundle['header']['per_lang_quota']}, "
          f"quota={bundle['header']['per_member_quota']}")


if __name__ == "__main__":
    main()

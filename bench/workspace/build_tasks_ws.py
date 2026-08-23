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
  xcollide    — same bare name declared in >=2 members: only the files bound
                by import to ONE named declaring member
  xalias      — only the files that bind the symbol under a DIFFERENT local
                name (renamed import). A subset filter: the aliasing statement
                spells out the original name, so a text search over-returns.
  xchain      — transitive blast radius A->B->C across three members: a change
                to a symbol of member A reaches member C only through a
                member-B symbol whose definition file references A.

xnew/xsubtypes/xalias are emitted only when their answer is a proper subset of the
xcallers set (a genuinely different answer, not a rephrasing); xcollide the
same, against the bare-name union across members. xcallers/
ximpact/xnew are the control (greppable) shapes and stay scoped to the
per-lib primary picks; xsubtypes and xcollide are structural and range over
every mined candidate. Every emitted task's ground truth is capped at MAX_GT_FILES by the
single gt_within_cap() predicate — the cap bounds the answer, so it is applied
to the emitted GT, not to a candidate-level proxy.

Prompts embed {WS_ROOT}; the runner substitutes the workspace root path.

KNOWN LIMITATIONS (three, deliberate). These are what the miner does TODAY, and
each is asserted by known_limitations() under --selftest so it cannot be quietly
"fixed" into a regression:

  * go and py emit ZERO xsubtypes  — go's real subtyping is implicit interface
    satisfaction and is NOT textually computable (a Go sub_matcher branch would
    be false coverage); py needs an alias-aware pattern (+ a re-freeze).
  * ts emits ZERO xalias           — a corpus fact, not a gating bug: nothing in
    nest-core/nest-microservices imports a mined nest-common symbol renamed.
  * xchain is NEST-ONLY            — the other clusters are only two members
    deep; closing it needs new corpus pins, not a miner change.

See the block above known_limitations() for the prerequisite each number waits
on. Read it before you change one.

Usage:
  python3 build_tasks_ws.py [--seed 1729] [--min-tasks 30] [--out tasks/tasks_ws.json]
  python3 build_tasks_ws.py --selftest
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from functools import lru_cache
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

# Registered kind -> subset partition. Emitted into the task-file header; the
# grader READS it from there and never hardcodes it, so adding or excluding a
# shape cannot silently rot the reported subsets.
SUBSET_OF_KIND = {
    "xcallers": "control",     # greppable: bare name is the answer key
    "ximpact": "control",
    "xnew": "control",
    "xsubtypes": "structural",
    "xcollide": "structural",
    "xalias": "structural",
    "xchain": "structural",
}
# Shapes mined and recorded but excluded from the SCORED structural subset.
# Declared at freeze (bar B5 freeze-discipline), not after mining.
EXCLUDED_AT_FREEZE = {
    "xalias": "change 0018 (aliased-import resolution) has not landed, so the "
              "index cannot answer alias tasks; excluded from the scored "
              "structural subset at freeze per bar B5, not after mining. "
              "Mined, counted and reported separately.",
}


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

# The heritage clause of a declaration is scanned STRUCTURALLY, not matched as
# a regex. `(?:extends|implements)[^{;]*?\bNAME\b` — the pattern this replaced —
# fired on TS type-parameter lists and on PHP docblock prose (see SUBTYPE_CASES
# for the corpus snippets). Measured against the first freeze: 35 of its 187
# xsubtypes GT entries were not declarations, and 13 tasks were WHOLLY wrong —
# answers a correct agent could only score 0.0 against while a search-everything
# agent scored 1.0.
#
# Each dialect is gated explicitly (learning
# ``dialect-specific-remedies-need-a-language-gate``): the only shared rule is
# "the name has to head a base-type entry between a declaration keyword and its
# opening brace". TS additionally has generics, so it tracks angle depth and
# everything inside `<...>` is excluded; PHP has none, so angle brackets are
# ordinary characters there (`<` is a comparison operator, and tracking it would
# corrupt the scan) but PHP has heredocs and `#` comments, which TS does not.
# Newlines are NOT a discriminator in either dialect — both spell genuine
# heritage clauses across lines.

_DECL_HEAD = {
    # php: `(` admits the anonymous `new class (...) implements X` form.
    "php": re.compile(r"\b(?:class|interface|trait|enum)\b(?=\s*[\w(])"),
    "ts": re.compile(r"\b(?:class|interface)\b(?=\s*[\w<])"),
}
_HERITAGE_SPLIT = re.compile(r"\b(?:extends|implements)\b")
_BASE_HEAD = re.compile(r"\s*\\?([\w\\.]+)")
_PHP_HEREDOC = re.compile(r"<<<[ \t]*(?P<q>['\"]?)(?P<id>\w+)(?P=q)\r?\n")
_HERITAGE_SCAN_LIMIT = 4000  # a heritage clause is never longer than this


def _close_quote(text, i, quote):
    """End offset of the string literal opened at `i`, or None if unterminated.

    Single/double quotes are bounded to their own LINE on purpose. An unpaired
    quote — a TS regex literal such as ``/['"]/``, an apostrophe in prose the
    comment pass did not catch — would otherwise blank an arbitrarily long span
    of real code and silently drop genuine declarations. Failing to mask is the
    safe direction here; over-masking is not.
    """
    j = i + 1
    n = len(text)
    while j < n:
        c = text[j]
        if c == "\\":
            j += 2
            continue
        if c == quote:
            return j + 1
        if c == "\n" and quote != "`":
            return None
        j += 1
    return None


def _mask_noncode(text, lang):
    """Blank comment and string spans, preserving offsets and line structure."""
    out = list(text)
    n = len(text)

    def blank(a, b):
        for k in range(a, min(b, n)):
            if out[k] != "\n":
                out[k] = " "

    i = 0
    while i < n:
        ch = text[i]
        nxt = text[i + 1] if i + 1 < n else ""
        if ch == "/" and nxt == "*":
            end = text.find("*/", i + 2)
            end = n if end < 0 else end + 2
            blank(i, end); i = end; continue
        if (ch == "/" and nxt == "/") or (
                lang == "php" and ch == "#" and nxt != "["):  # #[Attr] is not one
            end = text.find("\n", i)
            end = n if end < 0 else end
            blank(i, end); i = end; continue
        if lang == "php" and text.startswith("<<<", i):
            m = _PHP_HEREDOC.match(text, i)
            if m:
                close = re.compile(r"^[ \t]*" + re.escape(m.group("id")) + r"\b",
                                   re.M).search(text, m.end())
                end = close.end() if close else n
                blank(i, end); i = end; continue
        if ch in "'\"" or (lang == "ts" and ch == "`"):
            end = _close_quote(text, i, ch)
            if end is not None:
                blank(i, end); i = end; continue
        i += 1
    return "".join(out)


def _heritage_region(masked, start, generics):
    """The declaration header from `start` to its `{`/`;`, nested spans blanked.

    Blanking everything at bracket depth > 0 is what excludes a TS type-
    parameter list: `class Foo<T extends Base> implements Bar {` yields
    `` Foo          implements Bar ``, so `Base` is not a base type and `Bar`
    is. Commas inside `<...>` disappear with it, so the base list splits
    correctly.
    """
    chars = []
    depth = 0
    prev = ""
    for i in range(start, min(len(masked), start + _HERITAGE_SCAN_LIMIT)):
        ch = masked[i]
        if depth == 0 and ch in "{;":
            break
        if ch in "([" or (generics and ch == "<"):
            depth += 1
            chars.append(" ")
        elif ch in ")]" or (generics and ch == ">" and prev != "="):
            depth = max(0, depth - 1)  # `=>` guarded above; be total anyway
            chars.append(" ")
        else:
            chars.append(ch if depth == 0 else " ")
        prev = ch
    return "".join(chars)


@lru_cache(maxsize=None)
def declared_supertypes(text, lang):
    """Bare names this file declares a class/interface/trait/enum subtype OF.

    Alias-blind, like the py subtype pattern and for the same reason: PHP's
    ``use X as Y; class Z extends Y`` spells the supertype under the local
    name only, so `X` is not reported. Recorded in the README beside the
    equivalent python fact; closing it is change 0018's alias resolution, not a
    looser pattern here.
    """
    head = _DECL_HEAD.get(lang)
    if head is None:
        return frozenset()
    masked = _mask_noncode(text, lang)
    generics = lang == "ts"  # dialect gate: PHP has no type-parameter lists
    names = set()
    for m in head.finditer(masked):
        region = _heritage_region(masked, m.end(), generics)
        for clause in _HERITAGE_SPLIT.split(region)[1:]:
            for entry in clause.split(","):
                base = _BASE_HEAD.match(entry)
                if base:
                    names.add(re.split(r"[\\.]", base.group(1))[-1])
    return frozenset(names)


def declares_subtype(text, lang, bare) -> bool:
    """Does this file DECLARE a class/interface/trait/enum subtype of `bare`?"""
    return bare in declared_supertypes(text, lang)


def sub_matcher(kind, lang, bare):
    """Predicate ``f(text) -> bool`` for a sub-kind, or None if inapplicable."""
    if kind == "xnew" and lang in ("php", "ts"):
        pat = re.compile(r"new\s+\\?(?:[\w\\.]+[\\.])?" + re.escape(bare)
                         + r"\s*[(<]")
        return lambda text: bool(pat.search(text))
    if kind == "xsubtypes":
        if lang in ("php", "ts"):
            return lambda text: declares_subtype(text, lang, bare)
        if lang == "py":
            pat = re.compile(r"^class\s+\w+\([^)]*\b" + re.escape(bare)
                             + r"\b[^)]*\)", re.M)
            return lambda text: bool(pat.search(text))
    return None


# --------------------------------------------------------------------------- #
# xsubtypes DECLARATION cases — characterization, asserted under --selftest.
#
# The prompt this shape emits asks for files that "declare a class extending or
# implementing it", so ground truth must hold DECLARATIONS, which is a fact
# about a declaration header and not about the words `extends`/`implements`
# appearing somewhere in the file. Every snippet below is taken verbatim (or
# minimally trimmed) from the frozen corpus, and the two dialects fail in
# structurally different ways — learning
# ``dialect-specific-remedies-need-a-language-gate``:
#
#   ts   type-parameter lists. `explore<T extends HttpServer = any>(` and
#        `tryActivate<TContext extends string = ContextType>(... instance:
#        Controller,` are METHOD generics inside a class body; neither declares
#        anything. But a genuine TS heritage clause routinely spans newlines
#        (`class AbstractHttpAdapter<\n TServer = any,\n> implements
#        HttpServer<...>`), so "forbid newlines" alone is wrong for TS: the
#        discriminator is angle-bracket depth, not line breaks.
#   php  no generics at all, so angle depth is meaningless; the contamination is
#        DOCBLOCK PROSE ("An object that implements \Traversable which ...").
#        PHP heritage lists also span newlines legitimately, and PHP has the
#        anonymous form `new class (...) implements X`, which TS lacks.
#
# A single regex argued from either dialect mislabels the other.
# --------------------------------------------------------------------------- #

SUBTYPE_CASES = [
    # (label, lang, bare, snippet, expected)
    ("ts multi-line heritage clause is a declaration", "ts", "HttpServer",
     "export abstract class AbstractHttpAdapter<\n  TServer = any,\n"
     "  TRequest = any,\n  TResponse = any,\n> implements HttpServer<TRequest,"
     " TResponse>\n{\n  protected httpServer: TServer;\n", True),
    ("ts method type-parameter constraint is not", "ts", "HttpServer",
     "export class RouterExplorer {\n  public explore<T extends HttpServer ="
     " any>(\n    applicationRef: T,\n  ) {}\n}\n", False),
    ("ts param type after a generic method head is not", "ts", "Controller",
     "export class GuardsConsumer {\n  public async tryActivate<TContext"
     " extends string = ContextType>(\n    guards: CanActivate[],\n"
     "    instance: Controller,\n  ): Promise<boolean> {}\n}\n", False),
    ("ts class type-parameter constraint is not", "ts", "Injectable",
     "export class Module {\n  public addInjectable<T extends Injectable>(\n"
     "    injectable: Provider,\n  ) {}\n}\n", False),
    ("ts interface extends is a declaration", "ts", "INestApplicationContext",
     "export interface INestApplication\n  extends INestApplicationContext"
     " {\n  use(): this;\n}\n", True),
    ("php docblock prose is not a declaration", "php", "Traversable",
     "<?php\n/**\n * @param \\Traversable $namespaces\n *   An object that"
     " implements \\Traversable which contains the root paths\n *   keyed by"
     " the namespace.\n */\nclass EntityTypeManager extends"
     " DefaultPluginManager {\n}\n", False),
    ("php multi-line implements list is a declaration", "php", "CacheableInt",
     "<?php\nclass Foo extends Bar implements\n  CacheableInt,\n"
     "  OtherInt {\n}\n", True),
    ("php anonymous class implements is a declaration", "php",
     "InputCollectorInterface",
     "<?php\n$collector = new class () implements InputCollectorInterface {\n"
     "};\n", True),
    ("php # line comment prose is not a declaration", "php", "Countable",
     "<?php\n# a helper that implements Countable for callers\nclass Foo"
     " extends Bar {\n}\n", False),
    ("php attribute before the declaration is not a supertype", "php",
     "Constraint",
     "<?php\n#[Constraint(\n  id: 'PluginExists',\n)]\nclass"
     " PluginExistsConstraint extends SymfonyConstraint implements"
     " ContainerFactoryPluginInterface {\n}\n", False),
    ("php string literal holding code is not a declaration", "php", "Ghost",
     "<?php\n$src = 'class Spooky extends Ghost {}';\nclass Foo extends Bar"
     " {\n}\n", False),
    # py is NOT implicated by the finding and must not regress.
    ("py class base list is a declaration", "py", "RequestBase",
     "from werkzeug.wrappers import Request as RequestBase\n\n\n"
     "class Request(RequestBase):\n    pass\n", True),
    ("py annotation mentioning the name is not", "py", "RequestBase",
     "from werkzeug.wrappers import Request as RequestBase\n\n\n"
     "def handle(req: RequestBase) -> None:\n    pass\n", False),
]


def subtype_cases():
    """Assert the declaration cases above. Returns [(label, ok, detail)]."""
    out = []
    for label, lang, bare, snippet, want in SUBTYPE_CASES:
        match = sub_matcher("xsubtypes", lang, bare)
        got = bool(match(snippet)) if match else False
        out.append((f"xsubtypes declaration: {label}", got == want,
                    f"expected {want}, got {got}"))
    return out


# --------------------------------------------------------------------------- #
# Alias detection — does THIS file bind the symbol under a DIFFERENT local name?
#
# Stated correctly, `xalias` is a SUBSET filter, not a recall extension. The
# intuitive framing ("a text search for the symbol's own name misses the
# aliased files") is backwards: the aliasing statement itself spells out the
# original name, so a plain text search returns a strict SUPERSET of the
# aliasing files. The wanted answer is the subset that renames it.
#
# Language gating (learning: ``dialect-specific-remedies-need-a-language-gate``)
# is not decoration here — the four dialects put the rename in four structurally
# different places, and a regex argued from one of them is simply wrong for the
# other three:
#   php  `use Some\Ns\Thing as T;`  — rename attaches to the fully qualified
#        name in the use statement itself.
#   ts   `import { Thing as T } from 'pkg'` — rename lives INSIDE the specifier
#        braces, and the package that qualifies it is a separate clause.
#   py   `from mod import Thing as T` — rename inside the import clause, which
#        may be parenthesised across lines; the module is a separate clause.
#   go   `t "mod/path/pkg"` — the rename attaches to the IMPORT PATH and the
#        symbol's own name never appears in it at all; the default local name
#        is the path's last segment, so "aliased" means "alias != last segment"
#        (and `_`/`.` imports are not renames of a usable binding).
# --------------------------------------------------------------------------- #

def aliased_binding(text, symbol, lang) -> bool:
    if lang == "php":
        # FQCN, optionally written absolute at the use site.
        pat = re.compile(r"^use\s+\\?" + re.escape(symbol) + r"\s+as\s+(\w+)\s*;",
                         re.M)
        return any(a != bare_name(symbol, lang) for a in pat.findall(text))
    if lang == "ts":
        pkg, name = symbol.split(":", 1)
        for names, src in TS_IMPORT.findall(text):
            if src != pkg and not src.startswith(pkg + "/"):
                continue
            for raw in names.split(","):
                parts = raw.strip().removeprefix("type ").split(" as ")
                if len(parts) == 2 and parts[0].strip() == name:
                    if parts[1].strip() != name:
                        return True
        return False
    if lang == "py":
        mod, name = symbol.rsplit(".", 1)
        pat = re.compile(r"^from\s+" + re.escape(mod) + r"\s+import\s+"
                         r"(\([^)]*\)|[^\n]+)", re.M)
        for clause in pat.findall(text):
            for raw in clause.strip("()").replace("\\\n", ",").split(","):
                parts = raw.strip().split(" as ")
                if len(parts) == 2 and parts[0].strip() == name:
                    if parts[1].strip() != name:
                        return True
        return False
    if lang == "go":
        path = symbol.rsplit(".", 1)[0]
        default = path.rsplit("/", 1)[-1]
        imports = []
        for block in GO_IMPORT_BLOCK.findall(text):
            imports += GO_IMPORT_LINE.findall(block)
        imports += GO_IMPORT_ONE.findall(text)
        return any(p == path and a and a not in (default, "_", ".")
                   for a, p in imports)
    return False


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
    "xcollide": (
        "You are working in a multi-repo workspace rooted at {WS_ROOT}. "
        "Member projects (relative to that root): {MEMBERS}. "
        "The name `{BARE}` is declared in more than one member project, so a "
        "plain text search for it matches files bound to any of those "
        "declarations. Only one is meant here: {SYMDESC}, declared in the "
        "{LIB} project and written there as `{QUAL}`. List every file in the "
        "OTHER member projects (not {LIB}) that references THAT declaration. "
        "A file that references some other project's `{BARE}` does not "
        "count." + PROMPT_TAIL),
    "xalias": (
        "You are working in a multi-repo workspace rooted at {WS_ROOT}. "
        "Member projects (relative to that root): {MEMBERS}. "
        "{SYMDESC} is defined in the {LIB} project and written there as "
        "`{QUAL}`. Some files in the other member projects import it under a "
        "DIFFERENT local name (a renamed or aliased import) and then use it "
        "by that other name. List only those files: every file in the OTHER "
        "member projects (not {LIB}) whose import binds it to a local name "
        "other than `{BARE}`. A file that imports it under its own name "
        "`{BARE}` does not count, even though it references it."
        + PROMPT_TAIL),
    "xchain": (
        "You are working in a multi-repo workspace rooted at {WS_ROOT}. "
        "Member projects (relative to that root): {MEMBERS}. "
        "{HOP1} is about to change incompatibly. {SYMDESC} is defined in the "
        "{LIB} project, in a file that references that changing name, so the "
        "change propagates into {LIB} and then onward to whatever depends on "
        "`{BARE}`. Work out that second hop: list every file in the OTHER "
        "member projects (not {LIB}) that references `{QUAL}`, since those "
        "are the files the change reaches transitively." + PROMPT_TAIL),
}

# ------------------------------------------------------------------------- #
# xchain member selection — EXPLICIT, never derived from the role filter.
#
# `libs` in mine() is `"shared lib" in m["role"]`, which by construction
# excludes every middle member of a chain: nest-core's role is "consumer of
# nest-common (monorepo member)". Widening that role string in corpus.json
# would silently change what the PRIMARY pass mines and re-open the frozen
# baseline, so the chain pass names its members here instead and reuses the
# `namespaces` each member already declares.
#
# nest-only, and that is a recorded corpus fact rather than a gap to fill:
# the php, py and go clusters are two members deep (lib + consumer), so no
# A->B->C chain exists in them. Synthesising one would mean adding members.
# ------------------------------------------------------------------------- #
CHAIN_PASSES = [("nest-common", "nest-core", "nest-microservices")]


def qualified_form(symbol, lang):
    """How `symbol` is unambiguously written in ITS OWN language.

    Explicitly gated per language (learning:
    ``dialect-specific-remedies-need-a-language-gate``). These four forms are
    not interchangeable and no single format string is justified across them:
    PHP's leading-backslash absolute FQCN is not legal TS/Py/Go, and Go's
    ``pkg.Name`` selector — where the import path never appears at the use
    site — is not a PHP FQCN. A rule argued from one dialect and then applied
    to all four would mislabel three of them, which for this shape is fatal:
    the qualified form IS the disambiguator the prompt hands the agent.
    """
    if lang == "php":
        # Absolute FQCN: the leading backslash roots the name at the global
        # namespace. PHP-only — the other three have no such form.
        return "\\" + symbol
    if lang == "ts":
        # Named export; the binding lives in the import specifier, not in the
        # use site, so the package has to be named alongside the bare name.
        pkg, name = symbol.split(":", 1)
        return f"{name}, imported from '{pkg}'"
    if lang == "py":
        # Dotted module path; the import statement carries the full path.
        mod, name = symbol.rsplit(".", 1)
        return f"{mod}.{name}"
    # go: cross-package uses are ALWAYS the selector <pkg>.<Name>; the import
    # path appears only in the import block, so both are stated.
    path, name = symbol.rsplit(".", 1)
    return f"{path.rsplit('/', 1)[-1]}.{name}, from package {path}"

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


def gt_within_cap(gt) -> bool:
    """The authoritative 40-file cap, evaluated on the GT a task actually emits.

    The cap exists to bound the ANSWER the agent has to list, so it has to be
    applied to the emitted ground truth — never to a candidate-level proxy such
    as ``c["cross_files"]``. Those are different rules the moment a sub-kind's
    GT is not the candidate's whole reference set (a subset can pass while the
    candidate fails; ``ximpact`` adds the definition file, so it can fail while
    the candidate passes). Every emit site calls this one predicate, so the
    sites cannot drift apart and their comments cannot come to disagree.
    """
    return len(gt) <= MAX_GT_FILES


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
                BARE=bare_name(c["symbol"], c["lang"]),
                QUAL=qualified_form(c["symbol"], c["lang"]),
                # hop-1 of an xchain task; unused by every other template
                HOP1=c.get("hop1_desc", ""),
                LIB=member_rels[c["lib"]]) .replace(
                    "{WS_ROOT}", "{WS_ROOT}"),
            "gt_files": gt,
        })

    idx = 0
    picked = {}
    for c in candidates:
        if c["cross_files"] > MAX_GT_FILES:
            continue  # cheap pre-filter; gt_within_cap below is the real gate
        if picked.get(c["lib"], 0) >= PER_LIB_PRIMARY_CAP:
            continue
        kind = "xcallers" if idx % 2 == 0 else "ximpact"
        gt = list(c["gt"])
        if kind == "ximpact":
            gt = sorted(set(gt) | {c["def_file"]})
        if not gt_within_cap(gt):
            # ximpact adds the definition file, so a candidate sitting exactly
            # on the cap overflows it. Skip without consuming idx or a per-lib
            # slot: the next candidate fills this position.
            continue
        emit(kind, c, gt, idx)
        picked[c["lib"]] = picked.get(c["lib"], 0) + 1
        c["picked"] = True
        idx += 1

    texts_by_id = {mid: dict(t) for mid, t in texts.items()}
    for kind in ("xsubtypes", "xnew"):
        for c in candidates:
            if kind == "xnew" and not c.get("picked"):
                # xnew stays scoped to the primary picks: it is a control
                # (greppable) shape and the registered corpus holds it at 10.
                # xsubtypes is structural and ranges over EVERY candidate.
                continue
            match = sub_matcher(kind, c["lang"], bare_name(c["symbol"], c["lang"]))
            if match is None:
                continue
            hits = {}
            for mid, fs in c["by_member"].items():
                sel = [f for f in fs if match(texts_by_id[mid][f])]
                if sel:
                    hits[mid] = sel
            if not hits:
                continue
            gt = sorted({ws_rel(ws_root, next(
                x for x in members if x["id"] == mid), f)
                for mid, fs in hits.items() for f in fs})
            if gt == c["gt"]:
                continue
            if not gt_within_cap(gt):
                continue
            emit(kind, c, gt, idx)
            idx += 1

    # ---------------------------------------------------------------- #
    # xcollide — the same BARE name declared in >= 2 member projects.
    #
    # A plain-text search for the bare name returns the union of the
    # references to every declaration of it; the answer wanted is only the
    # files whose IMPORT binds them to one named declaring member. The
    # per-language symbol keys are already fully qualified (PHP FQCN,
    # `pkg:Name`, `module.Name`, `importpath.Name`), so that binding is
    # already computed above — the collision is purely on bare_name(), and
    # each candidate's own `gt` is by construction the import-bound subset.
    #
    # Union is taken over the candidates' import-bound reference sets rather
    # than over a raw text match, which makes it a SUBSET of what a text
    # search would return; a proper subset of this union is therefore a
    # proper subset of the text-search result too.
    # ---------------------------------------------------------------- #
    by_bare = {}
    for c in candidates:
        by_bare.setdefault(bare_name(c["symbol"], c["lang"]), []).append(c)
    for bare in sorted(by_bare):
        group = by_bare[bare]
        if len({c["lib"] for c in group}) < 2:
            continue  # one declaring member: nothing to disambiguate
        union = set()
        for c in group:
            union |= set(c["gt"])
        # total order, independent of dict/set iteration: (lib, lang, symbol)
        for c in sorted(group, key=lambda c: (c["lib"], c["lang"], c["symbol"])):
            gt = list(c["gt"])
            if not set(gt) < union:
                # Equal to the union means the other declarations contribute
                # no references of their own, so there is nothing a text
                # search over-returns here and the task is xcallers rephrased.
                continue
            if not gt_within_cap(gt):
                continue
            emit("xcollide", c, gt, idx)
            idx += 1

    # ---------------------------------------------------------------- #
    # xalias — files that bind the symbol under a DIFFERENT local name.
    #
    # A subset filter over each candidate's own reference set (see
    # aliased_binding() for why this is a subset and not an extension, and for
    # the four gated dialect forms). Runs after xcollide so the shapes already
    # frozen keep their ids. Ordering follows the same total order as the
    # other sub-kind passes: the seeded candidate order, then sorted files.
    # ---------------------------------------------------------------- #
    for c in candidates:
        hits = {}
        for mid, fs in c["by_member"].items():
            sel = [f for f in fs
                   if aliased_binding(texts_by_id[mid][f], c["symbol"], c["lang"])]
            if sel:
                hits[mid] = sel
        if not hits:
            continue
        gt = sorted({ws_rel(ws_root, next(
            x for x in members if x["id"] == mid), f)
            for mid, fs in hits.items() for f in fs})
        if gt == c["gt"]:
            # Every referencing file aliases it: nothing for the filter to
            # remove, so the task is xcallers rephrased, not structural.
            continue
        if not gt_within_cap(gt):
            continue
        emit("xalias", c, gt, idx)
        idx += 1

    # ---------------------------------------------------------------- #
    # xchain — transitive A -> B -> C across three members.
    #
    # NO NEW MACHINERY: this is the existing extraction run a second time
    # with the middle member treated as a lib (its own declared
    # `namespaces`), then joined against hop 1. See CHAIN_PASSES for why the
    # middle member is selected by name and not by the role filter.
    #
    #   hop 1  A -> B : which of B's own definition files reference A
    #   hop 2  B -> C : which of C's files import a B symbol
    #   join         : keep the hop-2 symbols whose B definition file is a
    #                  hop-1 file, i.e. the change in A really does reach C
    #                  through them. GT is the hop-2 file set.
    #
    # Runs last so every already-frozen shape keeps its ids. Totally ordered:
    # chain passes in declaration order, symbols by sorted key, files sorted.
    # ---------------------------------------------------------------- #
    by_id = {m["id"]: m for m in members}
    chain_stats = []
    for a_id, b_id, c_id in CHAIN_PASSES:
        if not {a_id, b_id, c_id} <= by_id.keys():
            continue
        a, b, cm = by_id[a_id], by_id[b_id], by_id[c_id]
        lang = b["lang"][0]
        b_defs = lib_definitions(b, texts[b["id"]])
        b_text = texts_by_id[b["id"]]
        # hop 2: C's references into B, keyed by B's own symbol keys
        hop2 = {}
        for rel, text in texts[cm["id"]]:
            for sym in extract_refs(text, lang, b["namespaces"]):
                hop2.setdefault(sym, set()).add(rel)
        n_imported = 0
        chain = []
        for sym in sorted(hop2):
            def_rel = find_def(sym, lang, b_defs)
            if def_rel is None:
                continue  # re-export or not declared in B
            n_imported += 1
            # hop 1: does B's definition file itself reference A?
            hop1 = sorted(extract_refs(b_text[def_rel], lang, a["namespaces"]))
            if not hop1:
                continue
            chain.append((sym, def_rel, hop1[0], sorted(hop2[sym])))
        emitted = 0
        for sym, def_rel, hop1_sym, files in chain:
            gt = sorted(ws_rel(ws_root, cm, f) for f in files)
            if not gt_within_cap(gt):
                continue
            emit("xchain", {
                "symbol": sym, "lang": lang, "lib": b["id"],
                "def_file": ws_rel(ws_root, b, def_rel),
                "consumers": [cm["id"]],
                "hop1_desc": describe(hop1_sym, lang),
            }, gt, idx)
            idx += 1
            emitted += 1
        chain_stats.append({
            "chain": [a_id, b_id, c_id],
            "b_definitions": len(b_defs),
            "imported_by_c": n_imported,
            "chain_symbols": len(chain),
            "chain_files": len({f for _, _, _, fs in chain for f in fs}),
            "emitted": emitted,
        })

    quota = {}
    lang_quota = {}
    for t in tasks:
        quota[t["defining_member"]] = quota.get(t["defining_member"], 0) + 1
        lang_quota[t["lang"]] = lang_quota.get(t["lang"], 0) + 1

    subsets = build_subsets(tasks)

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
        "xchain_passes": chain_stats,
        "xchain_note": "xchain is nest-only: a recorded corpus fact, not a "
                       "gap. The php, py and go clusters are two members deep "
                       "(lib + consumer), so no A->B->C chain exists in them; "
                       "no chain was synthesised and no member was added.",
        "subsets": subsets,
        "n_tasks": len(tasks),
    }
    if len(tasks) < min_tasks:
        print(f"WARNING: only {len(tasks)} tasks mined (< {min_tasks})",
              file=sys.stderr)
    return {"header": header, "tasks": tasks}, ws_root


def build_subsets(tasks):
    """The structural/control partition artifact, emitted into the header.

    Carries the registered kind -> subset map, per-shape n, per-shape
    per-language n, the shapes excluded from the SCORED structural subset at
    freeze, and the exclusion arithmetic against the registered floors. The
    grader reads all of this; it hardcodes none of it.
    """
    per_shape = {}
    per_shape_lang = {}
    for t in tasks:
        k = t["kind"]
        per_shape[k] = per_shape.get(k, 0) + 1
        per_shape_lang.setdefault(k, {})
        per_shape_lang[k][t["lang"]] = per_shape_lang[k].get(t["lang"], 0) + 1
    per_shape = dict(sorted(per_shape.items()))
    per_shape_lang = {k: dict(sorted(v.items()))
                      for k, v in sorted(per_shape_lang.items())}

    kinds = sorted(set(SUBSET_OF_KIND) | set(per_shape))
    unmapped = [k for k in kinds if k not in SUBSET_OF_KIND]
    if unmapped:  # a new shape with no registered subset must not be silent
        raise SystemExit(f"unmapped kinds in subset partition: {unmapped}")

    def members(subset, scored_only=False):
        return [k for k in kinds
                if SUBSET_OF_KIND[k] == subset
                and not (scored_only and k in EXCLUDED_AT_FREEZE)]

    def total(ks):
        return sum(per_shape.get(k, 0) for k in ks)

    scored_structural = members("structural", scored_only=True)
    excluded = [k for k in members("structural") if k in EXCLUDED_AT_FREEZE]
    control = members("control")

    def terms(ks):
        return " + ".join(f"{k} {per_shape.get(k, 0)}" for k in ks) or "0"

    n_struct = total(scored_structural)
    n_control = total(control)
    return {
        "map": {k: SUBSET_OF_KIND[k] for k in kinds},
        "control_kinds": control,
        "structural_kinds": members("structural"),
        "scored_structural_kinds": scored_structural,
        "excluded_from_scored": {
            k: {"n": per_shape.get(k, 0),
                "per_lang": per_shape_lang.get(k, {}),
                "reason": EXCLUDED_AT_FREEZE[k]}
            for k in excluded
        },
        "per_shape_n": per_shape,
        "per_shape_lang_n": per_shape_lang,
        "n_scored_structural": n_struct,
        "n_control": n_control,
        "registered_floors": {"structural": 105, "control": 40, "total": 145},
        "arithmetic": (
            f"structural excluding {', '.join(excluded) or 'nothing'} = "
            f"{terms(scored_structural)} = {n_struct} (>= 105); "
            f"control = {terms(control)} = {n_control} (>= 40); "
            f"total = {len(tasks)} (>= 145)"
        ),
        "meets_floors": {
            "structural": n_struct >= 105,
            "control": n_control >= 40,
            "total": len(tasks) >= 145,
        },
        "note": "control shapes are greppable (the bare name is the answer "
                "key); structural shapes are not. grade_ws.py reads this map "
                "from the header so a shape added or excluded here cannot "
                "leave the reported subsets stale.",
    }


# --------------------------------------------------------------------------- #
# KNOWN LIMITATIONS — characterization assertions
#
# Learning: ``known-limitations-need-a-characterization-test``. Each of the
# three shape gaps below is DELIBERATE and is recorded here as an assertion of
# what this miner DOES TODAY, not as a target. The detection signal for each is
# sitting right there in the corpus, so "closing" one looks like a small win —
# and each of these three would be WRONG, or would need a prerequisite that has
# not landed. Anyone who changes the behaviour must break one of these
# assertions on the way, read the prerequisite recorded next to it, and update
# the number deliberately. Do not "fix" the assertion to match new output
# without satisfying the stated prerequisite first.
#
# Run: python3 build_tasks_ws.py --selftest   (exits non-zero on failure)
# --------------------------------------------------------------------------- #

def known_limitations(bundle):
    """Assert TODAY's recorded gaps. Returns [(label, ok, detail)].

    Every label is spelled ``KNOWN LIMITATION`` on purpose: these are the
    current numbers, deliberately frozen, each with the prerequisite that must
    land before the number is allowed to move.
    """
    per_shape_lang = bundle["header"]["subsets"]["per_shape_lang_n"]
    out = []

    def n(kind, lang):
        return per_shape_lang.get(kind, {}).get(lang, 0)

    # (1) Go and Python emit ZERO xsubtypes. This asserts today's behaviour on
    # purpose.
    #   PREREQUISITE (go): NOT a Go branch in sub_matcher(). Go's real
    #   subtyping is IMPLICIT INTERFACE SATISFACTION — three corpus files
    #   define `Collect(ch chan<- prometheus.Metric)` and satisfy
    #   prometheus.Collector without ever naming it. A textual cross-member
    #   miner cannot compute that, and a textual `extends|implements`-shaped
    #   Go branch would emit FALSE COVERAGE. Closing this needs a real type
    #   checker (or type information from the index), not a regex.
    #   PREREQUISITE (py): an ALIAS-AWARE subtype pattern. Python's zero is
    #   real but small (~+4-5): flask subclasses werkzeug under renamed
    #   imports (`from werkzeug.wrappers import Request as RequestBase`), and
    #   the current pattern matches the original name only; the proper-subset
    #   guard then drops ~2 more. Moving this number requires teaching
    #   sub_matcher() the local alias binding, and re-freezing the corpus.
    for lang in ("go", "py"):
        got = n("xsubtypes", lang)
        out.append((
            f"KNOWN LIMITATION: {lang} emits zero xsubtypes (recorded, not a goal)",
            got == 0,
            f"expected 0, got {got}",
        ))

    # (2) TS emits ZERO xalias. This asserts today's behaviour on purpose.
    #   PREREQUISITE: NEW CORPUS PINS, not a miner change. This is a corpus
    #   FACT, not a gating bug: no mined nest-common symbol is imported under
    #   an alias anywhere in nest-core or nest-microservices. All four alias
    #   dialect branches in aliased_binding() work, TS included. Loosening the
    #   TS branch to make this non-zero would emit non-aliasing files.
    got = n("xalias", "ts")
    out.append((
        "KNOWN LIMITATION: ts emits zero xalias (corpus fact, not a gating bug)",
        got == 0,
        f"expected 0, got {got}",
    ))

    # (3) xchain is NEST-ONLY. This asserts today's behaviour on purpose.
    #   PREREQUISITE: a THIRD member in another cluster, i.e. new corpus pins
    #   — not a miner change. The php, py and go clusters are two members deep
    #   (lib + consumer), so no A->B->C chain exists in them to mine. (php has
    #   three members but two are sibling consumers of symfony, not a chain.)
    #   Synthesising a chain, or relaxing the hop-1 join to manufacture one,
    #   is explicitly not the fix.
    nest = ["nest-common", "nest-core", "nest-microservices"]
    emitting = [p["chain"] for p in bundle["header"]["xchain_passes"]
                if p["emitted"] > 0]
    libs = sorted({t["defining_member"]
                   for t in bundle["tasks"] if t["kind"] == "xchain"})
    langs = sorted({t["lang"] for t in bundle["tasks"] if t["kind"] == "xchain"})
    out.append((
        "KNOWN LIMITATION: xchain is nest-only (needs new corpus pins to change)",
        emitting == [nest] and libs == ["nest-core"] and langs == ["ts"],
        f"expected chains=[{nest}] libs=['nest-core'] langs=['ts'], "
        f"got chains={emitting} libs={libs} langs={langs}",
    ))
    return out


def selftest(bundle, ws_root):
    ok = True
    h = bundle["header"]
    cases = subtype_cases()
    for label, passed, detail in cases:
        if not passed:
            print(f"  [FAIL] {label} -- {detail}")
            ok = False
    print(f"  [{'ok' if all(c[1] for c in cases) else 'FAIL'}] xsubtypes "
          f"declaration cases: {sum(c[1] for c in cases)}/{len(cases)}")
    for label, passed, detail in known_limitations(bundle):
        print(f"  [{'ok' if passed else 'CHANGED'}] {label}"
              + ("" if passed else f" -- {detail}"))
        if not passed:
            ok = False
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

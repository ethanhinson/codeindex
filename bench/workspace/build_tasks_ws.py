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
  xcollide    — same bare name declared in >=2 members OF THE SAME LANGUAGE:
                only the files bound by import to ONE named declaring member.
                The key is language-gated (xcollide_key): across languages the
                file extension disambiguates completely and nothing is left
                but xcallers.
  xalias      — only the files that bind the symbol under a DIFFERENT local
                name (renamed import). A subset filter: the aliasing statement
                spells out the original name, so a text search over-returns.
  xchain      — transitive blast radius A->B->C across three members. ONLY the
                member-A symbol is named; the member-B intermediaries are the
                agent's to discover. GT is the member-C files that import a
                B-declared name whose declaring B file references the named A
                symbol — and, by the emit guard, that never name it themselves.

xnew/xsubtypes/xalias are emitted only when their answer is a proper subset of the
xcallers set (a genuinely different answer, not a rephrasing); xcollide the
same, against the LANGUAGE-GATED bare-name union across members; xchain when
its answer is
DISJOINT from the named symbol's own cross-member reference set (see
xchain_is_structural). xcallers/
ximpact/xnew are the control (greppable) shapes and stay scoped to the
per-lib primary picks; xsubtypes and xcollide are structural and range over
every mined candidate. Every emitted task's ground truth is capped at MAX_GT_FILES by the
single gt_within_cap() predicate — the cap bounds the answer, so it is applied
to the emitted GT, not to a candidate-level proxy.

Prompts embed {WS_ROOT}; the runner substitutes the workspace root path.

KNOWN LIMITATIONS (four, deliberate). These are what the miner does TODAY, and
each is asserted by known_limitations() under --selftest so it cannot be quietly
"fixed" into a regression:

  * go and py emit ZERO xsubtypes  — go's real subtyping is implicit interface
    satisfaction and is NOT textually computable (a Go sub_matcher branch would
    be false coverage); py needs an alias-aware pattern (+ a re-freeze).
  * ts emits ZERO xalias           — a corpus fact, not a gating bug: nothing in
    nest-core/nest-microservices imports a mined nest-common symbol renamed.
  * xchain is NEST-ONLY            — the other clusters are only two members
    deep; closing it needs new corpus pins, not a miner change.
  * xcollide emits ZERO            — corpus.json declares exactly one shared
    lib per language, so no bare name is declared by two members of the SAME
    language. Closing it needs new corpus pins; ungating the key is not the
    fix (that is the finding this zero came from).

See the block above known_limitations() for the prerequisite each number waits
on. Read it before you change one.

Usage:
  python3 build_tasks_ws.py [--seed 1729] [--min-tasks 30] [--out tasks/tasks_ws.json]
  python3 build_tasks_ws.py --selftest
"""

from __future__ import annotations

import argparse
import json
import os
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

# The pre-registered freeze floors (design D7 bar B5). SINGLE SOURCE: the
# header's ``registered_floors``, its ``arithmetic`` string and its
# ``meets_floors`` verdict are all DERIVED from this dict, and ``--selftest``
# FAILS when any entry is unmet -- the freeze arithmetic is binding, not
# decorative. README.md's prose copy is checked against this dict too.
# NOTE: ``control`` being 40 is a COINCIDENCE with MAX_GT_FILES above. They are
# different invariants (a subset-size floor vs. a per-task ground-truth cap);
# do not "deduplicate" them into one constant.
REGISTERED_FLOORS = {"structural": 105, "control": 40, "total": 145}

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


_MEMBER_TEXTS = {}  # member id -> [(rel, text)]; the tree is read ONCE


def member_files(member):
    """[(member-relative path, text)] for the member's own sources.

    Memoised on the member id: mine() reads every member once, and the
    shape_invariants() sweep re-derives each shape's predicate over the same
    texts. Without the memo the sweep would double `--selftest`'s disk work
    for no new information.
    """
    if member["id"] in _MEMBER_TEXTS:
        return _MEMBER_TEXTS[member["id"]]
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
    _MEMBER_TEXTS[member["id"]] = out
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
    # NOTE: this template names the HOP-1 symbol ONLY. It must never spell a
    # hop-2 (intermediary) name — doing so turns the task into `xcallers`
    # verbatim and voids its registration as a structural (non-greppable)
    # shape. `{QUAL}`/`{BARE}` of a hop-2 symbol are deliberately absent, and
    # xchain_is_structural() enforces the same invariant on the emitted GT.
    "xchain": (
        "You are working in a multi-repo workspace rooted at {WS_ROOT}. "
        "Member projects (relative to that root): {MEMBERS}. "
        "{SYMDESC} is defined in the {LIB} project and is about to change "
        "incompatibly. The {VIA} project references it, and the remaining "
        "member projects depend in turn on what {VIA} declares — so the "
        "change reaches them at one remove, through {VIA}, in files that "
        "never name it themselves. Work out that second hop. First: which "
        "files inside {VIA} reference it. Then: which names those particular "
        "files declare and export. List every file in the REMAINING member "
        "projects (neither {LIB} nor {VIA}) that imports one of THOSE names "
        "from {VIA}." + PROMPT_TAIL),
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
#
# A PASS IS A (LIB, VIA) PAIR, NOT A TRIPLE. The prompt asks for "every file
# in the REMAINING member projects (neither {LIB} nor {VIA})" and {MEMBERS}
# lists ALL ten members, so the ground truth has to be mined over exactly that
# set — every member except A and B — rather than over one nominated member C.
# Naming C in the pass instead was correct only by coincidence (today
# `nest/packages/common` holds no reference to `@nestjs/core`, and no non-TS
# member can import it), and an unrecorded coincidence is not a scope: a
# correct answer citing a file outside the nominated C would have been scored
# spurious the moment one appeared. The alternative — narrowing the prompt to
# name C's root — was REJECTED: pointing the agent at the one directory that
# can contain the answer shrinks the discovery the shape exists to measure.
# The scan is deliberately NOT language-gated either, for the same reason:
# "no PHP file can import a TS module" is another coincidence of the pins.
# The member C reached by a pass is therefore DERIVED (see `chain_stats`),
# which is also what keeps the nest-only selftest honest.
# ------------------------------------------------------------------------- #
CHAIN_PASSES = [("nest-common", "nest-core")]


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
        _, name = symbol.rsplit(".", 1)
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


def spells(name, text) -> bool:
    """Does `text` spell `name` as a WHOLE WORD?

    Substring containment is the wrong test and would reject honest tasks:
    the prompt for `NestModule` contains the letters of the intermediary
    `Module`, but no search for `Module` finds them — there is no word
    boundary inside an identifier. What must not happen is the prompt handing
    over an intermediary's name as a name.
    """
    return re.search(r"\b" + re.escape(name) + r"\b", text) is not None


def xchain_is_structural(gt, direct_refs, hop2_symbols, prompt, lang) -> bool:
    """The xchain NON-DEGENERACY guard — the shape's proper-subset equivalent.

    Every other shape refuses to emit an answer that a plain search for the
    name in the prompt already returns: xnew/xsubtypes/xalias require a PROPER
    SUBSET of the xcallers set, xcollide a proper subset of the bare-name
    union. xchain had no such guard, and without one it degenerated into
    `xcallers` verbatim — the finding this function answers.

    A proper-subset test is the wrong instrument HERE, because an xchain answer
    is not drawn from the named symbol's reference set at all: it is the set of
    member-C files that reach the symbol only through a member-B declaration.
    The distinguishing property is therefore DISJOINTNESS, which is strictly
    stronger: not one ground-truth file may itself reference the one name the
    prompt hands over. That is exactly "the bare name is not the answer key",
    which is how the header note partitions control from structural.

    Two clauses, both necessary:

      1. `gt` is disjoint from `direct_refs` — the member-C files that
         reference the named symbol directly. A file in both is reachable by
         searching for the named symbol, so the task would be partly
         greppable. The whole task is REJECTED rather than the file trimmed
         out: trimming would falsify the prompt, which asks for every file
         reached through the intermediary, and that file is one of them.
      2. the RENDERED prompt does not spell any intermediary's name as a whole
         word — otherwise it hands over the second hop it is asking the agent
         to find, which is the finding this guard answers. Checked against the
         rendered text, not against the template, because the leak the finding
         caught was a substitution (`{QUAL}`), not a literal.
         (Rejects nothing in the frozen corpus; it is the invariant, not a
         filter tuned to it — see xchain_guard_cases() for its RED case.)
    """
    if not gt or set(gt) & set(direct_refs):
        return False
    return not any(spells(bare_name(s, lang), prompt) for s in hop2_symbols)


def xcollide_key(c):
    """The xcollide COLLISION KEY — `(language, bare name)`, gated on purpose.

    Keying on the bare name ALONE groups declarations across languages, and a
    cross-language "collision" is not one: the prompt asks the agent to pick
    out the files bound to ONE declaration of `{BARE}`, and when the rival
    declaration is in another language the FILE EXTENSION already separates
    them completely. What is left of the task after the extension has done the
    work is `xcallers` verbatim — the same degeneracy `xchain_is_structural`
    answers for the chain shape, reached here through the grouping instead of
    through a regex. `xcollide` is registered STRUCTURAL, and structural is
    defined in the registration as "the bare name is not the answer key"; a
    trivially extension-separable group does not meet that.

    This is the learning `dialect-specific-remedies-need-a-language-gate`
    applied to the GROUPING rather than to a pattern: language is part of the
    identity of a name, so it belongs in the key, not in a later filter.
    """
    return (c["lang"], bare_name(c["symbol"], c["lang"]))


def xcollide_is_structural(gt, union) -> bool:
    """The xcollide PROPER-SUBSET guard.

    `union` is the union of the import-bound reference sets of every candidate
    in the collision group, so it is a SUBSET of what a plain text search for
    the bare name returns. An answer that is a PROPER subset of it is therefore
    a proper subset of the text-search result too: the search over-returns, and
    the disambiguation the prompt asks for is real work.

    Equality is the degenerate case and is refused: it means the rival
    declarations contribute no references of their own, so nothing is
    over-returned and the task is `xcallers` rephrased. (Nothing can exceed the
    union by construction, but `<` states the invariant rather than assuming
    it.) See xcollide_guard_cases() for the RED cases.
    """
    return set(gt) < set(union)


def ws_rel(ws_root, member, rel):
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

    def render(kind, c):
        """The task prompt EXACTLY as it is emitted.

        One rendering path, so the xchain guard inspects the same text the
        agent will read — the finding it answers was a template substitution,
        invisible in the template itself.
        """
        return PROMPTS[kind].format(
            WS_ROOT="{WS_ROOT}",
            MEMBERS=", ".join(sorted(member_rels.values())),
            SYMDESC=describe(c["symbol"], c["lang"]),
            BARE=bare_name(c["symbol"], c["lang"]),
            QUAL=qualified_form(c["symbol"], c["lang"]),
            # the intermediary MEMBER of an xchain task (never a hop-2
            # symbol); unused by every other template
            VIA=member_rels.get(c.get("via_member"), ""),
            LIB=member_rels[c["lib"]])

    def emit(kind, c, gt, idx):
        task = {
            "id": f"ws-{kind}-{bare_name(c['symbol'], c['lang'])}-{idx:03d}",
            "kind": kind, "rung": "rung1", "lang": c["lang"],
            "symbol": c["symbol"], "defining_member": c["lib"],
            "def_file": c["def_file"], "consumers": c["consumers"],
            "prompt": render(kind, c),
            "gt_files": gt,
        }
        # xchain records its intermediary in the task file (audit trail for the
        # guard); no other shape carries extras.
        task.update(c.get("extra") or {})
        tasks.append(task)

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
    # xcollide — the same bare name declared, IN ONE LANGUAGE, by >= 2
    # member projects.
    #
    # A plain-text search for the bare name returns the union of the
    # references to every declaration of it; the answer wanted is only the
    # files whose IMPORT binds them to one named declaring member. The
    # per-language symbol keys are already fully qualified (PHP FQCN,
    # `pkg:Name`, `module.Name`, `importpath.Name`), so that binding is
    # already computed above, and each candidate's own `gt` is by
    # construction the import-bound subset.
    #
    # The key is `xcollide_key()` — `(lang, bare)`, NOT `bare` alone. Read its
    # docstring before widening it: an ungated key manufactures cross-language
    # "collisions" whose disambiguation the file EXTENSION performs completely,
    # leaving `xcallers` behind in a shape registered as structural.
    #
    # Union is taken over the candidates' import-bound reference sets rather
    # than over a raw text match, which makes it a SUBSET of what a text
    # search would return; a proper subset of this union (xcollide_is_
    # structural) is therefore a proper subset of the text-search result too.
    #
    # On THIS corpus the gate empties the shape, and that is the honest
    # outcome rather than a shortfall to be engineered around: corpus.json
    # declares exactly one shared lib per language, so no bare name is
    # declared by two members of the same language and the >= 2 declaring
    # members test can never be met. The pass is recorded below and asserted
    # by known_limitations() (4) so the zero cannot go unnoticed, and the
    # guards keep their RED cases in xcollide_guard_cases().
    # ---------------------------------------------------------------- #
    by_key = {}
    for c in candidates:
        by_key.setdefault(xcollide_key(c), []).append(c)
    langs_of_bare = {}
    for lang, bare in by_key:
        langs_of_bare.setdefault(bare, set()).add(lang)
    xcollide_pass = {
        "key": "(lang, bare_name) — LANGUAGE-GATED; see xcollide_key()",
        "declaring_members_per_lang": {
            lang: len({c["lib"] for c in candidates if c["lang"] == lang})
            for lang in sorted({c["lang"] for c in candidates})},
        "groups_considered": len(by_key),
        "groups_spanning_two_languages": sum(
            1 for g in by_key.values() if len({c["lang"] for c in g}) > 1),
        "cross_language_bare_names_separated": sorted(
            b for b, ls in langs_of_bare.items() if len(ls) > 1),
        "multi_member_groups": [],
        "emitted": 0,
        "rejected_not_proper_subset": 0,
        "rejected_over_cap": 0,
        "guard": "xcollide_is_structural: GT a PROPER subset of the "
                 "language-gated bare-name union across declaring members",
    }
    for key in sorted(by_key):
        group = by_key[key]
        if len({c["lib"] for c in group}) < 2:
            continue  # one declaring member: nothing to disambiguate
        xcollide_pass["multi_member_groups"].append(
            {"lang": key[0], "bare": key[1],
             "libs": sorted({c["lib"] for c in group})})
        union = set()
        for c in group:
            union |= set(c["gt"])
        # total order, independent of dict/set iteration: (lib, lang, symbol)
        for c in sorted(group, key=lambda c: (c["lib"], c["lang"], c["symbol"])):
            gt = list(c["gt"])
            if not xcollide_is_structural(gt, union):
                xcollide_pass["rejected_not_proper_subset"] += 1
                continue
            if not gt_within_cap(gt):
                xcollide_pass["rejected_over_cap"] += 1
                continue
            emit("xcollide", c, gt, idx)
            xcollide_pass["emitted"] += 1
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
    #                  through them.
    #
    # THE TASK IS KEYED ON THE HOP-1 (member-A) SYMBOL, and only that symbol is
    # named in the prompt. Keying on the hop-2 symbol instead — what this pass
    # did before — forced the prompt to spell the intermediary out, at which
    # point the operative sentence was `xcallers` word for word and the shape
    # was greppable despite its structural registration. Keying on hop 1 is
    # also the only WELL-POSED unit: several B symbols can carry the same A
    # symbol onward, so naming A alone has a single answer only if GT is the
    # UNION over all of them. Hence the grouping below.
    #
    # GT is that union, and xchain_is_structural() then requires it to be
    # disjoint from C's own direct references to A — the non-degeneracy guard.
    #
    # Runs last so every already-frozen shape keeps its ids. Totally ordered:
    # chain passes in declaration order, hop-1 symbols by sorted key, files
    # sorted.
    # ---------------------------------------------------------------- #
    by_id = {m["id"]: m for m in members}
    chain_stats = []
    for a_id, b_id in CHAIN_PASSES:
        if not {a_id, b_id} <= by_id.keys():
            continue
        a, b = by_id[a_id], by_id[b_id]
        lang = b["lang"][0]
        a_defs = lib_definitions(a, texts[a["id"]])
        b_defs = lib_definitions(b, texts[b["id"]])
        b_text = texts_by_id[b["id"]]
        # The answer scope IS the prompt's scope: every member except A and B.
        # See CHAIN_PASSES for why this is a scan and not a nominated member.
        c_members = [m for m in members if m["id"] not in (a_id, b_id)]
        # hop 2: the remaining members' references into B, keyed by B's own
        # symbol keys. Entries are (member, rel) so GT can span members.
        hop2 = {}
        # Their DIRECT references into A — what a search for the named symbol
        # returns on its own, and what the guard requires GT to avoid.
        direct = {}
        for cm in c_members:
            for rel, text in texts[cm["id"]]:
                for sym in extract_refs(text, lang, b["namespaces"]):
                    hop2.setdefault(sym, set()).add((cm["id"], rel))
                for sym in extract_refs(text, lang, a["namespaces"]):
                    direct.setdefault(sym, set()).add((cm["id"], rel))
        n_imported = 0
        # hop-1 symbol -> the intermediaries carrying it and the C files they
        # reach. dict preserves insertion order; emission sorts explicitly.
        chain = {}
        for sym in sorted(hop2):
            def_rel = find_def(sym, lang, b_defs)
            if def_rel is None:
                continue  # re-export or not declared in B
            n_imported += 1
            # hop 1: does B's definition file itself reference A? EVERY hop-1
            # symbol it names is a task key (the old code kept hop1[0] only,
            # which silently dropped the rest).
            for hop1_sym in sorted(extract_refs(b_text[def_rel], lang,
                                                a["namespaces"])):
                g = chain.setdefault(hop1_sym, {"syms": set(), "bfiles": set(),
                                                "files": set()})
                g["syms"].add(sym)
                g["bfiles"].add(def_rel)
                g["files"] |= hop2[sym]
        emitted = 0
        rejected_greppable = 0
        reached = set()
        for hop1_sym in sorted(chain):
            g = chain[hop1_sym]
            a_def = find_def(hop1_sym, lang, a_defs)
            if a_def is None:
                continue  # A re-exports it; not A's own declaration
            gt = sorted(ws_rel(ws_root, by_id[mid], f) for mid, f in g["files"])
            direct_gt = sorted(ws_rel(ws_root, by_id[mid], f)
                               for mid, f in direct.get(hop1_sym, ()))
            if not gt_within_cap(gt):
                continue
            cand = {
                "symbol": hop1_sym, "lang": lang, "lib": a["id"],
                "def_file": ws_rel(ws_root, a, a_def),
                # DERIVED: the members the chain actually reaches, not a
                # nominated C.
                "consumers": sorted({mid for mid, _ in g["files"]}),
                "via_member": b["id"],
                "extra": {
                    "via_member": b["id"],
                    # the intermediaries the agent must DISCOVER. Recorded for
                    # audit; deliberately never rendered into the prompt.
                    "via_symbols": sorted(g["syms"]),
                    "via_files": sorted(ws_rel(ws_root, b, f)
                                        for f in g["bfiles"]),
                },
            }
            if not xchain_is_structural(gt, direct_gt, g["syms"],
                                        render("xchain", cand), lang):
                rejected_greppable += 1
                continue
            emit("xchain", cand, gt, idx)
            idx += 1
            emitted += 1
            reached |= set(cand["consumers"])
        chain_stats.append({
            # A, B, then the members the pass ACTUALLY reached — derived from
            # the emitted GT, so the nest-only selftest reads a measurement
            # rather than the declaration it is supposed to be checking.
            "chain": [a_id, b_id] + sorted(reached),
            "scanned": sorted(m["id"] for m in c_members),
            "b_definitions": len(b_defs),
            "imported_by_c": n_imported,
            "hop1_symbols": len(chain),
            "chain_files": len({f for g in chain.values() for f in g["files"]}),
            "emitted": emitted,
            "rejected_greppable": rejected_greppable,
            "guard": "xchain_is_structural: GT disjoint from C's direct "
                     "references to the named hop-1 symbol, and the rendered "
                     "prompt spells no intermediary's name",
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
        "xcollide_pass": xcollide_pass,
        "xcollide_note": "xcollide emits ZERO on this corpus, and that is a "
                         "CORPUS FACT, not a shortfall: corpus.json declares "
                         "exactly one shared lib per language, so no bare "
                         "name is declared by two members of the same "
                         "language. The collision key is language-gated on "
                         "purpose — a cross-language pair is separated "
                         "COMPLETELY by file extension, leaving xcallers "
                         "behind in a shape registered structural. Closing "
                         "this needs new corpus pins (a second declaring "
                         "member in some language), never an ungated key.",
        "xchain_passes": chain_stats,
        "xchain_note": "xchain is nest-only: a recorded corpus fact, not a "
                       "gap. The php, py and go clusters are two members deep "
                       "(lib + consumer), so no A->B->C chain exists in them; "
                       "no chain was synthesised and no member was added.",
        "xchain_shape": "keyed on the HOP-1 (member-A) symbol; only that "
                        "symbol is named in the prompt and the member-B "
                        "intermediaries are the agent's to discover. GT is "
                        "the union, over every intermediary carrying that "
                        "symbol onward, of the member-C files importing it, "
                        "and is required to be DISJOINT from member-C's own "
                        "direct references to the named symbol "
                        "(xchain_is_structural) — so the name in the prompt "
                        "is never the answer key, which is what this "
                        "header's subset note requires of a structural shape.",
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
    # A REGISTERED shape that emitted nothing must be visible as an explicit
    # zero, not absent: this header is what the README's per-shape table and
    # the exclusion arithmetic are reconciled against, and a silently missing
    # row reads as an oversight rather than as a recorded corpus fact.
    for k in SUBSET_OF_KIND:
        per_shape.setdefault(k, 0)
        per_shape_lang.setdefault(k, {})
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
        "registered_floors": dict(REGISTERED_FLOORS),
        "arithmetic": (
            f"structural excluding {', '.join(excluded) or 'nothing'} = "
            f"{terms(scored_structural)} = {n_struct} "
            f"(>= {REGISTERED_FLOORS['structural']}); "
            f"control = {terms(control)} = {n_control} "
            f"(>= {REGISTERED_FLOORS['control']}); "
            f"total = {len(tasks)} (>= {REGISTERED_FLOORS['total']})"
        ),
        "meets_floors": {
            name: actual >= REGISTERED_FLOORS[name]
            for name, actual in (("structural", n_struct),
                                 ("control", n_control),
                                 ("total", len(tasks)))
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
    per_shape = bundle["header"]["subsets"]["per_shape_n"]
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
    #   The membership check is stated over EVERY member a chain task touches
    #   (the named symbol's own member and the intermediary), not over
    #   `defining_member` alone. `defining_member` moved from `nest-core` to
    #   `nest-common` when the prompt was rewritten to name the hop-1 symbol —
    #   the task now asks about a symbol nest-common declares. That is a change
    #   of WHICH SYMBOL IS NAMED, not of the limitation: the chain is still
    #   nest-only, and its prerequisite (new corpus pins) is untouched. Phrased
    #   over the touched members, the assertion is stable against that move and
    #   still fails the moment a non-nest chain appears.
    nest = ["nest-common", "nest-core", "nest-microservices"]
    xchain = [t for t in bundle["tasks"] if t["kind"] == "xchain"]
    emitting = [p["chain"] for p in bundle["header"]["xchain_passes"]
                if p["emitted"] > 0]
    touched = sorted({t["defining_member"] for t in xchain}
                     | {t["via_member"] for t in xchain}
                     | {m for t in xchain for m in t["consumers"]})
    langs = sorted({t["lang"] for t in xchain})
    out.append((
        "KNOWN LIMITATION: xchain is nest-only (needs new corpus pins to change)",
        emitting == [nest] and touched == nest and langs == ["ts"],
        f"expected chains=[{nest}] members={nest} langs=['ts'], "
        f"got chains={emitting} members={touched} langs={langs}",
    ))

    # (4) xcollide emits ZERO. This asserts today's behaviour on purpose.
    #   PREREQUISITE: NEW CORPUS PINS — a SECOND declaring member in some one
    #   language — not a miner change. corpus.json declares exactly one shared
    #   lib per language (symfony/php, nest-common/ts, werkzeug/py,
    #   client_golang/go), so a same-language cross-member bare-name clash
    #   cannot exist here and the >= 2 declaring members test can never be
    #   met. The number moved 20 -> 0 when the collision key was language-
    #   gated: the 20 were 9 cross-language groups (php vs ts/py/go) whose
    #   "disambiguation" the FILE EXTENSION performs completely, plus a py
    #   group whose two declarations sit in the SAME member (werkzeug.sansio
    #   vs werkzeug.wrappers) and so pose no cross-member question at all.
    #   Two things are explicitly NOT the fix: ungating the key, and dropping
    #   the >= 2 declaring members test to admit within-member clashes. Both
    #   buy the count back by weakening what the shape asserts.
    #   The assertion is stated over the recorded pass, not over the emitted
    #   count alone, so it stays load-bearing at n = 0 and fails the moment a
    #   second same-language declaring member appears.
    xc = bundle["header"].get("xcollide_pass") or {}
    per_lang_libs = xc.get("declaring_members_per_lang") or {}
    got = per_shape.get("xcollide", 0)
    out.append((
        "KNOWN LIMITATION: xcollide emits zero — one declaring member per "
        "language (needs new corpus pins to change)",
        got == 0 and bool(per_lang_libs)
        and all(v == 1 for v in per_lang_libs.values())
        and xc.get("multi_member_groups") == [],
        f"expected 0 tasks and 1 declaring member per language, got "
        f"{got} tasks, per_lang={per_lang_libs}, "
        f"multi_member_groups={xc.get('multi_member_groups')!r}",
    ))
    return out


# --------------------------------------------------------------------------- #
# xchain NON-DEGENERACY cases — asserted under --selftest, from disk.
#
# The registration partitions the corpus as "control shapes are greppable (the
# bare name is the answer key); structural shapes are not", and grade_ws reads
# that partition out of the task header. xchain is registered structural, so
# the claim has to hold task by task, and this check re-derives it INDEPENDENTLY
# of the miner: it re-reads every ground-truth file off disk and re-runs the
# reference extractor over it, rather than trusting a number the miner wrote.
#
# Two properties, one per clause of xchain_is_structural():
#   * no ground-truth file references the named symbol — the name in the prompt
#     is not the answer key;
#   * the prompt does not spell any intermediary's name — the second hop is not
#     pre-solved in the prompt text (this is the finding that produced the
#     guard: the old template rendered the hop-2 symbol outright).
# --------------------------------------------------------------------------- #

def xchain_guard_cases():
    """Both clauses of xchain_is_structural(), each with its rejection case.

    Clause 1 rejects real corpus tasks (the count is in the task header's
    `xchain_passes[].rejected_greppable`); clause 2 rejects none today, so its
    RED case is stated here rather than left unexercised.
    """
    gt = ["../c/one.ts", "../c/two.ts"]
    clean = "The TypeScript export `Widget` from `@nestjs/common` ..."
    cases = [
        ("accepts a disjoint answer with unnamed intermediaries",
         (gt, ["../c/other.ts"], {"@nestjs/core:Gadget"}, clean, "ts"), True),
        ("clause 1 rejects a GT file that references the named symbol",
         (gt, ["../c/two.ts"], {"@nestjs/core:Gadget"}, clean, "ts"), False),
        ("clause 1 rejects an empty answer",
         ([], [], {"@nestjs/core:Gadget"}, clean, "ts"), False),
        ("clause 2 rejects a prompt spelling an intermediary",
         (gt, [], {"@nestjs/core:Gadget"},
          clean + " list files referencing `Gadget`", "ts"), False),
        ("clause 2 ignores an intermediary name merely embedded in a word",
         (gt, [], {"@nestjs/core:Module"},
          "The TypeScript export `NestModule` from `@nestjs/common` ...",
          "ts"), True),
    ]
    out = []
    for label, args, want in cases:
        got = xchain_is_structural(*args)
        out.append((f"xchain guard: {label}", got == want,
                    f"expected {want}, got {got}"))
    return out


def xcollide_guard_cases():
    """The collision key and the proper-subset guard, each with its RED case.

    Stated as unit cases because the frozen corpus emits ZERO xcollide tasks
    (see known_limitations (4)): a guard exercised only by live tasks would go
    unexercised here, and an unexercised guard is decoration. These cases hold
    whatever the corpus does, and the first two are the finding this shape's
    re-freeze answers — with `bare` alone as the key they both fail.
    """
    php = {"lang": "php", "symbol": "Symfony\\Component\\Mime\\Header\\Headers"}
    py = {"lang": "py", "symbol": "werkzeug.datastructures.Headers"}
    php2 = {"lang": "php",
            "symbol": "Symfony\\Component\\HttpFoundation\\HeaderBag\\Headers"}
    union = ["../a/one.php", "../a/two.php", "../b/three.php"]
    cases = [
        ("key separates two languages sharing a bare name "
         "(the extension already disambiguates those)",
         xcollide_key(php) != xcollide_key(py), True),
        ("key groups a genuine same-language clash",
         xcollide_key(php) == xcollide_key(php2), True),
        ("proper-subset accepts an answer the bare-name union over-returns",
         xcollide_is_structural(["../a/one.php"], union), True),
        ("proper-subset rejects an answer EQUAL to the union "
         "(nothing over-returned: the task is xcallers rephrased)",
         xcollide_is_structural(union, union), False),
        ("proper-subset rejects an answer outside the union",
         xcollide_is_structural(union + ["../c/four.php"], union), False),
    ]
    return [(f"xcollide guard: {label}", got == want,
             f"expected {want}, got {got}") for label, got, want in cases]


def _ref_namespace(symbol, lang):
    """The namespace argument extract_refs() expects for `symbol`'s language.

    Gated per language for the usual reason (learning
    ``dialect-specific-remedies-need-a-language-gate``): php_refs wants a
    backslash-terminated FQCN prefix, ts_refs a package, py_refs the TOP-LEVEL
    package (it compares ``mod.split(".")[0]``), go_refs an import path. One
    form applied to all four would silently match nothing in three of them —
    and a check that matches nothing passes for the wrong reason.
    """
    if lang == "php":
        return symbol.rsplit("\\", 1)[0] + "\\" if "\\" in symbol else symbol
    if lang == "ts":
        return symbol.split(":", 1)[0]
    if lang == "py":
        return symbol.split(".", 1)[0]
    return symbol.rsplit(".", 1)[0]  # go: import path


def xchain_nondegeneracy(bundle, ws_root):
    """Assert xchain tasks are non-greppable. Returns [(label, ok, detail)]."""
    tasks = [t for t in bundle["tasks"] if t["kind"] == "xchain"]
    if not tasks:
        return [("xchain non-degeneracy: no xchain tasks emitted", False,
                 "expected at least one")]
    greppable, spelled = [], []
    for t in tasks:
        ns = [_ref_namespace(t["symbol"], t["lang"])]
        for g in t["gt_files"]:
            p = ws_root / g
            if not p.is_file():
                continue
            if t["symbol"] in extract_refs(p.read_text(errors="replace"),
                                           t["lang"], ns):
                greppable.append(f"{t['id']}:{g}")
        for via in t.get("via_symbols", []):
            if spells(bare_name(via, t["lang"]), t["prompt"]):
                spelled.append(f"{t['id']}:{via}")
    # GT SCOPE == PROMPT SCOPE. The prompt demands "every file in the
    # REMAINING member projects (neither {LIB} nor {VIA})" over a {MEMBERS}
    # list naming all ten, so each pass must have SCANNED all ten minus its
    # two. Mining one nominated member C instead happens to give the same
    # answer on today's pins; this asserts the scope rather than relying on
    # that. A correct answer citing a file outside the mined scope would be
    # scored spurious, so the mismatch is a grading bug, not a cosmetic one.
    all_ids = {m["id"] for m in bundle["header"]["corpus"]["members"]}
    scope_bad = []
    for p in bundle["header"]["xchain_passes"]:
        a_id, b_id = p["chain"][0], p["chain"][1]
        want = sorted(all_ids - {a_id, b_id})
        if p.get("scanned") != want:
            scope_bad.append(f"{a_id}->{b_id}: scanned={p.get('scanned')} "
                             f"want={want}")
    return [
        ("xchain non-degeneracy: no GT file references the named symbol "
         "(the shape is not xcallers)", not greppable,
         f"{len(greppable)} greppable GT entries: {greppable[:5]}"),
        ("xchain non-degeneracy: the prompt never spells an intermediary "
         "(the second hop is not pre-solved)", not spelled,
         f"{len(spelled)} intermediaries named: {spelled[:5]}"),
        ("xchain scope: every pass mined the prompt's whole scope "
         "(all members but LIB and VIA)", not scope_bad,
         f"{len(scope_bad)} passes mis-scoped: {scope_bad[:3]}"),
    ]


def xcollide_nondegeneracy(bundle):
    """Assert every emitted xcollide collision is SAME-LANGUAGE.

    The prompt's whole demand is "disambiguate two declarations of `{BARE}`".
    If the colliding declarations sit in different languages, the file
    EXTENSION disambiguates them completely and the residue of the task is
    `xcallers` verbatim — the same degeneracy `xchain_is_structural` answers
    for the chain shape, arriving this time through the GROUPING rather than
    through a regex (learning: `dialect-specific-remedies-need-a-language-gate`
    applies to the collision key too).

    Two clauses, one over the emitted tasks and one over the miner's recorded
    audit trail, because at n = 0 the first clause alone would be vacuous:

      1. emitted xcollide tasks sharing a bare name all share a language;
      2. no group the miner CONSIDERED spans more than one language — i.e.
         the key really is language-gated, whatever it emitted.
    """
    by_bare = {}
    for t in bundle["tasks"]:
        if t["kind"] == "xcollide":
            by_bare.setdefault(bare_name(t["symbol"], t["lang"]), set()).add(
                t["lang"])
    mixed = sorted(f"{b}:{sorted(ls)}" for b, ls in by_bare.items()
                   if len(ls) > 1)
    audit = bundle["header"].get("xcollide_pass") or {}
    audit_mixed = audit.get("cross_language_bare_names_separated")
    return [
        ("xcollide non-degeneracy: every emitted collision is same-language "
         "(file extension is not the disambiguator)", not mixed,
         f"{len(mixed)} cross-language collisions: {mixed[:5]}"),
        ("xcollide non-degeneracy: the collision key is language-gated "
         "(no considered group spans two languages)",
         audit.get("groups_spanning_two_languages") == 0
         and isinstance(audit_mixed, list) and len(audit_mixed) > 0,
         f"audit says groups_spanning_two_languages="
         f"{audit.get('groups_spanning_two_languages')!r}, "
         f"separated={audit_mixed!r}"),
    ]


# --------------------------------------------------------------------------- #
# shape_invariants — the PER-TASK, CORPUS-WIDE sweep.
#
# THE FINDING THIS ANSWERS: the original per-task check over `gt_files` was
# existence, non-emptiness and arm-leak. Nothing re-derived the predicate the
# task's OWN SHAPE claims about its ground truth, so three mining passes could
# be added and the only thing standing behind their answers was "the path is on
# disk". That is exactly how the xsubtypes blocker got in: 35 of 187 GT entries
# were not subtype declarations and 13 tasks were wholly wrong.
#
# So for EVERY emitted task this sweep re-applies the shape's own predicate to
# the GT file's text and fails NAMING THE TASK ID. It is deliberately written
# against the same helpers the miner uses (`sub_matcher`, `aliased_binding`,
# `extract_refs`) rather than against a private re-implementation: the claim
# under test is "the emitted GT is what this shape says it is", not "two
# regexes agree". A predicate bug therefore still needs the unit cases
# (subtype_cases) — the two layers catch different faults, and neither
# subsumes the other.
#
#   xsubtypes  every GT file genuinely DECLARES a subtype of the symbol.
#   xalias     every GT file binds the symbol under a different local name,
#              AND GT is a PROPER SUBSET of the symbol's reference set — the
#              shape is a subset filter, so an equal set is xcallers rephrased.
#   xnew       every GT file instantiates the symbol, same proper-subset rule.
#   xcallers   every GT file really references the symbol.
#   ximpact    same, plus the def_file is present and is the ONLY entry from
#              the defining member (the shape adds the definition, not the
#              lib's other files).
#
# xchain and xcollide are already swept per-task by xchain_nondegeneracy() and
# xcollide_nondegeneracy() and are skipped here rather than duplicated.
# --------------------------------------------------------------------------- #

def _reference_index(ws_root):
    """(lib id, symbol) -> {ws-relative consumer file} — the xcallers set.

    Re-derived from the tree with the miner's own extractor so the sweep's
    "does this file reference the symbol" is the same question the miner
    answered, asked again from the emitted task. Member texts are memoised
    (see member_files), so this costs no extra disk reads.
    """
    _, members = load_members()
    idx = {}
    for lib in (m for m in members if "shared lib" in m["role"]):
        lang = lib["lang"][0]
        for m in members:
            if m["id"] == lib["id"] or lang not in m["lang"]:
                continue
            for rel, text in member_files(m):
                for sym in extract_refs(text, lang, lib["namespaces"]):
                    idx.setdefault((lib["id"], sym), set()).add(
                        ws_rel(ws_root, m, rel))
    return idx


def shape_invariants(bundle, ws_root):
    """Re-derive every task's own shape predicate over its GT files."""
    refs = _reference_index(ws_root)
    _, members = load_members()
    prefixes = {m["id"]: ws_rel(ws_root, m, "").rstrip("/") for m in members}
    cache = {}

    def text(rel):
        if rel not in cache:
            p = ws_root / rel
            cache[rel] = p.read_text(errors="replace") if p.is_file() else ""
        return cache[rel]

    swept = {"xsubtypes": 0, "xalias": 0, "xnew": 0, "xcallers": 0,
             "ximpact": 0}
    bad = {k: [] for k in ("declaration", "alias", "new", "reference",
                           "subset", "impact_def")}
    for t in bundle["tasks"]:
        kind = t["kind"]
        if kind not in swept:
            continue  # xchain / xcollide: swept by their own non-degeneracy
        swept[kind] += 1
        lang, sym = t["lang"], t["symbol"]
        gt = set(t["gt_files"])
        refset = refs.get((t["defining_member"], sym), set())

        if kind in ("xsubtypes", "xnew"):
            match = sub_matcher(kind, lang, bare_name(sym, lang))
            slot = "declaration" if kind == "xsubtypes" else "new"
            if match is None:
                bad[slot].append(f"{t['id']}: no {kind} matcher for {lang}")
            else:
                for g in sorted(gt):
                    if not match(text(g)):
                        bad[slot].append(f"{t['id']}:{g}")
        if kind == "xalias":
            for g in sorted(gt):
                if not aliased_binding(text(g), sym, lang):
                    bad["alias"].append(f"{t['id']}:{g}")
        if kind in ("xalias", "xnew"):
            if not gt < refset:
                bad["subset"].append(
                    f"{t['id']}: |gt|={len(gt)} |refs|={len(refset)} "
                    f"extra={sorted(gt - refset)[:3]}")
        if kind in ("xcallers", "ximpact"):
            # ximpact alone adds the definition file, which lives in the lib
            # and is not a reference to itself.
            for g in sorted(gt - ({t["def_file"]} if kind == "ximpact"
                                  else set())):
                if g not in refset:
                    bad["reference"].append(f"{t['id']}:{g}")
        if kind == "ximpact":
            own = sorted(g for g in gt
                         if g.startswith(prefixes[t["defining_member"]] + "/"))
            if own != [t["def_file"]]:
                bad["impact_def"].append(f"{t['id']}: {own}")

    empty = sorted(k for k, n in swept.items() if n == 0)
    return [
        (f"shape invariant: every xsubtypes GT file DECLARES a subtype "
         f"({swept['xsubtypes']} tasks swept)", not bad["declaration"],
         f"{len(bad['declaration'])} non-declarations: "
         f"{bad['declaration'][:5]}"),
        (f"shape invariant: every xalias GT file binds the symbol under "
         f"another name ({swept['xalias']} tasks swept)", not bad["alias"],
         f"{len(bad['alias'])} non-aliasing: {bad['alias'][:5]}"),
        (f"shape invariant: every xnew GT file instantiates the symbol "
         f"({swept['xnew']} tasks swept)", not bad["new"],
         f"{len(bad['new'])} non-instantiating: {bad['new'][:5]}"),
        ("shape invariant: xalias/xnew GT is a PROPER SUBSET of the symbol's "
         "reference set (the shape is a filter, not xcallers)",
         not bad["subset"],
         f"{len(bad['subset'])} not proper subsets: {bad['subset'][:5]}"),
        (f"shape invariant: every xcallers/ximpact GT file references the "
         f"symbol ({swept['xcallers'] + swept['ximpact']} tasks swept)",
         not bad["reference"],
         f"{len(bad['reference'])} non-referencing: {bad['reference'][:5]}"),
        ("shape invariant: every ximpact GT holds the def_file as its only "
         "entry from the defining member", not bad["impact_def"],
         f"{len(bad['impact_def'])} wrong: {bad['impact_def'][:5]}"),
        # A sweep over zero tasks passes for the wrong reason; the registered
        # shapes must actually have been visited.
        ("shape invariant: every registered non-chain shape was actually "
         "swept (no vacuous pass)", not empty,
         f"shapes with zero tasks swept: {empty}"),
    ]


def floor_cases(bundle):
    """BINDING check of the pre-registered freeze floors (bar B5).

    ``meets_floors`` used to be advisory: nothing read it, so the corpus could
    fall under a registered floor and ``--selftest`` still printed PASS. These
    cases make every entry a hard failure, and additionally pin README.md's
    prose copy of the same three numbers against REGISTERED_FLOORS so the
    registration document cannot drift away from the code.
    """
    subsets = bundle["header"]["subsets"]
    actuals = {"structural": subsets["n_scored_structural"],
               "control": subsets["n_control"],
               "total": bundle["header"]["n_tasks"]}
    cases = []
    for name, passed in subsets["meets_floors"].items():
        floor = REGISTERED_FLOORS[name]
        cases.append((
            f"registered floor {name} >= {floor} (actual {actuals[name]})",
            bool(passed),
            f"SHORTFALL: {name} = {actuals[name]}, {floor - actuals[name]} "
            f"under the registered floor of {floor}",
        ))

    # README.md's "Subset arithmetic, pinned at freeze" block states the same
    # three floors in prose. It is a registration document, so it stays
    # hand-written -- but it may not disagree with the code.
    labels = {"scored structural": "structural", "control": "control",
              "total": "total"}
    text = (HERE / "README.md").read_text()
    block = text.split("Subset arithmetic, pinned at freeze:", 1)
    if len(block) != 2:
        cases.append(("README floors pinned", False,
                      "README.md: 'Subset arithmetic, pinned at freeze:' "
                      "heading not found"))
        return cases
    stated = {}
    for line in block[1].splitlines():
        if not line.startswith("- "):
            if stated:
                break
            continue
        m = re.search(r"≥ registered (\d+)", line)
        if not m:
            continue
        for prose, name in labels.items():
            if line.startswith(f"- {prose}"):
                stated[name] = int(m.group(1))
                break
    passed = stated == REGISTERED_FLOORS
    cases.append((
        "README floors match REGISTERED_FLOORS",
        passed,
        f"README.md states {stated}, code says {dict(REGISTERED_FLOORS)}",
    ))
    return cases


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
    for label, passed, detail in (xchain_guard_cases()
                                  + xchain_nondegeneracy(bundle, ws_root)
                                  + xcollide_guard_cases()
                                  + xcollide_nondegeneracy(bundle)
                                  + shape_invariants(bundle, ws_root)
                                  + floor_cases(bundle)):
        print(f"  [{'ok' if passed else 'FAIL'}] {label}"
              + ("" if passed else f" -- {detail}"))
        if not passed:
            ok = False
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

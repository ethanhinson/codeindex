#!/usr/bin/env python3
"""Deterministic per-tool formatters: codeindex --json output -> the answer
shape the harness grader expects. NO model, and since the --json rewrite NO
regex over text output either — the JSON contract (internal/query/answers.go)
carries qname/file/line/ambiguous explicitly, which is exactly what the old
regexes were reverse-engineering.

Grader contracts (from bench/agent_ab/grade.py), targeted precisely:
  - caller_attribution: a 'CALLERS' region; each caller line must have the
    caller NAME before the FILE, name as the bare method (grader takes the last
    identifier before the file match). Truth is "name\tfile".
  - comprehension: 'DEFINITIONS:' section of file:line, then 'FILES:' section
    of repo-relative paths.
  - vague_find: the symbol name present + its file:line definition.

Every formatter takes the raw stdout of `codeindex <tool> ... --json`. If the
input does not parse as JSON (old binary, error text), the formatter returns
it unchanged — never worse than passthrough.
"""
from __future__ import annotations
import json


def _load(raw: str):
    try:
        v = json.loads(raw)
        return v if isinstance(v, dict) else None
    except (json.JSONDecodeError, TypeError):
        return None


def fmt_caller_attribution(raw: str) -> str:
    """callers --json -> 'CALLERS' block with 'Name file' per line (name first,
    so the grader's last-identifier-before-file extraction picks the caller).
    JSON 'name' is already the bare method (parent split off upstream)."""
    a = _load(raw)
    if a is None:
        return raw
    out = ["CALLERS:"]
    for c in a.get("callers", []):
        out.append(f"{c['name']} {c['file']}")
    return "\n".join(out) if len(out) > 1 else raw


def fmt_comprehension(raw_find: str, raw_grep: str) -> str:
    """find --json (def candidates) + grep --json (referencing files) ->
    DEFINITIONS/FILES sections."""
    find, grep = _load(raw_find), _load(raw_grep)
    if find is None and grep is None:
        return raw_find + "\n" + raw_grep
    defs, files = [], []
    for r in (find or {}).get("results", []):
        defs.append(f"{r['file']}:{r['line']}")
        files.append(r["file"])
    for g in (grep or {}).get("groups", []):
        files.append(g["file"])
    defs = list(dict.fromkeys(defs))
    files = list(dict.fromkeys(files))
    parts = ["DEFINITIONS:"]
    parts += defs[:1] if defs else []          # the top def is the answer
    parts += ["FILES:"] + files
    return "\n".join(parts)


def fmt_vague_find(raw: str) -> str:
    """find --json -> top hit's 'qname file:line' on a clean first line, then
    the full ranked listing (name + location per hit)."""
    a = _load(raw)
    if a is None or not a.get("results"):
        return raw
    lines = [f"{r['qname']} {r['kind']} {r['file']}:{r['line']}" for r in a["results"]]
    top = a["results"][0]
    return f"{top['qname']} {top['file']}:{top['line']}\n" + "\n".join(lines)


def _bare(qname: str) -> str:
    """Parent.Method -> Method (grader compares bare names)."""
    return qname.rsplit(".", 1)[-1]


def fmt_grep_occurrences(raw: str) -> str:
    """grep --json -> occurrence sites, enclosing-symbol name first then file.
    grep attributes hits to enclosing functions, so for call-shaped questions
    these lines double as caller pairs (the callers/grep blur, measured)."""
    a = _load(raw)
    if a is None or not a.get("groups"):
        return raw
    out = ["OCCURRENCES:"]
    files = []
    for g in a["groups"]:
        out.append(f"{_bare(g.get('qname') or g['file'])} {g['file']}")
        files.append(g["file"])
    out += ["FILES:"] + list(dict.fromkeys(files))
    return "\n".join(out)


def fmt_find_combined(raw_find: str, raw_grep: str) -> str:
    """find --json + grep --json -> one fixed shape serving every find-routed
    question: top-hit line, ranked listing, then DEFINITIONS/FILES sections.
    Type-blind: the same answer satisfies the vague-find grader (name +
    file:line present) and the comprehension grader (sections present)."""
    find, grep = _load(raw_find), _load(raw_grep)
    if find is None and grep is None:
        return raw_find + "\n" + raw_grep
    parts = []
    results = (find or {}).get("results", [])
    if results:
        top = results[0]
        parts.append(f"{top['qname']} {top['file']}:{top['line']}")
        parts += [f"{r['qname']} {r['kind']} {r['file']}:{r['line']}" for r in results]
        parts += ["DEFINITIONS:", f"{top['file']}:{top['line']}"]
    else:
        parts.append("DEFINITIONS:")
    files = [r["file"] for r in results]
    files += [g["file"] for g in (grep or {}).get("groups", [])]
    parts += ["FILES:"] + list(dict.fromkeys(files))
    return "\n".join(parts)


def route_answer(route: str, raw: str, raw_aux: str = "") -> str:
    """Type-blind dispatch: the classifier's route alone decides the answer
    shape. This is the honest end-to-end path — no harness-type leakage."""
    if route == "callers":
        return fmt_caller_attribution(raw)
    if route == "grep":
        return fmt_grep_occurrences(raw)
    if route == "find":
        return fmt_find_combined(raw, raw_aux)
    return raw


def format_answer(task_type: str, tool: str, raw: str, raw_aux: str = "") -> str:
    """Dispatch: given the harness task type + routed tool + raw --json
    output(s), return a grader-shaped answer."""
    if task_type in ("caller_attribution", "occurrences", "edit_impact"):
        # harness 'occurrences' is semantically caller-attribution
        return fmt_caller_attribution(raw)
    if task_type == "comprehension":
        return fmt_comprehension(raw, raw_aux)
    if task_type == "vague_find":
        return fmt_vague_find(raw)
    return raw

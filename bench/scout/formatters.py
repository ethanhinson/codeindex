#!/usr/bin/env python3
"""Deterministic per-tool formatters: raw codeindex output -> the answer shape
the harness grader expects. NO model — this is the "output normalization" step
that arm-C exposed as the real seam. One small parser per (tool, task-type).

Grader contracts (from bench/agent_ab/grade.py), targeted precisely:
  - caller_attribution: a 'CALLERS' region; each caller line must have the
    caller NAME before the FILE, name as the bare method (grader takes the last
    identifier before the file match). Truth is "name\tfile".
  - comprehension: 'DEFINITIONS:' section of file:line, then 'FILES:' section
    of repo-relative paths.
  - vague_find: the symbol name present + its file:line definition.
"""
from __future__ import annotations
import re

# codeindex caller line: "  <file>:<line>  <Parent.Name>|<Name>  [flags]"
_CALLER = re.compile(r"^\s*([\w./\-]+\.(?:go|ts|tsx|js|jsx|py|php)):(\d+)\s+([\w.]+)")
_DEF = re.compile(r"^def\s+([\w.]+)\s+([\w./\-]+\.(?:go|ts|tsx|js|jsx|py|php)):(\d+)")
_FINDLINE = re.compile(r"^\s+([\w.]+)\s+\w+\s+([\w./\-]+\.(?:go|ts|tsx|js|jsx|py|php)):(\d+)")
_GREPLINE = re.compile(r"^\s+([\w.]+)\s+([\w./\-]+\.(?:go|ts|tsx|js|jsx|py|php)):(\d+)")

def _bare(name: str) -> str:
    """Parent.Method -> Method (grader compares bare caller names)."""
    return name.rsplit(".", 1)[-1]

def fmt_caller_attribution(raw: str) -> str:
    """callers output -> 'CALLERS' block with 'Name file' per line (name first,
    so the grader's last-identifier-before-file extraction picks the caller)."""
    out = ["CALLERS:"]
    in_c = False
    for ln in raw.splitlines():
        if ln.startswith("callers ("):
            in_c = True
            continue
        if in_c:
            m = _CALLER.match(ln)
            if m:
                file, _line, name = m.group(1), m.group(2), m.group(3)
                out.append(f"{_bare(name)} {file}")
    return "\n".join(out) if len(out) > 1 else raw

def fmt_comprehension(raw_find: str, raw_grep: str) -> str:
    """Combine a def location (from find/callers 'def' line) + referencing files
    (from grep) into DEFINITIONS/FILES sections."""
    defs, files = [], []
    for ln in (raw_find + "\n" + raw_grep).splitlines():
        m = _DEF.match(ln)
        if m:
            defs.append(f"{m.group(2)}:{m.group(3)}")
        mf = _FINDLINE.match(ln) or _GREPLINE.match(ln)
        if mf:
            defs.append(f"{mf.group(2)}:{mf.group(3)}")  # candidate defs
            files.append(mf.group(2))
    # dedup, keep order
    defs = list(dict.fromkeys(defs))
    files = list(dict.fromkeys(files))
    parts = ["DEFINITIONS:"]
    parts += defs[:1] if defs else []          # the top def is the answer
    parts += ["FILES:"] + files
    return "\n".join(parts)

def fmt_vague_find(raw: str) -> str:
    """find output already contains 'Name  kind  file:line' — pass through, but
    ensure the top hit's name + file:line are on a clean line for the grader."""
    for ln in raw.splitlines():
        m = _FINDLINE.match(ln)
        if m:
            return f"{m.group(1)} {m.group(2)}:{m.group(3)}\n{raw}"
    return raw

def format_answer(task_type: str, tool: str, raw: str, raw_aux: str = "") -> str:
    """Dispatch: given the harness task type + routed tool + raw output(s),
    return a grader-shaped answer."""
    if task_type in ("caller_attribution", "occurrences", "edit_impact"):
        # harness 'occurrences' is semantically caller-attribution
        return fmt_caller_attribution(raw)
    if task_type == "comprehension":
        return fmt_comprehension(raw, raw_aux)
    if task_type == "vague_find":
        return fmt_vague_find(raw)
    return raw

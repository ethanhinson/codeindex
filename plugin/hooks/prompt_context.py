#!/usr/bin/env python3
"""codeindex UserPromptSubmit hook: make the tool's existence ALWAYS VISIBLE.

Why this exists (measured): with the tool documented in an always-visible
system prompt, agent adoption on branch-out tasks was 100% and cost fell 73%
(A/B v2). Packaged only as a lazy-loaded skill, adoption collapsed to ~10%
(v3a) — agents never consult the skill for "easy-looking" tasks. This hook
injects a ~90-token availability note on each prompt so the anchor rule is in
context at decision time. The skill remains for depth; this is the trigger.

Silent (no output) unless: the project is a Go repo AND the codeindex binary is
available. Never blocks; any error exits 0 with no output.
"""

import json
import os
import shutil
import sys

NOTE = (
    "Available in this repo: `codeindex` — a call-graph index (Go, TS/JS, "
    "Python, PHP). "
    "For who-calls-X / what-X-calls / what's-affected-by-changing-X (refactor "
    "impact, affected callers, dead code), run "
    "`codeindex callers <repo-root> <Symbol>` or `codeindex callees <repo-root> "
    "<Symbol>`. Its output is COMPLETE and always fresh: answer directly from "
    "the returned path:line references — do NOT re-verify by reading files, "
    "except entries flagged [ambiguous]. "
    "Do NOT use it to locate/find things (where is X defined, which files "
    "mention Y) — plain grep is cheaper there."
)


def main():
    try:
        payload = json.load(sys.stdin)
    except Exception:
        return
    cwd = payload.get("cwd") or os.getcwd()
    # supported-language repo check: a manifest or source file at the root
    manifests = ("go.mod", "package.json", "pyproject.toml", "setup.py",
                 "composer.json")
    exts = (".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".php")
    try:
        entries = os.listdir(cwd)[:300]
    except OSError:
        return
    supported = (any(m in entries for m in manifests)
                 or any(f.endswith(exts) for f in entries
                        if os.path.isfile(os.path.join(cwd, f))))
    if not supported:
        return
    if not (os.environ.get("CODEINDEX_BIN") or shutil.which("codeindex")):
        return
    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "UserPromptSubmit", "additionalContext": NOTE}}))


if __name__ == "__main__":
    try:
        main()
    except Exception:
        pass
    sys.exit(0)

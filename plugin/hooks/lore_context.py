#!/usr/bin/env python3
"""codeindex UserPromptSubmit hook: make lore always visible.

Why this exists (measured): always-visible notes drive adoption; lazy skills
collapse to ~10% on "easy-looking" tasks (v3a). This hook injects a ≤80-token
lore availability note on each prompt — only in repos that have .lore/ and only
when the codeindex binary is available. The note is the trigger; the MCP tools
and CLI are the depth.

Silent (no output) unless: the project has a .lore/ directory AND the codeindex
binary is available. Never blocks; any error exits 0 with no output.
"""

import json
import os
import shutil
import sys

NOTE = (
    "This repo keeps lore: committed decisions, work items, and notes (.lore/). "
    "Before architectural choices or when past decisions are referenced, use the "
    "lore_search / lore_for_symbol MCP tools (or "
    "`codeindex lore <root> search '<q>'`). "
    "When a decision is made or a non-obvious root cause found, record it with "
    "lore_add — include rejected alternatives. "
    "Active decisions are constraints."
)


def main():
    try:
        payload = json.load(sys.stdin)
    except Exception:
        return
    cwd = payload.get("cwd") or os.getcwd()
    if os.environ.get("CODEINDEX_HOOK_DISABLE") == "1":
        return
    if os.path.exists(os.path.join(cwd, ".codeindex", "hook-disabled")):
        return
    if not os.path.isdir(os.path.join(cwd, ".lore")):
        return
    b = os.environ.get("CODEINDEX_BIN")
    if b and os.path.exists(b):
        pass  # resolved
    elif shutil.which("codeindex"):
        pass  # resolved
    else:
        return
    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "UserPromptSubmit", "additionalContext": NOTE}}))


if __name__ == "__main__":
    try:
        main()
    except Exception:
        pass
    sys.exit(0)

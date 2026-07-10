#!/usr/bin/env python3
"""codeindex post-edit hook: after the agent edits a .go file, report the blast
radius of the symbol(s) it touched — compactly, once per symbol per session.

Every failure path is silent (exit 0, no output): a hook must never block edits.
Noise controls (spec: claude-plugin / "Post-edit blast-radius hook"):
  - fires only when an edited symbol has >=1 caller OUTSIDE the edited file
  - once per symbol per session (dedup file under .codeindex/)
  - <=150-token note, hard-capped
  - disable via CODEINDEX_HOOK_DISABLE=1 or a .codeindex/hook-disabled file
"""

import json
import os
import re
import shutil
import subprocess
import sys

MAX_CHARS = 600  # ~150 tokens
HUNK = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@", re.M)
SYM = re.compile(r"^sym\s+(\S+)\s+(\S+)\s+\S+\s+callers=(\d+)\s+external=(\d+)", re.M)


def run(cmd, cwd=None, timeout=10):
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True,
                          timeout=timeout)


def main():
    payload = json.load(sys.stdin)
    file_path = (payload.get("tool_input") or {}).get("file_path") or ""
    if not file_path.endswith(".go"):
        return

    # repo root: walk up from the file looking for .git
    d = os.path.dirname(os.path.abspath(file_path))
    repo = None
    while d != "/":
        if os.path.isdir(os.path.join(d, ".git")):
            repo = d
            break
        d = os.path.dirname(d)
    if not repo:
        return

    if os.environ.get("CODEINDEX_HOOK_DISABLE") == "1":
        return
    if os.path.exists(os.path.join(repo, ".codeindex", "hook-disabled")):
        return

    binary = os.environ.get("CODEINDEX_BIN") or shutil.which("codeindex")
    if not binary:
        return

    rel = os.path.relpath(file_path, repo)

    # changed line ranges from git (untracked/new files -> no hunks -> silent)
    diff = run(["git", "diff", "-U0", "--", rel], cwd=repo)
    ranges = []
    for m in HUNK.finditer(diff.stdout or ""):
        start = int(m.group(1))
        count = int(m.group(2) or "1")
        if count > 0:
            ranges.append((start, start + count - 1))
    if not ranges:
        return

    # enclosing symbols with external callers, deduped across ranges
    symbols = {}
    for start, end in ranges[:8]:
        out = run([binary, "enclosing", repo, rel, f"{start}:{end}"], cwd=repo,
                  timeout=12)
        for sm in SYM.finditer(out.stdout or ""):
            name, kind, callers, external = sm.group(1), sm.group(2), int(sm.group(3)), int(sm.group(4))
            if external >= 1:
                symbols[name] = (kind, callers, external)
    if not symbols:
        return

    # per-session dedup
    session = (payload.get("session_id") or "nosession")[:16]
    seen_path = os.path.join(repo, ".codeindex", f"hook_seen_{session}.txt")
    try:
        seen = set(open(seen_path).read().split())
    except OSError:
        seen = set()
    fresh = {n: v for n, v in symbols.items() if n not in seen}
    if not fresh:
        return
    try:
        os.makedirs(os.path.dirname(seen_path), exist_ok=True)
        with open(seen_path, "a") as f:
            f.write("".join(n + "\n" for n in fresh))
    except OSError:
        pass

    parts = []
    for name, (kind, callers, external) in list(fresh.items())[:3]:
        # top external caller files, bounded
        files = []
        try:
            cout = run([binary, "callers", repo, name, "--limit", "40"], cwd=repo,
                       timeout=12).stdout or ""
            for line in cout.splitlines():
                fm = re.match(r"\s+([\w./\-]+\.go):\d+", line)
                if fm and fm.group(1) != rel and fm.group(1) not in files:
                    files.append(fm.group(1))
                if len(files) >= 4:
                    break
        except Exception:
            pass
        loc = f" (e.g. {', '.join(files)})" if files else ""
        parts.append(f"'{name}' has {callers} caller(s), {external} outside this "
                     f"file{loc}")
    msg = ("codeindex: you edited " + "; ".join(parts) +
           f". Consider /codeindex:impact before changing behavior or signatures.")[:MAX_CHARS]

    log = os.environ.get("CODEINDEX_HOOK_LOG")
    if log:
        try:
            with open(log, "a") as f:
                f.write("injected " + " ".join(sorted(fresh)) + "\n")
        except OSError:
            pass

    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "PostToolUse", "additionalContext": msg}}))


if __name__ == "__main__":
    try:
        main()
    except Exception:
        pass  # silent by contract
    sys.exit(0)

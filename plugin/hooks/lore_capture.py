#!/usr/bin/env python3
"""Stop hook: pipe the session payload to `codeindex lore capture --stdin`.

Ambient channel of the lore engine: metadata-only session notes into the
private overlay (decays in ranking; promotable). Silent and exit-0 on every
failure path — a capture must never block the agent's stop.
"""
import json, os, shutil, subprocess, sys

def resolve_bin():
    b = os.environ.get("CODEINDEX_BIN")
    if b and os.path.exists(b):
        return b
    return shutil.which("codeindex")

def main():
    try:
        raw = sys.stdin.buffer.read()
        payload = json.loads(raw or b"{}")
        cwd = payload.get("cwd") or os.getcwd()
        if os.environ.get("CODEINDEX_HOOK_DISABLE") == "1":
            return
        if os.path.exists(os.path.join(cwd, ".codeindex", "hook-disabled")):
            return
        if not os.path.isdir(os.path.join(cwd, ".lore")):
            return
        binpath = resolve_bin()
        if not binpath:
            return
        subprocess.run([binpath, "lore", cwd, "capture", "--stdin"],
                       input=raw, capture_output=True, timeout=8)
    except Exception:
        pass

if __name__ == "__main__":
    main()

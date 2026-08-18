#!/usr/bin/env python3
"""Workspace-graph gate runner (design D7 arms).

A (control):   headless claude + shell over all member checkouts on disk —
               grep-across-repos. Honest, not blinded.
B (treatment): A + the workspace-graph MCP surface. GATED on openspec
               workspace-graph §3–4 (the engine does not exist yet); the
               runner refuses arm B until CODEINDEX_WS_MCP_BIN is set to a
               binary that serves `codeindex mcp <workspace-root>`.

Isolation: --setting-sources project,local on every run (the bench-hook-leak
rule — the global codeindex plugin contaminates controls otherwise).

Usage:
  python3 run_ws.py --arm A --smoke            # 2 tasks, 1 rep
  python3 run_ws.py --arm A [--reps 2] [--model MODEL] [--timeout 600]
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent / "agent_ab"))
from run_ab import parse_stream  # noqa: E402

TASKS = json.loads((HERE / "tasks" / "tasks_ws.json").read_text())
WS_ROOT = (HERE.parent.parent
           / json.loads((HERE / "corpus.json").read_text())["workspace_root"]
           ).resolve()
RESULTS = HERE / "results"
TRANSCRIPTS = RESULTS / "transcripts"
RUNS = RESULTS / "runs.jsonl"


def run_one(task, arm, rep, model, timeout):
    prompt = task["prompt"].replace("{WS_ROOT}", str(WS_ROOT))
    deny = "Agent Task NotebookEdit TodoWrite WebFetch WebSearch Edit Write"
    cmd = ["claude", "-p", prompt,
           "--allowedTools", "Bash Read Grep Glob",
           "--disallowedTools", deny,
           "--permission-mode", "bypassPermissions",
           "--setting-sources", "project,local",
           "--output-format", "stream-json", "--verbose"]
    if model:
        cmd += ["--model", model]
    if arm == "B":
        mcp_bin = os.environ.get("CODEINDEX_WS_MCP_BIN")
        if not mcp_bin:
            sys.exit("arm B is gated on the workspace engine (openspec "
                     "workspace-graph §3–4); set CODEINDEX_WS_MCP_BIN when "
                     "it exists")
        mcp_cfg = {"mcpServers": {"codeindex": {
            "command": mcp_bin, "args": ["mcp", str(WS_ROOT)]}}}
        cmd += ["--mcp-config", json.dumps(mcp_cfg),
                "--allowedTools", "mcp__codeindex"]
    key = f"{task['id']}_{arm}_r{rep}"
    t0 = time.time()
    timed_out = False
    try:
        proc = subprocess.run(cmd, cwd=str(WS_ROOT), capture_output=True,
                              text=True, timeout=timeout)
        lines = proc.stdout.splitlines()
    except subprocess.TimeoutExpired as e:
        timed_out = True
        lines = (e.stdout or "").splitlines() if isinstance(e.stdout, str) else []
    TRANSCRIPTS.mkdir(parents=True, exist_ok=True)
    (TRANSCRIPTS / f"{key}.jsonl").write_text("\n".join(lines))
    m = parse_stream(lines)
    return {"key": key, "task_id": task["id"], "kind": task["kind"],
            "rung": task["rung"], "arm": arm, "rep": rep,
            "model": model or "default",
            "duration_s": round(time.time() - t0, 1), "timed_out": timed_out,
            "started": datetime.now(timezone.utc).isoformat(), **m}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--arm", required=True, choices=["A", "B"])
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--model", default=None)
    ap.add_argument("--timeout", type=int, default=600)
    ap.add_argument("--smoke", action="store_true")
    ap.add_argument("--only", help="comma-separated task ids")
    args = ap.parse_args()

    tasks = TASKS["tasks"]
    if args.only:
        want = set(args.only.split(","))
        tasks = [t for t in tasks if t["id"] in want]
    if args.smoke:
        tasks = tasks[:2]

    RESULTS.mkdir(parents=True, exist_ok=True)
    done = set()
    if RUNS.exists():
        done = {json.loads(l)["key"] for l in RUNS.read_text().splitlines() if l}
    with RUNS.open("a") as out:
        for rep in range(1, args.reps + 1):
            for t in tasks:
                key = f"{t['id']}_{args.arm}_r{rep}"
                if key in done:
                    continue
                r = run_one(t, args.arm, rep, args.model, args.timeout)
                out.write(json.dumps(r) + "\n")
                out.flush()
                print(f"{key}: {r['duration_s']}s "
                      f"{'TIMEOUT' if r['timed_out'] else 'ok'}")


if __name__ == "__main__":
    main()

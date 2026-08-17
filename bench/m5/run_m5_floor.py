#!/usr/bin/env python3
"""M5 floor sweep — how small can the model be and still be lifted by the
index? EXPLORATORY (no pre-registered verdict): the deliverable is the curve
of success-with-index by model size, and where tool adoption breaks down.

Design:
- dominate suite only (the discriminating suite; 34 tasks).
- model ladder x {shell, mcp}: treatment is the REAL `codeindex mcp
  <repo-root>` stdio server — the surface any MCP-capable agent gets — NOT
  fuse's stale builtin codeindex tools (disabled in BOTH arms for surface
  purity; the M5 fuse family measured those builtins, see FINDINGS).
- fuse config cannot gain MCP servers from repo-planted .fuse.local.yml
  (tighten-only), so each (repo, treatment) gets a generated HOME dir whose
  trusted ~/.fuse/config.yml carries the user's gateway/model aliases plus
  the codeindex MCP server (treatment) or no MCP servers (control).
- control arms additionally get the failing-shim PATH + CODEINDEX_DISABLED=1;
  treatment arms get NEITHER (the MCP subprocess must run) and the binary is
  not on PATH.
- MCP treatment prompts carry INDEX_NOTE_MCP (adoption needs the note —
  measured in the fuse family).

Usage:
  python3 run_m5_floor.py --smoke                 # 1 dominate task x ladder
  python3 run_m5_floor.py --full
Flags: --models qwen3-coder,qwen-coder,qwen-local,qwen-cloud,glm
       --treatments shell,mcp  --timeout 600  --tasks N
"""

from __future__ import annotations

import argparse
import json
import os
import random
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
BENCH = REPO_ROOT / "bench"
BIN = BENCH / "agent_ab" / ".bin" / "codeindex"
FUSE = Path.home() / "dev" / "fuse" / "fuse"
USER_FUSE_CFG = Path.home() / ".fuse" / "config.yml"
RESULTS = HERE / "results"
TRACES = RESULTS / "fuse_traces_floor"
HOMES = HERE / ".fusehomes"
RUNS = RESULTS / "runs_m5_floor.jsonl"
TASKS = HERE / "tasks" / "tasks_m5.json"

os.environ.setdefault("AB_WORK", str(BENCH / "repos"))
sys.path.insert(0, str(BENCH / "agent_ab"))
sys.path.insert(0, str(BENCH))
sys.path.insert(0, str(HERE))
from run_ab import arm_a_shim_dir, git_reset  # noqa: E402
from build_tasks_m5 import repo_path  # noqa: E402
from run_m5_fuse import parse_trace, BASE_DISABLED, CODEINDEX_TOOLS, load_done  # noqa: E402

INDEX_NOTE_MCP = (
    "Note: this repository has a prebuilt code index exposed as MCP tools: "
    "'callers' lists every function that calls a symbol (with file:line), "
    "'impact' gives a symbol's blast radius (callers + callees), 'nav' gives "
    "a one-call orientation bundle for a symbol, and 'search'/'find' locate "
    "symbols from a description. For who-calls / impact / locate questions "
    "these are usually cheaper and more complete than grep.\n\n"
)


def make_home(repo: Path, treatment: str) -> Path:
    """Generated HOME whose trusted config carries the treatment."""
    home = HOMES / f"{repo.name}-{treatment}"
    fdir = home / ".fuse"
    fdir.mkdir(parents=True, exist_ok=True)
    cfg = yaml.safe_load(USER_FUSE_CFG.read_text())
    # builtin codeindex tools disabled in BOTH arms: the only index surface
    # under test is the MCP server
    cfg["permissions"] = {"mode": "auto",
                          "disabled": BASE_DISABLED + CODEINDEX_TOOLS}
    if treatment == "mcp":
        cfg["mcp_servers"] = [{
            "name": "codeindex", "transport": "stdio",
            "command": [str(BIN), "mcp", str(repo)],
        }]
    else:
        cfg.pop("mcp_servers", None)
    (fdir / "config.yml").write_text(yaml.safe_dump(cfg))
    return home


def run_one(task: dict, model: str, treatment: str, rep: int, repo: Path,
            timeout: int) -> dict:
    prompt = task["prompt"].replace("{REPO_PATH}", str(repo))
    if treatment == "mcp":
        prompt = INDEX_NOTE_MCP + prompt
    key = f"{task['id']}_{model}_{treatment}_r{rep}"
    TRACES.mkdir(parents=True, exist_ok=True)
    trace = TRACES / f"{key}.trace"

    env = dict(os.environ)
    env["HOME"] = str(make_home(repo, treatment))
    if treatment == "shell":
        env.pop("CODEINDEX_BIN", None)
        env["CODEINDEX_DISABLED"] = "1"
        env["PATH"] = f"{arm_a_shim_dir()}{os.pathsep}{env.get('PATH', '')}"
    else:
        env.pop("CODEINDEX_BIN", None)
        env.pop("CODEINDEX_DISABLED", None)

    cmd = [str(FUSE), "-model", model, "-approve-all", "-trace", str(trace),
           prompt]
    started = datetime.now(timezone.utc).isoformat()
    t0 = time.time()
    timed_out = False
    stderr_tail = ""
    try:
        proc = subprocess.run(cmd, cwd=str(repo), capture_output=True,
                              text=True, timeout=timeout, env=env)
        stderr_tail = proc.stderr[-500:]
    except subprocess.TimeoutExpired:
        timed_out = True
        stderr_tail = "TIMEOUT"
    dur = time.time() - t0

    metrics = parse_trace(trace)
    if timed_out:
        metrics["has_result"] = False
    return {
        "key": key, "task_id": task["id"], "type": task["type"],
        "suite": task["suite"], "repo": task["repo"],
        "arm": f"{model}|{treatment}", "model": model,
        "treatment": treatment, "rep": rep, "harness": "fuse-floor",
        "started": started, "duration_s": round(dur, 1),
        "timed_out": timed_out, "stderr_tail": stderr_tail,
        "transcript": str(trace.relative_to(REPO_ROOT)),
        "hook_fires": 0,
        **metrics,
    }


def build_index(repo: Path):
    subprocess.run([str(BIN), "build", str(repo)], capture_output=True,
                   check=True)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--smoke", action="store_true")
    ap.add_argument("--full", action="store_true")
    ap.add_argument("--tasks", type=int, default=0)
    ap.add_argument("--models",
                    default="qwen3-coder,qwen-coder,qwen-local,qwen-cloud,glm")
    ap.add_argument("--treatments", default="shell,mcp")
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--timeout", type=int, default=600)
    ap.add_argument("--seed", type=int, default=20260817)
    args = ap.parse_args()
    if not (args.smoke or args.full):
        args.smoke = True

    if not FUSE.exists():
        sys.exit(f"fuse binary missing: {FUSE}")
    models = [m.strip() for m in args.models.split(",") if m.strip()]
    treatments = [t.strip() for t in args.treatments.split(",") if t.strip()]
    tasks = [t for t in json.loads(TASKS.read_text())["tasks"]
             if t["suite"] == "dominate"]

    if args.smoke:
        tasks = tasks[:1]
        print(f"SMOKE MODE: 1 task x {len(models)} models x {len(treatments)}")
    elif args.tasks:
        tasks = tasks[: args.tasks]

    used = {t["repo"] for t in tasks}
    repo_paths = {}
    for name in used:
        rp = repo_path(name)
        if not rp.exists():
            sys.exit(f"repo clone missing: {rp}")
        git_reset(rp)
        build_index(rp)
        repo_paths[name] = rp
    print(f"indexes built for: {sorted(used)}")

    matrix = [(t, m, tr, r) for t in tasks for m in models
              for tr in treatments for r in range(args.reps)]
    random.Random(args.seed).shuffle(matrix)

    done = load_done(RUNS)
    RESULTS.mkdir(parents=True, exist_ok=True)
    ran = 0
    for task, model, treatment, rep in matrix:
        key = f"{task['id']}_{model}_{treatment}_r{rep}"
        if key in done:
            continue
        rp = repo_paths[task["repo"]]
        git_reset(rp)
        print(f"  [{ran+1}] {key} ...", flush=True)
        row = run_one(task, model, treatment, rep, rp, args.timeout)
        git_reset(rp)
        with open(RUNS, "a") as f:
            f.write(json.dumps(row) + "\n")
        print(f"       turns={row['num_turns']} tok={row['processed_tokens']} "
              f"codeindex_calls={row['codeindex_calls']} "
              f"answer_len={len(row.get('answer') or '')} "
              f"errors={row['n_gateway_errors']} "
              f"{'TIMEOUT' if row['timed_out'] else ''}")
        ran += 1

    print(f"\ndone: {ran} new runs -> {RUNS}")


if __name__ == "__main__":
    main()

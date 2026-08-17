#!/usr/bin/env python3
"""M5 runner — three arms over the adversarial task set, via headless
`claude -p` with per-run isolation and crash-safe capture.

Arms:
  A  frontier model + shell tools only (control; codeindex shimmed out AND
     CODEINDEX_DISABLED=1, same belt-and-suspenders as agent_ab).
  B  frontier model + shell tools + the REAL packaged plugin (--plugin-dir),
     binary discoverable via PATH + CODEINDEX_BIN.
  C  cheap explorer model + shell tools only (arm-A environment, cheap model).
     Exists to test the kill gate: if C matches B, the index doesn't pay.

Usage:
  python3 run_m5.py --smoke                    # 1 task/suite x 3 arms x 1 rep
  python3 run_m5.py --full --budget-usd 80
Flags: --arms A,B,C  --reps  --model-frontier  --model-cheap  --timeout
       --budget-usd  --seed  --tasks N
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

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
BENCH = REPO_ROOT / "bench"
BIN = BENCH / "agent_ab" / ".bin" / "codeindex"
PLUGIN_DIR = REPO_ROOT / "plugin"
TASKS = HERE / "tasks" / "tasks_m5.json"
RESULTS = HERE / "results"
TRANSCRIPTS = RESULTS / "transcripts"
RUNS = RESULTS / "runs_m5.jsonl"

os.environ.setdefault("AB_WORK", str(BENCH / "repos"))
sys.path.insert(0, str(BENCH / "agent_ab"))
sys.path.insert(0, str(BENCH))
from run_ab import parse_stream, arm_a_shim_dir, git_reset, claude_version  # noqa: E402

from build_tasks_m5 import repo_path  # noqa: E402


def build_index(repo: Path):
    subprocess.run([str(BIN), "build", str(repo)], capture_output=True,
                   check=True)


def run_one(task: dict, arm: str, rep: int, repo: Path, model: str,
            timeout: int) -> dict:
    prompt = task["prompt"].replace("{REPO_PATH}", str(repo))
    # all M5 tasks are read-only; hard-deny mutation and sub-agents (they
    # break token/turn accounting), same rationale as agent_ab
    allow = "Bash Read Grep Glob"
    deny = ("Agent Task NotebookEdit TodoWrite WebFetch WebSearch Edit Write")
    # --setting-sources project,local: drop USER-level settings — the codeindex
    # plugin is installed globally on this machine and its prompt hook would
    # otherwise tell every arm (including controls) that codeindex exists.
    # Verified by probe: default run answers YES to "were you told about
    # codeindex", project,local answers NO. Arm B gets the plugin explicitly
    # via --plugin-dir (session-scoped, independent of setting sources).
    cmd = ["claude", "-p", prompt, "--model", model,
           "--allowedTools", allow, "--disallowedTools", deny,
           "--permission-mode", "bypassPermissions",
           "--setting-sources", "project,local",
           "--output-format", "stream-json", "--verbose"]

    key = f"{task['id']}_{arm}_r{rep}"
    env = dict(os.environ)
    hook_log = None
    if arm in ("A", "C"):
        # control isolation: shim shadows any PATH codeindex; env kill-switch
        # catches absolute-path/alias routes the shim misses
        env.pop("CODEINDEX_BIN", None)
        env["CODEINDEX_DISABLED"] = "1"
        env["PATH"] = f"{arm_a_shim_dir()}{os.pathsep}{env.get('PATH', '')}"
    else:  # B: the shipped plugin surface, as measured in the note A/B
        cmd += ["--plugin-dir", str(PLUGIN_DIR)]
        env["PATH"] = f"{BIN.parent}{os.pathsep}{env.get('PATH', '')}"
        env["CODEINDEX_BIN"] = str(BIN)
        hook_log = RESULTS / "hooklogs" / f"{key}.log"
        hook_log.parent.mkdir(parents=True, exist_ok=True)
        env["CODEINDEX_HOOK_LOG"] = str(hook_log)

    started = datetime.now(timezone.utc).isoformat()
    t0 = time.time()
    timed_out = False
    try:
        proc = subprocess.run(cmd, cwd=str(repo), capture_output=True,
                              text=True, timeout=timeout, env=env)
        lines = proc.stdout.splitlines()
        stderr_tail = proc.stderr[-500:]
    except subprocess.TimeoutExpired as e:
        timed_out = True
        lines = (e.stdout or "").splitlines() if isinstance(e.stdout, str) else []
        stderr_tail = "TIMEOUT"
    dur = time.time() - t0
    hook_fires = 0
    if hook_log and hook_log.exists():
        hook_fires = len(hook_log.read_text().splitlines())

    TRANSCRIPTS.mkdir(parents=True, exist_ok=True)
    tpath = TRANSCRIPTS / f"{key}.jsonl"
    tpath.write_text("\n".join(lines))

    metrics = parse_stream(lines)
    return {
        "key": key, "task_id": task["id"], "type": task["type"],
        "suite": task["suite"], "repo": task["repo"], "arm": arm, "rep": rep,
        "model": model, "started": started, "duration_s": round(dur, 1),
        "timed_out": timed_out, "stderr_tail": stderr_tail,
        "transcript": str(tpath.relative_to(REPO_ROOT)),
        "hook_fires": hook_fires,
        **metrics,
    }


def load_done(runs_file: Path) -> set:
    done = set()
    if runs_file.exists():
        for ln in runs_file.read_text().splitlines():
            try:
                done.add(json.loads(ln)["key"])
            except Exception:
                pass
    return done


def spent_so_far(runs_file: Path) -> float:
    total = 0.0
    if runs_file.exists():
        for ln in runs_file.read_text().splitlines():
            try:
                total += json.loads(ln).get("cost_usd") or 0.0
            except Exception:
                pass
    return total


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--smoke", action="store_true")
    ap.add_argument("--full", action="store_true")
    ap.add_argument("--tasks", type=int, default=0, help="cap number of tasks")
    ap.add_argument("--arms", default="A,B,C")
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--model-frontier", default="claude-sonnet-4-6")
    ap.add_argument("--model-cheap", default="claude-haiku-4-5")
    ap.add_argument("--timeout", type=int, default=300)
    ap.add_argument("--budget-usd", type=float, default=80.0)
    ap.add_argument("--seed", type=int, default=20260816)
    args = ap.parse_args()
    if not (args.smoke or args.full):
        args.smoke = True

    data = json.loads(TASKS.read_text())
    tasks = data["tasks"]
    arms = [a.strip() for a in args.arms.split(",") if a.strip()]

    if args.smoke:
        picked, seen = [], set()
        for t in tasks:
            if t["suite"] not in seen:
                seen.add(t["suite"])
                picked.append(t)
        tasks = picked
        reps = 1
        print(f"SMOKE MODE: {len(tasks)} tasks x {len(arms)} arms x 1 rep")
    else:
        reps = args.reps
        if args.tasks:
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

    matrix = [(t, a, r) for t in tasks for a in arms for r in range(reps)]
    random.Random(args.seed).shuffle(matrix)

    done = load_done(RUNS)
    cli_ver = claude_version()
    RESULTS.mkdir(parents=True, exist_ok=True)
    ran = 0
    for task, arm, rep in matrix:
        key = f"{task['id']}_{arm}_r{rep}"
        if key in done:
            continue
        spent = spent_so_far(RUNS)
        if spent >= args.budget_usd:
            print(f"BUDGET STOP: ${spent:.2f} >= ${args.budget_usd}")
            break
        model = args.model_cheap if arm == "C" else args.model_frontier
        rp = repo_paths[task["repo"]]
        git_reset(rp)
        print(f"  [{ran+1}] {key} (spent ${spent:.2f}) ...", flush=True)
        row = run_one(task, arm, rep, rp, model, args.timeout)
        row["claude_version"] = cli_ver
        git_reset(rp)
        with open(RUNS, "a") as f:
            f.write(json.dumps(row) + "\n")
        c = row.get("cost_usd")
        print(f"       cost=${c if c else 0:.4f} turns={row.get('num_turns')} "
              f"codeindex_calls={row['codeindex_calls']} "
              f"answer_len={len(row.get('answer') or '')} "
              f"{'TIMEOUT' if row['timed_out'] else ''}")
        ran += 1

    print(f"\ndone: {ran} new runs, total spent ${spent_so_far(RUNS):.2f}")
    if args.smoke:
        print("Inspect results/transcripts/, then grade_m5.py + gate_m5.py "
              "before --full.")


if __name__ == "__main__":
    main()

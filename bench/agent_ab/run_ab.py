#!/usr/bin/env python3
"""A/B runner: execute each task in arm A (control) and arm B (codeindex), via
headless `claude -p`, with per-run isolation and crash-safe metric capture.

Verified environment reality (see README):
- claude 2.1.193, NO --max-turns; bound by per-run timeout + --budget-usd.
- stream-json result event carries total_cost_usd / num_turns / usage.
- file reads land in cache_creation_input_tokens (primary metric = cost).

Usage:
  python3 run_ab.py --smoke                 # 1 comp + 1 loc, both arms, 1 rep
  python3 run_ab.py --full --budget-usd 60  # full matrix
Flags: --tasks N, --reps R, --model, --timeout SEC, --budget-usd, --seed.
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
REPO_ROOT = HERE.parent.parent
BIN = HERE / ".bin" / "codeindex"
ARM_B_PROMPT = HERE / "prompts" / "arm_b_tools.md"
TASKS = HERE / "tasks" / "tasks.json"
RESULTS = HERE / "results"
TRANSCRIPTS = RESULTS / "transcripts"
RUNS = RESULTS / "runs.jsonl"

sys.path.insert(0, str(HERE.parent))
from build_tasks import repo_path, load_repos  # noqa: E402


def claude_version() -> str:
    try:
        return subprocess.run(["claude", "--version"], capture_output=True,
                              text=True, timeout=30).stdout.strip()
    except Exception:
        return "unknown"


def git_reset(repo: Path):
    subprocess.run(["git", "-C", str(repo), "checkout", "--", "."],
                   capture_output=True)
    # remove stray untracked files but keep the gitignored .codeindex index
    subprocess.run(["git", "-C", str(repo), "clean", "-fd", "-e", ".codeindex"],
                   capture_output=True)


def build_index(repo: Path):
    subprocess.run([str(BIN), "build", str(repo)], capture_output=True, check=True)


def parse_stream(lines: list[str]) -> dict:
    """Extract metrics from stream-json output lines."""
    tool_calls: dict[str, int] = {}
    codeindex_calls = 0
    result = None
    for ln in lines:
        ln = ln.strip()
        if not ln:
            continue
        try:
            ev = json.loads(ln)
        except json.JSONDecodeError:
            continue
        if ev.get("type") == "assistant":
            for b in ev.get("message", {}).get("content", []):
                if b.get("type") == "tool_use":
                    name = b.get("name", "?")
                    tool_calls[name] = tool_calls.get(name, 0) + 1
                    if name in ("Bash", "SlashCommand", "Skill"):
                        cmd = json.dumps(b.get("input", {})).lower()
                        if "codeindex" in cmd:
                            codeindex_calls += 1
        elif ev.get("type") == "result":
            result = ev
    u = (result or {}).get("usage", {}) or {}
    inp = u.get("input_tokens", 0)
    out = u.get("output_tokens", 0)
    cc = u.get("cache_creation_input_tokens", 0)
    cr = u.get("cache_read_input_tokens", 0)
    return {
        "cost_usd": (result or {}).get("total_cost_usd"),
        "num_turns": (result or {}).get("num_turns"),
        "input_tokens": inp, "output_tokens": out,
        "cache_creation_tokens": cc, "cache_read_tokens": cr,
        "processed_tokens": inp + out + cc,   # secondary metric
        "is_error": (result or {}).get("is_error"),
        "subtype": (result or {}).get("subtype"),
        "answer": (result or {}).get("result", ""),
        "tool_calls": tool_calls, "codeindex_calls": codeindex_calls,
        "has_result": result is not None,
    }


PLUGIN_DIR = REPO_ROOT / "plugin"


def run_one(task: dict, arm: str, rep: int, repo: Path, model: str,
            timeout: int, plugin_arm: bool = False) -> dict:
    prompt = task["prompt"].replace("{REPO_PATH}", str(repo))
    # bypassPermissions ignores --allowedTools (everything is permitted), so we
    # must HARD-DENY confounders: sub-agents (Agent/Task) break token/turn
    # accounting and can bury the answer in a non-final message. edit_impact
    # tasks need Edit/Write (both arms — parity); everything else is read-only.
    deny = "Agent Task NotebookEdit TodoWrite WebFetch WebSearch"
    allow = "Bash Read Grep Glob"
    if task["type"] == "edit_impact":
        allow += " Edit Write"
    else:
        deny += " Edit Write"
    cmd = ["claude", "-p", prompt, "--model", model,
           "--allowedTools", allow, "--disallowedTools", deny,
           "--permission-mode", "bypassPermissions",
           "--output-format", "stream-json", "--verbose"]

    key = f"{task['id']}_{arm}_r{rep}"
    env = dict(os.environ)
    hook_log = None
    if arm == "B":
        if plugin_arm:
            # v3: the REAL packaged plugin — skill, commands, hook. Binary is
            # discoverable via PATH + CODEINDEX_BIN; hook fires logged per run.
            cmd += ["--plugin-dir", str(PLUGIN_DIR)]
            env["PATH"] = f"{BIN.parent}:{env.get('PATH', '')}"
            env["CODEINDEX_BIN"] = str(BIN)
            hook_log = RESULTS / "hooklogs" / f"{key}.log"
            hook_log.parent.mkdir(parents=True, exist_ok=True)
            env["CODEINDEX_HOOK_LOG"] = str(hook_log)
        else:
            sysprompt = (ARM_B_PROMPT.read_text()
                         .replace("{CODEINDEX_BIN}", str(BIN))
                         .replace("{REPO_PATH}", str(repo)))
            cmd += ["--append-system-prompt", sysprompt]

    started = datetime.now(timezone.utc).isoformat()
    t0 = time.time()
    timed_out = False
    try:
        proc = subprocess.run(cmd, cwd=str(repo), capture_output=True, text=True,
                              timeout=timeout, env=env)
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
        "repo": task["repo"], "arm": arm, "rep": rep, "model": model,
        "started": started, "duration_s": round(dur, 1),
        "timed_out": timed_out, "stderr_tail": stderr_tail,
        "transcript": str(tpath.relative_to(REPO_ROOT)),
        "hook_fires": hook_fires,
        **metrics,
    }


def load_done(runs_file: Path) -> set:
    if not runs_file.exists():
        return set()
    done = set()
    for ln in runs_file.read_text().splitlines():
        try:
            done.add(json.loads(ln)["key"])
        except Exception:
            pass
    return done


def spent_so_far(runs_file: Path) -> float:
    if not runs_file.exists():
        return 0.0
    total = 0.0
    for ln in runs_file.read_text().splitlines():
        try:
            c = json.loads(ln).get("cost_usd")
            if c:
                total += c
        except Exception:
            pass
    return total


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--smoke", action="store_true")
    ap.add_argument("--full", action="store_true")
    ap.add_argument("--tasks", type=int, default=0, help="cap number of tasks")
    ap.add_argument("--reps", type=int, default=2)
    ap.add_argument("--model", default="claude-sonnet-4-6")
    ap.add_argument("--timeout", type=int, default=300)
    ap.add_argument("--budget-usd", type=float, default=60.0)
    ap.add_argument("--seed", type=int, default=1729)
    ap.add_argument("--tag", default="", help="suffix for tasks/runs files (e.g. v2)")
    ap.add_argument("--plugin-arm", action="store_true",
                    help="arm B uses --plugin-dir with the real plugin (v3 gate)")
    args = ap.parse_args()

    if not (args.smoke or args.full):
        args.smoke = True  # smoke is the default guard

    suffix = f"_{args.tag}" if args.tag else ""
    tasks_file = HERE / "tasks" / f"tasks{suffix}.json"
    runs_file = RESULTS / f"runs{suffix}.jsonl"
    data = json.loads(tasks_file.read_text())
    tasks = data["tasks"]
    repos = load_repos()

    if args.smoke:
        # one task of each distinct type present, up to 2
        seen, picked = set(), []
        for t in tasks:
            if t["type"] not in seen:
                seen.add(t["type"]); picked.append(t)
            if len(picked) == 2:
                break
        tasks, reps = picked, 1
        print(f"SMOKE MODE: {len(tasks)} tasks x 2 arms x 1 rep")
    else:
        reps = args.reps
        if args.tasks:
            tasks = tasks[: args.tasks]

    # Build indexes once per repo used.
    used_repos = {t["repo"] for t in tasks}
    repo_paths = {}
    for name in used_repos:
        rp = repo_path(name)
        if not rp.exists():
            sys.exit(f"repo clone missing: {rp}")
        git_reset(rp)
        build_index(rp)
        repo_paths[name] = rp
    print(f"indexes built for: {sorted(used_repos)}")

    # Build + seeded-shuffle the run matrix.
    import random
    matrix = [(t, arm, r) for t in tasks for arm in ("A", "B") for r in range(reps)]
    random.Random(args.seed).shuffle(matrix)

    done = load_done(runs_file)
    cli_ver = claude_version()
    RESULTS.mkdir(parents=True, exist_ok=True)
    ran = 0
    for task, arm, rep in matrix:
        key = f"{task['id']}_{arm}_r{rep}"
        if key in done:
            continue
        spent = spent_so_far(runs_file)
        if spent >= args.budget_usd:
            print(f"BUDGET STOP: ${spent:.2f} >= ${args.budget_usd}")
            break
        rp = repo_paths[task["repo"]]
        git_reset(rp)
        print(f"  [{ran+1}] {key} (spent ${spent:.2f}) ...", flush=True)
        row = run_one(task, arm, rep, rp, args.model, args.timeout,
                      plugin_arm=args.plugin_arm)
        row["claude_version"] = cli_ver
        row["task_sha"] = data["header"].get("generated_seed")
        git_reset(rp)
        with open(runs_file, "a") as f:
            f.write(json.dumps(row) + "\n")
        c = row.get("cost_usd")
        print(f"       cost=${c if c else 0:.4f} turns={row.get('num_turns')} "
              f"codeindex_calls={row['codeindex_calls']} "
              f"answer_len={len(row.get('answer') or '')} "
              f"{'TIMEOUT' if row['timed_out'] else ''}")
        ran += 1

    print(f"\ndone: {ran} new runs, total spent ${spent_so_far(runs_file):.2f}")
    if args.smoke:
        print("Inspect results/transcripts/*.jsonl and results/runs.jsonl, then "
              "run grade.py + report.py before --full.")


if __name__ == "__main__":
    main()

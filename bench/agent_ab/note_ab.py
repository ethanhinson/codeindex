#!/usr/bin/env python3
"""Prompt-note A/B: current plugin note vs nav-aware note (NEXT_STEPS 6b).

Both arms run the REAL packaged plugin via --plugin-dir; the ONLY difference
is one sentence in the UserPromptSubmit note advertising `codeindex nav`.
The nav variant is materialized from the live plugin source at startup, so
it cannot drift from what ships.

Gate (pre-registered): flip the shipped note only if the nav arm's success
delta >= -5pp AND median paired cost delta <= +10% AND agents actually call
nav (nav_calls > 0 somewhere). Everything else = keep the current note.

Usage:
  .venv/bin/python note_ab.py --smoke
  .venv/bin/python note_ab.py --full --reps 2 --budget-usd 15
"""
from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
BIN = HERE / ".bin" / "codeindex"
PLUGIN_SRC = REPO_ROOT / "plugin"
PLUGIN_NAV = HERE / ".plugin_nav"
TASKS = HERE / "tasks" / "tasks_v6.json"
RESULTS = HERE / "results"
TRANSCRIPTS = RESULTS / "transcripts_noteab"
RUNS = RESULTS / "note_ab_runs.jsonl"

sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent))
from run_ab import parse_stream, git_reset, load_done, spent_so_far  # noqa: E402
from build_tasks import repo_path  # noqa: E402
import grade as G  # noqa: E402

# The one-sentence treatment, inserted after the callers/callees sentence so
# the existing trust instruction ("output is COMPLETE...") covers nav too.
NAV_SENTENCE = (
    "To orient on a symbol you know — where defined, who calls it, which "
    "files reference it — run `codeindex nav <repo-root> <Symbol>`: one "
    "call returns all three. "
)


def materialize_nav_plugin() -> Path:
    """Copy the live plugin and add the nav sentence to its prompt note."""
    if PLUGIN_NAV.exists():
        shutil.rmtree(PLUGIN_NAV)
    shutil.copytree(PLUGIN_SRC, PLUGIN_NAV,
                    ignore=shutil.ignore_patterns("__pycache__"))
    hook = PLUGIN_NAV / "hooks" / "prompt_context.py"
    src = hook.read_text()
    # anchor on the end of the callers/callees sentence inside the NOTE literal
    anchor = '"<Symbol>`. Its output'
    assert src.count(anchor) == 1, "NOTE anchor not found — plugin note changed?"
    src = src.replace(
        anchor, '"<Symbol>`. "\n    "' + NAV_SENTENCE + '"\n    "Its output')
    hook.write_text(src)
    # sanity: the variant must import and print a note containing 'nav'
    chk = subprocess.run([sys.executable, str(hook)], input='{"cwd": "%s"}' %
                         repo_path("gin"), capture_output=True, text=True,
                         env={**os.environ, "CODEINDEX_BIN": str(BIN)})
    out = chk.stdout
    assert "codeindex nav" in out, f"variant note did not render: {out[:200]}"
    return PLUGIN_NAV


ARMS = {"cur": PLUGIN_SRC, "nav": PLUGIN_NAV}


def nav_calls_in(lines: list[str]) -> int:
    n = 0
    for ln in lines:
        try:
            ev = json.loads(ln)
        except json.JSONDecodeError:
            continue
        if ev.get("type") == "assistant":
            for b in ev.get("message", {}).get("content", []):
                if (b.get("type") == "tool_use" and b.get("name") == "Bash"
                        and " nav " in b.get("input", {}).get("command", "")):
                    n += 1
    return n


def grade_answer(task: dict, answer: str, repo: Path):
    prefix = str(repo).rstrip("/") + "/"
    gt = task["ground_truth"]
    tp = task["type"]
    if tp == "comprehension":
        return G.grade_comprehension(answer, gt, prefix)
    if tp == "vague_find":
        return G.grade_vague_find(answer, gt, prefix)
    return G.grade_caller_attribution(answer, gt, prefix)  # occurrences


def run_one(task: dict, arm: str, rep: int, repo: Path, model: str,
            timeout: int) -> dict:
    prompt = task["prompt"].replace("{REPO_PATH}", str(repo))
    deny = "Agent Task NotebookEdit TodoWrite WebFetch WebSearch Edit Write"
    cmd = ["claude", "-p", prompt, "--model", model,
           "--allowedTools", "Bash Read Grep Glob",
           "--disallowedTools", deny,
           "--permission-mode", "bypassPermissions",
           "--output-format", "stream-json", "--verbose",
           "--plugin-dir", str(ARMS[arm])]
    key = f"{task['id']}_{arm}_r{rep}"
    env = dict(os.environ)
    env.pop("CODEINDEX_DISABLED", None)
    env["PATH"] = f"{BIN.parent}:{env.get('PATH', '')}"
    env["CODEINDEX_BIN"] = str(BIN)

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

    TRANSCRIPTS.mkdir(parents=True, exist_ok=True)
    (TRANSCRIPTS / f"{key}.jsonl").write_text("\n".join(lines))

    m = parse_stream(lines)
    score, detail, unparseable = grade_answer(task, m.get("answer") or "", repo)
    return {
        "key": key, "task_id": task["id"], "type": task["type"], "arm": arm,
        "rep": rep, "model": model, "started": started,
        "duration_s": round(dur, 1), "timed_out": timed_out,
        "stderr_tail": stderr_tail if timed_out else "",
        "nav_calls": nav_calls_in(lines),
        "f1": round(score, 3), "success": score >= 0.5,
        "unparseable": unparseable, **m,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--smoke", action="store_true")
    ap.add_argument("--full", action="store_true")
    ap.add_argument("--reps", type=int, default=2)
    ap.add_argument("--model", default="claude-sonnet-4-6")
    ap.add_argument("--timeout", type=int, default=300)
    ap.add_argument("--budget-usd", type=float, default=15.0)
    args = ap.parse_args()

    materialize_nav_plugin()
    tasks = json.loads(TASKS.read_text())["tasks"]
    # one clone + index per repo the task set actually uses
    repos = {}
    for name in {t["repo"] for t in tasks}:
        rp = repo_path(name)
        assert rp.exists(), f"missing work clone for {name}: {rp}"
        subprocess.run([str(BIN), "build", str(rp)], capture_output=True,
                       check=True)
        repos[name] = rp

    if args.smoke:
        seen, picked = set(), []
        for t in tasks:
            if t["type"] not in seen:
                seen.add(t["type"]); picked.append(t)
        tasks, reps = picked[:2], 1
        print(f"SMOKE: {len(tasks)} tasks x 2 arms x 1 rep")
    else:
        reps = args.reps

    done = load_done(RUNS)
    RUNS.parent.mkdir(parents=True, exist_ok=True)
    with open(RUNS, "a") as out:
        for rep in range(reps):
            for i, task in enumerate(tasks):
                # alternate arm order per task to balance time-of-day drift
                order = ("cur", "nav") if i % 2 == 0 else ("nav", "cur")
                for arm in order:
                    key = f"{task['id']}_{arm}_r{rep}"
                    if key in done:
                        continue
                    spent = spent_so_far(RUNS)
                    if spent >= args.budget_usd:
                        print(f"BUDGET HIT (${spent:.2f}) — stopping.")
                        return
                    row = run_one(task, arm, rep, repos[task["repo"]],
                                  args.model, args.timeout)
                    out.write(json.dumps(row) + "\n")
                    out.flush()
                    git_reset(repos[task["repo"]])
                    print(f"{key}: f1={row['f1']} succ={row['success']} "
                          f"cost=${row['cost_usd'] or 0:.3f} "
                          f"turns={row['num_turns']} nav={row['nav_calls']} "
                          f"ci={row['codeindex_calls']}")
    print(f"\ntotal spent: ${spent_so_far(RUNS):.2f}")


if __name__ == "__main__":
    main()

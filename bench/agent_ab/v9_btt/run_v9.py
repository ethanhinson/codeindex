#!/usr/bin/env python3
"""v8 runner: concept-class A/B via headless claude -p (reuses run_ab parsing)."""
import argparse, json, os, subprocess, sys, time
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
AB = HERE.parent
sys.path.insert(0, str(AB))
from run_ab import parse_stream  # noqa: E402

REPO_ROOT = AB.parent.parent
BIN = Path("/tmp/codeindex-lit")
TASKS = json.loads((HERE / "tasks.json").read_text())
RESULTS = HERE / "results"
TRANSCRIPTS = RESULTS / "transcripts"
RUNS = RESULTS / "runs.jsonl"

def run_one(task, arm, rep, model, timeout):
    repo = REPO_ROOT / TASKS["header"]["repo_pins"][task["repo"]]["path"]
    prompt = task["prompt"].replace("{REPO_PATH}", str(repo))
    deny = "Agent Task NotebookEdit TodoWrite WebFetch WebSearch Edit Write"
    cmd = ["claude", "-p", prompt, "--model", model,
           "--allowedTools", "Bash Read Grep Glob", "--disallowedTools", deny,
           "--permission-mode", "bypassPermissions",
           "--output-format", "stream-json", "--verbose"]
    if arm == "B":
        sysprompt = (HERE / "../v8_search/arm_b_search.md").read_text() \
            .replace("{CODEINDEX_BIN}", str(BIN)).replace("{REPO_PATH}", str(repo))
        cmd += ["--append-system-prompt", sysprompt]
    key = f"{task['id']}_{arm}_r{rep}"
    t0 = time.time(); timed_out = False
    try:
        proc = subprocess.run(cmd, cwd=str(repo), capture_output=True, text=True, timeout=timeout)
        lines = proc.stdout.splitlines()
    except subprocess.TimeoutExpired as e:
        timed_out = True
        lines = (e.stdout or "").splitlines() if isinstance(e.stdout, str) else []
    TRANSCRIPTS.mkdir(parents=True, exist_ok=True)
    (TRANSCRIPTS / f"{key}.jsonl").write_text("\n".join(lines))
    m = parse_stream(lines)
    return {"key": key, "task_id": task["id"], "repo": task["repo"], "arm": arm,
            "rep": rep, "model": model, "duration_s": round(time.time()-t0, 1),
            "timed_out": timed_out,
            "started": datetime.now(timezone.utc).isoformat(), **m}

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--smoke", action="store_true")
    ap.add_argument("--reps", type=int, default=2)
    ap.add_argument("--timeout", type=int, default=300)
    ap.add_argument("--model", default=TASKS["header"]["model"])
    args = ap.parse_args()
    tasks = TASKS["tasks"][:1] if args.smoke else TASKS["tasks"]
    reps = 1 if args.smoke else args.reps
    RESULTS.mkdir(parents=True, exist_ok=True)
    done = set()
    if RUNS.exists():
        for ln in RUNS.read_text().splitlines():
            try: done.add(json.loads(ln)["key"])
            except Exception: pass
    with RUNS.open("a") as out:
        for rep in range(1, reps+1):
            for t in tasks:
                for arm in ("A", "B"):
                    key = f"{t['id']}_{arm}_r{rep}"
                    if key in done:
                        continue
                    r = run_one(t, arm, rep, args.model, args.timeout)
                    out.write(json.dumps(r) + "\n"); out.flush()
                    print(f"{key}: ok={r.get('has_result')} cost={r.get('total_cost_usd')} "
                          f"turns={r.get('num_turns')} ci_calls={r.get('codeindex_calls')}",
                          flush=True)

if __name__ == "__main__":
    main()

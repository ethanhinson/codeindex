#!/usr/bin/env python3
"""Workspace-graph gate runner, fuse-family — the OSS workspace corpus
through one-shot `fuse` driving the local LiteLLM gateway (m5-floor
mechanics; see run_m5_floor.py for the pedigree of each mechanism).

Arms (same D7 semantics as run_ws.py, different harness — NEVER compared
across harnesses):
  A (control):   fuse with shell tools only over the member checkouts.
                 Failing codeindex shim first on PATH + CODEINDEX_DISABLED=1;
                 generated HOME config carries NO MCP servers.
  B (treatment): A's shell surface + ONE MCP server: the workspace engine
                 served by $CODEINDEX_WS_MCP_BIN (`<bin> mcp <ws-root>`).
                 No shim, binary not on PATH — the MCP surface is the only
                 index route. Prompt carries INDEX_NOTE_WS (adoption needs
                 the note — measured in the m5 fuse family).

fuse's stale builtin codeindex tools are disabled in BOTH arms (surface
purity, same rationale as run_m5_floor.py). fuse config cannot gain MCP
servers from a repo-planted .fuse.local.yml (tighten-only), so each arm gets
a generated HOME dir whose trusted ~/.fuse/config.yml carries the user's
gateway/model aliases plus the workspace MCP server (B) or none (A).

The runner does NOT verify workspace-index absence for arm A — the driver
owns the B4 stash/verify — but it prints a loud warning if any .codeindex
directory is visible under bench/repos.

Rows append to results/runs.jsonl in the shape grade_ws.py consumes
unmodified: `tool_calls` is keyed by claude-CLI-style names (Bash/Grep/
Glob/Read, mcp__codeindex__*) via FUSE_TOOL_MAP, so grade_ws's shell_calls /
mcp_calls arithmetic works on fuse rows with zero grader changes. Fuse keys
embed the model, so they can never collide with run_ws.py's claude-CLI keys
in the shared file — but grade_ws pools by ARM only, so grade fuse runs from
a dedicated file (--runs-out) or a filtered copy, never mixed with claude
rows.

Usage:
  python3 run_ws_fuse.py --arm A --model qwen3-coder --smoke
  python3 run_ws_fuse.py --arm B --model qwen3-coder --smoke   # needs
      CODEINDEX_WS_MCP_BIN=<workspace-capable codeindex binary>
Flags: --tasks N  --reps 1  --timeout 900  --only id,id
       --include-excluded (run kinds excluded at freeze, e.g. xalias)
       --runs-out results/runs.jsonl
       --dry-run (parse tasks, generate the HOME config, print the fuse
                  command for the first selected task; run nothing)
       --reparse (re-derive every row in --runs-out from the traces already
                  in results/fuse_traces_ws/ — no fuse invocations; keyed
                  idempotently, replacing rows in place and preserving each
                  prior row's started/duration/timed_out)
"""

from __future__ import annotations

import argparse
import json
import os
import signal
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
BENCH = REPO_ROOT / "bench"
FUSE = Path.home() / "dev" / "fuse" / "fuse"
USER_FUSE_CFG = Path.home() / ".fuse" / "config.yml"
RESULTS = HERE / "results"
TRACES = RESULTS / "fuse_traces_ws"
HOMES = HERE / ".fusehomes"
DEFAULT_RUNS = "results/runs.jsonl"

TASKS_FILE = HERE / "tasks" / "tasks_ws.json"
WS_ROOT = (REPO_ROOT
           / json.loads((HERE / "corpus.json").read_text())["workspace_root"]
           ).resolve()

os.environ.setdefault("AB_WORK", str(BENCH / "repos"))
sys.path.insert(0, str(BENCH / "agent_ab"))
sys.path.insert(0, str(BENCH / "m5"))
from run_ab import arm_a_shim_dir  # noqa: E402
from run_m5_fuse import (BASE_DISABLED, CODEINDEX_TOOLS, MARKER,  # noqa: E402
                         load_done)

# Treatment note for arm B, prepended to the task prompt exactly as the m5
# fuse family prepends INDEX_NOTE_MCP (bare MCP tools in the tool list are
# not a treatment — measured, see run_m5_fuse.py). Grader-blind: names the
# tool surface, never any task's answer.
INDEX_NOTE_WS = (
    "Note: this multi-repo workspace has a prebuilt cross-repo code index "
    "exposed as MCP tools that operate over the WHOLE workspace (every "
    "member checkout at once): 'callers' lists every function that calls a "
    "symbol (with file:line) in any member, 'callees' lists what a symbol "
    "calls, 'impact' gives a symbol's blast radius (callers + callees) "
    "across all members, 'nav' gives a one-call orientation bundle for a "
    "symbol, and 'find'/'grep'/'search' locate symbols or text "
    "workspace-wide. Symbol anchors accept a member-qualified form like "
    "'member:Symbol'. For who-calls / impact / locate questions that span "
    "repositories these are usually cheaper and more complete than shell "
    "grep over the checkouts.\n\n"
)

# fuse trace tool name -> the claude-CLI-style name grade_ws.py counts.
# Shell surface maps into grade_ws's ("Bash", "Grep", "Glob", "Read");
# MCP tools ("mcp:codeindex/callers") map to the "mcp__" prefix its
# mcp_calls sum looks for. Unknown names pass through unmapped (counted in
# neither bucket — correct: they are neither shell nor MCP).
FUSE_TOOL_MAP = {
    "bash": "Bash",
    "grep": "Grep",
    "read_file": "Read",
    "segment_read": "Read",
    "list_directory": "Glob",
}


def map_tool(name: str) -> str:
    if name.startswith("mcp:"):
        return "mcp__" + name[4:].replace("/", "__")
    return FUSE_TOOL_MAP.get(name, name)


def parse_trace_ws(path: Path) -> dict:
    """m5 parse_trace mechanics, made LABEL-AWARE, + a per-tool call dict
    for grade_ws.py.

    MARKER carries two capture groups — event kind AND conversation label
    ("root" = the main conversation, "auto-classifier" = fuse's
    tool-approval subagent) — so MARKER.split yields (kind, label, body)
    triples. m5's parser ignored the label and survived 256 runs only
    because a CLEAN run's last content-RESP is always root (the classifier
    fires before tool execution, never after the final answer); on a run
    that ends at the gateway or mid-approval, the label-blind "last content
    RESP" is a classifier verdict — measured on the first ws smoke (3/4
    answers were `{"verdict":"allow",...}` junk). So: answer / turns /
    tool_calls come from root events ONLY; tokens sum ALL labels (the
    classifier's calls are real gateway cost, matching the m5 precedent)
    with the classifier share broken out.

    Terminal failure: if the last root event is a failure (gateway error
    RESP / ERROR block, never followed by a successful root RESP) the run
    produced no final answer — emit answer="" so grade_ws scores it 0
    honestly, never a stale earlier message. More generally is_error is
    "no final assistant message": a trace that ends mid-tool-flight (fuse
    loop-detector kill, crash) is just as answerless as a gateway 400 —
    the loop-kill evidence lives only in stderr, not the trace. Callers
    clear is_error when the run TIMED OUT (its own category, per the m5
    precedent: timed_out=true, is_error=false).

    Bodies between markers are mixed (pretty-printed JSON docs and plain
    log text); non-JSON bodies are skipped permissively, as in m5.
    """
    raw = path.read_text() if path.exists() else ""
    parts = MARKER.split(raw)
    # split stride is 3: parts[i]=kind, parts[i+1]=label, parts[i+2]=body
    n_req = n_err = 0
    inp = out = cls_inp = cls_out = 0
    answer = ""
    codeindex_calls = 0
    tool_calls: dict[str, int] = {}
    failed = False  # does the trace END in an unrecovered root failure?
    for i in range(1, len(parts) - 1, 3):
        kind, label, body = parts[i], parts[i + 1], parts[i + 2].strip()
        if kind == "ERROR":
            n_err += 1
            if label == "root":
                failed = True
            continue
        if kind == "REQ":
            if label == "root":
                n_req += 1
            continue
        if kind != "RESP":
            continue
        try:
            d = json.loads(body)
        except json.JSONDecodeError:
            continue
        u = d.get("usage") or {}
        if label != "root":
            cls_inp += u.get("prompt_tokens", 0)
            cls_out += u.get("completion_tokens", 0)
            continue
        inp += u.get("prompt_tokens", 0)
        out += u.get("completion_tokens", 0)
        if "error" in d and not d.get("choices"):
            failed = True
            continue
        msg = (d.get("choices") or [{}])[0].get("message", {})
        tcs = msg.get("tool_calls") or []
        for tc in tcs:
            fn = tc.get("function", {})
            name = fn.get("name", "")
            mapped = map_tool(name)
            tool_calls[mapped] = tool_calls.get(mapped, 0) + 1
            if "codeindex" in name:
                codeindex_calls += 1
            elif (name == "bash"
                  and "codeindex" in fn.get("arguments", "").lower()):
                codeindex_calls += 1
        if tcs:
            failed = False  # run continued past any earlier error
        elif msg.get("content"):
            answer = msg["content"]
            failed = False
    if failed:
        answer = ""
    shell_calls = sum(v for k, v in tool_calls.items()
                     if k in ("Bash", "Grep", "Glob", "Read"))
    mcp_calls = sum(v for k, v in tool_calls.items()
                   if k.startswith("mcp__"))
    return {
        "num_turns": n_req, "n_gateway_errors": n_err,
        "input_tokens": inp, "output_tokens": out,
        "classifier_tokens": cls_inp + cls_out,
        "processed_tokens": inp + out + cls_inp + cls_out,
        "cost_usd": None,
        "answer": answer, "tool_calls": tool_calls,
        "shell_calls": shell_calls, "mcp_calls": mcp_calls,
        "codeindex_calls": codeindex_calls, "has_result": bool(answer),
        # no final assistant message == abnormal termination (gateway
        # error, fuse loop-kill, crash); `failed` is already folded in
        # (it forces answer=""). Callers flip this off for timeouts.
        "is_error": not answer,
        "gateway_terminal_error": failed,
    }


def make_home(arm: str, mcp_bin: str | None) -> Path:
    """Generated HOME whose trusted config carries the arm's surface."""
    home = HOMES / f"ws-{arm}"
    fdir = home / ".fuse"
    fdir.mkdir(parents=True, exist_ok=True)
    cfg = yaml.safe_load(USER_FUSE_CFG.read_text())
    # builtin codeindex tools off in BOTH arms: the only index surface under
    # test is the workspace MCP server (surface purity, per run_m5_floor.py)
    cfg["permissions"] = {"mode": "auto",
                          "disabled": BASE_DISABLED + CODEINDEX_TOOLS}
    if arm == "B":
        cfg["mcp_servers"] = [{
            "name": "codeindex", "transport": "stdio",
            "command": [str(mcp_bin), "mcp", str(WS_ROOT)],
        }]
    else:
        cfg.pop("mcp_servers", None)
    (fdir / "config.yml").write_text(yaml.safe_dump(cfg))
    return home


def build_env(arm: str, mcp_bin: str | None) -> dict:
    env = dict(os.environ)
    env["HOME"] = str(make_home(arm, mcp_bin))
    env.pop("CODEINDEX_BIN", None)
    if arm == "A":
        # belt-and-suspenders per the bench-hook-leak rule: failing shim
        # first on PATH + CODEINDEX_DISABLED so the real binary refuses via
        # any route (settings flag alone leaves /opt/homebrew reachable)
        env["CODEINDEX_DISABLED"] = "1"
        env["PATH"] = f"{arm_a_shim_dir()}{os.pathsep}{env.get('PATH', '')}"
    else:
        # treatment: the MCP subprocess must run; binary NOT added to PATH
        env.pop("CODEINDEX_DISABLED", None)
    return env


def fuse_cmd(task: dict, arm: str, trace: Path, model: str) -> list[str]:
    prompt = task["prompt"].replace("{WS_ROOT}", str(WS_ROOT))
    if arm == "B":
        prompt = INDEX_NOTE_WS + prompt
    return [str(FUSE), "-model", model, "-approve-all", "-trace", str(trace),
            prompt]


def run_one(task: dict, arm: str, model: str, rep: int, mcp_bin: str | None,
            timeout: int) -> dict:
    key = f"{task['id']}_{arm}_{model}_r{rep}"
    TRACES.mkdir(parents=True, exist_ok=True)
    trace = TRACES / f"{key}.trace"
    env = build_env(arm, mcp_bin)
    cmd = fuse_cmd(task, arm, trace, model)

    started = datetime.now(timezone.utc).isoformat()
    t0 = time.time()
    timed_out = False
    stderr_tail = ""
    # own process group so a timeout kills fuse AND its MCP subprocess
    proc = subprocess.Popen(cmd, cwd=str(WS_ROOT), env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, start_new_session=True)
    try:
        _, stderr = proc.communicate(timeout=timeout)
        stderr_tail = (stderr or "")[-500:]
    except subprocess.TimeoutExpired:
        timed_out = True
        stderr_tail = "TIMEOUT"
        try:
            os.killpg(os.getpgid(proc.pid), signal.SIGKILL)
        except (ProcessLookupError, PermissionError):
            pass
        proc.communicate()
    dur = time.time() - t0

    metrics = parse_trace_ws(trace)
    if timed_out:
        metrics["has_result"] = False
        metrics["is_error"] = False  # timeout is its own category
    return {
        "key": key, "task_id": task["id"], "kind": task["kind"],
        "rung": task["rung"], "arm": arm, "model": model, "rep": rep,
        "harness": "fuse-ws", "started": started,
        "duration_s": round(dur, 1), "timed_out": timed_out,
        "stderr_tail": stderr_tail,
        "trace": str(trace.relative_to(REPO_ROOT)),
        **metrics,
    }


def warn_if_index_on_disk():
    """Loud warning only — the driver owns the B4 stash/verify."""
    try:
        out = subprocess.run(
            ["find", "-L", "bench/repos", "-name", ".codeindex",
             "-type", "d"],
            cwd=str(REPO_ROOT), capture_output=True, text=True, timeout=120,
        ).stdout.strip()
    except (subprocess.TimeoutExpired, OSError):
        return
    if out:
        print("!" * 72)
        print("WARNING: .codeindex directories visible under bench/repos "
              "during a CONTROL arm — the driver's B4 stash has not run:")
        for line in out.splitlines():
            print(f"  {line}")
        print("!" * 72)


def reparse(runs_path: Path) -> None:
    """Re-derive rows from every existing trace — no fuse invocations.

    Keyed idempotently: a row whose key matches a trace is REPLACED in
    place (order preserved), traces without a row are appended, rows
    without a trace (e.g. claude-CLI rows in a shared file) pass through
    untouched. Extraction-only fields are rebuilt; run-lifecycle fields
    (started / duration_s / timed_out / stderr_tail) keep the prior row's
    values — a reparse cannot re-observe them.
    """
    by_id = {t["id"]: t for t in json.loads(TASKS_FILE.read_text())["tasks"]}
    prior: dict[str, dict] = {}
    order: list[str] = []
    if runs_path.exists():
        for ln in runs_path.read_text().splitlines():
            if not ln:
                continue
            row = json.loads(ln)
            if row["key"] not in prior:
                order.append(row["key"])
            prior[row["key"]] = row
    n_new = n_upd = 0
    for trace in sorted(TRACES.glob("*.trace")):
        stem = trace.stem
        try:
            task_id, arm, model, r = stem.rsplit("_", 3)
            rep = int(r.lstrip("r"))
        except ValueError:
            print(f"  skip (unparseable key): {stem}")
            continue
        t = by_id.get(task_id)
        if t is None:
            print(f"  skip (unknown task id): {stem}")
            continue
        old = prior.get(stem, {})
        metrics = parse_trace_ws(trace)
        if old.get("timed_out"):
            metrics["has_result"] = False
            metrics["is_error"] = False  # timeout is its own category
        row = {
            "key": stem, "task_id": task_id, "kind": t["kind"],
            "rung": t["rung"], "arm": arm, "model": model, "rep": rep,
            "harness": "fuse-ws", "started": old.get("started"),
            "duration_s": old.get("duration_s"),
            "timed_out": old.get("timed_out", False),
            "stderr_tail": old.get("stderr_tail", ""),
            "trace": str(trace.relative_to(REPO_ROOT)),
            **metrics,
        }
        if stem in prior:
            n_upd += 1
        else:
            n_new += 1
            order.append(stem)
        prior[stem] = row
    runs_path.parent.mkdir(parents=True, exist_ok=True)
    with runs_path.open("w") as f:
        for k in order:
            f.write(json.dumps(prior[k]) + "\n")
    print(f"reparse: {n_upd} rebuilt, {n_new} new, {len(order)} total "
          f"-> {runs_path}")
    for k in order:
        row = prior[k]
        if row.get("harness") != "fuse-ws":
            continue
        a = (row.get("answer") or "").replace("\n", " | ")
        print(f"  {k}: tok={row['processed_tokens']} "
              f"(cls={row.get('classifier_tokens')}) "
              f"shell={row['shell_calls']} mcp={row['mcp_calls']} "
              f"turns={row['num_turns']} is_error={row.get('is_error')} "
              f"timed_out={row.get('timed_out')}")
        print(f"    answer[:100]: {a[:100]!r}")


def select_tasks(args) -> tuple[list[dict], set[str]]:
    data = json.loads(TASKS_FILE.read_text())
    tasks = data["tasks"]
    # excluded-at-freeze kinds come from the task-file header (xalias today);
    # a hardcoded set rots the moment the freeze changes
    excluded = set((data["header"].get("subsets") or {})
                   .get("excluded_from_scored") or {"xalias": None})
    if not args.include_excluded:
        tasks = [t for t in tasks if t["kind"] not in excluded]
    if args.only:
        want = set(args.only.split(","))
        tasks = [t for t in tasks if t["id"] in want]
    if args.smoke:
        picked = []
        for kind in ("xsubtypes", "xchain"):
            for t in tasks:
                if t["kind"] == kind:
                    picked.append(t)
                    break
        tasks = picked
    elif args.tasks:
        tasks = tasks[: args.tasks]
    return tasks, excluded


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--arm", choices=["A", "B"],
                    help="required unless --reparse")
    ap.add_argument("--model",
                    help="fuse model alias from ~/.fuse/config.yml; "
                         "required unless --reparse")
    ap.add_argument("--smoke", action="store_true",
                    help="first xsubtypes task + first xchain task, 1 rep")
    ap.add_argument("--tasks", type=int, default=0)
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--timeout", type=int, default=900)
    ap.add_argument("--only", help="comma-separated task ids")
    ap.add_argument("--include-excluded", action="store_true",
                    help="also run kinds excluded at freeze (xalias)")
    ap.add_argument("--runs-out", default=DEFAULT_RUNS,
                    help="runs jsonl, relative to bench/workspace")
    ap.add_argument("--dry-run", action="store_true",
                    help="generate HOME config + print the fuse command for "
                         "the first selected task; run nothing")
    ap.add_argument("--reparse", action="store_true",
                    help="rebuild rows in --runs-out from existing traces; "
                         "no fuse invocations")
    args = ap.parse_args()
    if args.smoke:
        args.reps = 1

    if args.reparse:
        reparse(HERE / args.runs_out)
        return
    if not args.arm or not args.model:
        ap.error("--arm and --model are required unless --reparse")

    mcp_bin = None
    if args.arm == "B":
        mcp_bin = os.environ.get("CODEINDEX_WS_MCP_BIN")
        if not mcp_bin:
            sys.exit("arm B is gated on the workspace engine (openspec "
                     "workspace-graph §3–4); set CODEINDEX_WS_MCP_BIN when "
                     "it exists")
        if not args.dry_run and not os.access(mcp_bin, os.X_OK):
            sys.exit(f"CODEINDEX_WS_MCP_BIN not executable: {mcp_bin}")
    if not FUSE.exists() and not args.dry_run:
        sys.exit(f"fuse binary missing: {FUSE} (make -C ~/dev/fuse build)")
    if not WS_ROOT.exists() and not args.dry_run:
        sys.exit(f"workspace root missing: {WS_ROOT}")

    tasks, excluded = select_tasks(args)
    if not tasks:
        sys.exit("no tasks selected")
    if args.smoke:
        print(f"SMOKE MODE: {len(tasks)} tasks "
              f"({', '.join(t['id'] for t in tasks)}) x 1 rep")
    if args.arm == "A":
        warn_if_index_on_disk()

    if args.dry_run:
        t = tasks[0]
        home = make_home(args.arm, mcp_bin)
        cmd = fuse_cmd(t, args.arm,
                       TRACES / f"{t['id']}_{args.arm}_{args.model}_r1.trace",
                       args.model)
        cfg = yaml.safe_load((home / ".fuse" / "config.yml").read_text())
        print(f"DRY RUN arm={args.arm} model={args.model}")
        print(f"  selected: {len(tasks)} tasks x {args.reps} rep(s); "
              f"excluded kinds: {sorted(excluded)}"
              f"{' (INCLUDED via flag)' if args.include_excluded else ''}")
        print(f"  cwd: {WS_ROOT}")
        print(f"  HOME: {home}")
        print(f"  config disabled tools: {cfg['permissions']['disabled']}")
        print(f"  config mcp_servers: {cfg.get('mcp_servers', '(none)')}")
        if args.arm == "A":
            print(f"  PATH shim: {arm_a_shim_dir()} (first on PATH), "
                  f"CODEINDEX_DISABLED=1")
        print(f"  first task: {t['id']} kind={t['kind']} rung={t['rung']}")
        print("  cmd: " + " ".join(
            c if len(c) < 120 else c[:117] + "..." for c in cmd))
        print(f"  prompt head: {cmd[-1][:200]!r}...")
        print(f"  runs append to: {HERE / args.runs_out}")
        return

    runs_path = HERE / args.runs_out
    runs_path.parent.mkdir(parents=True, exist_ok=True)
    done = load_done(runs_path)
    ran = 0
    with runs_path.open("a") as out:
        for rep in range(1, args.reps + 1):
            for t in tasks:
                key = f"{t['id']}_{args.arm}_{args.model}_r{rep}"
                if key in done:
                    continue
                print(f"  [{ran+1}] {key} ...", flush=True)
                row = run_one(t, args.arm, args.model, rep, mcp_bin,
                              args.timeout)
                out.write(json.dumps(row) + "\n")
                out.flush()
                print(f"       turns={row['num_turns']} "
                      f"tok={row['processed_tokens']} "
                      f"shell={row['shell_calls']} mcp={row['mcp_calls']} "
                      f"answer_len={len(row.get('answer') or '')} "
                      f"errors={row['n_gateway_errors']} "
                      f"{'TIMEOUT' if row['timed_out'] else ''}")
                ran += 1
    print(f"\ndone: {ran} new runs -> {runs_path}")
    print(f"grade with: python3 grade_ws.py --runs {args.runs_out}")


if __name__ == "__main__":
    main()

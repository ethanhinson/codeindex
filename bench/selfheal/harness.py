#!/usr/bin/env python3
"""Self-healing validation harness for the codeindex runtime-evidence pipeline.

Runs deterministic scenarios (bench/selfheal/scenarios/) that each generate a
temp app, profile it into a cxprof spool, run the pipeline (build -> ingest ->
search), and assert the observed-evidence signals. On assertion failure the
harness walks a remediation ladder; a remediation that fixes a scenario is
recorded in learned.json and applied proactively on subsequent runs.

Ladder:
  r1  extend sampling window 3x and re-profile        (too-few-samples)
  r2  re-run pipeline with the symlink-resolved path  (path aliasing)
  r3  rebuild index from scratch, spool preserved     (stale ledger/schema)
  r4  quarantine the spool; scenario failed-quarantined

Every run appends to runs.jsonl. Exit is non-zero iff any non-optional
scenario ends failed-quarantined. Stdlib only.

Usage: python3 bench/selfheal/harness.py [--only name[,name...]]
"""

import argparse
import datetime
import json
import os
import shutil
import subprocess
import sys

SELF_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(SELF_DIR))
BIN = os.environ.get("CODEINDEX_BIN", "/tmp/codeindex-selfheal")
LEARNED_PATH = os.path.join(SELF_DIR, "learned.json")
RUNS_PATH = os.path.join(SELF_DIR, "runs.jsonl")
QUARANTINE_DIR = os.path.join(SELF_DIR, "quarantine")

sys.path.insert(0, SELF_DIR)
from scenarios import SkipScenario, StepError  # noqa: E402
from scenarios.node_registry import NodeRegistryScenario  # noqa: E402
from scenarios.go_sdk import GoSdkScenario  # noqa: E402
from scenarios.node_symlink import NodeSymlinkScenario  # noqa: E402
from scenarios.php_excimer import PhpExcimerScenario  # noqa: E402

# Deterministic execution order.
SCENARIOS = [NodeRegistryScenario, GoSdkScenario, NodeSymlinkScenario, PhpExcimerScenario]
LADDER = ["r1", "r2", "r3"]


def log(msg):
    print(msg, flush=True)


def ensure_bin():
    if os.path.isfile(BIN) and os.access(BIN, os.X_OK):
        return
    log("building codeindex binary at %s" % BIN)
    subprocess.run(
        ["go", "build", "-o", BIN, "./cmd/codeindex"],
        cwd=REPO_ROOT, check=True,
    )


def load_learned():
    try:
        with open(LEARNED_PATH) as f:
            data = json.load(f)
        return data if isinstance(data, dict) else {}
    except (OSError, ValueError):
        return {}


def save_learned(learned):
    tmp = LEARNED_PATH + ".tmp"
    with open(tmp, "w") as f:
        json.dump(learned, f, indent=2, sort_keys=True)
        f.write("\n")
    os.replace(tmp, LEARNED_PATH)


def append_run(entry):
    with open(RUNS_PATH, "a") as f:
        f.write(json.dumps(entry, sort_keys=True) + "\n")


def all_pass(assertions):
    return bool(assertions) and all(assertions.values())


def apply_rung(sc, rung, flags):
    """Re-run only the step(s) a rung targets, then let the caller re-verify."""
    if rung == "r1":
        sc.profile(window_mult=3)
        sc.pipeline(resolved=flags["resolved"], rebuild=None)
    elif rung == "r2":
        # The ledger is idempotent per (path, hash); reset it so re-ingest
        # under the resolved root actually reprocesses the spool.
        sc.pipeline(resolved=True, rebuild="ledger")
    elif rung == "r3":
        sc.pipeline(resolved=flags["resolved"], rebuild="full")
    else:
        raise ValueError("unknown rung " + rung)


def quarantine(sc):
    files = sc.spool_files()
    if not files:
        return []
    ts = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    dest = os.path.join(QUARANTINE_DIR, "%s-%s" % (sc.name, ts))
    os.makedirs(dest, exist_ok=True)
    moved = []
    for p in files:
        target = os.path.join(dest, os.path.basename(p))
        shutil.move(p, target)
        moved.append(target)
    return moved


def run_scenario(cls, learned):
    ctx = {"repo_root": REPO_ROOT, "bin": BIN, "selfheal_dir": SELF_DIR, "log": log}
    sc = cls(ctx)
    pre = [r for r in learned.get(sc.name, []) if r in sc.ladder]
    flags = {
        "window_mult": 3 if "r1" in pre else 1,
        "resolved": "r2" in pre,
        "rebuild": "full" if "r3" in pre else None,
    }

    entry = {
        "ts": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
        "scenario": sc.name,
        "attempts": 0,
        "remediation_used": None,
        "assertions": {},
        "resolution_pct": None,
        "edges": None,
    }

    log("\n=== scenario: %s%s ===" % (sc.name, " (optional)" if sc.optional else ""))
    if pre:
        log("  learned remediations applied proactively: %s" % "+".join(pre))

    # -- prepare (skip gate) ----------------------------------------------
    try:
        sc.prepare()
    except SkipScenario as e:
        log("  SKIPPED: %s" % e)
        entry["status"] = "skipped"
        entry["skip_reason"] = str(e)
        append_run(entry)
        return "skipped", entry

    # -- first attempt (with learned flags) --------------------------------
    def attempt(fn):
        entry["attempts"] += 1
        try:
            fn()
            return sc.verify()
        except StepError as e:
            log("  step error: %s" % str(e).splitlines()[0])
            return {"step_completed": False, "step_error": str(e)[:500]}

    def first():
        sc.profile(window_mult=flags["window_mult"])
        sc.pipeline(resolved=flags["resolved"], rebuild=flags["rebuild"])

    log("  attempt 1 (window_mult=%d resolved=%s rebuild=%s)"
        % (flags["window_mult"], flags["resolved"], flags["rebuild"]))
    assertions = attempt(first)
    log("  assertions: %s" % json.dumps(
        {k: v for k, v in assertions.items() if isinstance(v, bool)}, sort_keys=True))

    remediation_used = None
    status = "passed" if all_pass(
        {k: v for k, v in assertions.items() if isinstance(v, bool)}) else None
    if status == "passed" and pre:
        remediation_used = "learned(%s)" % "+".join(pre)

    # -- remediation ladder -------------------------------------------------
    if status is None:
        for rung in LADDER:
            if rung not in sc.ladder:
                continue
            log("  remediation %s" % rung)
            assertions = attempt(lambda r=rung: apply_rung(sc, r, flags))
            boolmap = {k: v for k, v in assertions.items() if isinstance(v, bool)}
            log("  assertions: %s" % json.dumps(boolmap, sort_keys=True))
            if all_pass(boolmap):
                status = "passed"
                remediation_used = rung
                hist = learned.setdefault(sc.name, [])
                if rung not in hist:
                    hist.append(rung)
                    save_learned(learned)
                    log("  learned: %s -> %s" % (sc.name, hist))
                break
        if status is None:
            moved = quarantine(sc)
            status = "failed-quarantined"
            remediation_used = "r4"
            entry["quarantined"] = moved
            log("  r4: quarantined %d spool file(s)" % len(moved))

    entry["status"] = status
    entry["remediation_used"] = remediation_used
    entry["assertions"] = {k: v for k, v in assertions.items() if isinstance(v, bool)}
    if "step_error" in assertions:
        entry["step_error"] = assertions["step_error"]
    entry["resolution_pct"] = sc.resolution_pct
    entry["edges"] = sc.edges
    append_run(entry)

    log("  outcome: %s (attempts=%d, remediation=%s, resolution=%s%%, edges=%s)"
        % (status, entry["attempts"], remediation_used, sc.resolution_pct, sc.edges))
    return status, entry


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--only", help="comma-separated scenario names to run")
    args = ap.parse_args()

    ensure_bin()
    learned = load_learned()
    if not os.path.exists(LEARNED_PATH):
        save_learned(learned)  # bootstrap the memory file

    selected = SCENARIOS
    if args.only:
        wanted = set(args.only.split(","))
        selected = [c for c in SCENARIOS if c.name in wanted]
        unknown = wanted - {c.name for c in selected}
        if unknown:
            log("unknown scenario(s): %s" % ", ".join(sorted(unknown)))
            return 2

    results = []
    for cls in selected:
        status, entry = run_scenario(cls, learned)
        results.append((cls.name, cls.optional, status, entry))

    log("\n=== summary ===")
    hard_fail = False
    for name, optional, status, entry in results:
        log("  %-14s %s (attempts=%d, remediation=%s)"
            % (name, status, entry["attempts"], entry["remediation_used"]))
        if status == "failed-quarantined" and not optional:
            hard_fail = True
    return 1 if hard_fail else 0


if __name__ == "__main__":
    sys.exit(main())

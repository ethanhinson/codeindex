#!/usr/bin/env python3
"""Control-arm leak audit over agent transcripts — id-paired, batch-safe.

Replaces the ad-hoc post-hoc audit from the v10 campaign, which paired each
tool_result with the *positionally preceding* tool_use and therefore
misattributed results whenever the agent batched multiple tool calls in one
assistant message (FINDINGS_v10 "Open threads": attributes a grep result to a
preceding codeindex call). Here every tool_result joins its tool_use via
`tool_use_id` — the only correct join key in stream-json — so batching cannot
skew the counts.

A codeindex ATTEMPT is a tool_use (Bash/SlashCommand/Skill) whose input
mentions `codeindex`. Its verdict comes from the paired result:

  BLOCKED  — is_error, exit 127 / "command not found" / CODEINDEX_DISABLED
             marker, or empty output: the guard held.
  REAL     — substantive output came back: the control arm reached the index.
  UNPAIRED — no result in the transcript (truncated run); counted separately,
             never guessed.

`codeindex_calls` in runs.jsonl counts attempts, not successes (FINDINGS_v10);
REAL is the number that taints a control row. Exit non-zero with --fail-on-real
to gate a harness run.

Usage:
  python3 leak_audit.py results/transcripts/*_A_*.jsonl
  python3 leak_audit.py --json results/transcripts/          # dir = *.jsonl
  python3 leak_audit.py --selftest                           # pairing proof
"""
from __future__ import annotations
import argparse, json, re, sys
from pathlib import Path

BLOCK_PAT = re.compile(
    r"exit code:? 127|command not found|CODEINDEX_DISABLED|"
    r"codeindex: disabled|[Nn]o such file or directory", re.A)
TOOLS = {"Bash", "SlashCommand", "Skill"}


def result_text(block) -> str:
    c = block.get("content")
    if isinstance(c, str):
        return c
    if isinstance(c, list):
        return "\n".join(x.get("text", "") for x in c if isinstance(x, dict))
    return ""


def audit_events(events) -> dict:
    """Pair tool_use ↔ tool_result by id; classify codeindex attempts."""
    uses, results = {}, {}
    for ev in events:
        t = ev.get("type")
        if t == "assistant":
            for b in ev.get("message", {}).get("content", []) or []:
                if isinstance(b, dict) and b.get("type") == "tool_use":
                    uses[b.get("id")] = b
        elif t == "user":
            c = ev.get("message", {}).get("content")
            if isinstance(c, list):
                for b in c:
                    if isinstance(b, dict) and b.get("type") == "tool_result":
                        results[b.get("tool_use_id")] = b
    attempts, real, blocked, unpaired = [], [], [], []
    for uid, u in uses.items():
        if u.get("name") not in TOOLS:
            continue
        if "codeindex" not in json.dumps(u.get("input", {})).lower():
            continue
        att = {"id": uid, "tool": u.get("name"),
               "input": json.dumps(u.get("input", {}))[:160]}
        attempts.append(att)
        r = results.get(uid)
        if r is None:
            att["verdict"] = "UNPAIRED"
            unpaired.append(att)
            continue
        txt = result_text(r)
        if r.get("is_error") or BLOCK_PAT.search(txt) or not txt.strip():
            att["verdict"] = "BLOCKED"
            blocked.append(att)
        else:
            att["verdict"] = "REAL"
            att["evidence"] = txt[:200]
            real.append(att)
    return {"attempts": len(attempts), "real": real, "blocked": len(blocked),
            "unpaired": len(unpaired), "orphan_results":
            len([i for i in results if i not in uses])}


def audit_file(path: Path) -> dict:
    events = []
    for ln in path.read_text().splitlines():
        ln = ln.strip()
        if not ln:
            continue
        try:
            events.append(json.loads(ln))
        except json.JSONDecodeError:
            continue
    rep = audit_events(events)
    rep["file"] = path.name
    return rep


# --- selftest: the exact batched shape that broke positional pairing ------- #

def selftest() -> int:
    """One assistant message batches [codeindex-bash, grep]; the codeindex
    result is a 127 block, the grep result is rich. Positional pairing
    attributes grep's output to the codeindex call → false REAL leak.
    Id-pairing must report 1 attempt, 1 BLOCKED, 0 REAL."""
    events = [
        {"type": "assistant", "message": {"content": [
            {"type": "tool_use", "id": "t1", "name": "Bash",
             "input": {"command": "codeindex callers . Foo"}},
            {"type": "tool_use", "id": "t2", "name": "Grep",
             "input": {"pattern": "Foo"}},
        ]}},
        # results arrive in REVERSE order, in separate user messages
        {"type": "user", "message": {"content": [
            {"type": "tool_result", "tool_use_id": "t2",
             "content": [{"type": "text", "text": "Found 12 files\na.go\nb.go"}]},
        ]}},
        {"type": "user", "message": {"content": [
            {"type": "tool_result", "tool_use_id": "t1", "is_error": True,
             "content": [{"type": "text",
                          "text": "zsh: command not found: codeindex\nExit code: 127"}]},
        ]}},
    ]
    rep = audit_events(events)
    ok = (rep["attempts"] == 1 and rep["blocked"] == 1 and not rep["real"]
          and rep["unpaired"] == 0)
    print(f"selftest: attempts={rep['attempts']} blocked={rep['blocked']} "
          f"real={len(rep['real'])} unpaired={rep['unpaired']} -> "
          f"{'PASS' if ok else 'FAIL'}")
    return 0 if ok else 1


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("paths", nargs="*", help="transcript .jsonl files or dirs")
    ap.add_argument("--json", action="store_true", dest="as_json")
    ap.add_argument("--fail-on-real", action="store_true",
                    help="exit 1 if any REAL codeindex output is found")
    ap.add_argument("--selftest", action="store_true")
    args = ap.parse_args()
    if args.selftest:
        return selftest()
    files: list[Path] = []
    for p in map(Path, args.paths):
        files.extend(sorted(p.glob("*.jsonl")) if p.is_dir() else [p])
    if not files:
        ap.error("no transcript files given (or use --selftest)")
    reports = [audit_file(f) for f in files]
    total_real = sum(len(r["real"]) for r in reports)
    if args.as_json:
        print(json.dumps({"files": reports, "total_real": total_real}, indent=2))
    else:
        for r in reports:
            if r["attempts"] or r["unpaired"]:
                print(f"{r['file']}: attempts={r['attempts']} "
                      f"blocked={r['blocked']} real={len(r['real'])} "
                      f"unpaired={r['unpaired']}")
            for a in r["real"]:
                print(f"  REAL {a['tool']} {a['input']}")
                print(f"       -> {a['evidence'][:120]!r}")
        print(f"\n{len(files)} transcript(s): {total_real} REAL leak(s)")
    return 1 if (args.fail_on_real and total_real) else 0


if __name__ == "__main__":
    sys.exit(main())

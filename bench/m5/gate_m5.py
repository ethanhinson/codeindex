#!/usr/bin/env python3
"""M5 gate evaluator — paired stats per suite + the pre-registered GO/KILL
verdicts. Thresholds are read from the tasks header (stamped at generation
time by build_tasks_m5.py), never from this file.

GO gate  (B vs A):  dominate success within 5pp, savings rule cleared, recall
                    not lost; grepwin cost regression <=10%; break
                    false-confidence delta <=10pp.
KILL gate (C vs B): on dominate, C success within 5pp of B at <=1.10x B's
                    median cost -> the index fails to justify itself.

Each sub-verdict is WITHHELD (not passed) when its pair count is below
min_pairs_per_verdict — small smoke runs print numbers, not conclusions.

Usage: python3 gate_m5.py [--selftest]   (writes results/report_m5.md)
"""

from __future__ import annotations

import json
import statistics
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
TASKS = HERE / "tasks" / "tasks_m5.json"
GRADED = HERE / "results" / "graded_m5.jsonl"
REPORT = HERE / "results" / "report_m5.md"


def load_rows(path: Path) -> list[dict]:
    return [json.loads(ln) for ln in path.read_text().splitlines() if ln.strip()]


def index_by_pair(rows: list[dict]) -> dict:
    """(task_id, rep) -> {arm: best-effort single row}"""
    out: dict = {}
    for r in rows:
        out.setdefault((r["task_id"], r["rep"]), {})[r["arm"]] = r
    return out


def paired_pct_savings(pairs: dict, suite: str, base: str, treat: str,
                       field: str) -> list[float]:
    """Per-pair (base - treat)/base * 100 on `field`, suite-filtered."""
    vals = []
    for _, arms in pairs.items():
        a, b = arms.get(base), arms.get(treat)
        if not a or not b or a["suite"] != suite:
            continue
        av, bv = a.get(field), b.get(field)
        if av and bv is not None and av > 0:
            vals.append((av - bv) / av * 100)
    return vals


def suite_rows(rows: list[dict], suite: str, arm: str) -> list[dict]:
    return [r for r in rows if r["suite"] == suite and r["arm"] == arm]


def success_rate(rs: list[dict]) -> float | None:
    return sum(r["success"] for r in rs) / len(rs) if rs else None


def mean_recall(rs: list[dict]) -> float | None:
    vals = [r["grade_detail"].get("recall") for r in rs
            if r["grade_detail"].get("recall") is not None]
    return sum(vals) / len(vals) if vals else None


def fc_rate(rs: list[dict]) -> float | None:
    vals = [r for r in rs if "false_confidence" in r]
    return (sum(r["false_confidence"] for r in vals) / len(vals)
            if vals else None)


def fmt(v, pct=False, digits=1):
    if v is None:
        return "n/a"
    return f"{v*100:.{digits}f}%" if pct else f"{v:.{digits}f}"


def verdict_line(name: str, passed: bool | None, n: int, min_n: int,
                 detail: str) -> str:
    if n < min_n or passed is None:
        return f"- **{name}: WITHHELD** (n={n} < {min_n}) — {detail}"
    return f"- **{name}: {'PASS' if passed else 'FAIL'}** (n={n}) — {detail}"


def evaluate(rows: list[dict], gates: dict) -> str:
    pairs = index_by_pair(rows)
    go, kill = gates["go_gate"], gates["kill_gate"]
    min_n = gates["min_pairs_per_verdict"]
    lines = ["# M5 report — go/no-go gates", ""]

    # ---- per-suite / per-arm table -------------------------------------- #
    lines += ["## Per-suite metrics", "",
              "| suite | arm | n | success | mean recall | false-conf | "
              "median cost | median turns |",
              "|---|---|---:|---:|---:|---:|---:|---:|"]
    for suite in ("grepwin", "dominate", "break"):
        for arm in ("A", "B", "C"):
            rs = suite_rows(rows, suite, arm)
            if not rs:
                continue
            costs = [r["cost_usd"] for r in rs if r.get("cost_usd")]
            turns = [r["num_turns"] for r in rs if r.get("num_turns")]
            med_cost = f"${statistics.median(costs):.4f}" if costs else "n/a"
            med_turns = f"{statistics.median(turns):.0f}" if turns else "n/a"
            lines.append(
                f"| {suite} | {arm} | {len(rs)} | "
                f"{fmt(success_rate(rs), pct=True)} | "
                f"{fmt(mean_recall(rs), pct=True)} | "
                f"{fmt(fc_rate(rs), pct=True)} | {med_cost} | {med_turns} |")
    lines.append("")

    # ---- GO gate: B vs A ------------------------------------------------ #
    lines += ["## GO gate (B vs A)", ""]
    dom_a, dom_b = suite_rows(rows, "dominate", "A"), suite_rows(rows, "dominate", "B")
    n_dom = min(len(dom_a), len(dom_b))
    sa, sb = success_rate(dom_a), success_rate(dom_b)
    d_succ = (sb - sa) * 100 if sa is not None and sb is not None else None
    succ_ok = d_succ is not None and d_succ >= go["dominate_success_delta_min_pp"]
    lines.append(verdict_line(
        "dominate success", succ_ok if d_succ is not None else None, n_dom, min_n,
        f"B {fmt(sb, pct=True)} vs A {fmt(sa, pct=True)} "
        f"(delta {fmt(d_succ)}pp, min {go['dominate_success_delta_min_pp']}pp)"))

    cost_sav = paired_pct_savings(pairs, "dominate", "A", "B", "cost_usd")
    tok_sav = paired_pct_savings(pairs, "dominate", "A", "B", "processed_tokens")
    mc = statistics.median(cost_sav) if cost_sav else None
    mt = statistics.median(tok_sav) if tok_sav else None
    sav_ok = ((mc is not None and mc >= go["dominate_median_cost_savings_min_pct"])
              or (mt is not None
                  and mt >= go["dominate_median_processed_token_savings_min_pct"]))
    lines.append(verdict_line(
        "dominate savings", sav_ok if (mc is not None or mt is not None) else None,
        len(cost_sav), min_n,
        f"median cost savings {fmt(mc)}% / token savings {fmt(mt)}% "
        f"(rule: {go['dominate_savings_rule']})"))

    ra, rb = mean_recall(dom_a), mean_recall(dom_b)
    d_rec = (rb - ra) if ra is not None and rb is not None else None
    rec_ok = d_rec is not None and d_rec >= go["dominate_recall_delta_min"]
    lines.append(verdict_line(
        "dominate recall", rec_ok if d_rec is not None else None, n_dom, min_n,
        f"B {fmt(rb, pct=True)} vs A {fmt(ra, pct=True)} (blast-radius recall "
        f"must not drop)"))

    gw_cost = paired_pct_savings(pairs, "grepwin", "A", "B", "cost_usd")
    mgw = statistics.median(gw_cost) if gw_cost else None
    # savings negative = regression; regression pct = -savings
    gw_ok = mgw is not None and -mgw <= go["grepwin_median_cost_regression_max_pct"]
    lines.append(verdict_line(
        "grepwin non-regression", gw_ok if mgw is not None else None,
        len(gw_cost), min_n,
        f"median paired cost regression {fmt(-mgw if mgw is not None else None)}% "
        f"(max {go['grepwin_median_cost_regression_max_pct']}%)"))

    brk_a, brk_b = suite_rows(rows, "break", "A"), suite_rows(rows, "break", "B")
    fa, fb = fc_rate(brk_a), fc_rate(brk_b)
    d_fc = (fb - fa) * 100 if fa is not None and fb is not None else None
    fc_ok = d_fc is not None and d_fc <= go["break_false_confidence_delta_max_pp"]
    lines.append(verdict_line(
        "break false-confidence", fc_ok if d_fc is not None else None,
        min(len(brk_a), len(brk_b)), min_n,
        f"B {fmt(fb, pct=True)} vs A {fmt(fa, pct=True)} "
        f"(delta {fmt(d_fc)}pp, max +{go['break_false_confidence_delta_max_pp']}pp)"))
    lines.append("")

    # ---- KILL gate: C vs B on dominate ---------------------------------- #
    lines += ["## KILL gate (C vs B, dominate suite)", ""]
    dom_c = suite_rows(rows, "dominate", "C")
    sc = success_rate(dom_c)
    d_cs = (sc - sb) * 100 if sc is not None and sb is not None else None
    costs_b = [r["cost_usd"] for r in dom_b if r.get("cost_usd")]
    costs_c = [r["cost_usd"] for r in dom_c if r.get("cost_usd")]
    ratio = (statistics.median(costs_c) / statistics.median(costs_b)
             if costs_b and costs_c else None)
    killed = (d_cs is not None and ratio is not None
              and d_cs >= kill["c_success_delta_min_pp"]
              and ratio <= kill["c_cost_ratio_max"])
    n_kill = min(len(dom_b), len(dom_c))
    if n_kill < min_n or d_cs is None:
        lines.append(f"- **KILL verdict: WITHHELD** (n={n_kill} < {min_n}) — "
                     f"C success {fmt(sc, pct=True)} vs B {fmt(sb, pct=True)}, "
                     f"cost ratio {fmt(ratio, digits=2)}")
    elif killed:
        lines.append(f"- **KILL gate TRIGGERED** — the cheap explorer matches "
                     f"the index arm: C success {fmt(sc, pct=True)} vs B "
                     f"{fmt(sb, pct=True)} (delta {fmt(d_cs)}pp), median cost "
                     f"ratio {fmt(ratio, digits=2)} <= "
                     f"{kill['c_cost_ratio_max']}. Reconsider the core product.")
    else:
        lines.append(f"- **KILL gate not triggered** — C success "
                     f"{fmt(sc, pct=True)} vs B {fmt(sb, pct=True)} "
                     f"(delta {fmt(d_cs)}pp), median cost ratio "
                     f"{fmt(ratio, digits=2)}.")
    lines += [""]
    return "\n".join(lines)


def evaluate_compound(rows: list[dict], gates: dict) -> str:
    """Fuse-family (model-scale) gate: L/LX/S/SX arms, success + token proxy.
    Never mixes with claude-CLI arms — different harness."""
    cg = gates["compound_gate"]
    min_n = gates["min_pairs_per_verdict"]
    lines = ["## Fuse family (model-scale) — per-suite metrics", "",
             "| suite | arm | n | success | mean recall | false-conf | "
             "median tokens | median turns |",
             "|---|---|---:|---:|---:|---:|---:|---:|"]
    for suite in ("grepwin", "dominate", "break"):
        for arm in ("L", "LX", "S", "SX"):
            rs = suite_rows(rows, suite, arm)
            if not rs:
                continue
            toks = [r["processed_tokens"] for r in rs
                    if r.get("processed_tokens")]
            turns = [r["num_turns"] for r in rs if r.get("num_turns")]
            med_t = f"{statistics.median(toks):.0f}" if toks else "n/a"
            med_u = f"{statistics.median(turns):.0f}" if turns else "n/a"
            lines.append(
                f"| {suite} | {arm} | {len(rs)} | "
                f"{fmt(success_rate(rs), pct=True)} | "
                f"{fmt(mean_recall(rs), pct=True)} | "
                f"{fmt(fc_rate(rs), pct=True)} | {med_t} | {med_u} |")
    lines += ["", "## COMPOUND gate (SX vs L and SX vs S, dominate suite)", ""]
    dom = {a: suite_rows(rows, "dominate", a) for a in ("L", "LX", "S", "SX")}
    sl, ss, ssx = (success_rate(dom["L"]), success_rate(dom["S"]),
                   success_rate(dom["SX"]))
    n_cmp = min(len(dom["L"]), len(dom["S"]), len(dom["SX"]))
    d_scale = (ssx - sl) * 100 if ssx is not None and sl is not None else None
    d_attr = (ssx - ss) * 100 if ssx is not None and ss is not None else None
    scale_ok = (d_scale is not None
                and d_scale >= cg["sx_vs_large_shell_success_delta_min_pp"])
    attr_ok = (d_attr is not None
               and d_attr >= cg["sx_vs_small_shell_success_delta_min_pp"])
    lines.append(verdict_line(
        "compound scale-substitution",
        scale_ok if d_scale is not None else None, n_cmp, min_n,
        f"small+index {fmt(ssx, pct=True)} vs large+shell {fmt(sl, pct=True)} "
        f"(delta {fmt(d_scale)}pp, min "
        f"{cg['sx_vs_large_shell_success_delta_min_pp']}pp)"))
    lines.append(verdict_line(
        "compound index-attribution",
        attr_ok if d_attr is not None else None, n_cmp, min_n,
        f"small+index {fmt(ssx, pct=True)} vs small+shell "
        f"{fmt(ss, pct=True)} (delta {fmt(d_attr)}pp, min "
        f"+{cg['sx_vs_small_shell_success_delta_min_pp']}pp)"))
    slx = success_rate(dom["LX"])
    lines.append(f"- (info) large+index {fmt(slx, pct=True)} vs large+shell "
                 f"{fmt(sl, pct=True)} — within-fuse replication of the "
                 f"claude-family GO direction, no verdict attached.")
    lines.append("")
    return "\n".join(lines)


def evaluate_floor(rows: list[dict]) -> str:
    """Floor sweep table — EXPLORATORY, no pre-registered verdict. Groups by
    (model, treatment) on the dominate suite; the deliverable is the curve."""
    lines = ["## Floor sweep (model ladder × shell/mcp, dominate) — "
             "exploratory, no verdict", "",
             "| model | treatment | n | success | mean recall | false-conf | "
             "adoption | median tokens | timeouts |",
             "|---|---|---:|---:|---:|---:|---:|---:|---:|"]
    groups: dict = {}
    for r in rows:
        groups.setdefault((r["model"], r["treatment"]), []).append(r)
    for (model, tr) in sorted(groups):
        rs = groups[(model, tr)]
        toks = [r["processed_tokens"] for r in rs if r.get("processed_tokens")]
        adopted = sum(1 for r in rs if r.get("codeindex_calls", 0) > 0)
        med_tok = f"{statistics.median(toks):.0f}" if toks else "n/a"
        n_to = sum(1 for r in rs if r.get("timed_out"))
        lines.append(
            f"| {model} | {tr} | {len(rs)} | "
            f"{fmt(success_rate(rs), pct=True)} | "
            f"{fmt(mean_recall(rs), pct=True)} | "
            f"{fmt(fc_rate(rs), pct=True)} | "
            f"{adopted}/{len(rs)} | {med_tok} | {n_to} |")
    lines.append("")
    return "\n".join(lines)


def honesty_notes() -> str:
    lines = ["",
              "## Honesty notes", "",
              "- `dominate_callers` and `dominate_blast` ground truth comes "
              "from graph.db (unambiguous / resolved edges) — NOT arm-neutral. "
              "A codeindex recall bug would understate arm A on those types. "
              "Arm-neutral types (grepwin_*, dominate_tests, break_collision) "
              "carry the cross-check.",
              "- False-confidence uses the COVERAGE-line protocol; a missing "
              "line counts as a completeness claim (pre-registered).",
              "- Claude-CLI arms (A/B/C) and fuse arms (L/LX/S/SX) are "
              "separate harness families — deltas are only meaningful WITHIN "
              "a family. The fuse family reports tokens, not $ (the gateway "
              "prices nothing; local models cost ~$0).",
              ""]
    return "\n".join(lines)


def selftest():
    def row(task_id, suite, arm, success, cost, recall=None, fc=None, rep=0,
            tokens=1000):
        d = {"recall": recall} if recall is not None else {}
        r = {"task_id": task_id, "rep": rep, "suite": suite, "arm": arm,
             "success": success, "cost_usd": cost, "processed_tokens": tokens,
             "num_turns": 3, "grade_detail": d}
        if fc is not None:
            r["false_confidence"] = fc
        return r

    rows = []
    for i in range(12):
        rows.append(row(f"d{i}", "dominate", "A", True, 0.10, recall=0.6, fc=True))
        rows.append(row(f"d{i}", "dominate", "B", True, 0.05, recall=0.9, fc=False))
        rows.append(row(f"d{i}", "dominate", "C", False, 0.02, recall=0.4))
        rows.append(row(f"g{i}", "grepwin", "A", True, 0.02))
        rows.append(row(f"g{i}", "grepwin", "B", True, 0.021))
        rows.append(row(f"b{i}", "break", "A", True, 0.05, recall=0.8, fc=True))
        rows.append(row(f"b{i}", "break", "B", True, 0.05, recall=0.9, fc=True))
    gates = json.loads(TASKS.read_text())["header"]["m5_gates"] \
        if TASKS.exists() else __import__("build_tasks_m5").M5_GATES
    rep = evaluate(rows, gates)
    assert "dominate success: PASS" in rep, rep
    assert "dominate savings: PASS" in rep          # 50% cost savings
    assert "dominate recall: PASS" in rep
    assert "grepwin non-regression: PASS" in rep    # 5% regression <= 10%
    assert "break false-confidence: PASS" in rep    # equal rates, delta 0
    assert "KILL gate not triggered" in rep         # C fails on success
    # now make C match B -> kill fires
    rows = [r for r in rows if not (r["arm"] == "C")]
    for i in range(12):
        rows.append(row(f"d{i}", "dominate", "C", True, 0.05, recall=0.9))
    rep = evaluate(rows, gates)
    assert "KILL gate TRIGGERED" in rep, rep
    # small n -> withheld
    rep = evaluate(rows[:6], gates)
    assert "WITHHELD" in rep
    # compound gate: SX matches L (delta 0 >= -5) and beats S (+50 >= +10)
    frows = []
    for i in range(12):
        frows.append(row(f"d{i}", "dominate", "L", True, None, recall=0.8))
        frows.append(row(f"d{i}", "dominate", "LX", True, None, recall=0.9))
        frows.append(row(f"d{i}", "dominate", "S", i % 2 == 0, None, recall=0.5))
        frows.append(row(f"d{i}", "dominate", "SX", True, None, recall=0.9))
    rep = evaluate_compound(frows, gates)
    assert "compound scale-substitution: PASS" in rep, rep
    assert "compound index-attribution: PASS" in rep, rep
    # small model alone matching SX -> attribution fails
    frows = [r for r in frows if r["arm"] != "S"]
    for i in range(12):
        frows.append(row(f"d{i}", "dominate", "S", True, None, recall=0.9))
    rep = evaluate_compound(frows, gates)
    assert "compound index-attribution: FAIL" in rep, rep
    print("selftest OK: gate arithmetic + verdict semantics + compound gate")


def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--selftest", action="store_true")
    args = ap.parse_args()
    if args.selftest:
        selftest()
        return
    gates = json.loads(TASKS.read_text())["header"]["m5_gates"]
    sections = []
    if GRADED.exists():
        sections.append(evaluate(load_rows(GRADED), gates))
    fuse_graded = HERE / "results" / "graded_m5_fuse.jsonl"
    if fuse_graded.exists():
        sections.append(evaluate_compound(load_rows(fuse_graded), gates))
    floor_graded = HERE / "results" / "graded_m5_floor.jsonl"
    if floor_graded.exists():
        sections.append(evaluate_floor(load_rows(floor_graded)))
    if not sections:
        sys.exit("no graded files found — run grade_m5.py first")
    sections.append(honesty_notes())
    rep = "\n".join(sections)
    REPORT.write_text(rep)
    print(rep)
    print(f"(written to {REPORT})")


if __name__ == "__main__":
    main()

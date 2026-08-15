#!/usr/bin/env python3
"""Tier-1 Scout training-data generator + validator.

Walks an INDEXED repo and manufactures (task, gold-trajectory, gold-answer)
triples whose correct answer is derivable from the call graph — no human labels,
no LLM teacher. This is the overfitting-immune floor of a Navigator/Scout corpus:
every label comes from graph STRUCTURE, not hand-authored accept sets.

It then VALIDATES the generated data three ways (none of which check the index
against itself — that would be circular):

  1. ROUTING CONFORMANCE — does each generated label obey the empirically
     measured routing table (FINDINGS_v10)? A caller-attribution task must be
     labelled `callers`, not `grep`; a locate task must be `grep`/`find`. This is
     the check that actually catches a miswired generator.
  2. DISTRIBUTIONAL HEALTH — is the corpus non-degenerate? (signal present, not
     all one action; ambiguity/verify cases actually exist; answer sizes sane.)
  3. SOLVABILITY (structural proxy) — can the task be answered from the repo at
     all? A task whose gold answer set is empty/trivial is unlearnable noise.

Usage:
  python3 gen_tier1.py --repo ../repos/gin --lang go --binary codeindex \
      --sample 400 --out gin_tier1.jsonl
"""
from __future__ import annotations
import argparse, json, random, re, subprocess, sys
from pathlib import Path
from collections import Counter, defaultdict

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
import token_bench as tb  # noqa: E402
sys.path.insert(0, str(HERE))
import local_llm  # noqa: E402  (local Qwen via LM Studio; no external calls)

# Intent descriptions for paraphrasing — what the task MEANS, deliberately
# NOT naming the tool, so the rewrite can't telegraph the answer.
INTENT = {
    "caller_attribution": "find every place that calls/invokes/depends on the symbol",
    "token_refs": "find every place in the code that references or mentions the token",
    "vague_find": "locate a symbol by a rough description of what it does or its name",
}

def paraphrase_task(row, model, rng):
    """Local-LLM rewrite of a templated task into a naturalistic developer
    question. Intent preserved; tool keyword NOT leaked."""
    sym = row["symbol"]
    intent = INTENT[row["type"]]
    p = (f"A developer wants to: {intent}. The symbol/description is '{sym}'.\n"
         f"Write ONE short, natural question a developer would actually type — "
         f"vary the phrasing, do NOT use the words 'callers', 'grep', 'references' "
         f"or 'find' literally unless natural. Output ONLY the question.")
    try:
        q = local_llm.chat(p, model=model, temperature=0.9, max_tokens=40)
        return q.strip().strip('"').split("\n")[0]
    except Exception:
        return row["task"]  # fallback to template if local model unreachable

# ---- CLI wrappers -------------------------------------------------------- #

def run(binary, *args):
    r = subprocess.run([binary, *args], capture_output=True, text=True)
    return r.stdout

def parse_callers(out):
    """-> (defs:list, callers:list[(loc,sym,ambiguous)])"""
    callers, in_c = [], False
    for ln in out.splitlines():
        if ln.startswith("callers ("):
            in_c = True; continue
        if in_c and ln.startswith("  "):
            parts = ln.split()
            if len(parts) >= 2:
                amb = "[ambiguous]" in ln
                callers.append((parts[0], parts[1], amb))
    return callers

def parse_grep_symbols(out):
    """-> list of qualified symbol names that own occurrences"""
    syms = []
    for ln in out.splitlines():
        if ln.startswith("  ") and "hits=" in ln:
            syms.append(ln.split()[0])
    return syms

def parse_find(out):
    names = []
    for ln in out.splitlines():
        if ln.startswith("  "):
            names.append(ln.split()[0])
    return names

# ---- token vagueing (mirror recall_bench idea) --------------------------- #
SYN = {"get":"fetch","set":"put","new":"create","delete":"remove","make":"build",
       "handle":"process","render":"write","parse":"decode","bind":"attach"}

def tokenize(name):
    s = re.sub(r"([a-z0-9])([A-Z])", r"\1 \2", name)
    s = re.sub(r"[_\-.$]", " ", s)
    return [t.lower() for t in s.split() if t]

def vague_query(name, rng):
    toks = tokenize(name)
    if len(toks) >= 2:
        rng.shuffle(toks)
        if len(toks) >= 3 and rng.random() < 0.5:
            toks = toks[:-1]                      # drop a token
        toks = [SYN.get(t, t) for t in toks]     # synonym-swap some
    return " ".join(toks)

# ---- generation ---------------------------------------------------------- #

def generate(repo, lang, binary, sample, seed):
    rng = random.Random(seed)
    syms = tb.extract_symbols(Path(repo), lang)
    # dedup by name, prefer production (non-test) defs
    by_name = defaultdict(list)
    for s in syms:
        by_name[s.name].append(s)
    names = [n for n in by_name if len(n) >= 4]
    rng.shuffle(names)

    # track vague_find selection bias: how many candidates find FAILED to locate
    global _VAGUE_TRIED, _VAGUE_KEPT
    _VAGUE_TRIED = _VAGUE_KEPT = 0
    rows = []
    for name in names:
        if len(rows) >= sample:
            break
        defs = by_name[name]
        qname = name  # bare name; CLI resolves

        # --- caller_attribution ---
        cout = run(binary, "callers", repo, qname)
        callers = parse_callers(cout)
        if callers:
            amb = any(a for _, _, a in callers)
            traj = [{"tool": "callers", "args": {"symbol": qname}}]
            traj.append({"tool": "verify_ambiguous"} if amb else {"tool": "done"})
            if amb:
                traj.append({"tool": "done"})
            rows.append({
                "repo": Path(repo).name, "lang": lang, "type": "caller_attribution",
                "symbol": qname,
                "task": f"What functions or methods call '{qname}'? List each call site.",
                "gold_trajectory": traj,
                "gold_answer": sorted({loc for loc, _, _ in callers}),
                "ambiguous": amb, "answer_n": len(callers),
            })

        # --- token_refs (grep, attributed) ---
        # Taxonomy note: this class was called "occurrences" and clashed with
        # the A/B harness, whose "occurrences" type is semantically
        # caller-attribution (prompt says "functions that CALL X", gt is
        # caller_pairs). Renamed: token_refs = literal token references.
        gout = run(binary, "grep", repo, qname)
        gsyms = parse_grep_symbols(gout)
        if len(gsyms) >= 2:
            rows.append({
                "repo": Path(repo).name, "lang": lang, "type": "token_refs",
                "symbol": qname,
                "task": f"Which functions reference the literal token '{qname}'?",
                "gold_trajectory": [{"tool": "grep", "args": {"pattern": qname}}, {"tool": "done"}],
                "gold_answer": sorted(set(gsyms)),
                "ambiguous": False, "answer_n": len(gsyms),
            })

        # --- vague_find (fuzzy locate) ---
        if len(tokenize(name)) >= 2:
            _VAGUE_TRIED += 1
            vq = vague_query(name, rng)
            fout = run(binary, "find", repo, vq, "--limit", "5")
            hits = parse_find(fout)
            if hits and any(name in h or h in name or name.split(".")[-1] == h for h in hits):
                _VAGUE_KEPT += 1
                rows.append({
                    "repo": Path(repo).name, "lang": lang, "type": "vague_find",
                    "symbol": qname,
                    "task": f"Find the symbol whose name means roughly: '{vq}' (tokens may be reordered/dropped/synonymed).",
                    "gold_trajectory": [{"tool": "find", "args": {"query": vq}}, {"tool": "done"}],
                    "gold_answer": [name],
                    "ambiguous": False, "answer_n": len(hits),
                })
    return rows

# ---- VALIDATION ---------------------------------------------------------- #

# The routing table PROVEN in FINDINGS_v10 (task-type -> the tool that must
# lead the gold trajectory). This is the non-circular oracle.
ROUTING = {
    "caller_attribution": "callers",
    "token_refs": "grep",
    "vague_find": "find",
}

def validate(rows, repo, binary, rng):
    report = {}

    # 1a. LABEL CONSISTENCY (cheap, catches a coding typo — NOT a data-quality
    #     check; conformance here is near-tautological since the generator uses
    #     the same mapping. Reported honestly as such.)
    typo = sum(1 for r in rows if r["gold_trajectory"][0]["tool"] != ROUTING[r["type"]])
    report["label_consistency"] = {"mismatches_vs_own_mapping": typo,
                                   "note": "tautological guard; only catches gen typos"}

    # 1b. INDEPENDENT ROUTING TEST (non-circular): for a sample, run the OTHER
    #     tools too and confirm the gold tool is genuinely the cheapest that
    #     returns the answer. This is the real "is the label right" check.
    sample = rng.sample(rows, min(30, len(rows)))
    wins, ties, losses, detail = 0, 0, 0, []
    for r in sample:
        gold = r["gold_answer"]; sym = r["symbol"]
        # cost = # of result lines an agent must sift (lower=better); a tool
        # that doesn't contain the gold answer "loses" outright.
        def contains(out, ans):
            return sum(1 for a in ans if a.split(":")[0] in out or a in out)
        cand = {}
        for tool, call in (("callers", ("callers", repo, sym)),
                           ("grep", ("grep", repo, sym)),
                           ("find", ("find", repo, sym, "--limit", "10"))):
            out = run(binary, *call)
            nlines = len([l for l in out.splitlines() if l.startswith("  ")])
            covered = contains(out, gold)
            # a tool only "covers" if it returned >=1 result line AND hits >=60%
            # of the gold answer. This kills the false-positive where an EMPTY
            # result (0 lines) spuriously "covers" a small gold set.
            ok = nlines >= 1 and covered >= max(1, len(gold)) * 0.6
            cand[tool] = (ok, nlines)
        goldtool = ROUTING[r["type"]]
        covering = {t: n for t, (ok, n) in cand.items() if ok}
        if goldtool not in covering:
            losses += 1; detail.append((r["type"], sym, "gold tool did NOT cover", cand))
        else:
            best = min(covering.values())
            if covering[goldtool] == best:
                if list(covering.values()).count(best) > 1: ties += 1
                else: wins += 1
            else:
                losses += 1; detail.append((r["type"], sym, "another tool cheaper", cand))
    report["independent_routing"] = {
        "sampled": len(sample), "gold_tool_wins": wins, "ties": ties,
        "gold_tool_loses": losses,
        "pct_gold_optimal": round(100 * (wins + ties) / max(1, len(sample)), 1),
        "loss_examples": detail[:4],
    }

    # 2. DISTRIBUTIONAL HEALTH
    by_type = Counter(r["type"] for r in rows)
    first_actions = Counter(r["gold_trajectory"][0]["tool"] for r in rows)
    amb = sum(1 for r in rows if r.get("ambiguous"))
    verify_rows = sum(1 for r in rows if any(a.get("tool") == "verify_ambiguous" for a in r["gold_trajectory"]))
    ans_sizes = [r["answer_n"] for r in rows]
    dominant = first_actions.most_common(1)[0][1] / max(1, len(rows)) if rows else 0
    report["distribution"] = {
        "by_type": dict(by_type),
        "first_action_hist": dict(first_actions),
        "dominant_action_share": round(dominant, 3),
        "ambiguous_rows": amb,
        "verify_trajectory_rows": verify_rows,
        "answer_n_min": min(ans_sizes) if ans_sizes else 0,
        "answer_n_median": sorted(ans_sizes)[len(ans_sizes)//2] if ans_sizes else 0,
        "answer_n_max": max(ans_sizes) if ans_sizes else 0,
        "degenerate": dominant > 0.85,          # one action swamps -> no signal
        "has_verify_signal": verify_rows > 0,
    }

    # 3. SOLVABILITY (structural proxy): gold answer must be non-empty and,
    # for locate/vague, unambiguous enough (single gold) to be reachable.
    empty = sum(1 for r in rows if not r["gold_answer"])
    trivial_vague = sum(1 for r in rows if r["type"] == "vague_find" and r["answer_n"] > 8)
    vague_drop = round(100 * (1 - _VAGUE_KEPT / max(1, _VAGUE_TRIED)), 1)
    report["solvability"] = {
        "empty_answer_rows": empty,
        "vague_too_ambiguous_rows": trivial_vague,   # >8 find hits = hard to pin
        "pct_nonempty": round(100 * (1 - empty / max(1, len(rows))), 1),
        # SELECTION BIAS made visible: vague_find only kept rows find could
        # locate. This is the % of vague candidates find FAILED on (dropped).
        "vague_find_drop_rate_pct": vague_drop,
        "vague_tried": _VAGUE_TRIED, "vague_kept": _VAGUE_KEPT,
    }

    # overall verdict
    d = report["distribution"]; ir = report["independent_routing"]; sv = report["solvability"]
    report["VERDICT"] = {
        "labels_optimal": ir["pct_gold_optimal"] >= 80,   # gold tool genuinely wins
        "not_degenerate": not d["degenerate"],
        "verify_signal_present": d["has_verify_signal"],
        "mostly_solvable": sv["pct_nonempty"] >= 95,
    }
    report["VERDICT"]["trainable"] = all(report["VERDICT"].values())
    return report


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--repo", required=True)
    ap.add_argument("--lang", default="go")
    ap.add_argument("--binary", default="codeindex")
    ap.add_argument("--sample", type=int, default=400)
    ap.add_argument("--seed", type=int, default=1729)
    ap.add_argument("--out", default="tier1.jsonl")
    ap.add_argument("--paraphrase", action="store_true",
                    help="add naturalistic task_natural via LOCAL Qwen (LM Studio)")
    args = ap.parse_args()

    tb.RG = tb.resolve_rg(None)
    rows = generate(args.repo, args.lang, args.binary, args.sample, args.seed)

    if args.paraphrase:
        if not local_llm.available():
            print("WARN: local LLM (LM Studio :1234) unreachable — skipping paraphrase",
                  file=sys.stderr)
        else:
            model = local_llm.default_model()
            print(f"paraphrasing {len(rows)} tasks via LOCAL {model} ...", file=sys.stderr)
            prng = random.Random(args.seed + 2)
            for i, r in enumerate(rows):
                r["task_natural"] = paraphrase_task(r, model, prng)
                if (i + 1) % 50 == 0:
                    print(f"  {i+1}/{len(rows)}", file=sys.stderr)

    Path(args.out).write_text("\n".join(json.dumps(r) for r in rows) + "\n")

    report = validate(rows, args.repo, args.binary, random.Random(args.seed + 1))
    print(json.dumps(report, indent=2))
    print(f"\nwrote {len(rows)} rows -> {args.out}")
    v = report["VERDICT"]
    print("TRAINABLE-QUALITY:" , "YES" if v["trainable"] else "NO — " +
          ", ".join(k for k, val in v.items() if k != "trainable" and not val))


if __name__ == "__main__":
    main()

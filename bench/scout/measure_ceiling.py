#!/usr/bin/env python3
"""Measure the LOCAL-model routing ceiling on CALIBRATED naturalistic tasks.

Two corrections over the naive attempt:
  1. INTENT-PRESERVING paraphrase — natural phrasing that keeps the
     disambiguating cue. "what calls X" -> "what invokes X" (ok), NOT
     "what uses X" (erases caller-vs-reference distinction).
  2. ACCEPT-SET labels — callers and grep genuinely blur in natural language
     ("used" = both), so a callers-task answered with grep is SOFT-correct.
     find stays distinct (locating a symbol is a different question).

The output number is the gate: if a local 7B tops out low even here, no smaller
distilled model beats a good rule, and the model idea should be dropped.
All inference is local (LM Studio); no external calls.
"""
from __future__ import annotations
import json, sys, argparse, random
from collections import defaultdict
sys.path.insert(0, ".")
import local_llm

# intent-preserving paraphrase templates: natural, but the CUE survives.
# callers -> use call/invoke/depend verbs (NOT bare "use")
# grep    -> use reference/mention/appear/occur verbs
# find    -> use locate/where-is/named + a DESCRIPTION cue
PARA = {
    "caller_attribution": [
        "What invokes {S}?",
        "I'm about to change {S} — what depends on it?",
        "Which functions call into {S}?",
        "Show me the call sites of {S}.",
        "What would break if I changed {S}?",
    ],
    "occurrences": [
        "Everywhere the token {S} appears in the code.",
        "Which functions mention {S}?",
        "Show all occurrences of {S}.",
        "Where does the identifier {S} show up?",
        "Every spot that references {S}.",
    ],
    "vague_find": [
        "There's a function named something like {H} — where is it?",
        "Locate the symbol whose name is roughly {H}.",
        "What's the symbol called about {H}?",
        "Find the definition named like {H}.",
        "Where is the thing named {H} defined?",
    ],
}

def make_task(row, i):
    S = row["symbol"]
    import re
    m = re.search(r"means roughly: '([^']+)'", row.get("task", ""))
    H = m.group(1) if m else S
    return PARA[row["type"]][i % len(PARA[row["type"]])].replace("{S}", S).replace("{H}", H)

TOOLS = ("callers = which functions CALL/invoke the symbol. "
         "grep = which functions REFERENCE/mention the token (a superset of callers). "
         "find = LOCATE/identify a symbol by its name or description.")

def route_llm(task, model):
    p = (f"Available tools:\n{TOOLS}\n\n"
         f"A developer asks: \"{task}\"\n"
         f"Which ONE tool best answers this? Reply with exactly one word: "
         f"callers, grep, or find.")
    ans = local_llm.chat(p, model=model, temperature=0, max_tokens=6).lower()
    return ("callers" if "caller" in ans else "grep" if "grep" in ans
            else "find" if "find" in ans else "?")

def accept_set(gold):
    # callers <-> grep are mutually acceptable (natural-language "used" blur).
    if gold in ("callers", "grep"):
        return {"callers", "grep"}
    return {gold}

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default="gin_tier1.jsonl")
    ap.add_argument("--n", type=int, default=60)
    ap.add_argument("--seed", type=int, default=11)
    args = ap.parse_args()

    if not local_llm.available():
        sys.exit("local LLM (LM Studio :1234) unreachable")
    model = local_llm.default_model()
    rows = [json.loads(l) for l in open(args.data)]
    rng = random.Random(args.seed)
    # balanced sample across types
    bytype = defaultdict(list)
    for r in rows: bytype[r["type"]].append(r)
    per = max(1, args.n // len(bytype))
    samp = []
    for t, rs in bytype.items():
        samp += rng.sample(rs, min(per, len(rs)))

    print(f"model: {model} | tasks: {len(samp)} (calibrated, intent-preserving)\n")
    strict = soft = 0
    conf = defaultdict(lambda: [0, 0, 0])  # type -> [n, strict, soft]
    misses = []
    for i, r in enumerate(samp):
        task = make_task(r, i)
        gold = r["gold_trajectory"][0]["tool"]
        pred = route_llm(task, model)
        s = pred == gold
        sf = pred in accept_set(gold)
        strict += s; soft += sf
        c = conf[r["type"]]; c[0]+=1; c[1]+=s; c[2]+=sf
        if not sf:
            misses.append((gold, pred, task[:60]))

    n = len(samp)
    print(f"=== LOCAL ceiling on calibrated tasks ===")
    print(f"  STRICT (exact tool):     {strict}/{n} = {100*strict/n:.0f}%")
    print(f"  ACCEPT-SET (callers~grep): {soft}/{n} = {100*soft/n:.0f}%   <- the honest number")
    print(f"\n  by type (strict / accept-set):")
    for t, (nn, st, sf) in conf.items():
        print(f"    {t:<18} {100*st/nn:3.0f}% / {100*sf/nn:3.0f}%   (n={nn})")
    print(f"\n  hard misses (outside accept-set): {len(misses)}")
    for g, p, task in misses[:8]:
        print(f"    gold={g:<8} pred={p:<8} | {task}")
    print(f"\nGATE: accept-set >= ~85% -> distilling a 1.5B is justified (beat the "
          f"58% rule).\n      accept-set ~65% -> even a 7B can't route this; drop the "
          f"model, ship the rule.")

if __name__ == "__main__":
    main()

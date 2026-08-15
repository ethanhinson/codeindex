#!/usr/bin/env python3
"""Rule baseline router: task string -> tool, NO model.

The gate before any distillation. If a zero-ML keyword rule reproduces the
LLM-judge-validated gold tool as well on NATURALISTIC phrasings as on the
generator's own templates, then routing is rule-solvable and a distilled model
adds nothing *to routing* — the model would only be justified by the harder
decisions Tier-1 doesn't cover (query formulation, multi-hop, trust-vs-verify).

Two evals, deliberately:
  A. templated  — the generator's own task strings (trivial upper bound; a rule
     keyed on template phrasing hits ~100% by construction — NOT evidence).
  B. paraphrase — naturalistic rewrites where phrasing does NOT telegraph the
     tool. THIS is the real test of whether the rule generalizes.
"""
from __future__ import annotations
import json, re, sys
from pathlib import Path

# ---- the rule ------------------------------------------------------------ #
# intent keywords -> tool. Ordered: first matching bucket wins.
CALLERS = re.compile(r"\b(call|calls|called|caller|callers|invoke|invoked|invokes|"
                     r"who\s+calls|call\s*sites?)\b", re.I)
GREP = re.compile(r"\b(reference|references|referenced|refers?|use[sd]?|using|"
                  r"mention[sed]*|occurrence|occurrences|appears?|where.*used|"
                  r"every\s+(place|file|spot))\b", re.I)
FIND = re.compile(r"\b(find|locate|where\s+is|which\s+symbol|the\s+function\s+that|"
                  r"the\s+method\s+that|search\s+for|named?|whose\s+name)\b", re.I)

def route(task: str) -> str:
    """task -> {callers, grep, find}. A deliberately-dumb keyword router."""
    # callers is the most specific intent; check it before the broader 'use'
    if CALLERS.search(task):
        return "callers"
    if FIND.search(task):
        return "find"
    if GREP.search(task):
        return "grep"
    return "grep"  # default: literal search is the safe fallback

# ---- paraphrase generator (naturalistic, NON-telegraphing) --------------- #
# rewrites that a real dev/agent might use — deliberately avoiding the
# generator's template phrasing to test generalization, not memorization.
PARA = {
    "caller_attribution": [
        "I'm about to change {S}. What would break?",
        "Show me everything that depends on {S}.",
        "Who invokes {S}?",
        "List the call sites of {S}.",
        "Before I rename {S}, what calls it?",
    ],
    "token_refs": [
        "Where does the string '{S}' show up in the code?",
        "Every function that uses {S}.",
        "Find all references to {S}.",
        "What mentions {S}?",
        "Show occurrences of {S}.",
    ],
    "vague_find": [
        "Where's the thing that does {H}?",
        "I'm looking for a function about {H}.",
        "Locate the symbol for {H}.",
        "Which function handles {H}?",
        "Find the code named something like {H}.",
    ],
}

def paraphrase(row, i):
    S = row["symbol"]
    H = row.get("task", "")
    # for vague_find, extract the fuzzy hint the generator embedded
    m = re.search(r"means roughly: '([^']+)'", H)
    hint = m.group(1) if m else S
    tmpl = PARA[row["type"]][i % len(PARA[row["type"]])]
    return tmpl.replace("{S}", S).replace("{H}", hint)


def evaluate(rows):
    def score(get_task):
        by_type = {}
        for i, r in enumerate(rows):
            t = r["type"]; gold = r["gold_trajectory"][0]["tool"]
            pred = route(get_task(r, i))
            b = by_type.setdefault(t, {"n": 0, "hit": 0, "miss": []})
            b["n"] += 1
            if pred == gold:
                b["hit"] += 1
            elif len(b["miss"]) < 4:
                b["miss"].append({"task": get_task(r, i)[:70], "gold": gold, "pred": pred})
        return by_type

    templated = score(lambda r, i: r["task"])
    paraphr = score(lambda r, i: paraphrase(r, i))

    def summarize(bt, label):
        tot = sum(b["n"] for b in bt.values())
        hit = sum(b["hit"] for b in bt.values())
        print(f"\n=== {label}: {hit}/{tot} = {100*hit/tot:.1f}% routed correctly ===")
        for t, b in sorted(bt.items()):
            print(f"  {t:<18} {b['hit']:>3}/{b['n']:<3} = {100*b['hit']/b['n']:5.1f}%")
            for m in b["miss"]:
                print(f"      MISS gold={m['gold']:<8} pred={m['pred']:<8} | {m['task']}")
        return 100 * hit / tot

    a = summarize(templated, "A. TEMPLATED (trivial upper bound)")
    b = summarize(paraphr, "B. PARAPHRASE (the real test)")
    print(f"\nGATE: templated {a:.1f}% (expected ~100, proves nothing) | "
          f"paraphrase {b:.1f}% (THE number that matters)")
    print("READ: if paraphrase >= ~90%, routing is rule-solvable -> a distilled "
          "model is NOT justified for routing alone.\n"
          "      if paraphrase is low, the rule is brittle -> a model earns its "
          "keep on phrasing robustness.")
    return {"templated_pct": round(a, 1), "paraphrase_pct": round(b, 1)}


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else "gin_tier1.jsonl"
    rows = [json.loads(l) for l in open(path)]
    evaluate(rows)


if __name__ == "__main__":
    main()

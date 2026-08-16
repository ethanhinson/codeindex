#!/usr/bin/env python3
"""Per-task diff of the union arm: substring grep vs word-boundary grep.

Gate for flipping `codeindex nav` to -w (NEXT_STEPS item 6a): the union's
100%/0.95 was measured with substring grep, so word grep must hold that
score per-task before nav may adopt it. Prints any task whose f1 or
success changed, plus aggregate deltas.
"""
import json, sys
from pathlib import Path

HERE = Path(__file__).resolve().parent

def load(p):
    return {r["id"]: r for r in map(json.loads, open(HERE / p))}

def main():
    verdict_ok = True
    for ts in ("v6", "lphp", "nest"):
        sub = load(f"arm_c_union_sub_{ts}.jsonl")
        word = load(f"arm_c_union_word_{ts}.jsonl")
        assert sub.keys() == word.keys(), f"{ts}: task id sets differ"
        n = len(sub)
        f1s = sum(r["f1"] for r in sub.values()) / n
        f1w = sum(r["f1"] for r in word.values()) / n
        ss = sum(r["success"] for r in sub.values())
        sw = sum(r["success"] for r in word.values())
        print(f"{ts:>5}: n={n}  sub F1={f1s:.3f} succ={ss}/{n}   "
              f"word F1={f1w:.3f} succ={sw}/{n}")
        for tid in sub:
            a, b = sub[tid], word[tid]
            if a["f1"] != b["f1"] or a["success"] != b["success"]:
                print(f"       DIFF {tid} [{a['type']}]: "
                      f"f1 {a['f1']} -> {b['f1']}, "
                      f"success {a['success']} -> {b['success']}")
                if b["f1"] < a["f1"] or (a["success"] and not b["success"]):
                    verdict_ok = False
    print("\nVERDICT:", "word grep HOLDS — nav may flip to -w" if verdict_ok
          else "word grep REGRESSES — nav stays substring")
    return 0 if verdict_ok else 1

if __name__ == "__main__":
    sys.exit(main())

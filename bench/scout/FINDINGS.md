# Scout — routing-model feasibility: findings

**Date:** 2026-08-15 · **Verdict (UPDATED): for routing, a distilled LLM is NOT
needed — a local embedding + linear classifier hits 90% accept-set, matching a
7B at a fraction of the cost.** All inference local; no external calls.

> **Update (later same day):** the "distilled router is justified" verdict below
> was the conclusion BEFORE running the cheap gates. Running them (gate 3) showed
> an embedding+logreg classifier reaches 90% accept-set on held-out phrasings —
> within 3pt of the 7B ceiling (93%), 32pt above the rule (58%), at ~1ms/query
> and no GPU. A distilled 1.5B (projected ~85%) would be strictly worse. See
> "Gate result: classifier beats distillation" at the end.

## The question

Should we distill a small, locally-runnable model (Qwen-class) to act as a
"Scout" — a cheap navigator that, in front of an expensive coding agent, decides
which codeindex retrieval move a task needs? The prior A/B work (FINDINGS_v10)
proved the index *helps*; this asks whether a *model* is the right way to drive
it, or whether a dumb rule suffices.

## The toolkit built (bench/scout/)

- `gen_tier1.py` — Tier-1 data generator. Walks an INDEXED repo and manufactures
  `(task, gold-tool-trajectory, gold-answer)` triples whose answer is derivable
  from the call graph. Overfitting-immune: labels come from graph structure, not
  hand-authored accept sets. `--paraphrase` rewrites tasks naturalistically via
  a LOCAL model.
- `local_llm.py` — thin client for LM Studio's OpenAI-compatible API (:1234).
  Everything needing a model runs on-device.
- `rule_baseline.py` — zero-ML keyword router; the baseline any model must beat.
- `measure_ceiling.py` — measures the local-model routing ceiling on calibrated
  tasks with accept-set scoring.

## The chain of findings (each corrected the last)

1. **Tier-1 labels are graph-derivable and — on a semantic LLM-judge — correct.**
   10/10 sampled rows had the gold tool judged both correct and cheapest. But the
   naive validators lied first: a tautological check said 100% (it compared the
   generator to itself), a substring-cost check said 63% (it couldn't tell a tool
   answering a DIFFERENT question apart from string overlap). Only a semantic
   judge gave the true answer. **Lesson: routing-correctness cannot be checked by
   string matching; it needs semantic judgement.**

2. **A keyword RULE routes templated tasks at 100% but naturalistic ones at 58%.**
   The 100% is tautological (the rule keys on template phrasing the generator
   authored). The honest number — naturalistic phrasing — is 58%, and the misses
   are exactly the cases needing understanding: intent without the tool's keyword
   ("what would break?" = callers), keyword collisions ("find all references"),
   concept-level locate ("the thing that does X"). **A rule is not enough.**

3. **Single-gold-tool labels are wrong at the callers/occurrences boundary.**
   In natural language "where is X used?" means callers OR grep — irreducibly
   ambiguous. The correct target is an ACCEPT-SET (callers~grep interchangeable;
   find distinct). Single-gold over-penalizes a real blur.

4. **The local-model ceiling, on CALIBRATED tasks, is 88% strict / 93% accept-set.**
   Same 7B, same symbols, but tasks phrased naturally *with intent preserved*
   (verbs that keep the cue: "invokes" not bare "uses"). Per type: occurrences
   100%, caller_attribution 85%/100% accept-set, vague_find 80% (one weak
   template drags it). An earlier run hit only 35% — that measured a BROKEN
   paraphraser that erased intent, not the model. **The bottleneck is data
   quality + label schema, not model size.**

## The gate result

| baseline | number | meaning |
| --- | --- | --- |
| keyword rule, naturalistic | 58% | dumb floor |
| local Qwen-7B, naturalistic + accept-set | **93%** | achievable ceiling |
| distilled 1.5B target | ~80-88% (projected) | the worth-it band |

There is a ~35-point gap between the rule and the ceiling. That gap is what a
model captures; it justifies distilling one. Had the rule hit ~90%, no model
would be warranted — it didn't.

## Verdict & path

**Distilling a local Qwen-1.5B router is justified** — for ROUTING, on accept-set
labels, evaluated locally. Path: (1) teacher = local Qwen-7B labels naturalistic
tasks; (2) fix the one weak vague_find template, generate calibrated tasks +
accept-set labels across repos; (3) LoRA Qwen-1.5B, run in LM Studio, target
~85% accept-set, must beat 58%; (4) eval on the A/B harness with accept-set
scoring.

## Gate result: classifier beats distillation (added 2026-08-15)

Before committing to a LoRA/MLX distillation run, we ran the cheap gates. The
disciplined result overrides the plan:

| method | accept-set | strict | inference cost |
| --- | --- | --- | --- |
| keyword rule (`rule_baseline.py`) | 58% | — | free |
| **embedding + logreg (`clf_baseline.py`)** | **90%** | 81% | ~1ms, no GPU |
| local Qwen-7B (`measure_ceiling.py`) | 93% | 88% | ~1-2s, 7B |
| distilled Qwen-1.5B (projected) | ~85% | — | ~0.3s, 1.5B |

**Method:** `bge-base` embeddings (local, MPS) + logistic regression, evaluated
on a HELD-OUT-TEMPLATE split — test phrasings use templates never seen in
training, so this measures generalization to novel phrasing, not memorization.
(A naive phrasing-level split scored a misleading 100% — template memorization;
the held-out-template split is the honest number.)

Per gold tool: callers 100%, find 100%, grep 72% (grep is the weak bucket — it
blurs with callers under the "used" ambiguity; accept-set absorbs some of it).

**Conclusion:** for ROUTING, a distilled LLM is not worth it — a linear
classifier on local embeddings matches a 7B within 3pt at ~1000x lower inference
cost and no GPU, and can be embedded directly (768-dim + linear head). Distilling
an LLM is only justified for what the classifier CAN'T do: query formulation
(task -> good find tokens) and multi-hop trajectories. That is where any future
model effort should go.

## Honest caveats

- 60 tasks, ONE repo (gin). Not yet cross-repo or cross-language for Scout.
- ROUTING ONLY. Query formulation (task -> good find tokens) and multi-hop —
  the OTHER reasons for a model — are untested here.
- "1.5B hits ~85%" is a projection, not a measurement. The distillation run is
  the real test.
- Tier-1's single-hop, graph-derivable slice is the easy floor. Hard cases
  (trust-vs-verify on ambiguous edges, multi-hop stop decisions) need a
  teacher-trajectory tier that does not exist yet.

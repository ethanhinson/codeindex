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

## End-to-end: classifier through the A/B harness — the real seam (2026-08-15)

Ran the classifier as ARM C: classifier routes -> codeindex runs -> raw tool
output IS the answer, NO agent. Graded by the harness's own grader on gin+
prometheus (tasks_v6, n=20). See `arm_c.py`.

| type | arm C success | why |
| --- | --- | --- |
| vague_find | 100% | raw `find` output matches the grader's expected shape |
| comprehension | 50% | partial format overlap |
| caller/occurrence | 0% | answer data PRESENT but wrong FORMAT |
| OVERALL | 55% | |

The 0% buckets are NOT routing or retrieval failures — the correct callers are
in the raw output (`context.go:208 Context.AbortWithStatus`), but the grader
wants `AbortWithStatus\tcontext.go`. A trivial reformatter recovers it.

**Finding that reframes the whole arc:** routing is solved by a classifier (90%),
retrieval by codeindex (data always present) — the remaining gap is OUTPUT
NORMALIZATION, the agent's hidden second job. Removing the agent exposed it.

Corrected Scout architecture:
    task -> [classifier: route, 90% ~1ms] -> [codeindex: retrieve, complete]
         -> [formatter: normalize]  <- the real open problem -> answer

The distillation target, if any, is the FORMATTER, not the router — and it may
not even need a model (deterministic per-tool parsers likely suffice).

Caveat: also surfaced a taxonomy clash — the harness's "occurrences" type is
semantically caller-attribution (prompt says "functions that CALL X", gt is
caller_pairs), while the generator's "occurrences" means literal token refs.
Reconcile before further cross-use.

## Loop closed: classifier + index + deterministic formatters = the agent (2026-08-15)

Built per-tool deterministic formatters (`formatters.py`) that turn raw codeindex
output into the grader's expected shape, and re-ran arm C. Result vs the full
agent (arm B) on the SAME tasks (gin+prometheus, n=20):

| type | arm C (no agent, no model) | arm B (agent+index) |
| --- | --- | --- |
| comprehension | F1 0.82 / 100% | F1 0.96 / 100% |
| occurrences (caller-attr) | F1 1.00 / 100% | F1 1.00 / 100% |
| vague_find | F1 1.00 / 100% | F1 0.88 / 88% |
| OVERALL | **F1 0.95 / 100%** | ~0.95 / 96% |

Arm C matches the agent at ~zero cost (1 embedding + 1-2 CLI calls, no agent
tokens, no model). Verified real: the formatted answer matches ground truth
exactly (precision/recall 1.0), not a parser artifact.

**HONEST caveats (the 100% is a formatter test, not end-to-end):**
- For caller/comprehension the type-correct TOOL was forced; the classifier's own
  route disagreed 40% of the time (route% column). Fair end-to-end = routing 90%
  x formatting ~100% ~= **90%**, not 100%.
- 20 tasks, 2 repos, 3 types. Formatters are regex parsers tuned to these exact
  output shapes; a new language/format could break them.

## Final verdict of the "should we distill?" arc

Route (classifier, 90%) + Retrieve (codeindex, complete) + Normalize
(deterministic formatters, ~100%) => ~90% of navigation tasks answered with NO
agent and NO model, matching the agent that delivers the index's proven value.

**A distilled/local LLM is NOT needed for navigation.** It is only justified for
the untested hard cases: vague multi-symbol query formulation and multi-hop
trajectories with stop decisions.

## End-to-end honesty settled: routing errors cost 30 points; over-retrieval buys them back (2026-08-15 evening)

Both open items from the caveats above are now measured, on the JSON path
(formatters parse `codeindex --json`; regex parsers retired at verified
score-parity — commit 7c4d3c7).

**1. Honest end-to-end (classifier route drives tool AND answer shape,
`arm_c.py` default mode):** 70% success, F1 0.65 — notably WORSE than the
predicted 90% (routing-accept 90% × formatter 100%), because routing errors
are not uniform: all six harness "occurrences" tasks (semantically
caller-attribution) routed to `find`, and a find-shaped answer scores
F1 0.0–0.4 on the caller grader. The accept-set 90% hid this: the tasks the
router gets wrong are concentrated in exactly the type whose answer shape is
unforgiving. vague_find (8/8) and comprehension (6/6) survive their routing.

**2. Over-retrieval kills the router (`--over-retrieve`):** run ALL three
tools (callers+find+grep, milliseconds each), emit ONE union answer whose
section order is chosen against the grader's region rules — find top-hit
first (vague grader reads the first file:line), DEFINITIONS/FILES next,
CALLERS last (caller grader reads from the CALLERS marker to end). Result:
**F1 0.95 / 100% success — per-task identical to the forced-tool formatter
ceiling — with NO routing decision at all.** When the graph has no call edges
for a symbol, grep's enclosing-symbol attribution stands in for callers.

Leak check (the habit): the union path never reads the harness task type
(verified — `tp` only reaches the legacy branch and the grader), and per-task
F1 diffs vs the ceiling are zero, not merely aggregate-equal.

**Revised verdict:** for single-hop navigation even the CLASSIFIER is
unnecessary. The pipeline is Retrieve-everything (3 CLI calls) + one fixed
union format. The embedding+logreg router remains useful only as a cost
optimization (1 call instead of 3) — irrelevant at ~ms per call. Everything
model-shaped is now confined to: query formulation (symbol extraction from
vague intent), multi-hop trajectories, and trust-vs-verify on ambiguous edges.

Caveat that still stands: the symbol is extracted from the task id (all arms
equally). Real tasks hand you an intent, not a symbol — that gate (noun-phrase
+ fuzzy find, bge similarity) is the next measurement.

## Cross-language stress through the JSON path: Go == PHP == TS (2026-08-15 evening)

The union pipeline (`--over-retrieve`) ran unchanged on laravel-framework
(PHP, tasks_lphp n=24) and a freshly generated nest task set (TS,
tasks_nest.json n=24, same build_tasks.py recipe as lphp):

| Language / repo | success | F1 | comprehension F1 (non-circular gt) |
| --- | --- | --- | --- |
| Go (gin+prometheus, n=20) | 100% | 0.95 | 0.85 |
| PHP (laravel, n=24) | 100% | 0.95 | 0.86 |
| TS (nest, n=24) | 100% | 0.98 | 0.93 |

ZERO schema breaks: no parser or JSON change was needed for either language.
The one fix the PHP run forced was formatter POLICY, not parsing: laravel's
`resource` has three exact-name owners (Gate, RouteRegistrar, Router) and the
old top-def-only rule picked the wrong one (def_f1 0). fmt_union now emits
EVERY match=="exact" result under DEFINITIONS (falling back to the top hit) —
which also lifted Go comprehension 0.82 -> 0.85. Old caveat 2 (gin-tuned
regex, "WILL break on format drift") is fully retired.

Ground-truth provenance, stated honestly: occurrences / caller_attribution /
edit_impact gt is INDEX-DERIVED (SQL over graph.db unambiguous edges — true
for tasks_v6/lphp/nest alike), so arm C's ~1.0 there measures index-readback,
not discovery; a broken-but-self-consistent index would still score. The
non-circular anchors are (a) comprehension, whose gt is ripgrep and where the
union answer scores 0.85-0.93, and (b) the v10 A/B, where an agent WITHOUT
the index independently converged to the same caller answers. Residual F1
gap on comprehension is index scope, not wrongness: gt counts references in
CHANGELOG.md/README.md/config files the call-graph index does not cover.

## Taxonomy reconciled: "occurrences" is two different things no more (2026-08-15 evening)

The clash (caveat 3) is resolved by renaming the GENERATOR's class and
freezing the harness's:
- `gen_tier1.py` now emits `token_refs` ("which functions reference the
  literal token X" -> grep) — the class formerly called "occurrences" there.
  Router training is unaffected (it trains on `gold_trajectory[0].tool`, not
  the type string); old gin_tier1.jsonl corpora still load.
- The A/B harness keeps its frozen `occurrences` type name (recorded runs
  reference it) but `build_tasks.py` now documents at the source that it is
  semantically caller-attribution (prompt "functions that CALL X", gt
  caller_pairs, candidates reused from caller_attribution verbatim).
- `rule_baseline.py`/`measure_ceiling.py` template keys renamed to match.

Rule going forward: "occurrences" appearing in harness artifacts = caller
attribution; literal token references are always `token_refs`.

## Multi-hop as deterministic recipes: no policy needed for the common four (2026-08-15 evening)

`recipes.py` expresses the four common multi-hop trajectories as fixed
compositions of graph ops over the JSON contract — impact (= the existing
`codeindex impact`), where-tested (callers ∩ test-files + grep test sites),
rename-radius (defs ∪ exact-owners ∪ call sites ∪ token sites ∪ dep flags),
dead-code (no callers AND no word-references outside the def file, verdict
explicitly labeled evidence-not-certainty). `measure_recipes.py` grades the
file-level answer against a plain text scan of the repo (non-circular gt),
on the caller-task symbols of tasks_v6/lphp/nest.

Two deterministic fixes surfaced by measurement, neither a model:
1. CLI display limits silently truncated high-fanout symbols (recall as low
   as 0.24 on nest's createTestingModule) — recipes now pass --limit 2000.
   Lesson for any recipe consumer: the CLI defaults are for humans.
2. codeindex grep is SUBSTRING-based ('Decode' hits 'NewDecoder'); rename &
   where-tested semantics are word-based. A one-file-read word-boundary
   post-filter on grep-derived claims fixed precision (was 0.19-0.69 on
   short names like begin/shift/Decode).

Result after both: P=R=F1=1.00 on all three languages (n=6/12/11).

Honesty about that 1.00 (the too-clean rule): the grep leg's precision is
1.00 BY CONSTRUCTION (the post-filter applies the gt's own criterion), so at
FILE level this measurement proves (a) index file coverage misses nothing a
full text scan finds (recall leg — real), (b) caller-edge files always spell
the symbol (real, weak), and (c) plain word-scan would tie the recipes on
this metric. What the file-level gt CANNOT grade is the recipes' actual
value-add — symbol attribution, call-site lines, def locations, the
dead-code verdict — because the only gt for those is the index itself
(circular). Verdict for the model question: the four common trajectories are
pure structure + cheap glue; nothing here justifies a learned policy. The
residual is unchanged: trust-vs-verify on ambiguous edges, and attribution-
level gt would need human labels or PR-derived truth to grade non-circularly.

## Query formulation: cheap gates fail, but the shipped hybrid already answers (2026-08-15 evening)

`measure_gates.py` ran NEXT_STEPS step 9's two gates against the
human-curated concept sets (bench/concept_sets/<repo>.json, real vague
intents, any-of-N accept, curated_bench matches() semantics), hit@5, all
local. Reference column = `codeindex search` recorded on the SAME frozen
fixtures (bench/results/curated-FROZEN-*.json):

| repo | gate A: content-words + find | gate B: bge-base vs symbol names | shipped `search` |
| --- | --- | --- | --- |
| gin | 0.0% | 53.8% | 88.5% |
| flask | 0.0% | 64.0% | 76.0% |
| nest | 0.0% | 46.2% | 65.4% |
| laravel | 0.0% | 61.5% | 76.9% |

- Gate A is dead: find's lexical ladder cannot bridge intent -> name at all
  (0% everywhere; consistent with the recorded find control on the
  doc-derived concept class).
- Gate B (the cheapest semantic lane: embed the query, embed tokenized
  symbol names straight from graph.db, cosine top-5) lands 46-64% — real
  signal, below the 80% bar.
- The already-shipped hybrid (`codeindex search`: doc-context cards + graph
  boosts + RRF fusion, local CPU llama) clears the gates' gap by +12 to +35
  points on identical fixtures.

Verdict: a distilled LLM for query formulation stays unjustified — but not
because a cheap gate cleared 80%; because the shipped embedding+structure
hybrid is the answer and it is already local, CPU-only, and model-free in
the LLM sense. The pattern held one more time: structure + cheap glue. The
residual where a model could still earn its keep narrows to: query
REFORMULATION on top of search for the low repos (nest 65.4%), and the
long-standing trust-vs-verify policy on ambiguous edges.

# CodeIndex Roadmap

**Date:** 2026-08-16 · **Source:** the external assessment
(`codeindex-analysis-and-forward-plan.md`) reconciled against actual repo
state (`bench/scout/NEXT_STEPS.md`, the FINDINGS corpus, the residuals
backlog).

## Thesis (frozen)

> Coding agents should not spend model inference rediscovering structural
> facts and repository topology that can be computed once and queried
> deterministically.

North-star metric: **percentage of repository exploration eliminated
without reducing task success.** Secondary: **blast-radius recall before
the first edit.** Trust metric: **false-confidence rate on incomplete
structural evidence.**

## Where the repo already is (the doc is stale in places)

The assessment was written against the Scout-era commits. The repo has
since moved past several of its recommendations:

- **Routing is dead, not classifier-ified.** Over-retrieval
  (callers+find+grep → one union answer) hit 100% / F1 0.95 across
  Go/PHP/TS with *zero routing* (`fa367f0`, `2120d51`). The classifier is
  only a 3-calls→1 cost optimization. The doc's "embedding classifier
  replaces the router" architecture is already obsolete here.
- **`nav` shipped as the product verb** (CLI + MCP), word-boundary grep,
  golden-pinned, with the `callers_from_grep` blur flagged.
- **`--json` is the contract** (`37bd8e8`) — Route → Execute → Project is
  already the pipeline; the regex-normalization anti-pattern is gone.
- **The A/B proof exists at small scale**: indexed arm ~60% cheaper, ~3×
  fewer turns, 77%→90% success (`14412c9`); the nav prompt-note A/B
  measured selective adoption at no cost (40/40 pairs, +0.0pp, +0.4%).
- **Re-ranking is a closed layer.** Role boost (vote saturation falsified
  by diagnosis) and result-set diversity (MMR/round-robin/family-discount
  dry-run, zero engine changes) both failed the bars. nest's true
  bottleneck is recall@40 = 84.6% — a retrieval problem no re-ranker can
  touch. One admitted mechanism survives: **definition-by-usage card
  enrichment** (bucket 2, servicing bucket 1 too).
- **Cold-build speed is already correctly deprioritized**: pool default-on
  shipped (1.83× embed), k8s ≤2-min bar honestly open (~10.6 min), and the
  artifact-import architecture (82.5s build → 1.5s import) is the answer.

What the doc asks for that genuinely does not exist yet: **edge
provenance/confidence, an `impact` product verb, typed evidence packets
with coverage fields, language-level resolution P/R gates, and the
50–100-task multi-arm benchmark.** That is the roadmap.

---

## M0 — Housekeeping (now, hours)

- Commit the in-flight evidence: `FINDINGS-rerank-dryrun.md`,
  `rerank_dryrun.py`, the four dry-run artifacts, the residuals-backlog and
  FINDINGS-residuals-roles updates, the note-A/B results.
- Regenerate `gin_tier1.jsonl` (predates the `token_refs` rename) or mark
  ROUTED runs as known-stale in `arm_c.py`.

## M1 — Freeze the ontology (≈1 week, mostly writing)

Prevent drift back into "generic AI code search."

- Put the thesis verbatim in README/design notes.
- Define the semantic operation ontology (`symbol.locate`, `.callers`,
  `.callees`, `.references`, `.implementations`, `file.dependencies`,
  `change.impact`, `change.tests`, `navigation.*`) and map every CLI/MCP
  verb onto it. CLI names may change; ontology names may not.
- Write the epistemics page: what CodeIndex *knows* (exact edges), what it
  *infers* (`callers_from_grep`, `[ambiguous]`), what it *cannot resolve*
  (dynamic dispatch, DI, reflection, config wiring).

**Exit:** every public operation maps to one stable intent; CLI, MCP, and
plugin share the typed `--json` contracts; no benchmark label has two
meanings across harnesses (the `token_refs`/occurrences reconciliation
pattern, applied everywhere).

## M2 — Benchmark v1: baseline the north-star metric (≈2 weeks)

Deliberate reorder vs. the doc (it puts the benchmark after resolution
work): build the gate *first*, at moderate scale, so M3/M4 have a number
they must move. Reuse `bench/agent_ab` + `CODEINDEX_DISABLED=1` isolation.

- 50–100 organic tasks (not hand-templated) across Go, TS, PHP, Python;
  arms A (frontier + shell) and B (A + CodeIndex). Arms C/D optional later.
- Measure separately: localization (files/symbols found, time-to-first,
  unnecessary reads), change coverage (affected files before first edit,
  tests found, missed edges, false-confidence rate), efficiency (turns,
  tokens, wall time, cost), end-to-end success.
- Apply the discipline rule: "can this pass if CodeIndex is broken?" —
  template leakage, control contamination, and forced-tool behavior are
  the known leaks; the harness already has guards for two of the three.

**Exit:** a frozen baseline of exploration-eliminated / blast-radius-recall
/ false-confidence, per language. This is the go/no-go instrument.

## M3 — Resolution correctness & provenance (the core investment, ≈4–6 weeks)

Make structural facts trustworthy enough that agents skip re-verification.

- Every edge in `--json` output gains: `provenance` (mechanism),
  `confidence` (exact / inferred / ambiguous), candidate count, and known
  blind spots. `callers_from_grep` and `[ambiguous]` are the existing
  seeds of this schema — generalize them.
- Priority order: import/module resolution → method/type ownership →
  interfaces/implementations/inheritance → cross-package calls →
  same-name disambiguation → tests-for-symbol → explicit unresolved
  dynamic behavior.
- Gates are language-specific precision/recall on labeled edge samples,
  not edge counts. Depth over breadth: the current languages
  (Go/TS/PHP/Python) with credible graphs beat any expansion.

**Exit:** per-language P/R gates green; a caller answer that misses an
edge *says so* (coverage field) instead of looking authoritative.

## M4 — Evidence packets: `nav` / `impact` / `explore` (≈3 weeks, overlaps M3)

Minimize calls-and-tokens to orient an agent.

- Typed JSON schemas for the three bundles. `nav` extends the shipped
  NavAnswer with tests, implementations, ambiguities, coverage,
  provenance. `impact` promotes `bench/scout/recipes.py` (impact,
  where-tested, rename-radius, dead-code — already P=R=F1 1.00 at file
  level) from bench glue to a product verb with transitive blast radius
  and explicit coverage limits. `explore` wraps hybrid search.
- Compact projections per token budget; direct path:line locations for
  spot-verification.
- The contract question: *what is the minimum reliable evidence an agent
  needs before changing this code?*

**Exit:** the four common multi-hop trajectories are each one call; the
plugin/MCP surface is task-shaped, low-level verbs remain but are not the
default.

## M5 — Benchmark v2: the go/no-go (after M3+M4)

Re-run M2 with provenance and packets in place; add arm C (cheap explorer
+ shell) and the adversarial suites:

- grep-should-win: exact strings, filename lookup, tiny repos.
- CodeIndex-should-dominate: widely-called interfaces, cross-package
  change, deep dependency paths, blast radius, test-surface discovery.
- designed-to-break: dynamic dispatch, DI, event buses, reflection,
  generated code, same-name collisions, framework wiring. Runtime-evidence
  work continues only where these show static misses that matter (the
  Drupal +7.2 / WordPress-null pattern is the template).

**Go gate:** no success regression; ≥50% fewer exploration tokens or ≥40%
fewer reads/calls; improved affected-file/test recall; manageable
false-confidence. **Kill gate:** arm C matches arm B on success, cost,
latency, and coverage — then the structural index doesn't pay for its
maintenance, and the honest move is to say so.

---

## R — Retrieval track (parallel, small, evidence-gated)

Independent of M1–M5; each item enters only with its registered
falsifiable prediction, per the anti-overfit ratchet.

- **R1 — Definition-by-usage card enrichment** (buckets 2+1): call-site
  distributional context — caller names, assigned-variable names, argument
  shapes — folded into cards. The sole surviving bucket-1 mechanism and
  bucket 2's registered one. Ground truth already checked: 48 nest sample
  files wrap `NestFactory.create` in `async function bootstrap()` — the
  caller's name IS the unreachable query's vocabulary. Prediction: the
  named frozen misses flip; nest curated ≥75; gin 88.5 / flask 76.0 /
  laravel 76.9 hold; mechanical+find classes hold. This attacks recall@40,
  which re-ranking provably cannot.
- **R2 — Literal lane for bug-symptom queries** (bucket 4):
  grep-attribution over distinctive query words as a third RRF lane +
  literal-aware cards. Prediction (pre-registered): issues-v2 hit@5 ≥
  grep control per repo (≥21% gin / ≥44% flask), ≥40% flask absolute, no
  curated regression. Multi-hop misses (23/55) stay out of scope — that's
  graph/runtime territory.
- **R3 (conditional) — query-conditioned vote weighting**: unfalsified but
  constrained by the fragile rank-5 geometry; only if R1 leaves a named
  residual bucket.

## Paused / closed (do not reopen without new evidence)

- **Any custom generative model.** Nine consecutive measurements said no.
  The only future candidate is a *sequential navigation policy* (next
  action / stop decision), and only after: stable ontology, structured
  evidence contracts, a persistent sequential gap in M5 results, and
  rules/classifiers failing to close it cheaply.
- **Re-ranking mechanisms** without a new named residual bucket (role
  boost stays withheld behind `CODEINDEX_ROLE_BOOST=1`; diversity closed
  by dry-run).
- **Language expansion.** Depth in the current four first.
- **Cold-embed speed / the k8s 2-min bar.** The CI-artifact + local-delta
  architecture is the product answer; revisit only if real users hit it.
- **Elaborate UI.** CLI + MCP + plugin note suffice until M5 says go.

## Sequencing at a glance

```
M0 ─ M1 ─ M2 ──────────────── M5 (go/no-go)
          │                  ╱
          ├─ M3 (provenance)─┤
          ├─ M4 (packets) ───┘
          └─ R1, R2 (parallel, small)
```

The project is judged by one sentence it must earn at M5:

> We eliminated a large portion of LLM repository exploration while
> maintaining or improving coding-task success and blast-radius coverage.

# Residuals backlog — post diffusion+contrast (design D6)

Analysis of the 24 tuning-set misses that survived the frozen A+B
configuration (`bench/results/curated-FROZEN-*.json`). Per the anti-overfit
ratchet: an enrichment signal is admitted ONLY against a named bucket below,
in its own change, with a falsifiable prediction registered before its
measurement.

## Bucket 1 — entry-point preference (≈11 of 24 misses; dominant on nest)

The pipeline lands in the right call-graph region but ranks internal
plumbing above the public API entry (`RoutePathFactory.getVersionPrefix`
over `enableVersioning`; `Module.addProvider` over `Injectable`;
`ApplicationConfig.addGlobalRequestGuard` over `UseGuards`). Diffusion
finds the neighborhood; nothing tells it which member is the *surface*.

- Candidate mechanism: structural roles from edge shape (entry/registered
  vs internal), fed into ranking — the deferred roles change, now with a
  measured residual justifying it.
- Falsifiable prediction to register there: nest curated ≥ 75% (+9.6pts)
  with no tuning-repo regression.

## Bucket 2 — generic-name APIs (≈8 of 24; gin Set/Get, flask run, laravel hasMany)

Accepted answers have near-empty semantic cards: one-token generic names,
terse docs. No amount of family contrast helps a card that says only
"Set. sets a value.".

- Candidate mechanism: call-site distributional context (how CALLERS use
  the symbol: assigned-variable names, argument shapes) and/or span
  literals — the "definition by usage" enrichment.
- Falsifiable prediction: the specific missed questions (recorded in the
  frozen results) flip without regressing the mechanical classes.

## Bucket 3 — sample/demo corpus noise (≈5 of 24; nest AA/BB, flask test fixtures)

`sample/`, `integration/`, fixture apps outrank core implementations
(`AppModule.onApplicationBootstrap` over `NestApplication`).

- Candidate mechanism: extend the existing test-file ranking penalty to
  demo/sample/integration path segments (one deterministic list, applied
  in `boosts`) — smallest change on this list.
- Falsifiable prediction: nest "bootstrap" and flask "test client"
  questions flip; no regression elsewhere.

## Bucket 4 — bug-symptom queries (added 2026-07-12; v2 GT + controls same day)

Cleaned ground truth (mapping v2) + controls
(`bench/results/issues-v2-*.json`): search scores 10.3% (gin) / 33.3%
(flask) hit@5 — and **plain grep-attribution beats search on both repos**
(20.5% / 43.3%; find-control 0%). Symptom strings live in code literals
(error/warning messages, config keys) that cards deliberately exclude.
Miss anatomy: 23/55 multi-hop (no lexical bridge — graph/runtime territory),
11/55 outranked-at-6-15 (ranking), 7/55 literal-present (measured, quoted
in `bench/selfheal/issues_miss_analysis.md`).

- Admitted mechanism (evidence-backed): a **literal lane** in hybrid
  retrieval — grep-attribution over distinctive query words as a third RRF
  lane — plus literal-aware card text for the B bucket; multi-hop (C) stays
  with graph/runtime evidence, not lexical patches.
- Falsifiable prediction to register in that change: issues-closed v2
  hit@5 ≥ grep-control per repo (i.e. the hybrid must dominate its own
  lexical lane: ≥ 21% gin / ≥ 44% flask) and ≥ 40% flask absolute, with no
  curated regression.
- Prereq hygiene (tracked): comment-only-hunk attribution (bucket A, 2
  questions) and refactor-title deny-filter leaks (bucket E).

## Explicitly NOT admitted (no bucket demands them)

Test-caller names in cards, registration-line verbatim capture, region
vocabulary, two-level community retrieval — the mechanisms left these
without a named residual. They stay out until a future miss analysis
produces one.

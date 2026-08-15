# Residuals backlog — post diffusion+contrast (design D6)

Analysis of the 24 tuning-set misses that survived the frozen A+B
configuration (`bench/results/curated-FROZEN-*.json`). Per the anti-overfit
ratchet: an enrichment signal is admitted ONLY against a named bucket below,
in its own change, with a falsifiable prediction registered before its
measurement.

## Bucket 1 — entry-point preference (≈11 of 24 misses; dominant on nest)

**MEASURED 2026-08-15, WITHHELD from defaults** (full account:
`bench/engine/FINDINGS-residuals-roles.md`; artifacts
`bench/results/curated-ROLES-*.json`). Structural roles from the diffusion
subgraph's directed edges — user-side caller files (the filePenalty class)
vote for their callees as surface. nest 65.4→76.9 (bar ≥75 MET; every flip
is this bucket's thesis: Injectable, UseInterceptors, Injector) but gin
−3.9 / flask −4.0 broke the no-regression conjunction: hyper-generic
surface APIs with dozens of test-file votes (`Context.Status`) edge out
on-topic answers at the rank-5 boundary. Two-iteration budget spent
(iteration 1, foreign-dir counting, flooded flat repos). Ships as
experimental behind `CODEINDEX_ROLE_BOOST=1`.

The diagnosis was sharpened en route: the missed public APIs have GOOD
cards (`Injectable`'s doc literally matches its query) but neither lane's
top-50 retrieves them — only diffusion does — so ranking the union is the
right layer.

- Registered follow-up (NEW change, fresh 2-iteration budget): **vote
  saturation** — cap/squash distinct-voter counts so "demonstrated by user
  code at all" dominates and "demonstrated 40 times" adds little.
- Bars unchanged: nest curated ≥ 75%, no tuning-repo regression
  (gin 88.5 / flask 76.0 / laravel 76.9), mechanical + find classes hold.

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

**MEASURED 2026-08-15, SHIPPED** (full account:
`bench/engine/FINDINGS-residuals-roles.md`). The penalty list was extended
(sample/example/demo/benchmark/fixture/integration segments + `.spec.`)
AND — the part that mattered — moved outside the boostGamma compression
envelope, where 0.7 had been decaying to ≈0.88 (the compressed first cut
measured 0.0 change everywhere). Result: zero regression on every gate,
fixture symbols ranked below core across the board, and nest mechanical
strict rose 55.0→60.0. The registered flip predictions did NOT come true:
those questions' blockers are retrieval (bucket 1's territory), not
fixture noise — recorded as a partially-failed prediction, kept for the
ordering quality and the mechanical gain.

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

# Diversity re-ranking dry-run: falsified offline, zero engine changes

**Date:** 2026-08-16 · **Context:** successor-mechanism scouting after the
vote-saturation falsification (appendix of `FINDINGS-residuals-roles.md`).
Question: can result-set diversity (MMR / family round-robin / family-
relative discount) fix the nest clone-wall misses without regressing
gin/flask/laravel? Answer: **no configuration clears the bars.**

## Method (and its honesty gates)

`bench/rerank_dryrun.py` — fully offline over the SHIPPED binary's output
(role boost off, no engine changes): per question, top-40 from
`search --flat --limit 40`, card vectors joined from the repo's
`graph.db` (`symbols → symvec → vecs`; all-minilm-l6-v2 int8 blobs;
join coverage 100% on every repo), re-rankers applied, top-5 regraded
with `curated_bench.matches` semantics.

- **Reproduction gate (hard):** parsed baseline top-5s must reproduce the
  frozen per-question hits exactly (gin 23/26, flask 19/25, nest 17/26,
  laravel 20/26). Passed on all four before any re-ranking was read.
- **Known fidelity limit (recorded in each artifact):** `--flat` exposes
  no numeric scores, so re-rankers ran on rank-decay proxies. The
  recall@40 ceiling numbers are exact regardless; config deltas are
  directional.

Configs: `mmr_λ` (λ 0.9→0.5), `rr_τ` (round-robin over cosine≥τ connected
components, τ 0.995→0.95), `famz_τ_β` (within-family discount). Artifacts:
`bench/results/rerank-dryrun-{gin,flask,nest,laravel-framework}.json`.

## Results (hit@5 %)

| config | gin (hold 88.5) | flask (hold 76.0) | nest (bar ≥75) | laravel (hold 76.9) |
| --- | --- | --- | --- | --- |
| baseline / mmr_0.9 / rr_0.995–0.97 / famz_* | 88.5 | 76.0 | 65.4 | 76.9 |
| mmr_0.8 | **84.6 ✗** | **72.0 ✗** | 65.4 | 80.8 |
| mmr_0.7 | 84.6 ✗ | 72.0 ✗ | 69.2 | 76.9 |
| mmr_0.6 / mmr_0.5 | 80.8 ✗ | 72.0 ✗ | **73.1** (best, still <75) | 76.9 |
| rr_0.95 | 88.5 | 76.0 | 65.4 | **73.1 ✗** |

The frontier is monotone: every diversity strength that gains nest
(resolveSingleParam from rank 6, createParamDecorator from rank 6) loses
gin AND flask first. The flask loss is the SAME rank-5 question the role
boost broke ("clean up resources when the request ends" —
`do_teardown_request` evicted, this time by `stream_with_context` under
diversity pressure). Two independent mechanism families now fail
identically: rank-5 holds elsewhere are more fragile than nest's buried
answers are reachable.

## The decisive facts beyond the matrix

1. **recall@40 is the real bottleneck.** nest 84.6% (4 of 9 misses have NO
   accepted answer anywhere in the top-40: GET-route decorator, guards,
   bootstrap, route prefix), flask 92% (dev server, test client absent).
   gin/laravel 100%. No re-ranker of any kind can touch the unreachable
   misses — they are retrieval (vocabulary-gap) failures, confirming the
   bucket-1 sharpened diagnosis a third time.
2. **`Injectable` (rank 15) is beyond diversity's reach** — even mmr_0.5
   never surfaced it. MMR's practical reach here was ranks 6–8.
3. **The "clone wall" is not a wall in vector space.** rr/famz at
   τ ≥ 0.97 were no-ops on every repo: the Module.* near-clones sit BELOW
   0.97 cosine in minilm space. Dropping to τ=0.95 immediately caused the
   laravel loss (family-collapse kept `resolveRouteBindingQuery` as the
   family representative and evicted the accepted `Model.resolveRouteBinding`
   at rank 4) — accept sets often contain family members, so collapsing
   families evicts accepted answers as readily as noise.

## Verdict and what survives

Result-set diversity is CLOSED as a bar-clearing mechanism for bucket 1
(dry-run falsification; cost: zero engine changes, zero iterations —
the harness is reusable for any future re-ranker via `rerank_dryrun.py`).
The evidence now points away from the re-ranking layer entirely: the
reachable-by-reranking miss set is too small and too guarded by fragile
rank-5 holds. Surviving directions, in order of evidence alignment:

- **Retrieval-side vocabulary evidence** — definition-by-usage card
  enrichment (bucket 2's mechanism): code-intrinsic, so it holds on
  private repos regardless of history hygiene. (The commit-message lane
  was considered and rejected on product grounds the same day — commit
  quality is not consistent across private repos; see the backlog.)
  This attacks the recall@40 gap, which re-ranking cannot: e.g. 48 nest
  sample files wrap `NestFactory.create` in `async function bootstrap()`
  — the caller's name IS the unreachable query's vocabulary.
- Query-conditioned vote weighting remains unfalsified but is constrained
  by the same fragile rank-5 geometry documented here and in the
  vote-count diagnostic.

# Semantic search: concept-class recall gate — findings

**Date:** 2026-07-11 · **Change:** `semantic-code-search`

## Shipped configuration

- `codeindex search`: hybrid retrieval — lexical lane (find ladder over query
  + optional hints) fused with a vector lane (bundled all-MiniLM-L6-v2 Q8_0
  GGUF, statically linked llama.cpp, CPU-only) by reciprocal-rank fusion
  (lex k=60, vec k=10), graph boosts compressed to `boost^0.35`, results
  clustered by call-graph connectivity into a feature map.
- Symbol cards: tokenized qualified name, kind, signature, path tokens, doc
  context (block-top description with @tags skipped; Python docstrings below
  the def), top-8 caller/callee names. Content-hash keyed vectors: unchanged
  card text never re-embeds across incremental patches.
- Surfaces: CLI `search`/`model`, MCP `search` tool + `explore-feature`
  prompt, `/codeindex:explore` plugin command, editor rules snippet. NOT in
  the always-visible prompt note (v1/v3/v6 lesson holds).

## Pre-registered bars (registered 2026-07-11, before first measurement)

1. CONCEPT class (target's doc comment with every name-derived token
   stripped — describes the symbol without naming it): `search --flat`
   hit@5 ≥ 60%, no hints (conservative floor).
2. Existing vague/distinctive `find` classes: no regression.
3. Full-build embed overhead within budget (registered target ≤ 2 min added
   at kubernetes scale).

## Verdict: MIXED — 2 languages PASS strict, 2 fail with measured causes

All four quick-tier bench repos, seed 99, n=60, `bench/engine/concept-*.json`:

| Repo | Lang | search hit@5 | family hit@5¹ | find control | Verdict |
| --- | --- | --- | --- | --- | --- |
| gin | Go | **61.7%** | — | 0.0% | PASS |
| flask | Python | **61.7%** | — | 0.0% | PASS |
| nest | TS | 43.3% | **61.7%** | 0.0% | FAIL strict; family clears bar |
| laravel-framework | PHP | 35.0% | 35.0% | 0.0% | FAIL — generator validity limit |

¹ family hit@5 (post-hoc diagnostic, NOT the registered metric): counts a hit
when the top-5 contains any symbol whose doc phrase is IDENTICAL to the
target's — i.e. the query was ambiguous among clones by construction.

The lexical control at 0.0% everywhere is by construction (no name tokens
survive in the query) — all concept-class recall is the embedding lane's
contribution.

Non-regression (bar 2): gin vague-class hit@5 **93.5%** (bar ≥70%; original
94.4%, sample wobble; `find` scoring untouched). PASS.

### Failure analysis (each verified by reading actual misses)

- **nest (TS)**: 16.7 pts of misses are identical-doc sibling families —
  ~30 HTTP exception classes share one boilerplate doc, so the generated
  query ("defines an http for errors") is genuinely ambiguous and the top-5
  fills with correct-family clones. Escalation per D2 (bge-small-en-v1.5,
  3× embed cost) measured 45.0% — the failure is insensitive to model
  quality, locating it upstream of the model, in the query/scoring
  construction. Any of the returned siblings would satisfy a real user.
- **laravel (PHP)**: no sibling effect (family == strict). Laravel's docs
  are terse name-restatements ("Compile the limit portions of the query" →
  target `compileLimit`), so stripping name tokens leaves stopword soup
  ("the portions of the query") with no concept signal — the generator
  produces unanswerable queries on this doc style, and the results returned
  for those literal word-soups are reasonable. Live human-phrased queries
  behave well: "database schema migration rollback" → `Migrator.rollback` +
  `RollbackCommand` clusters; "how are failed queue jobs retried" →
  `Job.retryUntil`/`JobRetryRequested`/`Job.fail`.
- **Conclusion**: the proxy's core assumption — doc-minus-name approximates
  a human concept query — holds only where docs are standalone prose (Go,
  Python). Where docs accompany the name instead of replacing it (nest,
  laravel), the generator MANUFACTURES unanswerable queries; the corpus is
  not at fault and the strict numbers there are floors, not measurements.
  Symmetrically: TS/PHP currently have NO valid quantitative measure — live
  spot-checks are encouraging but anecdotal. The registered follow-up gate
  for TS/PHP is a human-curated concept set with any-of-N acceptable
  answers (task 6.1's original formulation), plus a measurability guard on
  the mechanical class (emit a case only if the residual query keeps ≥2
  informative content words; report the discard rate) so mechanical numbers
  are only reported where the proxy is valid.

### Registered iterations (2)

1. gin first run 56.7%: raw graph boosts (0.4–3.9×) dwarfed RRF rank deltas
   (~8%/rank), so 200-caller generics outranked semantically-right answers.
   Fixed: vec-lane RRF k 60→10, boosts^0.35, Python docstring extraction.
2. nest first run 45.0% exposed a card-poisoning bug: doc extraction took
   the 4 lines adjacent to the def — for long JSDoc/docblock comments that
   is the `@see`/`@param` tail, not the summary. Fixed: block-top,
   description-only extraction (regression test:
   `TestDocCommentJSDocSummaryNotTail`). gin/flask re-earned their PASS
   under the new cards; nest moved 45.0→43.3 (its failure is corpus
   degeneracy, above).

## Model comparison (candidate data for D2/D9)

| Model | size | gin concept hit@5 | notes |
| --- | --- | --- | --- |
| all-MiniLM-L6-v2 Q8_0 (bundled) | 24 MB | 61.7–63.3% | smallest passing; stays default |
| bge-small-en-v1.5 Q8_0 (pull) | 35 MB | **75.0%** | +11.7 pts on gin; nest 45.0% (degeneracy unaffected); ~2× embed time (gin build 18→40 s) |

`codeindex model pull bge-small-en-v1.5-q8_0` is the documented quality
upgrade. Full pull→use→re-embed→search cycle verified end-to-end.

## Embed-time budget (bar 3) — measured, extrapolation flagged

| Repo | symbols | cold build (incl. embed) |
| --- | --- | --- |
| gin | 1,179 | 10.6–19.9 s |
| flask | 1,577 | 10.2–20.6 s |
| codeindex (this repo) | 2,567 | 25.2 s |
| nest | 4,482 | 52.6–69 s |
| laravel-framework | 28,748 | **5 m 01 s** |

~6–10 ms/card batched (M-series, 8 threads, 32-seq packed encode; batching
took it from 31 ms/card). laravel confirms roughly linear scaling →
kubernetes (182k) extrapolates to ~30 min — **over the registered 2-min
ceiling; not measured at that scale**. Known lever: parallel llama contexts.
Overhead is one-time per card (content-addressed cache); the per-query patch
path loads no model when no files changed. Open engineering item; the recall
verdicts do not depend on it.

## Caveats recorded

- Batched inference wobbles ~2e-4 cosine by batch composition — below int8
  storage quantization (~8e-3); vectors are compute-once per content hash.
- Neighbor drift (D4): caller/callee names in cards refresh only when the
  symbol's own file changes or on full build — accepted, unmeasured.
- No hints were used in the gates; MCP clients supplying hints should only
  improve on the floor. Agent-level A/B (adoption/cost) has not run —
  `search` stays out of the ambient note until it does.
- One measurement hazard hit during this run: two concurrent builds of the
  same repo race on graph.db (second writer gets a disk I/O error). The
  affected laravel result was discarded and remeasured on a clean build
  (same number, 35.0%).

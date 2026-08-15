# Design: diffusion-contrast-retrieval

## Context

semantic-code-search shipped hybrid retrieval (lexical + vector lanes, RRF,
damped graph boosts, call-connectivity clustering). Its gate was mixed:
Go/Python passed (61.7%); nest failed strict at 43.3% with 16.7pts measured
as identical-doc sibling ambiguity; laravel's 35.0% was shown to be a
measurement artifact (the mechanical doc-phrase generator manufactures
unanswerable queries on name-restating doc cultures), leaving TS/PHP with no
valid quantitative measure. Full analysis:
`bench/engine/FINDINGS-semantic-search.md`.

The exploration that produced this change (recorded here deliberately)
rejected three alternative directions:
- **Card-content stages** (literals → tests → registrations → roles): treats
  the two measured corpora's symptoms; overfit risk flagged by the user.
- **Language server foundation**: LSPs solve reference precision, not any of
  our open problems (concept matching, roles, framework wiring — LSPs are
  equally blind to string-keyed/config wiring); and a daemon+toolchain
  foundation forfeits the measured product moat (single binary, air-gapped,
  fresh-on-query). LSP re-enters later only as an optional, measured oracle.
- **LLM summaries at index time**: still excluded (cost, freshness).

What survives: two corpus-agnostic mechanisms plus a measurement protocol
that can actually see them.

## Goals / Non-Goals

**Goals:**
- Relevance computed *in relation to the rest of the code*: seed scores
  diffuse over the graph at query time; card tokens weighted by what
  distinguishes a symbol from its structural siblings.
- Valid measurement for all four languages: curated any-of-N concept sets,
  frozen held-out protocol, measurability guard on the mechanical class.
- Iterability without re-embedding: diffusion parameters are query-time.
- Determinism end to end (seeded, reproducible; no wall-clock/randomness).

**Non-Goals:**
- Card-content enrichment (literals, test names, registrations, region
  vocab) — residuals-gated backlog, not this change.
- Structural role taxonomy, references edge kind, customer-framework (L3)
  detection, LSP/SCIP oracle, git-history/coverage evidence tiers.
- Changing the lexical `find` ladder or the ambient prompt note.

## Decisions

### D1: Diffusion = bounded personalized PageRank on the seed-induced subgraph
After lane fusion produces seed scores, build the induced subgraph: seeds
(top ~50 fused) plus nodes within 2 hops via call edges (both directions)
and extends/implements edges, capped (~2,000 nodes, degree-capped per node
to bound hub blowup). Run power iteration (damping α=0.85, ~12 rounds,
restart mass proportional to fused seed scores; edges row-normalized,
direction-symmetric). Final score = fused score blended with diffused mass
(`final = (1-λ)·fused + λ·diffused`, λ default 0.4, tuning-repo-fit, frozen
before held-out). Rationale: PPR is the standard graph-retrieval smoother;
bounding to the induced subgraph keeps it O(seed neighborhood), not O(repo).
Alternative considered: global precomputed PageRank — static importance,
not query-conditioned; rejected (that is what caller-count boosts already
approximate).

### D2: Diffusion replaces neighbor-names-in-cards as the neighborhood carrier
Cards keep name/signature/path/doc; the top-8 caller/callee name lists are
REMOVED from card text (their job — neighborhood semantics — moves to D1,
query-conditioned and radius-free). Effect measured, not assumed: the
curated sets run before/after with cards-with-neighbors vs cards-without +
diffusion; if removal regresses, neighbors stay and D1 is additive.
Side benefit: card churn from neighbor drift (accepted caveat in the parent
change) disappears if removal holds.

### D3: Contrast = family-relative token weighting at card-build time
Family = symbols sharing a parent type, plus symbols in the same directory
module. Compute phrase/token document frequency within family and across
the corpus (two SQL group-bys at build time). Card doc text drops phrases
whose family-df ≥ 0.8 when the family has ≥ 5 members (boilerplate
suppression: "defines an http exception for" vanishes across nest's 30
exceptions); tokens unique to the symbol within its family are emphasized
by inclusion in a `distinct:` card field. Deterministic, corpus-agnostic,
no thresholds tuned per repo (0.8/5 are frozen constants registered here,
before measurement). Text-level weighting (deletion/emphasis) is used
because the embedder consumes plain text — there is no per-token weight
channel.

### D4: Clustering and entry selection consume the diffused subgraph
The feature map clusters over the diffusion subgraph's edges (not just
edges among final results), so a cluster can include the connective tissue
between two hits; entry selection prefers diffused mass over raw caller
count. Two-level "region-first" retrieval thus emerges from D1's machinery
instead of a separate community index (deferred unless residuals demand).

### D5: Curated concept-eval protocol (the gate)
- Question sets: ~25/repo, written from README/feature docs by a human (or
  agent-drafted, human-pruned), each with an any-of-N acceptable-answer set
  (symbol names, qualified where needed). Authored and FROZEN before any
  mechanism lands (bench fixtures, `bench/concept_sets/<repo>.json`).
- Split: tuning = gin, flask, nest, laravel-framework. Held-out = prometheus
  (Go), vscode (TS, large), one PHP repo ≠ laravel (candidate: symfony) —
  indexed and queried exactly once, after parameters freeze.
- Mechanical class keeps running with a measurability guard: a case is
  emitted only if the name-stripped residual keeps ≥2 informative content
  words (non-stopword, len ≥3); discard rate reported per repo. Mechanical
  numbers are reported only where the guard passes ≥50% of candidates.
- Pre-registered bars (before first measurement of the new mechanisms):
  1. Curated hit@5 (any-of-N) ≥ 65% per tuning repo after tuning.
  2. Held-out curated hit@5 ≥ 60% per repo, one shot, no post-hoc tuning.
  3. Falsifiable mechanism predictions: nest mechanical strict ≥ 55%
     (sibling ties broken by D3); gin/flask mechanical ≥ current 61.7%
     (non-regression).
  4. `find` vague/distinctive classes: non-regression.
  5. Latency: search p50 on laravel-scale (~29k symbols) ≤ 2× current;
     diffusion step alone ≤ 150 ms p50.
- Iteration budget: two registered iterations on tuning repos; held-out is
  burned once. A failed held-out bar ships nothing new by default and the
  verdict is recorded (house rule).

### D6: Residuals-gated enrichment backlog
After D1–D5 measurement, misses on curated sets are analyzed and bucketed.
Card-content signals (span literals, test-caller names, registration lines,
region vocabulary) are admitted ONLY against a named residual bucket, each
with its own falsifiable prediction, as follow-up changes. This is the
anti-overfit ratchet: mechanisms first, content only where mechanisms
measurably stop.

## Risks / Trade-offs

- [Diffusion drags in popular hubs (log/util functions) via high-degree
  nodes] → degree cap per node in subgraph construction + row
  normalization; hub damping validated on tuning repos.
- [λ blend fit to tuning repos = quiet overfit channel] → λ frozen before
  held-out; held-out is one-shot; λ recorded in findings.
- [D2 removal of neighbor names could regress vector recall] → explicitly
  measured (D2's paired run); reversible without schema impact.
- [Contrast constants (0.8 family-df, ≥5 members) are editorial] →
  registered here before measurement; changing them costs a registered
  iteration.
- [Curated sets encode the author's framing] → any-of-N answer sets +
  authored from repo docs (not from search output); frozen before runs;
  authorship noted in fixtures.
- [vscode held-out index is hours-scale to embed] → budgeted one-time cost;
  embed-time ceiling remains an open item from the parent change and is NOT
  a bar here.
- [Ordering: parent change unarchived] → archive `semantic-code-search`
  before this change's delta applies; tracked in tasks.

## Migration Plan

No schema change. Card-text changes (D2/D3) shift content hashes → one-time
re-embed on next build via existing model-stamp/content-hash machinery.
Rollback = revert binary; vectors regenerate likewise.

## Open Questions

- Does D2 (dropping neighbor names from cards) hold, or is diffusion only
  additive? Answered by the paired run, not by argument.
- Held-out PHP repo choice (symfony vs drupal) — pick by index build
  tractability when authoring fixtures.
- Whether entry selection should blend diffused mass with the
  registration-signal from the parent change's line-above scan — deferred;
  roles are out of scope here.

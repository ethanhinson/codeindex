# Proposal: diffusion-contrast-retrieval

## Why

The semantic-code-search gate came back mixed, and the failure analysis showed why: relevance today is computed almost entirely from *card content*, and card content inherits whatever assumptions we bake in (doc culture, neighbor radius, boilerplate). Patching cards per observed failure (more literals for laravel, tie-breakers for nest) risks overfitting to the two corpora we happened to measure. Both measured failure modes — identical-doc sibling ties (nest, 16.7pts) and sparse/terse doc semantics (laravel) — are instances of missing *corpus-agnostic mechanisms*, not missing words: relevance should flow along the graph at query time, and card tokens should be weighted by what distinguishes a symbol from its structural siblings. We have the graph; nobody else in this product class uses it this way.

Equally: TS and PHP currently have *no valid quantitative measure* (the mechanical doc-phrase proxy manufactures unanswerable queries on name-restating doc cultures). Shipping new mechanisms without fixing measurement first would leave us tuning blind — so this change makes the measurement protocol a first-class deliverable and the gate for everything else.

## What Changes

- **Measurement first — curated concept evaluation**: hand-authored concept question sets (~25/repo, any-of-N acceptable answers, written and frozen BEFORE mechanisms land) for the tuning repos (gin, flask, nest, laravel-framework) and held-out repos (prometheus, vscode, one non-laravel PHP repo); a measurability guard on the existing mechanical class (emit a case only if the residual query keeps ≥2 informative content words; report discard rate); a frozen one-shot held-out protocol with pre-registered bars.
- **Mechanism A — query-time score diffusion**: after hybrid lane fusion, seed scores propagate over call/dependency edges (bounded personalized-PageRank on the seeds' 2-hop induced subgraph, deterministic), and final ranking uses diffused mass. Replaces the fixed baked-into-vectors neighbor radius as the carrier of "relevance in relation to the rest of the code"; tunable at query time without re-embedding.
- **Mechanism B — contrastive card weighting**: card text is weighted against the symbol's structural family (same parent type, same module) and corpus priors — family-common boilerplate phrases are suppressed, family-distinctive tokens emphasized. Kills identical-doc sibling ties and boilerplate dominance with one corpus-agnostic rule; changes card text → content hashes → existing re-embed machinery handles the rest.
- **Feature-map clustering rides diffusion**: clusters and entry selection consume the diffused subgraph instead of only intra-result edges (two-level "region first" retrieval emerges from the same machinery; no separate community index).
- **Card-content enrichments demoted**: literals/test-names/registrations/region-vocab (previously sketched as stages) are NOT implemented here; they become a residuals-gated backlog — only signals that the post-A+B miss analysis still demands, each with its own falsifiable prediction.
- Explicitly out of scope (separate future changes): structural role taxonomy, references edge kind, L3 registrar/customer-framework detection, LSP/SCIP oracle precision measurement, git-history and coverage-trace evidence tiers.

## Capabilities

### New Capabilities

- `diffusion-ranking`: query-time score propagation over the symbol graph — seed construction from fused lanes, bounded deterministic PPR, diffusion-aware final ranking and feature-map clustering, latency budget, degradation behavior.
- `concept-eval`: the curated evaluation harness — any-of-N question-set format and authoring rules, measurability guard for the mechanical class, tuning/held-out split, frozen one-shot protocol, pre-registered bars and reporting.

### Modified Capabilities

- `semantic-search`: the symbol-card requirement changes — card text becomes contrast-weighted against structural siblings (family-common phrase suppression, distinctive-token emphasis), and ranking/clustering requirements change from "fused + boosted" to "fused + diffused". NOTE: the `semantic-search` spec currently lives as a delta in the unarchived `semantic-code-search` change; archive that change first so this delta applies against main specs.

## Impact

- **Code**: `internal/search` (seeding, PPR, ranking, clustering rework; card contrast weighting feeds `internal/engine` card builder), `internal/graph` (bounded neighborhood-edge queries for the diffusion subgraph; family/document-frequency queries for contrast), `bench/` (curated question sets as fixtures, guard + any-of-N scoring in `recall_bench.py` or a sibling harness).
- **No new runtime dependencies**; no schema change expected (diffusion reads existing edges; contrast is card-build-time). Card text changes force a one-time re-embed on next build (existing model-stamp/content-hash machinery).
- **Ordering dependency**: archive `semantic-code-search` before archiving this change (delta applies on its spec).
- **Perf risk**: diffusion adds per-query graph work — bounded subgraph + registered latency bar (see design).
- **Bench cost**: held-out repos include vscode (large TS) — one-time index+embed runs are hours-scale; budgeted in tasks.

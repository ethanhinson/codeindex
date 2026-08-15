# Tasks: diffusion-contrast-retrieval

## 1. Measurement first (the gate — lands before any mechanism)

- [x] 1.1 Author curated concept sets for the four tuning repos (gin, flask, nest, laravel-framework): ~25 questions each from README/feature docs, any-of-N acceptable answers, provenance + repo pin recorded; freeze as `bench/concept_sets/<repo>.json` *(26/25/26/26 kept; accepts verified by symbol-table lookup only)*
- [x] 1.2 Pick and pin the held-out set (prometheus v3.1.0; vscode 1.96.2; symfony v7.2.2); author + freeze their curated sets WITHOUT running search against them *(26 questions each; answers verified via parse-only nollama indexes, symbol lookups only)*
- [x] 1.3 Harness: any-of-N curated scoring mode, fixture loading with repo-pin verification, held-out run recording (frozen params, binary/model identity, per-question results) *(bench/curated_bench.py)*
- [x] 1.4 Measurability guard in the mechanical class (≥2 informative content words; discard-rate reporting; per-repo validity verdict)
- [x] 1.5 Baseline run: current shipped pipeline against all tuning-repo curated sets (the number A+B must beat); record *(gin 80.8, flask 76.0, nest 34.6, laravel 50.0 — bench/results/curated-baseline-*.json)*

## 2. Mechanism B — contrastive cards

- [x] 2.1 Family and document-frequency queries in `internal/graph` (same-parent + same-module families; phrase/token df per family and corpus)
- [x] 2.2 Card builder: boilerplate suppression (family-df ≥ 0.8, family ≥ 5) + `distinct:` field for family-unique tokens; constants frozen per design D3
- [x] 2.3 Unit tests: sibling-family suppression (exception-family fixture), determinism/content-hash stability, small-family passthrough
- [x] 2.4 Paired D2 measurement on tuning repos: cards with vs without neighbor-name lists (interaction with diffusion measured in 3.5)

## 3. Mechanism A — diffusion

- [x] 3.1 Bounded neighborhood-edge queries in `internal/graph` (2-hop induced subgraph over calls + extends/implements, node and per-node degree caps)
- [x] 3.2 Deterministic PPR in `internal/search`: restart mass from fused seeds, row-normalized symmetric edges, fixed damping/iterations; blend `final = (1-λ)·fused + λ·diffused`
- [x] 3.3 Graceful degradation: cap overflow → capped-subgraph or fused-only, within latency budget, deterministic
- [x] 3.4 Feature-map rework: cluster over diffusion subgraph, entry by diffused mass; keep `--flat`
- [x] 3.5 Unit tests: diffusion on synthetic graphs (neighbor pull-in, hub containment, determinism), exact-name precedence, degradation; paired run resolving D2 (neighbors-in-cards × diffusion)
- [x] 3.6 Latency measurement on laravel-scale index (p50 budget per design D5.5)

## 4. Tuning, freeze, held-out gate

- [x] 4.1 Tune λ and lane/seed sizes on tuning repos ONLY (≤2 registered iterations); record every iteration
- [x] 4.2 Verify falsifiable predictions: nest mechanical strict ≥ 55%; gin/flask mechanical non-regression; `find` classes non-regression
- [x] 4.3 Freeze parameters; build + embed held-out repos (one-time cost, vscode is hours-scale); run held-out curated sets ONCE
- [x] 4.4 Record verdict in `bench/engine/FINDINGS-diffusion-contrast.md` (pass → ship; fail → ship nothing new by default, record); include residual miss analysis bucketed for the enrichment backlog (design D6)

## 5. Ship & docs

- [x] 5.1 Archive `semantic-code-search` (ordering dependency for this change's semantic-search delta)
- [x] 5.2 README + MCP `search` description updates only if behavior-visible changes warrant (routing law unchanged); FINDINGS cross-links
- [x] 5.3 Backlog file for residuals-gated enrichments: each admitted signal names its residual bucket + falsifiable prediction (future changes, not this one)

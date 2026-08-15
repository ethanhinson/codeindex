# Proposal: literal-lane-retrieval

## Why

The issue-corpus lab (v2 ground truth + controls) measured that plain grep
attribution beats semantic search on real bug-symptom queries on both
tested repos (gin 20.5% vs 10.3%; flask 43.3% vs 33.3%): symptom language
lives verbatim in code string literals (error/warning messages, config
keys), which the embedding cards deliberately exclude and the current lanes
never see. The winning control is machinery `codeindex grep` already ships
— it belongs inside `search` as evidence, not outside it as a rival.

## What Changes

- **Third retrieval lane (literal evidence)** inside hybrid search: the
  distinctive content words of the query run through the existing grep
  attribution machinery; attributed symbols join RRF fusion as a ranked
  lane. Always on (30–80ms local compute, zero token cost), with
  **influence self-weighted from its own result statistics**: multi-word
  co-occurrence weighs up, hit dispersion (generic words) attenuates
  (IDF-style), quote-shaped queries boost.
- **Exactness ladder rung 2**: a query phrase found verbatim in file
  content within a symbol's span pins that symbol directly below
  exact-name precedence and above all fused scores — exactness is what
  deterministic matching does perfectly and embeddings never can.
- **MCP `search` gains optional `error_text`** so agents holding a stack
  trace or quoted error pass it explicitly (maximum lane authority), and
  the tool description's routing law adds the symptom-query clause.
- **Gate (pre-registered in the residuals backlog, bucket 4)**: fused
  search must dominate its own grep lane on the v2 issues class (≥21% gin,
  ≥44% flask) with zero regression on the frozen curated tuning sets and
  the `find` classes.
- Miner hygiene rider: `issues_corpus.py` reads `GITHUB_TOKEN` from the
  environment or repo-root `.env` (user is adding a PAT), raising the
  title budget for future corpus expansion; existing fixtures stay frozen.

## Capabilities

### New Capabilities

- `literal-lane`: the lane itself — distinctive-word selection, grep
  attribution as ranked evidence, self-weighting rules, verbatim-phrase
  precedence rung, `error_text` input.

### Modified Capabilities

- `semantic-search`: the Hybrid concept search requirement gains the third
  lane and the exactness ladder. NOTE archive ordering: this delta applies
  after `diffusion-contrast-retrieval` and `runtime-evidence-stack`
  (chain of pending deltas on the same requirement).

## Impact

- `internal/search` (lane + ladder + fusion term; Semantic gains repo root
  in opts), `internal/query` (root plumbing), `internal/mcpserver`
  (`error_text`, description), `bench/selfheal/issues_corpus.py` (.env
  PAT). No schema change; no new dependencies; latency stays inside the
  frozen 2× budget.

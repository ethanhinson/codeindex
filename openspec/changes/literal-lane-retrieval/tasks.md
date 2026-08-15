# Tasks: literal-lane-retrieval

## 1. Lane + ladder

- [x] 1.1 Distinctive-word selection (Go stopword mirror of the bench guard) + lane construction over `search.Grep` (co-occurrence first, grep order second) per design D1/D2
- [x] 1.2 Self-weighting `conf = coocc·disp·shape` (frozen constants, D3) and the litK=20 RRF term in `Semantic` (root plumbed via SemanticOpts)
- [x] 1.3 Verbatim-phrase precedence rung (+900, quoted-or-full-query condition, top-3 cap) per D4
- [x] 1.4 Unit tests: word selection, co-occurrence ordering, dispersion attenuation, phrase pin below exact-name, lane-off-when-no-distinctive-words

## 2. Surfaces

- [x] 2.1 MCP `search` gains `error_text` (quote-shaped authority) + routing-law description update; CLI `--error-text` flag for parity
- [x] 2.2 MCP test: error_text plumbing and description contents

## 3. Gate (bars pre-registered in residuals backlog bucket 4 + design D6)

- [x] 3.1 Issues-v2 run: search ≥ grep-control per repo (≥21% gin / ≥44% flask)
- [x] 3.2 Non-regression: frozen curated tuning sets (88.5/76.0/65.4/76.9) + `find` classes + latency budget
- [x] 3.3 Record verdict + residual movement in `bench/engine/FINDINGS-literal-lane.md`

## 4. Rider

- [x] 4.1 `issues_corpus.py`: read GITHUB_TOKEN from env or repo-root `.env` (never committed; fixtures stay frozen); raise budget when authenticated

# Design: literal-lane-retrieval

## Context

Measured by the issue-corpus lab: grep attribution beats semantic search on
bug-symptom queries (controls, `bench/results/issues-v2-controls-*.json`);
miss anatomy shows 7/55 literal-present misses and 11/55 outranked-at-6-15,
both addressable by promoting literal evidence into fusion. The 23/55
multi-hop misses are explicitly OUT of scope (graph/runtime territory).

## Goals / Non-Goals

**Goals:** third RRF lane from grep attribution, self-weighted; verbatim-
phrase precedence rung; `error_text` MCP input; gate per bucket-4 bars.
**Non-Goals:** literal-aware embedding cards (B-bucket, only 7 misses —
revisit after this lane's residuals); tree-sitter literal-only filtering
(reserve if plain content matching proves noisy); multi-hop mechanisms.

## Decisions (constants FROZEN here, before measurement)

### D1: Distinctive-word selection
Query tokens, lowercased; drop stopwords (Go mirror of the bench guard
list) and tokens < 3 chars; rank by length desc (proxy for
distinctiveness), take top 2. Zero distinctive words → lane inactive.

### D2: Lane construction
For each selected word: `search.Grep(st, root, word, 15)`; keep groups with
a non-nil tier-0 symbol. Lane ranking: symbols hit by BOTH words first
(by combined hit count desc), then single-word symbols in grep order.
Lane RRF term: `conf * 1/(litK + rank)`, litK = 20 (steep — grep order is
already quality-sorted, defs-first).

### D3: Self-weighting (the "when to trust it" answer — always run, weight by evidence)
`conf = coocc * disp * shape`, each factor in [0.3, 1.5]:
- coocc: 1.5 if any co-occurring symbol exists, else 1.0.
- disp: 1/log10(10 + rawHits_total) normalized so ≤30 raw hits → ~1.0 and
  3000 hits → ~0.55 (generic words self-attenuate).
- shape: 1.5 if the query contains a quoted span OR `error_text` was
  supplied; else 1.0.
Deterministic, computed from the lane's own results; no upfront classifier.

### D4: Exactness ladder rung 2 (verbatim phrase pin)
If the query (or `error_text`) contains a quoted phrase — or the full
query, when ≥3 content words — and that phrase occurs verbatim (case-
insensitive) in file content inside a tier-0 symbol's span, that symbol
gets +900 (below the +1000 exact-name pin, above every fused score).
Implemented with one additional Grep call for the exact phrase; capped at
the top 3 phrase-matched symbols to bound pathological queries.

### D5: MCP surface
`error_text` (optional string): treated as a quoted phrase for D4 and
sets shape=1.5 in D3; its distinctive words join D1's selection. Routing
law in the tool description gains: "error messages / symptom text →
include it via error_text". Ambient note untouched (house rule).

### D6: Gate
Pre-registered (residuals backlog bucket 4): issues-v2 hit@5 ≥ grep-control
per repo (≥ 21% gin, ≥ 44% flask); frozen curated tuning sets
non-regression (gin 88.5 / flask 76.0 / nest 65.4 / laravel 76.9); `find`
classes non-regression; latency within the frozen 2× budget. Iteration
budget: two registered iterations on D1–D4 constants; issues-v2 fixtures
are already frozen and are NOT retuned.

## Risks / Trade-offs

- [Concept queries pick up junk literal hits] → disp attenuation + steep
  litK + the curated non-regression bar (the gate exists to catch exactly
  this).
- [Two grep calls + optional phrase grep add latency] → ripgrep at repo
  scale is tens of ms; measured against the frozen budget.
- [Phrase pin on a generic phrase] → quoted-or-≥3-content-words condition
  + top-3 cap.

## Migration Plan

Additive; no schema change; rollback = revert binary.

## Open Questions

- Whether lane evidence should also feed diffusion seeds (currently: yes
  implicitly, since fused score seeds diffusion). Measured by the gate.

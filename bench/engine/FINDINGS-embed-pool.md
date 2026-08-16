# Parallel embed contexts (kubernetes-scale embed budget) — implementation landed, timing sweep blocked

**Date:** 2026-08-16 (early morning) · **Registered bar (parent change):**
≤2 min embed overhead added at kubernetes scale (182k symbols). Current
extrapolation ~30 min at ~10 ms/card — the known lever is parallel llama
contexts; this change builds that lever.

## What landed (opt-in; default behavior unchanged)

- `internal/embed/shim.{h,c}`: `ci_embed_clone` — a new llama_context on
  the already-loaded model (own thread pool, own state; the model is
  freed exactly once, by the owning handle). Context params unchanged
  (2048-token packed batches, ≤32 seqs, mean pooling, ≤8 threads).
- `internal/embed/local.go`: the provider is now a pool of contexts
  behind a checkout channel; `Embed` checks a context out per call.
  Pool size from `CODEINDEX_EMBED_CTX` (default **1** — exactly the
  historical configuration), per-context threads from
  `CODEINDEX_EMBED_THREADS` (default NumCPU, shim-capped at 8).
  `Concurrency()` exposes the pool size.
- `internal/engine/embedpass.go`: the embed loop fans batches out across
  `Concurrency()` workers; a single collector owns the sqlite
  transaction (vectors are hash-keyed — write order is irrelevant).
  With one worker it degenerates to the historical serial loop.

## Correctness — verified tonight (timing-independent)

Three cold gin builds, full vec-table fingerprint
(`SELECT hash, hex(vec) … ORDER BY hash | sha256`):

| build | config | fingerprint |
| --- | --- | --- |
| 1 | ctx=4 | `da4d7354e5b075faf5ca787f` |
| 2 | ctx=4 | `da4d7354e5b075faf5ca787f` |
| 3 | ctx=1 | `da4d7354e5b075faf5ca787f` |

Pooled builds are deterministic AND bit-identical to the serial build:
batches are sliced by index before dispatch, so batch composition — the
only thing ggml wobbles on — never depends on which context encodes it.
Curated gin on a pooled (ctx=4) index: **88.5%**, the frozen baseline
exactly (follows from the identical vectors; confirmed end-to-end).

## Timing — BLOCKED on measurement environment, numbers discarded

Every overnight wall-clock from this laptop is contaminated and none are
recorded as evidence:

- laravel cold full build read "13m48s" (historical awake number: 5m01s)
  with `real` 4.5 h — the machine slept mid-run.
- A gin 1×8 build read 27.8 s (awake baseline 17.8 s), and the CONTROL —
  the pre-pool binary, same conditions — read 33.2 s internal / **439 s
  real**: the environment, not the pool, is the slowdown. macOS
  overnight/lidded operation throttles background work onto E-cores and
  dozes between phases; `caffeinate -dimsu` does not fully compensate.

The trustworthy awake baseline stands at: gin parse 1.1 s / full 17.8 s
(≈14 ms/card, ~4.6 of 8 threads utilized — the single context leaves
most of the machine idle, which is the pool's headroom argument).

## How to finish (the handoff)

`bench/embed_pool_sweep.sh <binary>` — run on an awake, plugged-in,
display-on machine. Its first run (gin 1×8) is a SANITY GATE: within
~25% of 17.8 s internal, or abort. Phase 1 sweeps context×thread shapes
on gin; phase 2 re-baselines laravel 1×8 clean and runs the two phase-1
winners. Then: pick the shipped default for `CODEINDEX_EMBED_CTX`,
extrapolate ms/card × 182k against the 2-minute bar honestly, and record
here. Until that sweep, the default stays 1 context and shipped behavior
is byte-identical to before this change.

# Parallel embed contexts (kubernetes-scale embed budget) — measured; 1.8×; bar NOT met

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

## Timing — measured 2026-08-16 midday (awake, sanity-gated)

The overnight hazard first, because it nearly poisoned the record: every
lidded/idle wall-clock from this laptop was garbage (a 19 s gin build
read 439 s real ON THE PRE-POOL CONTROL BINARY; laravel read "13m48s" vs
5m01s historical — doze + E-core throttling; `caffeinate -dimsu` does
not compensate). All overnight numbers were DISCARDED; the sweep script
carries a sanity gate so this class of contamination can't recur
silently. The midday sweep passed the gate decisively (gin 1×8 8.0 s,
real == internal, bracket rerun 7.9 s).

Phase 1 — gin (1,179 cards), cold full builds:

| config | total | user CPU |
| --- | --- | --- |
| 1×8 (historical) | 8.0 s | 44 s |
| 2×8 | 8.3 s | 86 s |
| 2×4 | 6.9 s | 42 s |
| 4×4 | 5.5 s | 55 s |
| 4×2 | 6.1 s | 40 s |
| **6×2** | **4.8 s** | 43 s |
| 8×1 | 5.5 s | 36 s |

Phase 2 — laravel-framework (28,171 cards; parse ≈16 s of each total):

| config | total | embed | ms/card | user CPU |
| --- | --- | --- | --- | --- |
| 1×8 | 3m15s | ~179 s | 6.4 | 1149 s |
| 6×2 | 2m02s | ~107 s | 3.8 | 1193 s |
| 8×1 | 2m11s | ~116 s | 4.1 | 962 s |
| **10×1** | **1m54s** | **~98 s** | **3.5** | 1008 s |

**The finding:** many single-thread contexts beat one wide context, and
2×8 is SLOWER than 1×8. Cause visible in the user-CPU column: Accelerate
BLAS already multi-threads inside each encode (1×8 burns ~6.4
core-seconds per wall-second), so extra ggml threads collide while extra
contexts fill scheduling gaps. Net: **1.83× embed speedup** — real, but
bounded by BLAS already saturating the machine, not by the pool.

## Verdict vs the registered bar

kubernetes extrapolation at best-measured 3.5 ms/card × 182k ≈ **10.6
min. The ≤2 min bar is NOT met** (needs another ~5×; candidate levers if
ever justified: Metal backend, a smaller/faster quantization for the
bundled model, or coarser cards). The bar's spirit — embed cost must not
dominate adoption — is materially improved: laravel cold build 3m15s →
1m54s on this machine. The 1.8× ships; the bar stays open and honest.

## Shipped default (measured, overridable)

`poolConfig`: contexts = min(8, max(1, NumCPU/2)), 1 thread each when
pooled (this machine: 7×1 — bracketed by measured 6×2 and 8×1, within 4%
of each other); `CODEINDEX_EMBED_CTX=1` reproduces the historical serial
config exactly. Verified under the new default: full suite green, gin
cold build 6.6 s, vec fingerprint `da4d7354…` — STILL bit-identical to
the 1×8 build (int8 quantization absorbs the threads=1 reduction-order
difference), curated gin 88.5 (frozen baseline).

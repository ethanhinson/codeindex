# Full indexing benchmark — consolidated (spec targets vs measured)

**Date:** 2026-07-10 · **Engine:** schema v3, 4 languages, call + dependency
edges, lexical resolution · **Machine:** Apple Silicon arm64, 14 workers
(spec baseline is 8-core x86; absolute numbers directional per spec rule)
**Harness:** `codeindex bench` now measures the full performance-spec surface:
cold build, incremental patch, query p50/p95 *including the lazy re-check*,
index size vs walked source, peak RSS. JSON per repo in `bench/engine/full/`.

## Results

| Repo (tier) | Files | Symbols | LOC | Cold build | Incr (1 file) | Query p50/p95 | Index ratio | Peak RSS | inc==full |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gin (S) | 91 | 1,179 | 18.9k | 170 ms | 5.1 ms | 1.1 / 1.4 ms | 2.50× | 51 MB | ✅ |
| flask (S) | 83 | 1,577 | 17.9k | 118 ms | 4.2 ms | 1.9 / 2.2 ms | 1.78× | 43 MB | ✅ |
| prometheus (M) | 792 | 8,991 | 291k | 1.75 s | 9.0 ms | 6.6 / 8.4 ms | 1.76× | 181 MB | ✅ |
| nest (M) | 1,653 | 4,482 | 96.9k | 511 ms | 23.7 ms | 16.0 / 16.8 ms | 1.94× | 76 MB | ✅ |
| laravel (M) | 2,453 | 28,700 | 435k | 6.66 s | 18.6 ms | 17.5 / 18.4 ms | **3.26×** | 248 MB | ✅ |
| kubernetes (L) | 11,005 | 116,056 | 3.06M | 34.7 s | 113.8 ms | 110.3 / 118.9 ms | 1.98× | 879 MB | ✅ |

## Against every performance-spec target

| Target | Budget (S / M / L) | Worst measured | Verdict |
| --- | --- | --- | --- |
| Cold build | 3 s / 30 s / 5 min | 170 ms / 6.7 s / 34.7 s | ✅ all |
| Incremental (1 file) | 150 / 300 / 750 ms | 5 / 24 / 114 ms | ✅ all, ≥3× headroom |
| **Query p95 incl. re-check** | 75 / 150 / 400 ms | 2.2 / 18.4 / 118.9 ms | ✅ all — first time measured; k8s p95 is ~30% of budget |
| Index ≤2× source (M/L; S reported) | — | k8s 1.98×, prom 1.76×, nest 1.94× | ✅ except **laravel 3.26× — recorded deviation** (PHP symbol density + import edges; fix = string interning, core 2.1/9.4) |
| Peak build memory ≤1 GB (L) | 1 GB | kubernetes **879 MB** (in-process; 837 MB via /usr/bin/time on build alone) | ✅ first time measured |
| inc == full rebuild | required | true ×6 | ✅ |

## Notes

- Query p95 at kubernetes scale (119 ms) is dominated by the lazy re-check
  stat walk, exactly as designed — the graph lookup remains single-digit ms.
- laravel cold build grew 3.8 s → 6.7 s with dependency edges + resolution
  (still 4.5× under budget).
- Peak-RSS measurement initially mis-decoded darwin's `ru_maxrss` (bytes) via
  a bad heuristic; fixed with `runtime.GOOS` and cross-checked against
  `/usr/bin/time -l` (837 MB build-only vs 879 MB build+bench in-process).
- Remaining spec items not covered here: CI regression guard (core 9.6),
  official 8-core x86 baseline run (skeleton 7.4), branch-switch case (9.8).

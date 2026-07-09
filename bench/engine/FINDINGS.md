# Engine walking-skeleton — findings

**Date:** 2026-07-09
**What:** the thin real Go engine slice (tree-sitter parse → name-based symbols +
call edges → SQLite → file-level Merkle + fast-path → build + incremental patch),
built to measure the two things the proxy spikes could not: **parse/patch
throughput** and **incremental-vs-full-rebuild correctness**.
**Machine:** Apple Silicon (arm64), Go 1.26.5, CGO (tree-sitter-go + go-sqlite3).
Reproduce: `codeindex bench <repo> <out.json>`.

## Results

| Repo | Go files | Symbols | LOC | Cold build | Throughput | Incremental (1 file) | inc == full |
| ---- | -------- | ------- | --- | ---------- | ---------- | -------------------- | ----------- |
| gin | 91 | 1,179 | 18.8k | 152 ms | 124k lines/s | 3.9 ms | ✅ |
| prometheus | 565 | 8,530 | 253k | 1.43 s | 177k lines/s | 24.1 ms | ✅ |
| kubernetes | 10,999 | 116,033 | 3.05M | 31.7 s | 96k lines/s | 118.8 ms | ✅ |

(Go-only, `vendor/` excluded — that is why kubernetes shows ~11k files vs the
~26k the token spike saw including vendored deps.)

## Against the `core-indexing-engine` performance targets

| Metric | Target | Measured | Verdict |
| ------ | ------ | -------- | ------- |
| Cold build (small ~50k) | ≤ 3 s | gin 0.15 s | ✅ far under |
| Cold build (medium ~500k) | ≤ 30 s | prometheus 1.4 s @ 253k | ✅ far under |
| Cold build (large ~5M) | ≤ 5 min | kubernetes 31.7 s @ 3.05M | ✅ under |
| Incremental (1 file), large | ≤ 750 ms | kubernetes 119 ms | ✅ under |
| Incremental, medium | ≤ 300 ms | prometheus 24 ms | ✅ under |
| Incremental == full rebuild | required | true on all 3, incl. 116k symbols | ✅ proven |

**All targets are met with comfortable headroom**, single-machine, single binary.

## Findings

1. **Incremental patching is provably correct — at scale.** The graph produced by
   an incremental patch is byte-for-byte identical (normalized, id-independent) to
   a full rebuild, verified on real kubernetes (116k symbols) and in unit tests
   covering body edits, hot-symbol renames, and add+delete. This retires the
   biggest engine-only unknown. The enabling design choice: **deterministic
   name resolution** (candidates ordered by file, then line) so both build paths
   converge, and comparison on **content keys, not row ids**.

2. **Throughput comfortably clears the targets.** ~100–180k lines/s cold build
   with concurrent parsing; the serial SQLite write is the next bottleneck if we
   ever need more (batched inserts / prepared statements would help), but there is
   no need at the current targets.

3. **Incremental latency is dominated by the change-detection walk, as predicted.**
   kubernetes single-file patch is 119 ms — most of it the O(files) stat walk (Go
   stat is much faster than the Python proxy's ~185 ms, and `vendor/` is
   excluded). It already meets the 750 ms budget *without* directory-mtime
   shortcutting, so that optimization is headroom, not a requirement to hit target.

4. **Single static binary works.** The CGO build (tree-sitter-go + go-sqlite3)
   produces one ~7.9 MB executable — the distribution model holds.

## Scope / caveats

- **Go only, calls only, name-based** — this is the skeleton's remit. Edge
  *accuracy* (precise resolution) is `core-indexing-engine` change 2; this proves
  throughput and incremental *consistency with a full rebuild of the same
  extractor*, which is the correctness property that matters for incrementality.
- **Schema deviation:** the skeleton denormalizes the owning file as a path on
  `symbols`/`edges` rather than a `file_id`. The full engine should switch to
  `file_id`; noted so it is not silently carried forward.
- **Numbers are arm64 laptop**, not the spec's 8-core x86 baseline — report is
  directional; re-run on the baseline machine for the official record.

# Findings: string interning (schema v7)

Date: 2026-07-11 · Six pinned repos · Baselines: bench/engine/size-baseline-v6.json

## Headline

Interning repeated strings (one `strs` table; `symbols`/`edges` become views
over int-referenced base tables) cut index size **31–59% in absolute bytes**
on every repo. The kubernetes index is now **smaller than its source**
(0.89×); laravel dropped from 3.28× to **1.51×**, finally meeting the ≤2×
budget that core-indexing-engine task 9.4 required and that we had recorded
as a deviation since the language-adapters change.

## Size (index bytes, bench ratio definition)

| repo | v6 | v7 | Δ bytes | v7 ratio |
|---|---|---|---|---|
| kubernetes | 227.9 MB | 94.4 MB | −59% | **0.89×** |
| laravel | 47.2 MB | 20.6 MB | −56% | **1.51×** (budget ≤2× MET) |
| prometheus | 17.0 MB | 9.0 MB | −47% | 1.01× |
| nest | 6.4 MB | 3.2 MB | −50% | 1.12× |
| gin | 1.5 MB | 1.0 MB | −31% | 1.77× |
| flask | 1.1 MB | 0.7 MB | −38% | 1.29× |

The win is exactly where the design predicted: edge rows carried ~120 bytes
of repeated TEXT (src path, name, qualifier, ns hint, kind, confidence) plus
TEXT b-tree duplication; both collapse to ints.

## Gates

- **incremental == full: PASS ×6.** Resolution semantics byte-identical —
  Snapshot content keys unchanged, whole read surface (34 queries) runs
  verbatim against the views.
- **Query p95 unchanged**: kubernetes 110.7 ms (was 107.8) — SQLite flattens
  the views into indexed int lookups; the pre-registered D4 fallback
  (explicit two-step lookups in resolve()) was NOT needed.
- **Cold build +20–100%**, the cost of intern lookups at insert: kubernetes
  82.5s (was 55.5s, budget 5min), laravel 15.9s, prometheus 7.8s, gin 698ms,
  nest 2.7s, flask 507ms. All within budgets; recorded honestly.
- **Depmaps**: attach/overlay round-trip green — map files share the v7
  format; attach interns SQL-side (INSERT OR IGNORE + join), stale caches
  regenerate via the existing version-mismatch mechanism.

## Verdict

Ship. The recorded size deviation is closed. If cold-build cost ever matters
more than it does today, the next lever is batching intern lookups per file
(one pass over a file's strings before its inserts) — not needed now.

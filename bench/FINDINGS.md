# Token-savings validation — first findings

**Date:** 2026-07-09
**Harness:** `bench/token_bench.py` (pre-implementation spike)
**Token counter:** tiktoken `cl100k_base` (ratios are tokenizer-robust; absolute
counts are approximate for Claude — swap in the Anthropic `count_tokens` API via
`ANTHROPIC_API_KEY` for exact Claude numbers)
**Config:** seed 7, sample 40 (25 for kubernetes), smart-K=5, index ref cap=50

## What we measured

For sampled symbols in each repo, tokens to answer a navigation question two ways:

- **BASELINE (grep + read):** grep for the symbol, then read the matching files
  into context (`naive` = all referencing files; `smart` = top-5 by match count).
- **INDEX (codeindex):** compact `path:line  signature` references.

`rg` grep-by-name is a faithful proxy for the MVP's name-based edges, so this
measures the real token delta of the output contract without the engine built.

## Results (median savings = baseline ÷ index tokens)

| Repo | Lang | Size | `def` | `callers` (naive / smart) | `outline` | median index tokens (def/callers/outline) |
| ---- | ---- | ---- | ----- | ------------------------- | --------- | ------------------------------------------ |
| gin | Go | small | **499×** | **243× / 215×** | 6.4× | 17 / 34 / 1904 |
| prometheus | Go | medium | **313×** | **181× / 137×** | 13.3× | 21 / 114 / 548 |
| nest | TS | medium | 8.8× | 11.9× / 8.5× | 17.5× | 20 / 210 / 8 |
| kubernetes | Go | large (~1.4M LOC) | **215×** | **145× / 101×** | 9.3× | 26 / 144 / 746 |

## Findings

1. **The core claim holds — and then some.** For `def` and `callers` (the two
   most common navigation actions), savings are **100–500×** in large-file Go
   codebases and hold at the very-large (kubernetes) tier. The ≥10× target is
   comfortably met for these query types.

2. **Savings scale with file size / codebase structure — the win is biggest
   exactly where grep+read hurts most.** `nest` (well-factored TS, ~170-token
   median files) drops to ~9–12× because reading the defining file is already
   cheap. `gin`/`prometheus`/`kubernetes` (larger Go files) see 100×+. The value
   proposition is strongest on large files and large repos.

3. **`callers` is the headline query.** Largest absolute baseline cost
   (13k–19k tokens to read all referencing files) collapsing to ~114–144 index
   tokens. This is also the query agents run most while navigating.

4. **`outline` is the weak query — do not market 10× for it.** Median 6–17×,
   frequently below 10×, because a file's full symbol list isn't tiny. It still
   saves tokens, but the ≥10× target should be scoped to `def` / `callers` /
   `dependencies`, not `outline`.

5. **Absolute answers are small and context-friendly.** Median index answers are
   17–210 tokens — trivially fit any context window (validates the ~≤500-token
   target for def/callers). Exception: `outline` of very large files can exceed
   that (gin 1904) → needs `--limit` / pagination.

6. **Structured (JSON) output costs ~1.5–1.7× the text tokens.** e.g. kubernetes
   callers 144 → 202. Practical implication for MCP/IDE consumers: default to
   compact text; when structured output is required, use a compact schema. The
   premium is real but answers still fit easily in context.

7. **Latency reinforces the indexed-storage decision.** A full-repo `rg` scan on
   kubernetes takes ~0.65s per query. Grep-by-name is fine as a *token* proxy,
   but would blow the interactive/IDE latency budget at the large tier — which is
   exactly why the design stores the graph in indexed SQLite rather than scanning
   on each query.

## Practical implications for consumers (Claude & IDEs)

- **Claude (agent) via CLI/MCP:** the token win is real and large for the queries
  agents actually run (callers/def/deps). A skill should steer Claude to query
  the index *before* grep+read. Compact text output is the most token-efficient
  channel.
- **IDEs (VS Code / Cursor / JetBrains) eventually via LSP/MCP:** need low latency
  (sub-100ms feel) and stable structured output — this favors the SQLite index +
  a compact JSON schema, and confirms the query-latency budgets in the
  `performance` spec matter as much as the token budgets.
- **Output caps matter:** broad symbols (hundreds of callers) and `outline` of
  huge files need a `--limit` so answers stay bounded and context-friendly.

## Confirmed with Claude's exact tokenizer

Re-ran gin/prometheus/nest via the Anthropic `count_tokens` endpoint
(`bench/results/quick-claude.json`, sample 20). Ratios hold and the weakest case
improves: gin def 400×/callers 255×, prometheus def 198×/callers 92×, and
**nest (small-file TS) def rose to 14.4× and callers to 10.8×** — both clear ≥10×
now, because Claude's tokenizer scales both sides up (nest def baseline 452 vs
tiktoken's 170). Absolute index answers run ~1.3–1.5× larger than cl100k but stay
tiny (def ~28–37, callers ~42–105 tokens). Conclusion unchanged and slightly
strengthened.

## Caveats / honesty

- Token counts use `cl100k_base`, not Claude's exact tokenizer — ratios are
  robust, absolute numbers approximate. Set `ANTHROPIC_API_KEY` for exact counts.
- Symbol extraction and edges are **name-based regex/grep proxies**, matching the
  MVP resolver. The real engine's precise resolver (change 2) will change edge
  *accuracy*, not materially the token *volume* measured here.
- Baselines model an agent reading whole files; a very disciplined agent reading
  only ranges would narrow the gap — `smart` (top-5 files) is the conservative
  bound and still shows 8–215×.

## Re-index / incremental update findings (`reindex_bench.py`)

The token spike could not touch the re-index path. This second spike measures the
two incremental-update assumptions that are testable without the engine.

### Change-detection walk cost (the lazy per-query re-check)

| Repo | Files | MB | stat-walk (fast-path) | full content hash (est) |
| ---- | ----- | -- | --------------------- | ----------------------- |
| gin | 111 | 0.7 | 1.1 ms | 7.7 ms |
| prometheus | 1358 | 23.6 | 9.3 ms | 61.5 ms |
| nest | 1932 | 4.5 | 12.7 ms | 63.7 ms |
| kubernetes | 25,931 | 230.9 | **185 ms** | **~980 ms** |

### Edge blast-radius (inbound references per defined symbol)

| Repo | median | p90 | max | % hot (>100) | % cold (≤10) |
| ---- | ------ | --- | --- | ------------ | ------------ |
| gin | 2 | 21 | 59 | 0% | 70% |
| prometheus | 5 | 101 | 4005 | 13.3% | 80% |
| nest | 7 | 58 | 151 | 10% | 63.3% |
| kubernetes | 2 | 248 | 2891 | 13.3% | 73.3% |

Commit churn (prometheus, 84 commits): **median 1 code file/commit, p90 6, max 10.**

### Findings

1. **Change detection, not the graph query, is the latency risk at scale.**
   Full content hashing of kubernetes (~1s) would blow the 400 ms large-tier
   query budget on every query. Consequences for the design:
   - The **size+mtime fast path is mandatory**, not optional (stat-walk 185 ms).
   - Even stat-ing 26k files (185 ms) is a large fraction of the budget →
     **directory-mtime shortcutting** (descend only into changed directories) and
     **ignoring vendored/generated trees** (kubernetes's 231 MB is dominated by
     `vendor/`) are required, not nice-to-haves.

2. **Typical edits are cheap; hot symbols ripple.** Median inbound references are
   2–7, so editing a normal symbol re-resolves a handful of edges. But 10–13% of
   symbols are "hot" (>100 refs, up to ~4000). Key distinction the data forces:
   - **Re-parse is always just the changed file** — the claim "incremental work ∝
     changed files" holds for *parsing*.
   - The ripple is edge **re-resolution** (name lookups, cheap per edge), and it
     only needs to run when the changed file's **set of defined symbol names**
     changes. Editing a function body does not ripple; adding/removing/renaming a
     symbol does. So edge re-resolution is ∝ references-to-changed-*names*, and
     must stay index-lookup cheap even for hot symbols (up to a few thousand).

3. **Real change-sets are tiny.** Median 1 file/commit (agents edit one file at a
   time), so the common incremental case is: re-parse 1 file + patch its symbols +
   possibly re-resolve a handful of inbound edges. This is the design's sweet spot.

### Still NOT covered (engine-only)

- **Parse + patch throughput** — re-parsing a changed file with tree-sitter and
  writing SQLite. The proxy has no parser or store. Must be benchmarked once the
  Go engine exists (OpenSpec task 9.2).
- **Correctness of incremental patching** — that an incremental update yields a
  graph identical to a full rebuild. Requires the engine; specced as a scenario
  in `code-indexing` ("Rebuilding over an existing index") and covered by
  integration task 10.1.

## Reproduce

```
python3 bench/token_bench.py --only gin,prometheus,nest --sample 40 --seed 7 \
    --work <clone-dir> --out bench/results/quick.json
python3 bench/token_bench.py --only kubernetes --sample 25 --seed 7 \
    --work <clone-dir> --out bench/results/kubernetes.json
python3 bench/reindex_bench.py --only gin,prometheus,nest,kubernetes --sample 30 \
    --seed 7 --churn prometheus --work <clone-dir> --out bench/results/reindex.json
```

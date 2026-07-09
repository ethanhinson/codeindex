# bench/ — token-savings validation spike

A pre-implementation harness that empirically tests the assumption behind
codeindex: that answering navigation questions from a compact symbol index costs
far fewer tokens than the grep-and-read approach agents use today.

This runs **without the engine**. The MVP resolves edges by symbol *name*, so
`ripgrep` grep-by-name is a faithful proxy for the index's edges, and
language-aware regexes proxy its symbol definitions. We measure the token delta
of the output contract, which is what actually saves tokens.

> Distinct from the real benchmark harness (OpenSpec task group 9), which will
> benchmark the actual Go engine for build/incremental/query *latency*. This
> spike validates the *token* assumption only.

## Requirements

- Python 3.9+
- A real `ripgrep` binary (the harness auto-detects one; the shell's `rg` is
  often a wrapper function, so pass `--rg /path/to/rg` if auto-detect fails)
- `tiktoken` (`pip install tiktoken`) for token counting — optional; falls back
  to a char/4 heuristic. Set `ANTHROPIC_API_KEY` (+ `pip install anthropic`) to
  count with Claude's exact tokenizer.
- Network access to shallow-clone the pinned repos in `repos.json`

## Run

```
python3 token_bench.py --only gin,prometheus,nest --sample 40 --seed 7 \
    --work /tmp/codeindex-bench-repos --out results/quick.json
```

Flags: `--only` (repo names), `--sample` (symbols per repo), `--seed`,
`--limit` (max index refs per query), `--smart-k` (files a "smart" agent reads),
`--rg`, `--work`, `--out`.

## Corpora

Pinned in `repos.json` (fixed commits/tags). `quick: true` repos clone fast;
`kubernetes`/`vscode` validate the very-large tier.

## Output

Per-repo, per-query-type medians: savings ratio (naive & smart baselines),
% of queries meeting ≥10×, and absolute index/JSON/baseline token counts.
Results are written as JSON and printed as a table.

See `FINDINGS.md` for the first run's results and interpretation.

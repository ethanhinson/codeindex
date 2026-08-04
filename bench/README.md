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

## impact_bench.py — blast-radius accuracy

Measures codeindex's impact/blast-radius recall (did it find every real
dependent?) and precision (false positives), per-language and aggregate, with a
separately broken-out `[ambiguous]`-flag subset score.

Uses a **hybrid oracle**: for Go/TypeScript, compiles real repos under renamed
symbols (`go build` / `tsc --noEmit`); compilation failures are ground truth.
For JavaScript/Python/PHP and to validate ambiguous cases, uses authored
fixtures under `bench/impact_fixtures/<lang>/manifest.json`. Ambiguity (same-name
collisions) lives only in authored fixtures.

**Pre-registered bar**: aggregate recall ≥ 0.95, per-language recall ≥ 0.90;
precision is reported but not gated (v1).

### Run

```
# Fixtures only (all languages)
python3 impact_bench.py --binary <codeindex> --lang go,ts,py,js,php

# With a real Go repo (sample 30 symbols, seed 99)
python3 impact_bench.py --binary <codeindex> --repo <clone> --repo-lang go \
    --sample 30 --seed 99
```

Flags: `--binary` (required), `--repo` (real repo clone for CompileOracle),
`--repo-lang` (go|ts), `--sample` (symbols per run), `--seed`, `--lang` (comma-separated),
`--fixtures` (path to `impact_fixtures/`), `--out` (results JSON path).

### Output

- `bench/results/impact.json`: machine-readable per-language and aggregate scores,
  bar pass/fail verdicts, and per-symbol status.
- `bench/impact-FINDINGS.md`: human-readable summary table and pass/fail verdict.

**Behavior note**: a missing toolchain (no `go` / `tsc` in PATH) records that
language as `"not_run": true` — never a silent pass.

### First run findings

Overall blast-radius recall 1.000 on authored fixtures (10 symbols graded, all
languages). The `[ambiguous]` subset revealed a difference: codeindex omits the
flag on same-name collisions for Go/JS/Py/TS (amb recall 0.000) but flags them
correctly for PHP (1.000).

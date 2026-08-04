<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0005 — Blast-radius accuracy benchmark — impact-set recall vs. false positives](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0005-blast-radius-accuracy-benchmark.md)**
<!-- docket:backlink:end -->

# Blast-radius accuracy benchmark — results

Change: #0005 · Branch: feat/blast-radius-accuracy-benchmark · PR: (opened at finish) · Plan: docs/superpowers/plans/2026-08-04-blast-radius-accuracy-benchmark.md · ADRs: 10

## Verify (human)

Automated: `cd bench && python3 test_impact_bench.py` → 20/20 pass (also `PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 python3 -m pytest test_impact_bench.py` → 20 passed; the bare `python3 -m pytest` invocation crashes on an unrelated opentelemetry pytest plugin in this pyenv — use one of the two forms above).

Manual checks worth running at the merge gate (the harness's real value is a real-repo run, which CI does not do):
- [ ] Run the fixtures pass end-to-end against a freshly built binary: `go build -o /tmp/codeindex ./cmd/codeindex && cd bench && python3 impact_bench.py --binary /tmp/codeindex --lang go,ts,py,js,php` — confirm it writes `bench/results/impact.json` + `bench/impact-FINDINGS.md` and prints `aggregate recall=1.000 agg_pass=True`.
- [ ] Run the CompileOracle path against a real Go repo (this is the diagnostic run the benchmark exists for): `python3 impact_bench.py --binary /tmp/codeindex --repo <go-clone> --repo-lang go --sample 30 --seed 99` (the clone must be indexed first via `codeindex build <go-clone>`). Inspect the real-repo recall vs. the pre-registered bar (agg ≥ 0.95, per-lang ≥ 0.90).
- [ ] `tsc` is absent in the dev environment, so a `--repo-lang ts` real run reports the language as "not run" (never a silent pass). Confirm behavior if a TS toolchain is available.

## Findings

- **The `[ambiguous]` flag is not emitted for same-name collisions in 4 of 5 languages.** The corrected ambiguous-subset metric (see ADR-0010) surfaced a real resolver gap on the authored collision fixtures: codeindex resolves the collision callers correctly (overall blast-radius recall 1.000) but does NOT tag them `[ambiguous]` for Go, JS, Python, and TypeScript (ambiguous-recall 0.000), while PHP does flag them (1.000). This is the first hard evidence about how much the `[ambiguous]` flag can be trusted — and the answer, today, is "only in PHP." This is exactly the kind of accuracy signal the benchmark was built to produce; the fixture manifests were NOT tuned to hide it.
- **ADR-0010** records the non-obvious measurement decision behind that finding: the ambiguous-subset is scored against the fixture's AUTHORED `ambiguous:` expectation (`expected = truth if authored_ambiguous else ∅`), not against the tool's own self-reported flags. The naive self-report intersection returns a vacuous 1.000 when the tool flags nothing, masking the gap; this was caught in review and fixed (fix round 1 of Task 6).
- **Plan deviations (all reviewed, none behavioral-risk):**
  - Task 1: the plan's golden test asserted `precision == 0.5` for an empty-truth / one-false-positive case; per the plan's own stated scorer semantics the correct value is `0.0`. Corrected in the test; reviewer confirmed.
  - Task 5: the plan's sampler SQL used a placeholder column `callee_id`; the real codeindex `edges` view exposes `dst_symbol_id` and the `symbols` view exposes `tier`. The implementation uses the real schema (`s.tier = 0`, `id IN (SELECT dst_symbol_id FROM edges)`), matching `recall_bench.py`'s access pattern.
  - Go same-name collisions cannot live in one package; the Go fixture uses two sub-packages (`pkga`/`pkgb`) to construct the collision.

## Follow-ups

- **Resolver accuracy gap (would be its own change):** codeindex omits the `[ambiguous]` flag on same-name collisions for Go/JS/Python/TS. A targeted resolver-accuracy change could add the flag where two definitions share a name. (Auto-capture is disabled in this repo, so no stub was minted; filing this as a `docket-new-change` is a human decision.)
- **Real-repo dynamic-language oracle (spec-noted):** JS/Python/PHP currently rely solely on authored fixtures. A later change could add real-repo runs for the dynamic languages under a test-break oracle once the fixture number is trusted (spec "Follow-up work this enables").
- **Real-repo run numbers:** the committed `impact.json`/`impact-FINDINGS.md` reflect the fixture suite only (10 symbols, hand-authored to be solvable → recall 1.000). The honest large-scale accuracy number comes from a `--repo` CompileOracle run, which is the manual check above.

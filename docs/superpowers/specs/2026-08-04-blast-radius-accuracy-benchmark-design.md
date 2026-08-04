<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0005 — Blast-radius accuracy benchmark — impact-set recall vs. false positives](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0005-blast-radius-accuracy-benchmark.md)**
<!-- docket:backlink:end -->

# Blast-radius accuracy benchmark — design

- **Change:** [#0005](../../changes/active/0005-blast-radius-accuracy-benchmark.md)
- **Status:** design complete (build-ready)
- **Date:** 2026-08-04

## Problem

`bench/` today proves codeindex saves tokens versus grep (`token_bench.py`) and that
`find` recalls the right symbol for vague queries (`recall_bench.py`, pre-registered
`hit@5 >= 70%`). Nothing measures the accuracy of the **impact / blast-radius** surface —
whether "who calls this / what breaks if I change it?" returns the *complete* set of
dependents. Because call resolution is name-based and flags collisions with `[ambiguous]`
(ADR-0007 keeps output references-only), an agent that trusts an incomplete impact set and
ships a change is worse off than one that grepped. This benchmark produces accuracy evidence
to sit beside the existing token-savings evidence.

`recall_bench.py` measures a *different* surface (the `find`/locate command) and is not
touched by this work.

## What it measures

For a symbol **S**, codeindex returns an impact set **I** (its dependents / callers). The
benchmark compares I against a ground-truth set **G**:

- **Recall** = |I ∩ G| / |G| — did codeindex find every real dependent? The safety metric;
  a false negative is what lets an agent ship a regression.
- **Precision** = |I ∩ G| / |I| — noise / false-positive rate.
- Reported **per-language and aggregate**, with a separately broken-out score over the subset
  of results codeindex tagged `[ambiguous]`, to quantify how much that flag can be trusted.

## Ground-truth oracle — hybrid

Two backends behind one interface; a single shared scorer consumes both.

### CompileOracle — Go, TS (real repos)
1. Pick symbol S (see Sampling). Rename S's **declaration** to a nonce (e.g. `Foo` → `Foo_zzq`).
   Rename, not delete, so each reference yields a clean "undefined name" resolution error rather
   than a delete-cascade of unused/undefined noise.
2. Run the toolchain non-emitting: `go build ./...` for Go, `tsc --noEmit` for TS.
3. Every site that now fails to resolve S is a member of G. Parse the compiler diagnostics for
   `file:line` locations, keep only errors attributable to the renamed identifier.
4. Restore the source (or operate on a throwaway checkout) so runs are repeatable.

### FixtureOracle — JS, Python, PHP (authored fixtures)
- Small hand-written repos under `bench/impact_fixtures/<lang>/` with a `manifest.json` declaring,
  per symbol, its true dependent set (list of `(file, enclosing-symbol)` edges).
- G = the authored edges directly; no compiler needed. This is how the dynamic languages — which
  have no static break signal for a deleted/renamed callee — get honest ground truth.
- Go and TS also get a fixture apiece, on which CompileOracle runs as a **validation** step
  (assert the fixture genuinely breaks) — but the compiler is never the oracle for JS/Python/PHP.

## Sampling — and ambiguity handling

- **Real repos (Go/TS):** deterministic seeded sample (following `recall_bench.py --seed` /
  `--sample`), restricted to **uniquely-named** function/method symbols in that repo. Rationale:
  the compiler cannot tell us *which* same-named declaration a broken site referenced, so a
  same-name symbol produces a dirty G. Unique names keep the CompileOracle clean.
- **Fixtures (all five, and the only home for ambiguity):** deliberately author same-name-in-
  different-modules, method/function name collisions, shadowing, and re-exports — cases where we
  *know* the true edges by construction. The `[ambiguous]`-subset accuracy number therefore comes
  entirely from fixtures, where it is trustworthy.

This split is the crux: real-repo scale for the clean case, authored truth for the hard case.

## Scoring & normalization

- Normalize both G and I to a **(file, enclosing-symbol)** identity before set comparison. A
  compile diagnostic reports a call-*site* line while codeindex reports the caller *symbol*, so
  each error line is mapped up to its enclosing function/method to compare like-for-like. Fixture
  manifests are authored already in this identity.
- **Pre-registered bar** (in the `recall_bench.py` tradition, fixed before running):
  **aggregate recall ≥ 0.95** and **per-language recall ≥ 0.90**. Precision is **reported but not
  gated** in v1 — a false positive costs an agent one wasted look; a false negative costs a
  regression. Missing the bar triggers a follow-up change; it does not block anything.

## Components & outputs

- **`bench/impact_bench.py`** — the harness, mirroring the existing `token_bench.py` /
  `recall_bench.py` shape: Python, deterministic, seeded, pre-registered. CLI shape:
  `python3 impact_bench.py --binary <codeindex> [--repo <clone>] [--sample N] [--seed S]
  [--lang go,ts,js,py,php]`.
- **Oracle backends** — `CompileOracle` and `FixtureOracle` behind one interface returning G for a
  given S.
- **Impact runner** — invokes the codeindex impact/callers query for S and parses references into I,
  normalized to (file, enclosing-symbol), preserving the per-result `[ambiguous]` flag.
- **Scorer** — computes recall, precision, and the ambiguous-subset scores; aggregates per-language
  and overall.
- **Outputs** — machine-readable JSON to `bench/results/` and a human-readable `bench/impact-FINDINGS.md`
  (matching the existing `FINDINGS.md` / `efficacy-FINDINGS.md` convention).

## Error handling

- **Missing toolchain** (`go` / `tsc` absent) → that language is recorded as **"not run"**, never
  silently counted as a pass. Aggregate scores state which languages actually ran.
- **Empty compiler ground truth** — a mutated real-repo symbol that yields *no* compile error
  (genuinely unused, or reached only via dynamic dispatch) is **excluded from scoring** with a
  logged reason; it cannot be graded from an empty G. (Fixture symbols with zero authored
  dependents are still valid — they assert codeindex returns an empty impact set.)
- **Rename collision** — restricting real-repo sampling to unique names prevents a nonce rename from
  colliding with another declaration; a residual collision is skipped and logged.

## Testing

- A tiny golden fixture with a hand-verified expected score, asserting the scorer computes the
  correct recall/precision (including an ambiguous case). This makes the benchmark itself
  trustworthy and keeps runs deterministic under a fixed seed.

## Out of scope

- Changing the deterministic resolver — this measures the existing resolver, it does not alter it.
- Fixing any accuracy gaps the benchmark reveals (each becomes its own change).
- Real-repo runs for JS/Python/PHP (no consistent static oracle) — a later change may add them via
  a test-break oracle once the fixture-based number is trusted.
- Semantic / embedding retrieval of any kind.

## Follow-up work this enables

- If the pre-registered bar is missed for a language, a targeted resolver-accuracy change.
- A later change to add real-repo runs for the dynamic languages under a test-break oracle.

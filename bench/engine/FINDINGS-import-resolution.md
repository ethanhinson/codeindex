# Findings: import-aware project resolution (Stage 1 + Stage 2)

Date: 2026-07-11 · Schema v6 · Six pinned repos, same trees as all prior engine
benchmarks. Baselines are the post-lexical/post-depmap ambiguous-call-edge
counts recorded in precision-results.json / FINDINGS-depmaps.

## Headline

Scope preference (Stage 1) plus import binding (Stage 2) removed **12–29% of
ambiguous call edges per repo** — kubernetes −83,685 edges (−23.5%) — with
incremental==full holding on all six repos at both stages, query p95 unchanged,
and build cost within every budget. No type inference, no toolchains: file
location + persisted import sources only.

## Ambiguity metric (ambiguous call edges)

| repo | baseline | Stage 1 | Stage 2 | total Δ | Stage-2 share |
|---|---|---|---|---|---|
| kubernetes | 355,442 | 301,060 | **271,757** | **−23.5%** | −9.7% vs S1 |
| gin | 1,668 | 1,187 | 1,189 | −28.7% | +0.2% (see below) |
| prometheus | 25,647 | 21,089 | 20,621 | −19.6% | −2.2% |
| nest | 2,455 | 2,027 | 1,991 | −18.9% | −1.8% |
| laravel | 64,593 | 60,688 | 56,539 | −12.5% | −6.8% |
| flask | 1,065 | 938 | 938 | −11.9% | 0.0% |

Stage-1 residue justified Stage 2 (design D1): the k8s residue was 301k, and
Go alias binding alone removed another 29.3k. In kubernetes, 135,564 call
edges carry an import hint; 85,607 of them are unambiguous.

**gin's +2 is honesty, not regression.** Binding sits above same-scope, so an
alias call like `binding.Default(...)` in `context.go` — which same-scope had
silently collapsed onto gin's *own* `Default()` (wrong-but-confident) — now
binds into `binding/`, where two build-tagged files (`binding.go` /
`binding_nomsgpack.go`) both define it: genuinely two lexical candidates,
flagged ambiguous. Same pattern for `StringToBytes` (bytesconv go1.19/go1.20
variants). Build tags are invisible to a lexical indexer; the flag is the
correct output.

## Spot-checks

- **k8s staging suffix (spec scenario)**: `wait.PollImmediate` through the
  `k8s.io/apimachinery/pkg/util/wait` alias resolves **unambiguous** to
  `staging/src/k8s.io/apimachinery/pkg/util/wait/poll.go` — the bidirectional
  path-suffix match covers the staging/vendor layout with no go.mod parsing.
- **laravel anchors unchanged**: `BelongsToMany::firstOrCreate` still resolves
  to exactly 1 definition (v5 behavior preserved).
- **PHP use-binding**: `use A\B\Helper` resolves the import edge to A\B's
  Helper, not a same-named class elsewhere (unit-proven; laravel
  extends/implements now 2,846 unambiguous / 583 ambiguous / 408 unresolved).
- **External hints fall through totally**: testify/`assert.Error`,
  stdlib `io.WriteString` — no mapped namespace, resolution identical to
  pre-change (binding-never-worse, unit-proven).

## Gates

- **incremental == full**: PASS ×6 at Stage 1 and ×6 at Stage 2 (including
  a binding-shift patch test: creating the imported module re-binds the call
  on an incremental patch identically to a rebuild).
- **Query p95**: kubernetes 107.8 ms (p50 101.0 ms, incl. freshness
  re-check) — unchanged, within budget.
- **Build cost**: gin 524ms · flask 244ms · nest 1.09s · prometheus 4.7s ·
  laravel 13.8s · kubernetes 55.5s. All within budgets (k8s: 5min). This is
  1.5–3× the pre-scope numbers — the price of per-edge ladder probes and
  scope-grouped re-resolution. Recorded honestly; first ReResolveNames cut
  (correlated-subquery UPDATE per group) blew kubernetes past 10 minutes and
  was replaced with in-memory grouping + batched id updates before gating.
- **Index size**: kubernetes 227.9 MB (2.15× source; dst_ns strings add ~5%).
  Peak RSS 1013 MB — within budget; string interning remains the recorded fix
  for laravel's size deviation.

## Mechanism (what shipped)

- tier-0 symbols carry derived namespaces (Go dir, Py dotted path, TS file
  path, PHP declared `namespace X;` with dir fallback).
- Import edges persist their source in `edges.dst_ns` (TS specifier resolved
  against the importing dir, Py module, PHP use-path stripped to its namespace
  part, Go path verbatim); Go alias-selector calls persist the aliased import
  path as a hint.
- Ladder: qualified-t0 → qualified-t1 → **bound-t0 → bound-t1** → same-scope →
  plain-t0 → plain-t1. Bound candidates match by language-shaped suffix
  alignment (`nsMatch`); every step deterministic; hints fall through totally.
- Re-resolution groups per (name, qualifier, caller-ns, hint) — reproduces
  insert-time results exactly, which is what the equivalence gate checks.

## Verdict

Ship. Stage 2's residue (k8s 271,757) is dominated by calls with no lexical
evidence at all (unqualified cross-package names, interface-dispatch shapes) —
the honest floor for a lexical indexer. Next recorded lever if wanted:
attaching depmaps re-tiers external-hint fallthroughs (testify-style) from
wrong-ambiguous to dep-bound.

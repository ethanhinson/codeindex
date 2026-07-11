# Lexical resolution (precise-resolution) — findings

**Date:** 2026-07-10 · **Change:** `precise-resolution`

## What shipped

Schema v2 (`symbols.parent`, `edges.dst_qualifier`, versioned + auto-rebuild);
parent attribution + lexical call qualifiers in all four adapters
(receiver types, `this`/`self`/`cls`/`$this`, `Foo::`, uppercase-identifier
hints); qualified-first resolution with total fallback; qualified display and
anchors (`Type.method` / `Type::method`) across CLI + MCP.

## Correctness

incremental == full rebuild passes on ALL SIX pinned repos with the new
resolver (gin, prometheus, kubernetes, nest, flask, laravel). New tests:
collision-collapsed-by-qualifier, wrong-hint fallback, equivalence under a
qualified rename, schema-version rebuild.

## Precision metric (ambiguous `calls` edges, before → after)

| Repo | Before | After | Reduction | Upgraded to unambiguous |
| ---- | ------ | ----- | --------- | ----------------------- |
| laravel (PHP) | 76,235 | 64,593 | **−15.3%** | **+11,642** |
| nest (TS) | 2,793 | 2,455 | −12.1% | +338 |
| gin (Go) | 1,742 | 1,668 | −4.2% | +74 |
| flask (Py) | 1,086 | 1,065 | −1.9% | +21 |
| kubernetes (Go) | 361,490 | 355,442 | −1.7% | +6,139 |
| prometheus (Go) | 25,911 | 25,647 | −1.0% | +1,391 |

Language-shaped exactly as designed: `$this`/`this`-heavy languages win most;
Go/Python ambiguity is mostly cross-package/duck-typed — beyond lexical reach
(that residue needs import/type resolution, a different change).

**The user-visible win is the qualified anchor**, which works regardless of
edge statistics: `BelongsToMany::firstOrCreate` → 1 def, 1 caller (bare
`firstOrCreate`: 4 defs, 54 flagged callers).

## Agent A/B v5 (laravel, plugin arm) — GREEN

Pre-registered expectation: median paired cost reduction ≥30%, success delta
≥ −5pp. Measured ($2.38, 32 runs, 8 tasks × 2 arms × 2 reps):

- **median cost reduction 64.1%** (95% CI 56.8–82.8) — 2× the bar
- **win rate 100%** (8/8 tasks), adoption 100%, 0 unparseable, 0 timeouts
- success **B 100% vs A 93.8%** — fourth consecutive run in which the plugin
  arm never produced a wrong answer; the control failed a caller-attribution
  task outright (F1 0.43: hand-attributing PHP callers is genuinely hard)

**The v2/v4 boundary holds off-Go at near-Go magnitude** (64% on PHP vs 73%
Go), now with the precise resolver underneath.

## Notes

- v5 smoke caught a grader bug pre-spend (file regexes were `.go`-only after
  the language expansion) — fixed; both arms had been equally affected.
- Measured-savings citations across surfaces can now say "Go and PHP repos."

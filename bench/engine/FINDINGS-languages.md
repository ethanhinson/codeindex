# Language adapters (TS/JS, Python, PHP) — engine findings

**Date:** 2026-07-10
**Change:** `language-adapters`
**Machine:** Apple Silicon (arm64), same as prior engine findings.

## Per-language results (real pinned repos, `codeindex bench`)

| Repo | Lang | Files | Symbols | LOC | Cold build | Incremental (1 file) | inc == full |
| ---- | ---- | ----- | ------- | --- | ---------- | -------------------- | ----------- |
| nest v10.4.15 | TS | 1,653 | 4,482 | 96.9k | 358 ms | 25.2 ms | ✅ |
| flask 3.1.0 | Python | 83 | 1,577 | 17.9k | 96 ms | 3.6 ms | ✅ |
| laravel/framework v11.38.0 | PHP | 2,453 | 28,700 | 435k | 3.85 s | 20.7 ms | ✅ |

All within the per-tier budgets with headroom; the incremental==full-rebuild
equivalence check (the correctness proof) passes for every language.

## Query spot-checks (task 5.3)

- **TS (nest)**: `callers forRoutes` → exact def (`packages/core/middleware/builder.ts:80`
  with multi-line signature) + 29 callers (the `configure` methods).
- **Python (flask)**: `callers render_template` → exact def
  (`src/flask/templating.py:138`) + 41 callers across examples/docs.
- **PHP (laravel)**: `callers firstOrCreate` → all 4 same-named definitions
  listed, 54 callers correctly flagged `[ambiguous]` (the honest multi-def
  case — name-based semantics identical to Go).

Hook verified live on a Python edit (flask `render_template`: 41 callers,
external files listed, exact re-run command injected).

## Known extraction limits (by design — same contract as Go)

- Anonymous functions/lambdas/callbacks are not symbols (agents anchor on
  named things); their call sites attribute to the innermost *named* enclosing
  symbol or `<top>`.
- Name-based resolution: dynamic dispatch, duck typing, PHP magic methods, and
  TS overloads resolve by final name with `[ambiguous]`/unresolved flags —
  precision work remains the deferred resolution change.
- PHP: standard `<?php` files; TSX grammar used for `.tsx`/`.jsx`.

## Notes

- The walk is now registry-driven: adding a language never touches
  `merkle.Walk` again. `node_modules`/`vendor` exclusion unchanged (and now
  load-bearing for TS repos).
- Agent A/B savings (−73%/−62%) remain Go-measured; consumption surfaces say
  so. Engine-level correctness is what this change proves for new languages.

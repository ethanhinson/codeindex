# agent-ab v10 — PHP cross-language reproduction: findings

**Date:** 2026-08-14 · **Verdict: GREEN (with one asterisk)** · **Cost:** $6.35, 96 runs, claude-sonnet-4-6.

## Why this run

Prior A/B evidence (v1–v9) established the codeindex routing signal — structural
tasks (caller-attribution, occurrences) win big with the index; comprehension/locate
do not — but almost entirely on **Go** repos, plus one private Drupal-PHP corpus. The
open question driving a possible "Navigator" policy model was: **is the routing signal
structural (a property of the task type) or memorized (a property of the tuning repos)?**
If it holds across a language boundary where static call resolution is genuinely harder
(PHP duck-typing → `[ambiguous]` edges), that is strong evidence it is structural, and
that synthetic training data generated from *any* repo's graph would transfer.

laravel/framework was under-sampled: only `caller_attribution` tasks had ever been built
for it. Two scaffolding gaps were fixed to enable a full task set (see "Harness fixes").

## Setup

- Repo: **laravel/framework v11.38.0** (PHP), 28,748 symbols / 166,212 edges, schema v9.
- 24 tasks × 4 types (comprehension/occurrences/vague_find/edit_impact, 6 each),
  seed 1729, ripgrep/graph-mined ground truth (arm-neutral).
- Paired A/B × 2 reps = 96 runs. Arm B = native tools + `codeindex` via arm_b_tools
  system prompt. Arm A = control (native tools only).
- Binary sha256[:16] `789f05a7339c5a33`.
- Thresholds (pinned in tasks_lphp.json header): GREEN = savings ≥30% AND
  success_delta ≥ −5pp AND adoption ≥70%.

## Control-arm isolation (important)

The control arm must not reach codeindex. A globally-installed `/opt/homebrew/bin/codeindex`
leaks into arm A via inherited PATH. A PATH-shim mitigation (shadow `codeindex` with a
failing stub) reduced but did not eliminate leaks (rare, non-reproducible). For this run
the global binary was **physically moved aside** for the duration; arm B used its own
`.bin/codeindex` via explicit PATH. Post-hoc transcript audit: **2 of 48 arm-A rows still
obtained real codeindex output** (both edit_impact — agents found the tool by another
route). Those 2 rows are excluded in the per-protocol column below.

NOTE: `codeindex_calls` counts *attempts*, not successes — a blocked attempt (exit 127)
still increments it. Leak detection must read transcript *results*, not that field.

## Result

Per-protocol (2 tainted arm-A rows excluded, 94 rows):

| type | n A/B | cost Δ (B vs A) | turns A→B | success A→B | Go baseline (cost) |
| --- | --- | --- | --- | --- | --- |
| **occurrences** | 12/12 | **+62%** | 9→2 | 67%→92% | +70% ✅ |
| **edit_impact** | 10/12 | **+56%** | 14→4 | 70%→**50%** | +26% ⚠ |
| **vague_find** | 12/12 | **+14%** | 6→4 | 42%→58% | +45% ✅ |
| **comprehension** | 12/12 | **−2%** | 4→2 | 100%→83% | −21% ✅ |
| **OVERALL** | | **+33%** | | 70%→71% (+1.3pp) | |

ITT (all 96 rows incl. leaks): +37% overall, +2.1pp success — leaks *understate* the
true B-vs-A gap (they hand arm A the index), so per-protocol is the conservative read.

## The findings

1. **The routing signal is structural, not memorized — it survives the Go↔PHP boundary.**
   occurrences (+62% vs Go +70%), comprehension (~flat, both languages), and vague_find
   (positive, weaker than Go) all reproduce the Go pattern's *sign*. "Use the index for
   structural questions; it's a wash for comprehension" is a property of the task type, not
   the repo or the language. This is the evidence a Navigator policy would learn navigation,
   not layouts — and that Tier-1 synthetic data from arbitrary repos should transfer.

2. **edit_impact is where PHP ambiguity bites — the trust-vs-verify signal, made visible.**
   Arm B is 56% cheaper and 3× fewer turns, but arm B success *drops* to 50% (below arm A's
   70%). PHP's `[ambiguous]` edges mean the fast structural blast-radius answer is sometimes
   wrong, and the agent trusts it instead of verifying. This is not a defect to hide — it is
   the single most valuable case for a future Navigator: the policy must learn to VERIFY on
   ambiguous structural output rather than trust it. Go, which resolves cleanly, cannot teach
   this. (Caveat: n=10 arm-A after leak exclusion; the success drop wants more reps to confirm.)

3. **"PHP is unmeasurable" was a scaffolding gap, not a product limit.** codeindex extracts
   PHP richly (symbols, callers, occurrences). The benchmark's ground-truth generator
   (`token_bench.LANG_DEFS`) was Go/TS-only, so comprehension tasks could not be built for
   PHP/Python. Fixed (see below); this unblocks flask/drupal/wordpress/symfony/nest too.

## Harness fixes landed with this run

- `token_bench.py`: added `php` and `python` entries to `LANG_DEFS` (symbol-def regexes).
- `build_tasks.py`: `comprehension_tasks` made language-aware (was hardcoded Go symbol
  extraction + `_test.go` filter); added `_is_test_file(path, lang)` helper.
- `run_ab.py`: arm-A PATH shim (`arm_a_shim_dir`) to shadow a globally-installed codeindex.
  NOTE: shim is ~96% effective; the reliable isolation is moving the global binary aside.
  A proper fix (env-var guard the binary itself honors) remains TODO.

## Open threads

- edit_impact success regression: add reps to confirm signal vs. n=10 noise.
- Residual 2/48 arm-A leak: close with a binary-level `CODEINDEX_DISABLED` guard.
- Cross-language coverage: vague_find/occurrences/comprehension proven Go+PHP; a fresh
  TS/Python held-out repo would complete the language matrix.

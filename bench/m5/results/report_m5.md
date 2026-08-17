# M5 report — go/no-go gates

## Per-suite metrics

| suite | arm | n | success | mean recall | false-conf | median cost | median turns |
|---|---|---:|---:|---:|---:|---:|---:|
| grepwin | A | 1 | 100.0% | n/a | n/a | $0.0281 | 2 |
| grepwin | B | 1 | 100.0% | n/a | n/a | $0.1375 | 2 |
| grepwin | C | 1 | 100.0% | n/a | n/a | $0.0171 | 3 |
| dominate | A | 1 | 100.0% | 100.0% | 0.0% | $0.1439 | 11 |
| dominate | B | 1 | 100.0% | 100.0% | 0.0% | $0.0344 | 2 |
| dominate | C | 1 | 100.0% | 85.7% | 100.0% | $0.0519 | 16 |
| break | A | 1 | 100.0% | 100.0% | 0.0% | $0.0324 | 2 |
| break | B | 1 | 100.0% | 100.0% | 0.0% | $0.0432 | 3 |
| break | C | 1 | 100.0% | 100.0% | 0.0% | $0.0861 | 11 |

## GO gate (B vs A)

- **dominate success: WITHHELD** (n=1 < 10) — B 100.0% vs A 100.0% (delta 0.0pp, min -5pp)
- **dominate savings: WITHHELD** (n=1 < 10) — median cost savings 76.1% / token savings 73.5% (rule: cost>=30% OR processed_tokens>=50%)
- **dominate recall: WITHHELD** (n=1 < 10) — B 100.0% vs A 100.0% (blast-radius recall must not drop)
- **grepwin non-regression: WITHHELD** (n=1 < 10) — median paired cost regression 389.8% (max 10%)
- **break false-confidence: WITHHELD** (n=1 < 10) — B 0.0% vs A 0.0% (delta 0.0pp, max +10pp)

## KILL gate (C vs B, dominate suite)

- **KILL verdict: WITHHELD** (n=1 < 10) — C success 100.0% vs B 100.0%, cost ratio 1.51

## Fuse family (model-scale) — per-suite metrics

| suite | arm | n | success | mean recall | false-conf | median tokens | median turns |
|---|---|---:|---:|---:|---:|---:|---:|
| grepwin | L | 18 | 33.3% | n/a | n/a | 8556 | 1 |
| grepwin | LX | 18 | 77.8% | n/a | n/a | 8683 | 2 |
| grepwin | S | 18 | 61.1% | n/a | n/a | 9928 | 2 |
| grepwin | SX | 18 | 72.2% | n/a | n/a | 10094 | 2 |
| dominate | L | 34 | 38.2% | 35.8% | 63.6% | 41164 | 4 |
| dominate | LX | 34 | 52.9% | 51.2% | 54.5% | 20140 | 3 |
| dominate | S | 34 | 26.5% | 27.6% | 81.8% | 91174 | 10 |
| dominate | SX | 34 | 64.7% | 61.5% | 50.0% | 11491 | 2 |
| break | L | 12 | 75.0% | 75.0% | 25.0% | 20901 | 2 |
| break | LX | 12 | 58.3% | 58.3% | 41.7% | 15443 | 2 |
| break | S | 12 | 50.0% | 43.1% | 58.3% | 20650 | 4 |
| break | SX | 12 | 83.3% | 80.6% | 25.0% | 18404 | 4 |

## COMPOUND gate (SX vs L and SX vs S, dominate suite)

- **compound scale-substitution: PASS** (n=34) — small+index 64.7% vs large+shell 38.2% (delta 26.5pp, min -5pp)
- **compound index-attribution: PASS** (n=34) — small+index 64.7% vs small+shell 26.5% (delta 38.2pp, min +10pp)
- (info) large+index 52.9% vs large+shell 38.2% — within-fuse replication of the claude-family GO direction, no verdict attached.


## Honesty notes

- `dominate_callers` and `dominate_blast` ground truth comes from graph.db (unambiguous / resolved edges) — NOT arm-neutral. A codeindex recall bug would understate arm A on those types. Arm-neutral types (grepwin_*, dominate_tests, break_collision) carry the cross-check.
- False-confidence uses the COVERAGE-line protocol; a missing line counts as a completeness claim (pre-registered).
- Claude-CLI arms (A/B/C) and fuse arms (L/LX/S/SX) are separate harness families — deltas are only meaningful WITHIN a family. The fuse family reports tokens, not $ (the gateway prices nothing; local models cost ~$0).

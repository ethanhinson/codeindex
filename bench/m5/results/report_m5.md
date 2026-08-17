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
| grepwin | L | 1 | 100.0% | n/a | n/a | 8549 | 2 |
| grepwin | LX | 1 | 100.0% | n/a | n/a | 8688 | 2 |
| grepwin | S | 1 | 100.0% | n/a | n/a | 15464 | 3 |
| grepwin | SX | 1 | 100.0% | n/a | n/a | 9988 | 2 |
| dominate | L | 1 | 100.0% | 100.0% | 0.0% | 34272 | 5 |
| dominate | LX | 1 | 100.0% | 100.0% | 0.0% | 55172 | 9 |
| dominate | S | 1 | 0.0% | 14.3% | 100.0% | 212574 | 35 |
| dominate | SX | 1 | 100.0% | 100.0% | 0.0% | 10438 | 2 |
| break | L | 1 | 100.0% | 100.0% | 0.0% | 20901 | 4 |
| break | LX | 1 | 100.0% | 100.0% | 0.0% | 15785 | 3 |
| break | S | 1 | 100.0% | 100.0% | 0.0% | 10761 | 2 |
| break | SX | 1 | 100.0% | 100.0% | 0.0% | 57890 | 10 |

## COMPOUND gate (SX vs L and SX vs S, dominate suite)

- **compound scale-substitution: WITHHELD** (n=1 < 10) — small+index 100.0% vs large+shell 100.0% (delta 0.0pp, min -5pp)
- **compound index-attribution: WITHHELD** (n=1 < 10) — small+index 100.0% vs small+shell 0.0% (delta 100.0pp, min +10pp)
- (info) large+index 100.0% vs large+shell 100.0% — within-fuse replication of the claude-family GO direction, no verdict attached.


## Honesty notes

- `dominate_callers` and `dominate_blast` ground truth comes from graph.db (unambiguous / resolved edges) — NOT arm-neutral. A codeindex recall bug would understate arm A on those types. Arm-neutral types (grepwin_*, dominate_tests, break_collision) carry the cross-check.
- False-confidence uses the COVERAGE-line protocol; a missing line counts as a completeness claim (pre-registered).
- Claude-CLI arms (A/B/C) and fuse arms (L/LX/S/SX) are separate harness families — deltas are only meaningful WITHIN a family. The fuse family reports tokens, not $ (the gateway prices nothing; local models cost ~$0).

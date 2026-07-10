# agent-ab-efficacy — report

**v3 GATE: PASS**

- ✅ locate regression ≤10%: measured -7.4%
- ✅ branch-out savings ≥50%: measured +62.3%
- ✅ hook fire-rate ≥80% on edit tasks: measured 100%
- ✅ hook false fires ≤0: measured 0

**Verdict: YELLOW**  (pre-registered thresholds)

- primary metric: median paired reduction in `total_cost_usd`

## Headline (intent-to-treat)

- paired tasks: 16
- **median cost reduction: 12.5%** (95% CI (-4.4, 46.7))
- median processed-token reduction: 6.4%
- win rate (B cheaper): 56.2%
- success: A 93.8% vs B 100.0%  (delta 6.2 pp)
- codeindex adoption (arm B): 62.5%
- total experiment cost: $4.37   unparseable: 0   timeouts: 0

## Per-protocol (codeindex actually used every arm-B rep)

- paired tasks: 10
- median cost reduction: 44.7% (95% CI (13.3, 77.9))
- ITT vs per-protocol gap: 32.2 pp (discoverability limited — tool helps when used)

## Per-type breakdown

| type | n | median cost Δ | adoption | hook fire-rate |
|------|---|---------------|----------|----------------|
| caller_attribution | 6 | +62.3% | 100% | 0% |
| comprehension | 6 | -7.4% | 0% | 0% |
| edit_impact | 4 | +25.5% | 100% | 100% |

## Thresholds

- **GREEN**: savings >= 30% AND success_delta >= -5pp AND adoption >= 70%
- **YELLOW**: savings 10-30%, OR adoption 40-70% with per-protocol savings >= 30%
- **RED**: savings < 10% (ITT), OR success_delta < -5pp, OR adoption < 40% with per-protocol savings < 30%

## Per-task (cost $, primary)

| task | cost A | cost B | reduction % | succ A | succ B |
|------|--------|--------|-------------|--------|--------|
| callattr-gin-IsDebugging | 0.0698 | 0.0842 | -21% | 1.0 | 1.0 |
| callattr-gin-Written | 0.0531 | 0.0304 | 43% | 1.0 | 1.0 |
| callattr-gin-nameOfFunction | 0.0548 | 0.0292 | 47% | 1.0 | 1.0 |
| callattr-prometheus-Be64 | 0.1917 | 0.0333 | 83% | 1.0 | 1.0 |
| callattr-prometheus-FloatBucketsMatch | 0.1723 | 0.0315 | 82% | 1.0 | 1.0 |
| callattr-prometheus-OpenDBReadOnly | 0.1465 | 0.0324 | 78% | 1.0 | 1.0 |
| comp-gin-Delims | 0.0230 | 0.0334 | -45% | 1.0 | 1.0 |
| comp-gin-File | 0.1036 | 0.0914 | 12% | 1.0 | 1.0 |
| comp-gin-IsType | 0.0321 | 0.0358 | -12% | 1.0 | 1.0 |
| comp-prometheus-ContainsSameLabelset | 0.0329 | 0.0340 | -3% | 1.0 | 1.0 |
| comp-prometheus-CountWarningsAndInfo | 0.0182 | 0.0296 | -62% | 1.0 | 1.0 |
| comp-prometheus-runTest | 0.0440 | 0.0451 | -3% | 1.0 | 1.0 |
| editimp-gin-Written | 0.0759 | 0.0658 | 13% | 1.0 | 1.0 |
| editimp-gin-mappingByPtr | 0.0874 | 0.0913 | -4% | 0.0 | 1.0 |
| editimp-prometheus-ProcessBuilder | 0.1176 | 0.0732 | 38% | 1.0 | 1.0 |
| editimp-prometheus-convertTimeStamp | 0.1448 | 0.0761 | 47% | 1.0 | 1.0 |

## Provenance

- repo pins: {'gin': {'slug': 'gin-gonic/gin', 'commit': 'v1.10.0'}, 'prometheus': {'slug': 'prometheus/prometheus', 'commit': 'v3.1.0'}}
- task seed: 4242   bootstrap seed: 20260709
- model(s): ['claude-sonnet-4-6']   claude: ['2.1.193 (Claude Code)']

# agent-ab-efficacy — report

**v3 GATE: FAIL**

- ❌ locate regression ≤10%: measured -43.9%
- ❌ branch-out savings ≥50%: measured -11.3%
- ✅ hook fire-rate ≥80% on edit tasks: measured 100%
- ✅ hook false fires ≤0: measured 0

**Verdict: RED**  (pre-registered thresholds)

- primary metric: median paired reduction in `total_cost_usd`

## Headline (intent-to-treat)

- paired tasks: 16
- **median cost reduction: -11.3%** (95% CI (-27.9, -3.6))
- median processed-token reduction: -15.7%
- win rate (B cheaper): 18.8%
- success: A 93.8% vs B 100.0%  (delta 6.2 pp)
- codeindex adoption (arm B): 62.5%
- total experiment cost: $5.58   unparseable: 0   timeouts: 0

## Per-protocol (codeindex actually used every arm-B rep)

- paired tasks: 10
- median cost reduction: -4.3% (95% CI (-13.7, 21.2))
- ITT vs per-protocol gap: 7.0 pp (consistent)

## Per-type breakdown

| type | n | median cost Δ | adoption | hook fire-rate |
|------|---|---------------|----------|----------------|
| caller_attribution | 6 | -11.3% | 100% | 0% |
| comprehension | 6 | -43.9% | 0% | 0% |
| edit_impact | 4 | +28.9% | 100% | 100% |

## Thresholds

- **GREEN**: savings >= 30% AND success_delta >= -5pp AND adoption >= 70%
- **YELLOW**: savings 10-30%, OR adoption 40-70% with per-protocol savings >= 30%
- **RED**: savings < 10% (ITT), OR success_delta < -5pp, OR adoption < 40% with per-protocol savings < 30%

## Per-task (cost $, primary)

| task | cost A | cost B | reduction % | succ A | succ B |
|------|--------|--------|-------------|--------|--------|
| callattr-gin-IsDebugging | 0.0698 | 0.0712 | -2% | 1.0 | 1.0 |
| callattr-gin-Written | 0.0531 | 0.0625 | -18% | 1.0 | 1.0 |
| callattr-gin-nameOfFunction | 0.0548 | 0.0596 | -9% | 1.0 | 1.0 |
| callattr-prometheus-Be64 | 0.1917 | 0.2180 | -14% | 1.0 | 1.0 |
| callattr-prometheus-FloatBucketsMatch | 0.1723 | 0.1785 | -4% | 1.0 | 1.0 |
| callattr-prometheus-OpenDBReadOnly | 0.1465 | 0.1875 | -28% | 1.0 | 1.0 |
| comp-gin-Delims | 0.0230 | 0.0338 | -47% | 1.0 | 1.0 |
| comp-gin-File | 0.1036 | 0.1458 | -41% | 1.0 | 1.0 |
| comp-gin-IsType | 0.0321 | 0.0377 | -17% | 1.0 | 1.0 |
| comp-prometheus-ContainsSameLabelset | 0.0329 | 0.0351 | -7% | 1.0 | 1.0 |
| comp-prometheus-CountWarningsAndInfo | 0.0182 | 0.0277 | -52% | 1.0 | 1.0 |
| comp-prometheus-runTest | 0.0440 | 0.0663 | -51% | 1.0 | 1.0 |
| editimp-gin-Written | 0.0759 | 0.0797 | -5% | 1.0 | 1.0 |
| editimp-gin-mappingByPtr | 0.0874 | 0.0757 | 13% | 0.0 | 1.0 |
| editimp-prometheus-ProcessBuilder | 0.1176 | 0.0654 | 44% | 1.0 | 1.0 |
| editimp-prometheus-convertTimeStamp | 0.1448 | 0.0780 | 46% | 1.0 | 1.0 |

## Provenance

- repo pins: {'gin': {'slug': 'gin-gonic/gin', 'commit': 'v1.10.0'}, 'prometheus': {'slug': 'prometheus/prometheus', 'commit': 'v3.1.0'}}
- task seed: 4242   bootstrap seed: 20260709
- model(s): ['claude-sonnet-4-6']   claude: ['2.1.193 (Claude Code)']

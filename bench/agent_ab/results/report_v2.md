# agent-ab-efficacy — report

**Verdict: GREEN**  (pre-registered thresholds)

- primary metric: median paired reduction in `total_cost_usd`

## Headline (intent-to-treat)

- paired tasks: 16
- **median cost reduction: 73.0%** (95% CI (61.6, 81.5))
- median processed-token reduction: 75.4%
- win rate (B cheaper): 93.8%
- success: A 96.9% vs B 100.0%  (delta 3.1 pp)
- codeindex adoption (arm B): 100.0%
- total experiment cost: $8.79   unparseable: 1   timeouts: 0

## Per-protocol (codeindex actually used every arm-B rep)

- paired tasks: 16
- median cost reduction: 73.0% (95% CI (61.6, 81.5))
- ITT vs per-protocol gap: 0.0 pp (consistent)

## Thresholds

- **GREEN**: savings >= 30% AND success_delta >= -5pp AND adoption >= 70%
- **YELLOW**: savings 10-30%, OR adoption 40-70% with per-protocol savings >= 30%
- **RED**: savings < 10% (ITT), OR success_delta < -5pp, OR adoption < 40% with per-protocol savings < 30%

## Per-task (cost $, primary)

| task | cost A | cost B | reduction % | succ A | succ B |
|------|--------|--------|-------------|--------|--------|
| callattr-gin-AbortWithError | 0.0923 | 0.0397 | 57% | 1.0 | 1.0 |
| callattr-gin-AbortWithStatus | 0.1796 | 0.0349 | 81% | 1.0 | 1.0 |
| callattr-gin-IsDebugging | 0.0970 | 0.0291 | 70% | 1.0 | 1.0 |
| callattr-gin-Next | 0.2369 | 0.0438 | 82% | 1.0 | 1.0 |
| callattr-gin-Open | 0.1031 | 0.1047 | -2% | 1.0 | 1.0 |
| callattr-gin-Written | 0.0787 | 0.0304 | 61% | 1.0 | 1.0 |
| callattr-gin-mappingByPtr | 0.0993 | 0.0382 | 62% | 0.5 | 1.0 |
| callattr-gin-writeContentType | 0.1020 | 0.0548 | 46% | 1.0 | 1.0 |
| callattr-prometheus-AddInterval | 0.3371 | 0.0357 | 89% | 1.0 | 1.0 |
| callattr-prometheus-LastCheckpoint | 0.2211 | 0.0341 | 85% | 1.0 | 1.0 |
| callattr-prometheus-LastChunkSnapshot | 0.1596 | 0.0385 | 76% | 1.0 | 1.0 |
| callattr-prometheus-MaxOOOTime | 0.3507 | 0.1009 | 71% | 1.0 | 1.0 |
| callattr-prometheus-OpenBlock | 0.6918 | 0.1063 | 85% | 1.0 | 1.0 |
| callattr-prometheus-WriteFile | 0.3101 | 0.0780 | 75% | 1.0 | 1.0 |
| callattr-prometheus-appendSample | 0.2774 | 0.0978 | 65% | 1.0 | 1.0 |
| callattr-prometheus-nodeName | 0.1606 | 0.0287 | 82% | 1.0 | 1.0 |

## Provenance

- repo pins: {'gin': {'slug': 'gin-gonic/gin', 'commit': 'v1.10.0'}, 'prometheus': {'slug': 'prometheus/prometheus', 'commit': 'v3.1.0'}}
- task seed: 1729   bootstrap seed: 20260709
- model(s): ['claude-sonnet-4-6']   claude: ['2.1.193 (Claude Code)']

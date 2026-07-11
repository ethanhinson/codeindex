# agent-ab-efficacy — report

**v6 GATE: PASS**

- ✅ distinctive regression ≤10%: measured -9.4%
- ✅ vague-find savings ≥30%: measured +38.8%
- ✅ occurrences savings ≥30%: measured +69.2%

**Verdict: GREEN**  (pre-registered thresholds)

- primary metric: median paired reduction in `total_cost_usd`

## Headline (intent-to-treat)

- paired tasks: 20
- **median cost reduction: 41.0%** (95% CI (15.6, 61.6))
- median processed-token reduction: 39.3%
- win rate (B cheaper): 85.0%
- success: A 85.0% vs B 95.0%  (delta 10.0 pp)
- codeindex adoption (arm B): 75.0%
- total experiment cost: $5.21   unparseable: 0   timeouts: 0

## Per-protocol (codeindex actually used every arm-B rep)

- paired tasks: 14
- median cost reduction: 57.0% (95% CI (34.4, 70.7))
- ITT vs per-protocol gap: 16.0 pp (discoverability limited — tool helps when used)

## Per-type breakdown

| type | n | median cost Δ | adoption | hook fire-rate |
|------|---|---------------|----------|----------------|
| comprehension | 6 | -9.4% | 17% | 0% |
| occurrences | 6 | +69.2% | 100% | 0% |
| vague_find | 8 | +38.8% | 100% | 0% |

## Thresholds

- **GREEN**: savings >= 30% AND success_delta >= -5pp AND adoption >= 70%
- **YELLOW**: savings 10-30%, OR adoption 40-70% with per-protocol savings >= 30%
- **RED**: savings < 10% (ITT), OR success_delta < -5pp, OR adoption < 40% with per-protocol savings < 30%

## Per-task (cost $, primary)

| task | cost A | cost B | reduction % | succ A | succ B |
|------|--------|--------|-------------|--------|--------|
| comp-gin-Delims | 0.0330 | 0.0320 | 3% | 1.0 | 1.0 |
| comp-gin-File | 0.0943 | 0.0576 | 39% | 1.0 | 1.0 |
| comp-gin-IsType | 0.0368 | 0.0336 | 9% | 1.0 | 1.0 |
| comp-prometheus-ContainsSameLabelset | 0.0331 | 0.0448 | -35% | 1.0 | 1.0 |
| comp-prometheus-CountWarningsAndInfo | 0.0285 | 0.0366 | -29% | 1.0 | 1.0 |
| comp-prometheus-runTest | 0.0391 | 0.0476 | -22% | 1.0 | 1.0 |
| occur-gin-Abort | 0.1078 | 0.0316 | 71% | 1.0 | 1.0 |
| occur-gin-Open | 0.1147 | 0.0653 | 43% | 1.0 | 1.0 |
| occur-gin-mappingByPtr | 0.1052 | 0.0389 | 63% | 1.0 | 1.0 |
| occur-prometheus-ActiveAlerts | 0.1980 | 0.0313 | 84% | 1.0 | 1.0 |
| occur-prometheus-Decode | 0.3112 | 0.0374 | 88% | 1.0 | 1.0 |
| occur-prometheus-DetectReset | 0.0981 | 0.0317 | 68% | 1.0 | 1.0 |
| vague-gin-DisableConsoleColor | 0.0394 | 0.0287 | 27% | 0.5 | 1.0 |
| vague-gin-SetHTMLTemplate | 0.3075 | 0.0289 | 91% | 0.5 | 1.0 |
| vague-gin-isTrustedProxy | 0.0381 | 0.0283 | 26% | 1.0 | 1.0 |
| vague-gin-setWithProperType | 0.0310 | 0.0278 | 10% | 1.0 | 1.0 |
| vague-prometheus-CustomInfiniteScroll | 0.0767 | 0.0354 | 54% | 0.5 | 1.0 |
| vague-prometheus-PostingsCardinalityStats | 0.0579 | 0.0288 | 50% | 0.5 | 1.0 |
| vague-prometheus-closeAndDrain | 0.0492 | 0.0389 | 21% | 1.0 | 1.0 |
| vague-prometheus-setScrapeFailureLogger | 0.0727 | 0.0290 | 60% | 0.0 | 0.0 |

## Provenance

- repo pins: {'gin': {'slug': 'gin-gonic/gin', 'commit': 'v1.10.0'}, 'prometheus': {'slug': 'prometheus/prometheus', 'commit': 'v3.1.0'}}
- task seed: 6006   bootstrap seed: 20260709
- model(s): ['claude-sonnet-4-6']   claude: ['2.1.193 (Claude Code)']

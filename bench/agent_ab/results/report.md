# agent-ab-efficacy — report

**Verdict: RED**  (pre-registered thresholds)

- primary metric: median paired reduction in `total_cost_usd`

## Headline (intent-to-treat)

- paired tasks: 24
- **median cost reduction: -16.9%** (95% CI (-28.9, -1.6))
- median processed-token reduction: -8.8%
- win rate (B cheaper): 25.0%
- success: A 89.6% vs B 89.6%  (delta 0.0 pp)
- codeindex adoption (arm B): 81.2%
- total experiment cost: $7.89   unparseable: 2   timeouts: 2

## Per-protocol (codeindex actually used every arm-B rep)

- paired tasks: 18
- median cost reduction: -26.3% (95% CI (-38.1, -13.2))
- ITT vs per-protocol gap: -9.4 pp (consistent)

## Thresholds

- **GREEN**: savings >= 30% AND success_delta >= -5pp AND adoption >= 70%
- **YELLOW**: savings 10-30%, OR adoption 40-70% with per-protocol savings >= 30%
- **RED**: savings < 10% (ITT), OR success_delta < -5pp, OR adoption < 40% with per-protocol savings < 30%

## Per-task (cost $, primary)

| task | cost A | cost B | reduction % | succ A | succ B |
|------|--------|--------|-------------|--------|--------|
| comp-gin-Logger | 0.0340 | 0.0599 | -77% | 1.0 | 1.0 |
| comp-gin-PureJSON | 0.0315 | 0.0362 | -15% | 1.0 | 1.0 |
| comp-gin-ShouldBindBodyWithXML | 0.0296 | 0.0401 | -35% | 1.0 | 1.0 |
| comp-gin-Stream | 0.0319 | 0.0393 | -23% | 1.0 | 1.0 |
| comp-gin-With | 0.0512 | 0.0402 | 22% | 1.0 | 1.0 |
| comp-gin-WriteContentType | 0.0622 | 0.0491 | 21% | 1.0 | 1.0 |
| comp-gin-countParams | 0.0300 | 0.0355 | -19% | 1.0 | 1.0 |
| comp-gin-debugPrintWARNINGNew | 0.0273 | 0.0395 | -45% | 1.0 | 1.0 |
| comp-prometheus-EnrichParseError | 0.0290 | 0.0362 | -25% | 1.0 | 1.0 |
| comp-prometheus-GetCreatedTimestamp | 0.0327 | 0.0422 | -29% | 1.0 | 1.0 |
| comp-prometheus-HPoint | 0.0530 | 0.0406 | 23% | 1.0 | 1.0 |
| comp-prometheus-NewTestEngineWithOpts | 0.0317 | 0.0338 | -7% | 1.0 | 1.0 |
| comp-prometheus-OverlapsClosedInterval | 0.0344 | 0.0485 | -41% | 1.0 | 1.0 |
| comp-prometheus-addServiceLabels | 0.0284 | 0.0401 | -41% | 1.0 | 1.0 |
| comp-prometheus-readResponse | 0.0381 | 0.0410 | -8% | 1.0 | 1.0 |
| comp-prometheus-sdCheckResult | 0.0287 | 0.0394 | -37% | 1.0 | 1.0 |
| loc-gin-2088 | 0.0934 | 0.0890 | 5% | 1.0 | 1.0 |
| loc-gin-4460 | 0.0303 | 0.0308 | -1% | 0.0 | 0.0 |
| loc-gin-4622 | 0.0629 | 0.0618 | 2% | 1.0 | 1.0 |
| loc-gin-4638 | 0.0324 | 0.0414 | -28% | 1.0 | 1.0 |
| loc-prometheus-15185 | 0.3189 | 0.2180 | 32% | 0.0 | 0.0 |
| loc-prometheus-18358 | 0.3331 | 0.3707 | -11% | 1.0 | 0.5 |
| loc-prometheus-18534 | 0.2622 | 1.0272 | -292% | 0.5 | 1.0 |
| loc-prometheus-19114 | 0.0277 | 0.0282 | -2% | 1.0 | 1.0 |

## Provenance

- repo pins: {'gin': {'slug': 'gin-gonic/gin', 'commit': 'v1.10.0'}, 'prometheus': {'slug': 'prometheus/prometheus', 'commit': 'v3.1.0'}}
- task seed: 1729   bootstrap seed: 20260709
- model(s): ['claude-sonnet-4-6']   claude: ['2.1.193 (Claude Code)']

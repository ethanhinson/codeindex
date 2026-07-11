# agent-ab-efficacy — report

**Verdict: GREEN**  (pre-registered thresholds)

- primary metric: median paired reduction in `total_cost_usd`

## Headline (intent-to-treat)

- paired tasks: 8
- **median cost reduction: 64.1%** (95% CI (56.8, 82.8))
- median processed-token reduction: 66.8%
- win rate (B cheaper): 100.0%
- success: A 93.8% vs B 100.0%  (delta 6.2 pp)
- codeindex adoption (arm B): 100.0%
- total experiment cost: $2.38   unparseable: 0   timeouts: 0

## Per-protocol (codeindex actually used every arm-B rep)

- paired tasks: 8
- median cost reduction: 64.1% (95% CI (56.8, 82.8))
- ITT vs per-protocol gap: 0.0 pp (consistent)

## Thresholds

- **GREEN**: savings >= 30% AND success_delta >= -5pp AND adoption >= 70%
- **YELLOW**: savings 10-30%, OR adoption 40-70% with per-protocol savings >= 30%
- **RED**: savings < 10% (ITT), OR success_delta < -5pp, OR adoption < 40% with per-protocol savings < 30%

## Per-task (cost $, primary)

| task | cost A | cost B | reduction % | succ A | succ B |
|------|--------|--------|-------------|--------|--------|
| callattr-laravel-framework-fileExists | 0.0746 | 0.0303 | 59% | 1.0 | 1.0 |
| callattr-laravel-framework-flashInput | 0.0585 | 0.0319 | 45% | 1.0 | 1.0 |
| callattr-laravel-framework-formatCommandString | 0.0805 | 0.0317 | 61% | 1.0 | 1.0 |
| callattr-laravel-framework-getContents | 0.0960 | 0.0311 | 68% | 1.0 | 1.0 |
| callattr-laravel-framework-insertGetId | 0.1483 | 0.0255 | 83% | 0.5 | 1.0 |
| callattr-laravel-framework-mediumInteger | 0.0769 | 0.0332 | 57% | 1.0 | 1.0 |
| callattr-laravel-framework-morphMap | 0.2914 | 0.0462 | 84% | 1.0 | 1.0 |
| callattr-laravel-framework-whereIntegerNotInRaw | 0.1038 | 0.0313 | 70% | 1.0 | 1.0 |

## Provenance

- repo pins: {'laravel-framework': {'slug': 'laravel/framework', 'commit': 'v11.38.0'}}
- task seed: 5150   bootstrap seed: 20260709
- model(s): ['claude-sonnet-4-6']   claude: ['2.1.193 (Claude Code)']

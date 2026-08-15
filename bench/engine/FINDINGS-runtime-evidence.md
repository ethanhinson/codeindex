# Runtime evidence — WordPress/Drupal gate findings

**Date:** 2026-07-12 · **Change:** `runtime-evidence-stack` (group 4, design D8)

## Verdict: bars as registered NOT met; evidence VALUE demonstrated on the corpus the premise fits; nothing to withhold (runtime evidence is opt-in by construction)

## Setup (all pre-registered / disclosed)

- Corpora: WordPress 6.7.1 (27,352 symbols) and Drupal core 11.1.0 (55,577
  symbols), indexed at schema v9.
- Fixtures: 28 questions each, hook-tagged (14/15 hook:true), authored from
  platform documentation, symbol-verified, frozen before any measurement
  (`bench/concept_sets/runtime/`).
- Evidence: FULL bootstraps in containers — WordPress really installed via
  the SQLite drop-in and exercised through front-end renders/WP_Query/
  filters/REST (1,083 stacks, 205 files, 791 crossing plugin.php hook
  dispatch); Drupal really installed (standard profile) and exercised with
  289 kernel requests + cron + entity cycles (983 stacks, 510 files,
  ModuleHandler crossings). Ingestion: 4,025 observed edges at 95–100%
  frame resolution on request workloads.
- Static baselines ran with spools quarantined (fresh-on-query would have
  auto-ingested them — caught before contamination).
- DISCLOSED implementation correction, not an iteration: the first
  augmented run was identical to static because design D6's heat-in-boosts
  was implemented only for cluster-entry selection, never for ranking. The
  gate diagnosis caught it; heat now multiplies into the frozen compressed
  boost envelope. Both registered iterations remain UNSPENT.

## Results

| corpus | mode | overall hit@5 | hook subset |
| --- | --- | --- | --- |
| wordpress | static | 89.3% | 85.7% |
| wordpress | runtime-augmented | 89.3% | 85.7% |
| drupal | static | 46.4% | 53.3% |
| drupal | runtime-augmented | **53.6%** (+7.2) | **60.0%** (+6.7) |

Non-regression: gin curated-x2 identical to the decimal (79.2%) with zero
obs rows — corpora without profiles are structurally unaffected.

### Against the registered bars

1. Static-first: ✓ measured.
2. ≥ +15 points and ≥55% absolute: **drupal +7.2 / 53.6% — FAIL** (both,
   the absolute by 1.4 points); **wordpress — unreachable** (static 89.3%).
3. Hook subset ≥ 2× static: drupal 1.13× — FAIL; wordpress unreachable.
4. Non-regression: ✓.
5. SDK overhead ≤3%: NOT MEASURED — gate evidence used Excimer directly;
   applies when the PHP SDK ships (stage 3).

## What the gate actually taught

- **The premise was half right, and the fixture tags found which half.**
  "Static analysis is blind to hook dispatch" is true at the graph-edge
  layer everywhere — but at the RETRIEVAL layer, WordPress's procedural
  naming culture (self-describing top-level callbacks: `wp_publish_post`,
  `redirect_canonical`) lets lexical/semantic lanes find hook callbacks
  without edges. Static hook-subset: 85.7%. The registered WP bars were
  written under a falsified premise — a registration flaw, recorded, not
  patched post-hoc.
- **Drupal is the corpus the premise fits** (generic methods behind
  `invokeAll` on namespaced classes), and there runtime evidence produced
  the largest single-mechanism jump measured on any corpus this week
  (+7.2 overall) — driven by exactly the predicted physics: executed
  implementations now outrank never-executed interface declarations and
  `.api.php` doc stubs, because heat exists only where code ran.
- Residual drupal hook misses are interface/impl near-ties where the
  interface still wins on vector similarity, plus matcher strictness
  (accepts name implementations; tops show `MailManager` type vs
  `MailManager.mail`). Both registered iterations remain available to a
  future pass; not spent here.

## Shipped state

Nothing required withholding: observed-evidence ranking activates only
where profiles exist — explicit, user-supplied evidence, inert otherwise
(the same explicit-invocation principle the literal-lane verdict landed
on). No ambient claims changed; the MCP surface already disclosed
provenance. Bars 2–3 remain unmet as registered, so no promotion language
is added anywhere.

## Next-gate candidates (in evidence order)

1. Spend the two registered iterations on the diagnosed near-ties
   (interface/impl vector ties; heat exponent within the frozen envelope)
   against the same frozen fixtures.
2. Re-register WordPress-class bars on trigger-framed questions (phrasings
   that don't lexically match callback names) — costs a new fixture set,
   honestly harder.
3. The deferred held-out application corpus (pinned OSS app via its test
   suite) — the application-code axis, still unmeasured with runtime
   evidence.

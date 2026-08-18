# CodeIndex Roadmap — Kimi Debate Synthesis

**Date:** 2026-08-16 · **Process:** three Kimi-K2 agents (Advocate, Critic, Strategist)
debated the existing `ROADMAP.md` in parallel against live repo state, wrote to a
shared blackboard, then were reconciled here.

This document is **not** a replacement for `ROADMAP.md`; it is the debate record and
a recommended set of amendments. The frozen thesis, milestones M0–M5, and the R track
are restated only where a debate finding changes them.

---

## 1. The debate in one paragraph

The **Advocate** expanded every milestone into sub-tasks, DoDs, dependencies, and
estimates, and — crucially — audited the code: the edge schema already has
`confidence_id`/`kind_id` columns (`internal/graph/store.go`), the resolver already
emits ambiguous verdicts, `impact` already exists as a CLI verb, R2's literal lane
was already built *and withheld*, and the runtime-evidence machinery (Drupal +7.2 /
WordPress-null) already shipped. So M3/M4/R2 are promotions of live seeds, not
greenfield — which de-risks the plan materially. The Advocate also surfaced **seven
silent gaps** (CI, packaging, organic-task sourcing, provenance-computation mechanism,
MCP ergonomics, trust-model docs, schema-migration). The **Critic** attacked the
empirical foundations: the headline "60% cheaper / 77%→90%" is the *best cell* of the
v10 matrix (one repo, one task type, one model), the comprehension cell *regressed*
(100%→83%) and was never re-run at n=30, the union-arm result was produced with output
ordering tuned against the grader's own scoring regions (a fourth leak class the
discipline rule doesn't cover), M2/M3 estimates hide the real long poles (labeled edge
truth, organic task assembly), and the kill gate only exists at M5 — after the largest
investment is sunk. The **Strategist** agreed the plan's benchmark-first reorder is
right but noted the one exception it makes: the cheapest decisive measurement (the kill
gate) is scheduled *after* the most expensive item (M3). It proposed three alternatives
and recommended **C — risk-sequenced**: run M5's adversarial suites + arm C against the
*current* engine now, pre-register the fork criteria, then either stop or proceed with
M3's scope set by measured failure classes.

---

## 2. Critic findings — severity-ranked, with mitigations

| # | Finding | Sev | Accepted mitigation |
|---|---------|-----|---------------------|
| 1 | **Distribution is missing entirely.** No prebuilt binaries, brew tap, or release CI; install needs Go+C toolchain+cmake+vendored llama. `nollama` silently amputates `search`, the feature the R track tunes. Every M2/M5 win lands on an install base of ~1. | HIGH | Add a **D-track** (below): goreleaser + brew tap + prebuilt binaries, parallel, rides the M4/M5 window. Register the explicit "no telemetry / local-only usage journal" decision. |
| 2 | **Headline A/B numbers are best-cell.** "+60% / 77→90%" is the edit_impact n=30 re-run on laravel/one model; overall table is +33% cost / +1.3pp success; the comprehension cell regressed 100→83% and was *never* re-run at n=30. | HIGH | M2 pre-registers the headline as a per-language × per-task-type matrix, **n≥30 per cell**, and explicitly re-runs the comprehension cell. Rule: no single-cell number in README/roadmap without matrix context. |
| 3 | **Discipline rule blind to grader-co-design.** The union 100%/F1 0.95 result used "section order tuned to grade.py's region rules" — a fourth leak class. Harness also has 3 open calibration defects (stale `gin_tier1.jsonl`, taxonomy clash, leak-audit pairing bug). | HIGH | Add the 4th leak class to the discipline rule ("was any output ordering tuned against the grader's scoring regions?"); require grader-blind formatting in M2; add a grader-audit (cross-grader agreement) to M2 exit; fix leak-audit bug + regenerate `gin_tier1.jsonl` as a hard M0/M1 gate. |
| 4 | **M2/M3 estimates hide the real costs.** M3 exit needs labeled edge truth across 4 languages — the project's own record calls attribution-level truth "unmeasurable" without human labels. M2's "50–100 organic tasks" is the riskiest bullet and gets one clause. | HIGH | Re-baseline: **M2 = 3–4 wks** (gated on corpus frozen + grader audit), **M3 = 8–10 wks** with a week-4 checkpoint (import/module resolution gate green or descope the tail). Add an explicit bench-cost budget line. |
| 5 | **Provenance semantics under-specified.** With name-based resolution, `exact` can only mean "one visible symbol of this name," not "the true target." Coverage field's layer (graph vs retrieval) is unspecified. Confidence has no calibration plan. | MED-HIGH | M1 epistemics page defines confidence classes as **resolver-visibility claims**, not correctness claims. M3 exit requires per-class calibration numbers (not just P/R) and a written policy for which layer a coverage field describes per verb. |
| 6 | **Kill gate only at M5.** Arm C (cheap explorer + shell) runs after M3+M4 investment is sunk; scout already showed a no-agent pipeline matching the agent at F1 0.95 on navigation. | MED | Move an **arm-C smoke probe (n≥30, one repo, two task types)** into M2 with a registered soft-kill threshold. M5 keeps the full battery. *(This is the Strategist's C1, absorbed — see §3.)* |
| 7 | **Framework-hostile corpora only appear at M5.** Tuning repos (gin/flask/nest/laravel) are low-indirection; surprises come from hook-dispatch corpora (WordPress/Drupal). M3 is tuned on friendly corpora and tested on hostile ones at the most expensive moment. | MED | Pull one designed-to-break corpus (DI-heavy TS or PHP app) into M3's labeled edge samples — not to fix it, to price the blindness early. Feeds the coverage-field model. |
| 8 | **R2 omits its own prior failure; R-track bars lack n/CI.** R2's literal lane already failed the conjunction (absolute thresholds); the roadmap presents it as a live item. R1's n≈25 hold-bars have no confidence intervals. | MED | Rewrite R2 to cite the withheld finding and import the absolute→relative threshold lesson. State n and tie-handling per bar. Consider an "accumulating evidence" tier for mechanisms that fail the conjunction but show no named harm. |
| 9 | **Smaller:** M1 "1 wk" underprices the verb-mapping/contract-freeze event; R3's "fragile rank-5 geometry" is an n=25 artifact; R1's 48-file ground check is one corpus's naming culture; runtime-evidence is a ghost track of both M3 and M5 with no explicit scope statement. | LOW-MED | Scope M1 to the ontology+epistemics docs, defer contract freeze to M4. Restate R3 in mechanism terms only. Register "R1 helps the weakest repo" as an acceptable outcome. State explicitly whether runtime-evidence is in-scope or a parallel track like R. |

---

## 3. Strategist alternatives — and the recommended path

| Path | Time to kill/go signal | Time to full M5 (happy) | Main risk |
|------|------------------------|--------------------------|-----------|
| Current M0→M5 | ~9–12 wk (at M5) | ~9–12 wk | kill gate after biggest investment |
| **A — thin slice** | ~3–4 wk | ~10–13 wk (rework) | circular gt on lite bench |
| **B — product-first** (M4 before M3) | ~9–12 wk | ~9–12 wk | unprovenanced packets = false-confidence generator |
| **C — risk-sequenced** (recommended) | ~2–3 wk (C1) | ~10–13 wk | fork criteria must be pre-registered |

### Recommended path: **C, absorbing A's thin slice**

Order the same work items by *information-value × cost-of-being-wrong*, not by artifact
type:

```
M0 ─ M1-thin (2–3d) ─ C1: M5 adversarial suites + arm C, run NOW (≈1 wk)
                          │
                          ├─ pre-register fork criteria
                          ▼
                       C2: decision fork
                          ├─ KILL        → stop, publish, saved ~10 wk
                          ├─ GO-WITH-PAIN → M2-full + M3-scoped-by-C1 + M4-thin (∥)
                          └─ GO-NO-PAIN   → M3 shrinks to coverage-only; freed wks → R1/R2 + M2-full
                          ▼
                       M4-full + M5-full (re-run, adversarial suites already built)
```

**Why C:** every prior win in this repo came from running the cheap falsifying gate
*before* the expensive build. The current roadmap makes exactly one exception — for
the most expensive item (M3) and the most decisive measurement (the kill gate). C1
answers three questions at once: (i) does a cheap explorer + shell already match arm B?
(ii) does provenance matter to agent outcomes at all? (iii) which *specific* resolution
classes fail in ways that change task outcomes — turning M3's priority list from
assertion into measurement.

**The trade-off the team must accept:** C1's verdict rests on partly index-derived
ground truth (the circularity `FINDINGS` itself flags), so the go/kill read carries a
stated confidence caveat, and **"go" means "go build the organic-gt benchmark," not "go
declare victory."** If the team cannot pre-register fork criteria it will actually
honor, C degrades into motivated reasoning and the current fixed ordering is safer.

**Absorbed from B:** let packet schemas (written during M2) bound M3's scope — define
the interface first, implement provenance only for the edges packets expose.

---

## 4. Reconciled milestone amendments

The Advocate's full per-milestone expansion (sub-tasks, DoDs, dependencies, estimates)
is adopted as the working spec, with these amendments from the debate:

### M0 — Housekeeping (~1 day, expanded)
- Add: minimal **CI workflow** (`.github/workflows/`): build with and without `nollama`,
  run `go test ./...` + golden suite, golden-test gate. ~1 day. Does not wait for M5.
- Make `gin_tier1.jsonl` regeneration (or known-stale marking) a **hard gate**, not a
  housekeeping bullet (Critic #3).
- Fix the leak-audit pairing bug before any M2 run trusts its counts.

### M1 — Freeze the ontology (2–3 days thin, contract freeze deferred to M4)
- **Thin version for path C:** ship `docs/ontology.md` + the epistemics page in 2–3 days;
  the full verb→ontology mapping + `--json` contract freeze trails into M4 where the
  packet schemas live.
- **Critical addition (Critic #5):** the epistemics page defines confidence classes as
  **resolver-visibility claims** (`exact` = "exactly one visible symbol of this name,"
  not "the true target"). This is the schema contract M3 implements against; getting the
  enum wrong costs a schema migration.
- State explicitly whether the **runtime-evidence stack** (shipped, schema v9, ingest
  verb, SDKs) is in-scope of M1–M5 or a parallel track like R. Right now it's a ghost
  dependency of both M3 (provenance format) and M5 (adversarial suite).

### M2 — Benchmark v1 (re-baselined to 3–4 weeks)
- **Long pole is named and owned:** assemble 50–100 organic tasks by extending the
  existing `issues_corpus.py` miner (already reads `GITHUB_TOKEN`), ~15–25 per language,
  with an explicit owner and a per-language quota (Advocate gap #3).
- **Pre-register the headline** as a per-language × per-task-type matrix, n≥30 per cell;
  explicitly re-run the comprehension-success cell at n=30 (Critic #2).
- **Add the 4th leak class** to the discipline rule and a grader-audit (cross-grader
  agreement) to the exit (Critic #3).
- **Absorb an arm-C smoke probe** (n≥30, one repo, two task types) with a registered
  soft-kill threshold (Critic #6 / Strategist C1).
- **Add a bench-cost budget line** (n runs × arms × reps × frontier-model pricing).

### M3 — Resolution correctness & provenance (re-baselined to 8–10 weeks)
- **De-risked by existing seed:** the `confidence_id`/`kind_id` columns and ambiguous
  resolver verdicts already exist — sub-task 1 (generalize the schema, thread mechanism
  tags from adapter through resolution, replace deterministic-first with recorded
  candidate-count) is plumbing, ~1 week. Items 4–7 (interfaces/inheritance,
  cross-package, disambiguation, tests-for-symbol) are real algorithm work.
- **Week-4 checkpoint:** import/module resolution gate green, or descope the tail of the
  priority list (Critic #4).
- **Pull one designed-to-break corpus** (DI-heavy TS/PHP) into the labeled edge samples
  to price blindness early (Critic #7). Feeds the coverage-field model.
- **M3 exit adds:** per-class calibration numbers (not just P/R); a written policy for
  which layer (graph vs retrieval) a coverage field describes per verb (Critic #5).
- **Schema-migration story:** export/import artifacts carry the provenance schema
  version; upgrades republish artifacts (else every M3 upgrade is a cold-build regression
  for teams using the 82.5s→1.5s artifact-import) (Advocate gap #7).

### M4 — Evidence packets: nav / impact / explore (3 weeks, starts at M3 schema-freeze)
- **Dependency honesty:** M4 starts at M3's schema-freeze sub-milestone, not at M3
  start — the "overlaps M3" framing was load-bearing and false for the provenance-bearing
  half of M4 (Critic #4). Schemas can be drafted during M3.1 against stubbed coverage.
- **Promote, don't build:** `impact` already exists as a CLI verb; `recipes.py` is
  already P=R=F1 1.00 at file level. M4 is productization (typed schemas, transitive
  blast radius, budget projections) + the "explicit-over-automatic" MCP-ergonomics rule
  from the `error_text` precedent (Advocate gap #5).
- **Fold the trust-model doc** into M3/M4's exit (Advocate gap #6).

### M5 — Benchmark v2: the go/no-go (2 weeks, adversarial suites pre-built by C1)
- On path C, the adversarial suites and arm-C wiring are already built (C1), so M5 is a
  *re-run* — strengthening the before/after claim.
- **Gates unchanged but sharpened:** GO = no success regression **at n≥30 per cell per
  language** AND (≥50% fewer exploration tokens OR ≥40% fewer reads/calls) AND improved
  affected-file/test recall AND manageable false-confidence. KILL = arm C matches arm B.
- Feed runtime-evidence into designed-to-break: the Drupal +7.2 / WordPress-null pattern
  is the template — runtime evidence continues ONLY where static shows a measured miss
  that matters.

### D-track — Distribution (NEW, parallel, 3–4 days, rides M4/M5 window)
- goreleaser + Homebrew tap + prebuilt binaries for both variants (llama / nollama).
- A documented upgrade/schema-migration policy.
- Register the explicit "no telemetry / local-only usage journal" decision (Critic #8c).
- Does not block the science; blocks the adoption the science is FOR.

### R track (parallel, small, evidence-gated) — status-corrected
- **R1 — Definition-by-usage card enrichment (1–2 wks):** strongest open retrieval item;
  ground truth confirmed (48 nest `bootstrap()` wrappers). Gate: named frozen misses flip;
  nest curated ≥75; gin 88.5 / flask 76.0 / laravel 76.9 hold; mechanical+find hold.
  Attacks recall@40 = 84.6%, which re-ranking provably cannot. Register "helps the
  weakest repo" as an acceptable outcome, not an embarrassment (Critic #9).
- **R2 — Literal lane (1 wk to REDESIGN, not build):** already built and WITHHELD.
  Remaining work is a relative/normalized re-design (IDF-style, mirroring contrast's
  family-df RATIO lesson), then re-gate against unchanged bars. Rewrite to cite the prior
  failure and import the absolute→relative threshold constraint (Critic #8).
- **R3 — Query-conditioned vote weighting (conditional, 3–5 days):** enter ONLY if R1
  leaves a named residual bucket; restated in mechanism terms, not "fragile rank-5
  geometry" (which is an n=25 artifact) (Critic #9).
- **Add:** state n and tie-handling per R-track bar; consider an "accumulating evidence"
  tier for mechanisms that fail the conjunction but show no named harm (Critic #8c).

### Paused / closed — confirmed with one note
- Re-ranking, language expansion, cold-embed speed, generative models: all correctly
  paused. **"Elaborate UI"** is more adoption-coupled than the roadmap admits (see
  D-track) but defensible as sequencing.

---

## 5. Sequencing at a glance (reconciled — path C)

```
M0 (1d +CI) ─ M1-thin (2–3d) ─ C1: adversarial suites + arm C, run NOW (≈1 wk)
                               │  pre-register fork criteria
                               ▼
                            C2: fork  ├─ KILL → stop
                                      ├─ GO-WITH-PAIN → M2-full (3–4wk) + M3-scoped-by-C1 (8–10wk) + M4-thin (∥)
                                      └─ GO-NO-PAIN   → M3 coverage-only; freed wks → R1/R2 + M2-full
                               │
                               ▼
                          M4-full (3wk) + M5-full (2wk, re-run) + D-track (3–4d ∥, M4/M5 window)
                          R1 (1–2wk ∥), R2 (1wk ∥) — pull forward on the GO-NO-PAIN branch
```

The project is judged by one sentence it must earn at M5:

> We eliminated a large portion of LLM repository exploration while maintaining or
> improving coding-task success and blast-radius coverage.

Path C buys the cheapest possible read on whether that sentence is earnable *before*
the core spend — the one exception the current plan makes to its own falsify-first
discipline, removed.

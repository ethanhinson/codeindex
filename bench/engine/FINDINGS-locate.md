# Locate: find + enriched grep — findings (phase 1)

**Date:** 2026-07-10 · **Change:** `locate-and-enriched-grep`

## Shipped

- `codeindex find`: convention tokenizer (camel/snake/acronym/digit
  boundaries, one splitter for all four languages), ~50-group static
  synonym/stem table, deterministic match ladder (exact > token-set > prefix >
  all-tokens > subsequence; synonyms at 0.8) × graph boosts (log callers, tier,
  kind, test penalty). In-memory scan: kubernetes 182k symbols score inside a
  ~1.6 s end-to-end CLI call (incl. fresh-on-query patch + process start).
- `codeindex grep`: ripgrep (or internal fallback) + per-hit enclosing-symbol
  attribution (span binary search), def-line marking, dedup-by-symbol with
  counts, defs-first ranking, `N raw hits → M symbols` compression line.
- Routing shipped in the prompt note + two MCP tools (find/grep) whose
  descriptions carry it: distinctive full name → plain grep; partial/vague →
  find; occurrence understanding → codeindex grep.

## Offline recall (pre-registered bar: vague-class hit@5 ≥ 70%)

| Repo | casefold | token-join | reorder | synonym | token-drop | VAGUE hit@5 | Verdict |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gin | 100% | 100% | 100% | 90.9% | 90.0% | **94.4%** | PASS |
| kubernetes | 100% | 97.5% | 97.5% | 85.7% | 67.5% | **83.2%** | PASS |
| laravel | 100% | 100% | 100% | 100% | 57.5% | **81.5%** | PASS |

**The embeddings trigger does NOT fire.** Weakest class is token-drop (a
2-token name losing one token leaves a generic survivor among thousands);
caller-count ranking recovers most. Deterministic, seeded, reproducible
(`bench/recall_bench.py`, results in `bench/engine/recall-*.json`).

## Live spot-checks

- k8s `find "config load"` (reversed, vague) → `LoadConfig` first, then
  usage-ranked candidates (`ToRawKubeConfigLoader`, callers=69).
- k8s `find "fetch pod"` (synonym) → `getPod` (callers=73) first.
- gin `grep ServeHTTP` → 17 raw hits → 6 attributed symbols, definition first.

## Pending

Agent A/B v6 gate (distinctive ≤10% regression / vague ≥30% / occurrences
≥30%) — phase 2 of this change.

## Agent A/B v6 — gate verdict (2026-07-10)

**v6 GATE: FAIL on the distinctive class; PASS on both new classes.**
($5.4 + $0.9 iteration, 92 runs total, 0 timeouts.)

| Class | Threshold | Measured | |
| --- | --- | --- | --- |
| vague_find | savings ≥30% | **+35.2%** | ✅ |
| occurrences | savings ≥30% | **+70.5%** | ✅ |
| distinctive (comprehension) | regression ≤10% | **−28.5%**, −25.5% after the registered wording iteration | ❌❌ |

Supporting: overall mixed-set median +32.4%, success **B 95% vs A 85%**
(+10 pp — the accuracy edge extends to locate tasks), adoption 100% on the new
classes, 0 unparseable.

### Interpretation and shipped configuration

The tools win their domains decisively. The failure is the **ambient
trigger**: sanctioning `find` in the always-visible note over-applies it to
distinctive names plain grep answers in one call (v1's lesson, re-confirmed
twice — adoption on distinctive tasks fell only to 58% after the sharpened
wording). The iteration budget is spent, so per pre-registration:

- **SHIPPED**: `codeindex find` + `codeindex grep` (CLI + MCP tools, whose
  descriptions carry the exact-name caveat), the recall-benchmarked matcher,
  README documentation.
- **WITHHELD**: locate routing in the always-visible prompt note — reverted to
  the v4-gated text ("locate → plain grep"). Agents reach find/grep via MCP
  descriptions or when instructed, not ambiently.
- Any future note change proposing locate routing requires its own gate.

The meta-lesson now measured three times (v1, v3, v6): the always-visible
note is a powerful and blunt instrument — whatever it sanctions WILL be
over-applied. Tool quality and trigger discipline are separate products.

## v7 — mechanical iteration: GATE PASS (2026-07-10)

Diagnosis (v6b transcripts): the regression wasn't only find over-use — the
ambient note primed Bash-CLI workflows over the native Grep tool, and the
def+files task shape forced multiple probes (no single call answered both
halves). Two mechanical fixes:

1. **One-call completeness**: `callers` output ends with
   `referenced in N file(s): …` — over-application became cheap, not costly.
2. **Escalation-only routing**: "grep FIRST; ONLY IF grep didn't find it,
   codeindex find/grep" — a checkable pre-condition, not a judgment call.

Result ($2.6, 40 fresh arm-B runs, v6 arm-A shared controls):

| Class | Threshold | v6 | v6b (wording) | **v7 (mechanical)** |
| --- | --- | --- | --- | --- |
| distinctive | regression ≤10% | −28.5% ❌ | −25.5% ❌ | **−9.4% ✅** |
| vague_find | savings ≥30% | +35.2% | +35.2% | **+38.8% ✅** |
| occurrences | savings ≥30% | +70.5% | +70.5% | **+69.2% ✅** |

Distinctive-class codeindex adoption across note versions: 100% → 58% →
**17%** — the procedural condition finally disciplined the trigger, and the
one-call output made the residual usage nearly free.

**Shipped configuration**: find/grep tools + escalation-only note routing
(now gate-passed). The withheld-routing note from v6 is superseded.

Meta-lesson refined: wording iterations move ambient-trigger behavior weakly
(100→58%); PROCEDURAL conditions + making misuse cheap move it decisively
(→17% and inside the bound). Design the tool so misuse doesn't cost, then
gate the trigger on observable conditions.

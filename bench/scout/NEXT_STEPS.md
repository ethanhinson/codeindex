# Scout — handoff for the next agent

**Read this + `FINDINGS.md` before touching anything in `bench/scout/`.**
Last updated 2026-08-15.

## What Scout is (one paragraph)

A cheap "navigator" that sits in front of an expensive coding agent and answers
code-navigation questions (who calls X, where is X, what references X) using the
`codeindex` call-graph index — so the expensive model is only invoked for actual
reasoning. The original idea was to distill a small local LLM (Qwen) for this.
**The evidence says a model is NOT needed for navigation** (see below).

## The bottom line (what's proven)

The navigation pipeline decomposes into three stages, and all three are solved
cheaply and locally, no GPU, no model:

    task ──▶ [ROUTE]  ──▶ [RETRIEVE]  ──▶ [NORMALIZE] ──▶ answer
             classifier    codeindex      deterministic
             90%, ~1ms      complete       formatters, ~100%

- **Route** (`clf_baseline.py`): embedding (local bge-base/MPS) + logistic
  regression classifies task→tool {callers, grep, find}. **90% accept-set** on a
  held-out-TEMPLATE split — within 3pt of a local Qwen-7B (93%), 32pt above a
  keyword rule (58%). Trains in seconds.
- **Retrieve**: `codeindex` CLI. The answer data is ALWAYS present in its output.
- **Normalize** (`formatters.py`): regex parsers turn raw CLI output into the
  answer shape the grader wants. ~100% on tested types.
- **End-to-end** (`arm_c.py`): classifier + index + formatter, NO agent, matches
  the full agent (arm B) on gin+prometheus (n=20): F1 0.95 / 100% vs ~0.95 / 96%.
  **Honest number is ~90%** (routing 90% × formatting ~100%), not 100% — see
  caveat 1.

**Conclusion: do not distill a model for navigation routing.** A classifier +
index + formatters delivers the value FINDINGS_v10 proved, at ~zero cost.

## The files (all in bench/scout/)

| file | what it does | trust level |
| --- | --- | --- |
| `gen_tier1.py` | generates (task, gold-tool, answer) triples from an indexed repo; `--paraphrase` via local LLM | works; single-hop only |
| `local_llm.py` | LM Studio (:1234) OpenAI-compat client | works |
| `rule_baseline.py` | zero-ML keyword router (the floor: 58%) | works |
| `measure_ceiling.py` | local-model routing ceiling on calibrated tasks (93%) | works |
| `clf_baseline.py` | embedding + logreg router (90%) — **the router** | works; gin-only |
| `formatters.py` | raw codeindex output → grader shape | works; regex, brittle |
| `arm_c.py` | end-to-end: classifier+index+formatter vs the agent | works; forces tool by type (caveat 1) |

Run the venv-dependent ones with `bench/scout/.venv/bin/python` (isolated env
with sentence-transformers + sklearn + torch/MPS). `.venv/`, `*.jsonl`,
`judge_sample*.json` are gitignored — regenerate with `gen_tier1.py`.

Prereqs: LM Studio running on :1234 (has Qwen2.5-Coder-7B loaded); a `codeindex`
binary on PATH; indexed repos under `bench/repos/`.

## HONEST caveats (do not skip — several "100%" here were artifacts)

1. **arm_c.py forces the type-correct tool** for caller/comprehension, so its
   100% is a FORMATTER test, not a router test. The classifier's own route
   disagreed 40% of the time (`route%` column). Fair end-to-end ≈ 90%. To make it
   truly end-to-end, drive the retrieval tool from the classifier's prediction,
   not the harness type — and expect failures where routing is wrong.
2. **Everything is gin-heavy.** Router trained on gin only; formatters tuned to
   gin/prometheus output shapes and Go/PHP file extensions. Untested on TS/Python
   output, monorepo paths, or unusual symbol names. Formatters are regex — they
   WILL break on format drift.
3. **Taxonomy clash:** the harness calls a task type "occurrences" but means
   caller-attribution (prompt says "functions that CALL X", gt is `caller_pairs`).
   The generator's "occurrences" means literal token refs (grep). Reconcile the
   vocabulary before reusing either across the boundary.
4. **A pattern to internalize:** FOUR times this session a "100% / all-green"
   result was a measurement artifact (tautological validator, substring cost
   metric, leaked control arm, template-memorization split). When a number looks
   too clean, find the leak BEFORE reporting it. The habit that caught them:
   ask "what would make this pass even if the thing under test were broken?"

## What is NOT solved (where a model might still earn its keep)

Navigation routing is done. These are untouched and are the only places a
distilled/local LLM is plausibly justified:

- **Query formulation**: turning a vague intent ("add retry to the webhook") into
  good `find` tokens/hints + the right symbol to target. The classifier assumes
  the symbol is already known/extractable; real tasks don't hand you the symbol.
- **Multi-hop trajectories**: "find impl → its callers → its tests", with a STOP
  decision. Tier-1 is single-hop only. This is the genuinely hard policy problem
  the whole Scout idea was originally about.
- **Trust-vs-verify on ambiguous edges**: PHP `[ambiguous]` callers (FINDINGS_v10
  finding 2) — when to trust the structural answer vs. read to confirm.

## Recommended next steps (in order)

1. **Make arm_c.py truly end-to-end** — drive the tool from the classifier's
   route (not harness type), report the real ~90%, and see where routing errors
   actually cost answers. Cheapest, highest-honesty next move. (~30 min, local.)
2. **Stress the router + formatters cross-language** — run arm C on a laravel
   (PHP) and a TS task set. If formatters break on PHP/TS output shapes, that's
   the real robustness work, and it's deterministic-code work, not model work.
3. **Reconcile the occurrences taxonomy** between generator and harness (caveat 3)
   so cross-use stops silently mislabeling.
4. **ONLY THEN consider a model** — and only for query formulation or multi-hop,
   not routing. If you do: generate multi-hop teacher trajectories (Tier-2, does
   not exist yet), and the honest baseline to beat is the classifier+formatter
   pipeline, not the raw agent.

## Where the evidence lives

- `bench/scout/FINDINGS.md` — full chain, every correction, the numbers.
- `bench/agent_ab/FINDINGS_v10.md` — the A/B proof the index helps (Go+PHP,
  cross-language, the routing table these findings build on).
- Public sanitized writeup: https://ethanhinson.github.io/codeindex/
- Git history: search `bench/scout` and `scout/` branch merges on main.

## Environment gotchas (cost real time this session)

- Global Python env has a transformers/tokenizers version clash — use the
  `.venv`. Don't `pip install -U` into the global env.
- LM Studio's embedding worker is broken (bad install path); use the local
  sentence-transformer in `.venv` for embeddings, NOT the LM Studio /embeddings
  endpoint. LM Studio /chat/completions works fine.
- The A/B harness (`bench/agent_ab/run_ab.py`) rebuilds the index per run (~2-3
  min) before agent runs start; be patient before assuming a hang.
- `codeindex` control-arm isolation: `CODEINDEX_DISABLED=1` makes the binary
  refuse (exit 127) — used so the A/B control arm can't leak the index.

# Scout — handoff for the next agent

**Read this + `FINDINGS.md` before touching anything in `bench/scout/`.**
Last updated 2026-08-15 (evening — supersedes the morning handoff's ordering;
the morning context below is still accurate and worth reading).

## Handoff 2026-08-15 evening: `--json` landed, tree state, and the plan

### What just happened (uncommitted — commit it first, see step 1)

`codeindex` now emits structured JSON from every query command: `callers`,
`callees`, `impact`, `dependents`, `deps`, `find`, `grep`, `enclosing` all
take `--json`. Design: each query builds ONE answer struct
(`internal/query/answers.go`), rendered two ways — `Text()` is the historical
format (a published contract: agent prompts and `bench/scout/formatters.py`
parse it) and `encoding/json` is the machine format. One retrieval path, two
renders — they cannot drift. The JSON carries what the regex formatters were
reverse-engineering: `qname`/`file`/`line`, explicit `ambiguous` flags, dep
provenance, `callers_total` alongside the limit-truncated list, `[]` never
null. Cold-build disclosure goes to stderr, so `--json` stdout always parses.

Verified two ways:
- Byte-identity: old vs new binary diffed on 12 command/shape combinations
  (gin + prometheus, incl. truncation, ambiguous flags, missing symbols,
  file-vs-symbol deps) — ALL IDENTICAL.
- `internal/query/query_test.go` pins the text format byte-for-byte with
  golden strings on a fixture repo, plus JSON invariants (totals vs shown,
  empty-array semantics). Change those goldens only deliberately, WITH the
  downstream consumers.

Modified (uncommitted): `internal/query/query.go` (rewritten as builders +
thin `*Text` wrappers; MCP server untouched), new `internal/query/answers.go`
+ `query_test.go`, `cmd/codeindex/main.go` (`--json` flag, `emit()`, flag
parsing fixed — `--limit` now works in any position), `internal/config/config.go`
(see stash note), `go.mod`/`go.sum` (sqlite-vec dep), `README.md`, this file.

### CRITICAL tree-state facts (cost real time to discover)

1. **There is stashed WIP that will conflict with the new query.go/main.go.**
   The untracked semantic-search files (`internal/search/semantic.go`,
   `internal/embed/`, `internal/engine/embedpass.go`, `internal/query/searchtext.go`,
   `internal/runtime/`, `internal/graph/vec.go|obs.go`, `cmd/codeindex/model.go|ingest.go`)
   are HALF of a WIP; the other half sits in `git stash`:
   - `stash@{0}` "pre-existing WIP tracked files" — schema v9 in
     `internal/graph/store.go` (creates `vecs`/`symvec`/`obs_ledger` tables),
     `mcpserver.go` (adds the `search` tool), `engine.go` (`Stats.Embedded`),
     and **`internal/query/query.go` deltas**: (a) `oneLine()` collapsing
     multi-line signatures in def lines, (b) runtime-spool ingestion inside
     `open()`, (c) a concept-query hint in find's no-match message.
   - `stash@{1}` "WIP semantic-search subcommand" — **`cmd/codeindex/main.go`**:
     a `search` subcommand.
   Both stashes conflict textually with the rewritten files. The deltas are
   small — port them into the new structure by hand (oneLine belongs in
   `defRefs`/`writeDefs`; spool ingest in `query.open()`; the find hint in
   `FindAnswer.Text()`), do NOT try a mechanical `stash pop`.
   `internal/config/config.go` was already restored byte-identical to the
   stashed version, so that one merges clean.
2. **Pre-existing test failures — not regressions.** `graph` (vec tests),
   `search` (semantic tests), `runtime`, `mcpserver` (search_test), and
   `engine` (vet: `Stats.Embedded`) fail because untracked WIP tests expect
   the stashed code. They failed before the JSON work. Passing packages:
   `query`, `config`, `adapter/*`, `depmap`, `embed`, `merkle`, `progress`,
   `readmodel`, `webserver`, and the cmd build.
3. **Bench indexes were schema v9, now rebuilt v7.** The committed binary is
   schema v7; gin/prometheus indexes were v9 (built by a WIP binary) and were
   rebuilt on first query. If you apply the stash (schema v9), the next query
   rebuilds them again (~2-3 min each). Harmless, but don't mistake it for a hang.

### The plan (in order)

1. **Commit the JSON work** (~15 min). Two commits: (a) `config`+`go.mod`
   tree-fix ("restore stashed config surface so the WIP files compile"),
   (b) the query refactor + `--json` + tests + docs. Don't fold in the
   untracked WIP files — they belong to the semantic-search change.
2. **Integrate the stashed WIP** (~1-2 h, decision needed). Either port the
   three query.go deltas + the `search` subcommand into the new structure and
   drop the stashes, or decide the semantic-search experiment is benched and
   leave the stashes alone (but then the untracked tests keep failing — the
   honest alternative is moving the WIP to a branch). Ask Ethan if unclear;
   `bench/engine/FINDINGS-semantic-search.md` has the experiment's status.
3. **Port `formatters.py` (and `arm_c.py`) to `--json`** (~1 h). This deletes
   the regex-parsing risk (old caveat 2) entirely. The formatter becomes:
   read JSON, emit the grader's shape. While there, wire the retrieval calls
   in `arm_c.py` to pass `--json`.
4. **Make arm_c truly end-to-end** (~30 min, unchanged from morning plan).
   Drive the tool from the CLASSIFIER's route, not the harness type. Report
   the honest ~90% and see where routing errors actually cost answers.
5. **Then try over-retrieval instead of better routing** (~1 h, new idea).
   Retrieval is ~free (ms per CLI call). When classifier confidence is low —
   or always, for the callers/grep blur (the 72% bucket) — run BOTH tools and
   pick by which JSON answer is non-empty / matches the expected shape.
   Hypothesis: end-to-end moves from ~90% toward the formatter ceiling with
   no model. Measure against step 4's number.
6. **Cross-language stress** (laravel PHP + a TS repo) through the JSON path.
   Now a schema-contract test, not regex hardening. If PHP/TS output shapes
   break the pipeline, the fix belongs in the JSON schema, not in parsers.
7. **Reconcile the occurrences taxonomy** (unchanged: harness "occurrences"
   means caller-attribution; generator's means literal token refs).
8. **Multi-hop as deterministic recipes BEFORE any model.** Enumerate the
   common trajectories (impact, dead-code, rename-radius, where-tested) as
   fixed compositions of graph ops — `codeindex impact` and the
   `codeindex:impact` skill are already this pattern. Measure coverage on
   real multi-hop tasks; only the residual justifies a learned policy.
9. **Query formulation: run the cheap gates first.** Before distilling
   anything for "vague intent -> symbol": (a) noun-phrase extraction +
   `codeindex find`'s fuzzy matching, (b) bge-base embedding similarity of
   task vs symbol names (already local in `.venv`). If either clears ~80%,
   the model is dead here too. Every time this project removed the model,
   the task turned out to be structure + cheap glue — assume that until a
   measured gate says otherwise.

### Discipline reminder (it caught four fake 100%s)

When a number looks too clean, find the leak BEFORE reporting it. Ask: "what
would make this pass even if the thing under test were broken?" The JSON
byte-identity diff and the golden tests exist so YOU can refactor without
re-earning that trust — keep them green.

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

## Recommended next steps (morning list — SUPERSEDED)

Superseded by "The plan" in the evening handoff above, which absorbs these
(arm_c end-to-end = step 4, cross-language = step 6, taxonomy = step 7,
model-only-if-gates-fail = steps 8-9).

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

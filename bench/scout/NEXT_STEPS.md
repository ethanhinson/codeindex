# Scout — handoff for the next agent

**Read this + `FINDINGS.md` before touching anything in `bench/scout/`.**
Last updated 2026-08-16. The 9-step plan is COMPLETE (`5123707` →
`999c46c`), and the post-plan "sensible next moves" list below is fully
worked: items 1–5 DONE (`nav` verb, `grep -w`, nest residuals, embed pool
default-on, settings stash merge), and item 6's first candidate (`nav`
flipped to `-w` grep behind a re-run of the union arm) is DONE too. The
only remaining candidate is the plugin prompt-note mention of `nav`,
gated behind an A/B. The k8s ≤2-min embed bar stays honestly open
(~10.6min extrapolated).

## What happened this session (all committed to main)

1. **`--json` landed and is the contract** (`37bd8e8`). Every query command
   emits structured JSON; text stays byte-pinned by golden tests
   (`internal/query/query_test.go`). One answer struct, two renders.
2. **The semantic-search WIP is fully integrated** (`ef35367`). No more
   stash trap: schema v9 (vecs/symvec/obs_ledger), `codeindex search`,
   `model`, `ingest`, MCP search tool, cxprof SDK — all on main, ALL
   packages' tests green. Only `stash@{0}` remains, kept solely for
   `.claude/settings.local.json` permission entries the agent was (rightly)
   blocked from merging — union-merge by hand, then `git stash drop`.
3. **Formatters read JSON** (`7c4d3c7`), score-parity with the old regexes.
4. **Honest end-to-end measured** (`9d8d5c1`): classifier-routed arm_c =
   70% / F1 0.65. Routing errors concentrate in caller-shaped tasks.
5. **Over-retrieval killed the router** (`fa367f0`): run callers+find+grep,
   emit ONE union answer (section order tuned to grade.py's region rules) →
   **100% / F1 0.95, per-task identical to the formatter ceiling, zero
   routing.** The classifier is now only a 3-calls→1 cost optimization.
6. **Cross-language holds** (`2120d51`): laravel PHP 100%/0.95, fresh nest
   TS set 100%/0.98 (tasks_nest.json committed). Zero schema/parser changes
   — one formatter POLICY fix (DEFINITIONS = all match=="exact" owners).
7. **Taxonomy reconciled** (`f397d06`): generator's literal-token class is
   now `token_refs`; harness "occurrences" stays (frozen name) and is
   documented at source as caller-attribution.
8. **Multi-hop = recipes** (`0a9e473`): `recipes.py` (impact, where-tested,
   rename-radius, dead-code) over the JSON contract; `measure_recipes.py`
   grades vs plain text scan → P=R=F1=1.00 on 3 languages AFTER two
   deterministic fixes (pass `--limit 2000`; word-boundary post-filter on
   grep claims — codeindex grep is substring-based). FINDINGS records which
   part of that 1.00 is by construction.
9. **Query-formulation gates measured** (`d369090`): content-words+find =
   0% on the curated concept sets; bge-vs-symbol-names = 46-64%; the
   SHIPPED `codeindex search` = 65-88% on the same frozen fixtures. No
   distilled LLM justified; the local hybrid is the answer.

Also: the whole bench evidence corpus is committed (`999c46d`) — engine
FINDINGS, concept_sets, curated results, selfheal harness, v8/v9 A/B docs.
`bench/repos/` (3.5G clones) and per-run transcripts are gitignored.

## The bottom line (upgraded from the last handoff)

    task ──▶ [RETRIEVE ALL: callers+find+grep, ~ms each] ──▶ [UNION FORMAT] ──▶ answer

No model. No router. Single-hop navigation AND the four common multi-hop
trajectories are structure + cheap glue, measured across Go/PHP/TS. Every
time this project reached for a model, the measurement said no — that prior
now has nine more data points.

## Where a model is STILL plausibly justified (the full residual)

- **Query reformulation on top of `search`** for the weak repos (nest 65.4%
  on curated; its failure mode is identical-doc sibling families — see
  bench/engine/FINDINGS-semantic-search.md).
- **Trust-vs-verify on `[ambiguous]` edges** (PHP especially) — when to
  accept the structural answer vs read the file to confirm.
- **Attribution-level multi-hop grading** first needs non-circular gt
  (human labels or PR-derived truth); until then the recipes' symbol-level
  value-add is unmeasurable and a policy is unjustifiable BY DEFAULT.

## Sensible next moves — status after this session

1. ~~Surface the union answer as a product verb~~ **DONE: `codeindex nav
   <repo> <anchor>`** + MCP `nav` tool. One pass (callers+find+grep,
   substring exactly as measured), NavAnswer with the exact-owners
   DEFINITIONS policy and the grep-stands-in-for-callers blur baked in
   (flagged `callers_from_grep`). Golden-pinned. See FINDINGS (night entry).
2. ~~`grep -w`~~ **DONE**: both backends (rg `-w` / `\b(?:...)\b` wrap);
   recipes.py post-filter deleted in favor of `_word_grep`; re-measured
   1.00/1.00/1.00 on tasks_v6/lphp/nest. JSON gains `"word":true` only when
   set (goldens untouched). MCP grep gained the optional `word` arg.
3. ~~nest concept quality~~ **WORKED 2026-08-15 (late night)** — the stale
   "sibling-family" framing was wrong: contrast cards already fixed that
   layer; the real residuals were buckets 3+1 of the diffusion-contrast
   backlog. Shipped: file-class penalty extended to sample corpora and
   moved outside the compression envelope (zero regression; nest
   mechanical 55→60). Withheld: entry-point roles (nest curated
   65.4→76.9 gated behind CODEINDEX_ROLE_BOOST=1; gin/flask regressed —
   named cause: unbounded user-side votes; registered follow-up: vote
   saturation, fresh budget). See bench/engine/FINDINGS-residuals-roles.md.
4. ~~kubernetes-scale embed budget~~ **MEASURED AND SHIPPED 2026-08-16
   (midday)**: context pool default ON — min(8, NumCPU/2) single-thread
   contexts (measured winner shape; 2×8 is SLOWER than 1×8 because BLAS
   already saturates within one encode). laravel cold build 3m15s→1m54s
   (embed 1.83×, 3.5ms/card); indexes bit-identical to serial; curated
   88.5 holds. **The ≤2-min k8s bar is NOT met** (extrapolates ~10.6min;
   next levers if ever justified: Metal backend, faster quantization,
   coarser cards) — recorded honestly, bar stays open. Overnight laptop
   timings are GARBAGE (doze/E-core throttling; 439s real on a 19s
   build) — `bench/embed_pool_sweep.sh` carries a sanity gate. See
   bench/engine/FINDINGS-embed-pool.md.
5. ~~settings.local.json stash merge~~ **DONE** (commit `5ed8eda`, with
   Ethan approving the write): 123 stash-only allow entries + current 137 =
   260, every other stash file verified subsumed by main, stash@{0}
   dropped. The stash list is empty — the WIP-stash saga is fully closed.
6. NEW candidates surfaced by this session — first half DONE 2026-08-16:
   ~~`nav` with `-w` grep~~ **DONE behind the required re-run**: union arm
   re-measured substring-vs-word on all 3 sets (substring reproduced the
   recorded scores; word held 100% success and raised F1 on every set —
   only 3 tasks changed, all up, all short-anchor comprehension). Nav now
   greps word-boundary; goldens unchanged, suite green. See FINDINGS
   (2026-08-16 entry). Side effect: `arm_c.py --over-retrieve` no longer
   trains the router (the on-disk gin_tier1.jsonl predates the
   `token_refs` rename — regenerate before any ROUTED run). Remaining:
   plugin prompt-note mention of `nav` — still ONLY behind an A/B (the
   ~155-token note is a measured artifact).

## The files (bench/scout/)

| file | what | trust |
| --- | --- | --- |
| `arm_c.py` | end-to-end arm: default=classifier-routed, `--over-retrieve` (the winner), `--forced-tools` (ceiling ref) | measured, 3 langs |
| `formatters.py` | JSON → grader shapes incl. `fmt_union` | score-parity proven |
| `recipes.py` | 4 multi-hop recipes over `--json` | file-level 1.00, see caveat |
| `measure_recipes.py` | recipe coverage vs text-scan gt | non-circular at file level |
| `measure_gates.py` | query-formulation gates on curated sets | run 2026-08-15 |
| `clf_baseline.py` | embedding+logreg router (90% accept-set) | now optional |
| `gen_tier1.py` | tier-1 corpus generator (`token_refs` class) | works |
| `rule_baseline.py` / `measure_ceiling.py` / `local_llm.py` | baselines/ceiling/LM-Studio client | works |

Run venv-dependent scripts with `bench/scout/.venv/bin/python`. `*.jsonl`
outputs are gitignored — regenerate. Binary: build from main (schema v9);
first query on a v7-indexed bench repo rebuilds (~10s gin, ~5min laravel —
not a hang).

## Discipline reminder (it caught four fake 100%s, then two more this session)

When a number looks too clean, find the leak BEFORE reporting it. Ask: "what
would make this pass even if the thing under test were broken?" This session
it caught (5) needing a type-leak check on fmt_union and (8)'s 1.00 being
part-tautological — both are documented in FINDINGS instead of shipped as
magic. Keep the golden tests and the byte-identity habit green.

## Where the evidence lives

- `bench/scout/FINDINGS.md` — the full chain, now through the union verdict.
- `bench/agent_ab/FINDINGS_v10.md` — the A/B proof the index helps.
- `bench/engine/FINDINGS-semantic-search.md` — the search experiment + gates.
- Public sanitized writeup: https://ethanhinson.github.io/codeindex/

## Environment gotchas (unchanged)

- Use `.venv` — global python has a transformers/tokenizers clash.
- LM Studio /embeddings is broken; use sentence-transformers in `.venv`
  (bge-base, MPS). /chat/completions works.
- `CODEINDEX_DISABLED=1` makes the binary refuse (exit 127) — control-arm
  isolation for A/B runs.
- Two concurrent builds of one repo race on graph.db (second writer gets a
  disk I/O error) — serialize bench builds.

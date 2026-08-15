# Self-healing validation harness — findings

**Date:** 2026-07-12 · **Change:** `selfheal-validation-harness`
**Built by three parallel subagents (PHP lab, issue miner, harness core), integrated and re-run by the main session.**

## Scenario matrix: ALL GREEN, zero remediations needed

| scenario | runtime | outcome | resolution | observed edges |
| --- | --- | --- | --- | --- |
| node-registry | Node (SDK) | passed, attempt 1 | 40%¹ | 1 (registry dispatch bridged) |
| go-sdk | Go (SDK) | passed, attempt 1 | — | — |
| node-symlink | Node via symlinked root | passed, attempt 1 | 40%¹ | 1 |
| php-excimer | PHP 8.3 + Excimer in Docker | passed, attempt 1 | **100%** | 4 (do_action → handlers) |

¹ 40% is the honest expected value: top-level script frames and the SDK's own
frames are legitimately outside the app's symbol spans; the 30% assertion
floor reflects that, not a deficiency.

The PHP result is the WordPress-shaped proof at container scale: a
string-keyed `add_action`/`do_action` framework — statically edge-free —
produces observed edges, `[observed]` disclosure, and dispatcher+handlers
sharing a cluster. Container→host path translation is solved at the emitter
(repo-relative frames), per design D1.

## The self-healing loop

`learned.json` is EMPTY after real runs — no scenario needed remediation —
and that emptiness is itself the honest result. The loop was validated with
synthetic scenarios: fail→r1 fail→r2 pass recorded `{"flaky":["r2"]}`; the
next run applied r2 proactively (`remediation_used="learned(r2)"`); an
unfixable scenario walked the ladder and ended failed-quarantined, loudly.
Every attempt journals to `runs.jsonl` (11 entries), so a scenario that
always needs remediation can never hide.

Non-obvious mechanics recorded for future maintainers: the ingest ledger's
(path, hash) idempotence means path-based remediations must reset state
(r2/r3 rebuild the index around the stashed spool); the Node SDK's async
spool write requires the app to await the file; Node resolves module
symlinks itself, and ingest's dual EvalSymlinks covers the macOS /tmp alias
— which is why node-symlink passes with no remediation.

## The issue corpus — the finding that matters

Closed issues with fix commits, mined from deepened local history (titles
fetched under a lifetime-capped, cached, unauthenticated budget — 118
requests total including the agent's iterations; final runs are 0-network):

**v2 correction (2026-07-12, same day):** the v1 numbers below were
dominated by ground-truth quality, not retrieval quality — the
function-name-based remapping (mapping v2: hunk-header xfuncname +
added-definition names; span fallback only for byte-identical files) moved
the two repos in OPPOSITE directions, proving the point:

| repo | questions v1→v2 | v1 hit@5 | **v2 hit@5** | find-control | **grep-control** | curated |
| --- | --- | --- | --- | --- | --- | --- |
| gin | 52→39 | 19.2% | **10.3%** | 0.0% | **20.5%** | 88.5% |
| flask | 36→30 | 13.9% | **33.3%** | 0.0% | **43.3%** | 76.0% |

**The controls flip the headline: plain grep-attribution BEATS semantic
search on real-issue queries, on both repos.** Issue titles carry literal
symptom strings — error messages, warning text, config keys — that lexical
attribution exploits and the embedding cards deliberately exclude. Measured
example: "console logger HTTP status code bug" → `responseWriter.WriteHeader`,
whose body contains the string `"[WARNING] Headers were already written.
Wanted to override status code"`. Our own `grep` verb finds it; our `search`
verb cannot.

Miss anatomy (55 v2 search misses, bucketed with code-level evidence in
`bench/selfheal/issues_miss_analysis.md`): multi-hop with no lexical bridge
23 (the honest hard core — router-tree/binding internals), outranked-at-6-15
11 (ranking, recoverable), symptom-literal-present 7 (literal cards convert
these), ground-truth-residue 2 (comment-only hunks), other 12 (incl.
filter-leaked refactor titles).

## Follow-ups filed (residuals discipline — nothing patched silently here)

1. **New residual bucket: bug-symptom queries** (added to the
   diffusion-contrast residuals backlog as bucket 4). Candidate mechanisms:
   error-message/string-literal card enrichment (symptom text lives in
   throw/log statements — bucket 2's call-site work overlaps), and
   runtime-heat ranking once field profiles exist. Falsifiable target to
   register in that change: issues-closed hit@5 ≥ 40% without curated
   regression.
2. **Miner precision**: blame/function-name-based commit→symbol mapping to
   kill line-drift; needed before this class can gate anything.
3. **Filter leak**: title deny-list uses word boundaries ("golint" evades
   `\blint\b`); minor, tracked.

## Assets now standing

`bench/selfheal/harness.py` (matrix + ladder + memory), `scenarios/*`,
`php/` (Dockerfile, hook app, excimer→cxprof adapter with the Excimer API
facts documented), `issues_corpus.py` + frozen fixtures + caches. One
command re-validates the entire runtime-evidence pipeline across three
runtimes, containers included.

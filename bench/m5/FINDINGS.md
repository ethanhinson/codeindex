# M5 harness — build, test, verify (2026-08-16)

## What was built

The go/no-go benchmark from `ROADMAP.md` M5: three adversarial suites
(grepwin / dominate / break), three arms (A frontier+shell, B +plugin,
C cheap explorer+shell), pre-registered GO/KILL gates, and a mechanical
false-confidence metric (COVERAGE-line protocol). 64 frozen tasks across
gin/flask/nest/laravel (seed 20260816, sha1 9de57d5faa02). See README.md.

Verified: all three selftests green (generator gt re-verified against actual
repo lines; grader semantics incl. false-confidence; gate arithmetic incl.
KILL trigger and WITHHELD-below-n=10). Smoke run 9/9 completed, graded, gated.

## Finding 1 — global-plugin hook leak (control contamination, FIXED)

The first smoke showed arms A and C **attempting** `codeindex callers`.
Cause: the codeindex plugin is installed globally on this machine
(`~/.claude/settings.json`: `"codeindex@codeindex": true`, marketplace path
`/Users/ethanhinson/codeindex`); its UserPromptSubmit hook injects the
"Available in this repo: codeindex" note into EVERY claude session —
including headless bench runs. The execution shim held (exit 127, verified in
transcripts), so no data leaked, but the control arms knew the tool existed
and burned turns trying it.

Fix: `--setting-sources project,local` on every arm (bench clones have no
`.claude/`, so this yields a pristine agent). Probe-verified both ways:
default run answers YES to "were you told about codeindex", isolated run
answers NO, and arm B with `--plugin-dir` still answers YES. Contaminated
smoke archived as `results/runs_m5_hookleak.jsonl.bak`.

**Wider blast radius:** any headless benchmark run on this machine after the
global plugin install had this hook active in BOTH arms. The note A/B
(5795182) used `--plugin-dir` in both arms with only the nav sentence as
delta, so its *paired* result is probably sound (equal contamination), but
absolute adoption counts from any earlier run that assumed a codeindex-blind
control should be treated as suspect. Future harnesses must pass
`--setting-sources project,local` (or `--bare` where API-key auth is
available).

## Finding 2 — clean smoke signals (n=1 per cell; numbers, not conclusions)

- **dominate (WriteHeaderNow callers):** B answered from one codeindex call
  in 2 turns at $0.034; A needed 11 turns of grep+read at $0.144 (−76% cost,
  −73% processed tokens). C (haiku) took 16 turns, hit recall 0.857, and
  claimed `COVERAGE: complete` — the false-confidence metric caught exactly
  the failure mode it was registered for, on its first live run.
- **grepwin (unique string):** B paid $0.1375 vs A's $0.0281 (~4.9×) for the
  same 1-line answer — the plugin's prompt/context overhead is real money on
  tasks where the index is useless. This is the adversarial-adoption cost the
  grepwin gate (≤10% regression) exists to bound; watch it in the full run.
- All GO/KILL sub-verdicts correctly WITHHELD at n=1.

## Finding 3 — fuse family (model-scale arms) bring-up (2026-08-16 evening)

Per Ethan's direction, the campaign gained a second harness family testing the
COMPOUNDING claim: value compounds if a SMALL model + index matches a LARGE
model without one. Implemented as `run_m5_fuse.py` — one-shot `fuse`
(~/dev/fuse) through the local LiteLLM gateway, arms L/LX/S/SX
({glm-5.2 cloud, qwen3-coder-30b local} × {shell, index}), compound gate
pre-registered in the tasks header. Families are never cross-compared.

Bring-up surfaced and fixed three real defects:

1. **fuse's native codeindex tools were broken against the current CLI** —
   they call `codeindex <sub> <symbol>` with no repo root, and the CLI's
   usage error exits 0, so the tool would have returned the error text as a
   successful result. Bench fix: a PATH wrapper injecting `$CODEINDEX_REPO`.
   (Upstream fuse fix + the exit-0 usage bug in codeindex itself are backlog.)
2. **fuse's adapter 500-looped on LM Studio replay** when a model emitted an
   empty-arguments tool call (qwen3-coder does): `"arguments": ""` is invalid
   JSON for the OpenAI wire format and LM Studio's template rejects the
   replayed conversation. Patched in ~/dev/fuse (adapter.go normalizes empty
   arguments to `{}` on the wire); fuse model tests green.
3. **Zero adoption without a prompt note** — first clean smoke (12/12 runs,
   no errors): NEITHER model touched the codeindex tools in the index arms.
   glm ground through 7-turn grep+read chains (58–70k tokens) either way;
   qwen3-coder answered the caller task from one grep in 2 turns with
   call-site expressions instead of function names — f1 0.00 while claiming
   COVERAGE: complete (a false-confidence catch on a *treatment* arm).
   Archived as `runs_m5_fuse_nonote.jsonl.bak`. Conclusion, consistent with
   the note-A/B lesson: bare tools in a tool list are not a treatment; the
   treatment is tools + note in BOTH families. Index arms now prepend a
   4-line INDEX_NOTE to the task prompt.

## Finding 4 — the note flips adoption, and the compound signal appears (n=1)

Second fuse smoke (12/12 clean, INDEX_NOTE in treatment arms): adoption went
0/6 → 3/6 index-arm runs, and the dominate row is the compounding claim in
miniature — read it as one story:

| arm | model | turns | tokens | f1 |
|---|---|---:|---:|---:|
| S (shell) | qwen3-coder-30b local | 35 | 212,574 | 0.23 + FALSE-CONF |
| SX (index) | qwen3-coder-30b local | 2 | 10,438 | **1.00** |
| L (shell) | glm-5.2 cloud | 5 | 34,272 | 1.00 |
| LX (index) | glm-5.2 cloud | 9 | 55,172 | 1.00 |

The small local model without the index burned 20× the tokens to produce a
wrong-but-confident answer; WITH the index it matched the large model's
perfect answer at ~30% of the large model's tokens, for $0. That is exactly
the pre-registered compound gate's shape (SX ≥ L, SX ≫ S) — at n=1,
verdict correctly WITHHELD. The full campaign decides.

## Cost / next step

Smoke spend: $0.57 + $0.58 (claude family), ~$0.02 cloud + $0 local ×2
(fuse family), probe cents.
Full campaign:

    # claude family: 64 tasks x 3 arms = 192 runs, est. $10-25
    python3 bench/m5/run_m5.py --full --budget-usd 80
    python3 bench/m5/grade_m5.py
    # fuse family: 64 tasks x 4 arms = 256 runs, ~$1-3 cloud + $0 local,
    # wall-time bound by the local 30B (keep LM Studio loaded; serialize)
    python3 bench/m5/run_m5_fuse.py --full
    python3 bench/m5/grade_m5.py --family fuse
    # combined report (evaluates whichever graded files exist)
    python3 bench/m5/gate_m5.py

Backlog (recorded, not hidden): dynamic-dispatch/DI/event-bus break tasks
need hand-labeled arm-neutral gt; nest yields only 1 grepwin_string; flask
only 2 dominate_callers at the ≥8-char filter; graph-gt caveat in README.

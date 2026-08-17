# M5 — the go/no-go benchmark (adversarial suites + kill gate)

Implements roadmap milestone **M5** (`ROADMAP.md`): can CodeIndex credibly
claim it eliminates repository exploration without hurting task success — and
does it survive its own kill gate (a cheap explorer + shell matching it)?

Built on the verified `bench/agent_ab` machinery (headless `claude -p`,
stream-json metrics, control-arm shim + `CODEINDEX_DISABLED=1` isolation,
budget guard, resume-by-key).

## Arms — two harness families, never cross-compared

**Claude-CLI family** (`run_m5.py`) — the shipped product surface:

| arm | model | tools | codeindex |
|---|---|---|---|
| A | frontier (default `claude-sonnet-4-6`) | Bash/Read/Grep/Glob | shimmed out + env kill-switch |
| B | same frontier | same | the REAL packaged plugin (`--plugin-dir plugin/`) |
| C | cheap explorer (default `claude-haiku-4-5`) | same | shimmed out (kill-gate arm) |

**Fuse family** (`run_m5_fuse.py`) — the model-scale (compounding) claim,
driven by one-shot `fuse` through the local LiteLLM gateway (cloud +
local models). The value proposition compounds if a SMALL model + index
matches a LARGE model without one — no new model needed:

| arm | model | codeindex |
|---|---|---|
| L | large (default `glm` = cloud/glm-5.2) | tools disabled + shim |
| LX | large | native `codeindex_*` fuse tools + PATH wrapper |
| S | small (default `qwen3-coder` = local/qwen3-coder-30b, ~$0) | tools disabled + shim |
| SX | small | native tools + wrapper |

Deltas are only meaningful WITHIN a family (different harnesses, system
prompts, tool sets). The fuse family reports tokens, not $ — the gateway
prices nothing and local models are free.

All M5 tasks are read-only; Edit/Write and sub-agents are hard-denied in every
claude arm, and write/web/agent/skill tools are disabled via a repo-planted
`.fuse.local.yml` in every fuse arm (sub-agents break token/turn accounting —
same rationale as agent_ab).

## Suites (64 tasks, 4 repos × 4 languages)

| suite | types | designed so that | gt source |
|---|---|---|---|
| grepwin | `grepwin_string`, `grepwin_filename` | **grep wins** — unique exact strings, unique filenames. Measures whether arm B wastes money on the index when it shouldn't be used. | ripgrep / filesystem (arm-neutral) |
| dominate | `dominate_callers`, `dominate_tests`, `dominate_blast` | **codeindex dominates** — widely-called symbols (12–120 callers), test-surface discovery, two-level blast radius. | rg for tests (neutral); graph.db for callers/blast (**not** neutral — see below) |
| break | `break_collision` | **static graphs look confidently wrong** — same-name symbols defined in 2–6 places. Measures the false-confidence rate. | regex definition extraction (arm-neutral) |

Repos: gin (Go), flask (Python), nest (TS), laravel-framework (PHP), pinned
per `bench/repos.json`, clones in `bench/repos/` (`AB_WORK` overrides).

### False-confidence protocol (pre-registered)

`dominate_callers`, `dominate_blast`, and `break_collision` prompts demand a
final `COVERAGE: complete|incomplete` line.
**false_confidence = recall < 1.0 AND the claim is not 'incomplete'** (a
missing line counts as a completeness claim). This is the roadmap's trust
metric, made mechanical.

## Pre-registered gates

Stamped into `tasks/tasks_m5.json`'s header at generation time; `gate_m5.py`
reads them from there, never from its own source. Sub-verdicts are
**WITHHELD** below 10 pairs — smoke runs print numbers, not conclusions.

**GO gate (B vs A):**
- dominate: success within −5pp, median paired savings (cost ≥30% OR
  processed tokens ≥50%), mean recall not lower (blast-radius recall).
- grepwin: median paired cost regression ≤10% (adversarial-adoption check).
- break: false-confidence rate at most +10pp vs arm A.

**KILL gate (C vs B, dominate):** C success within −5pp of B at ≤1.10× B's
median cost ⇒ the structural index fails to justify its maintenance burden.

**COMPOUND gate (fuse family, dominate):** SX success ≥ L success − 5pp
(the index substitutes for model scale) AND SX success ≥ S success + 10pp
(the index, not the small model alone, closes the gap).

## Honesty notes (read before trusting a number)

1. `dominate_callers` / `dominate_blast` ground truth comes from graph.db
   (unambiguous name edges; level 2 via `dst_symbol_id`). This reuses the
   hand-check-verified agent_ab pattern but is **not arm-neutral**: a
   codeindex recall bug would understate arm A there. The arm-neutral types
   carry the cross-check; targets are restricted to unique, ≥8-char names
   (short generic names like `close` were observed to poison gt with
   unrelated receiver calls and are excluded).
2. `break_collision` gt uses the same regex definition extraction as the
   shipped comprehension tasks — approximate (arrow functions etc. are
   missed), but identical for every arm.
3. Known generation shortfalls are printed, not hidden: nest yields only 1
   `grepwin_string` (few long unique message literals); flask only 2
   `dominate_callers` at the ≥8-char filter.
4. Dynamic-dispatch / DI / event-bus break tasks are **not yet generated** —
   arm-neutral ground truth there needs hand labeling (M5 backlog; the
   collision class plus the false-confidence metric covers the
   "confidently incomplete" risk mechanically until then).
5. Can a broken codeindex pass? Arm A/C isolation is shim + kill-switch
   (verified pattern); gt is arm-neutral except where flagged; the gate
   withholds verdicts under 10 pairs. Residual leak: graph-gt tasks (point 1).

## Run

```
go build -o bench/agent_ab/.bin/codeindex ./cmd/codeindex
python3 bench/m5/build_tasks_m5.py            # regenerates tasks (seeded)
python3 bench/m5/build_tasks_m5.py --selftest
python3 bench/m5/grade_m5.py --selftest
python3 bench/m5/gate_m5.py --selftest

# claude-CLI family
python3 bench/m5/run_m5.py --smoke            # 3 tasks x 3 arms, inspect first
python3 bench/m5/run_m5.py --full --budget-usd 80
python3 bench/m5/grade_m5.py

# fuse family (needs the LiteLLM gateway up + ~/dev/fuse/fuse built;
# local model backend must be loaded in LM Studio)
python3 bench/m5/run_m5_fuse.py --smoke       # 3 tasks x 4 arms
python3 bench/m5/run_m5_fuse.py --full
python3 bench/m5/grade_m5.py --family fuse

python3 bench/m5/gate_m5.py                   # writes results/report_m5.md
                                              # (evaluates whichever graded
                                              # files exist)
```

Fuse-family prerequisites discovered during bring-up (see FINDINGS.md):
fuse's native codeindex tools omit the repo-root argument the CLI requires —
the runner injects it via a PATH wrapper; and fuse's adapter needed an
empty-tool-call-arguments normalization (patched in ~/dev/fuse) or LM Studio
500s on conversation replay.

`results/` (runs_m5.jsonl, transcripts, hooklogs, report) is generated
output. Runs are resumable: existing keys in runs_m5.jsonl are skipped.

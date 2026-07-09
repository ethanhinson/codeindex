## 1. Environment verification (do this first — nothing else depends on guesses)

- [x] 1.1 Verified toolchain: binary builds to `bench/agent_ab/.bin/codeindex`; claude 2.1.193. Flags recorded in README. KEY FINDING: no `--max-turns` (use timeout+budget); `--allowedTools` comma/space; `--permission-mode bypassPermissions` for headless; `--append-system-prompt` inline
- [x] 1.2 Verified pinned repos (gin, prometheus) present; `codeindex build` + `query` work in the documented format
- [x] 1.3 Probe done (`results/probe/probe1.jsonl`): result event has total_cost_usd/num_turns/usage; tool_use blocks parseable. KEY FINDING: file reads land in cache_creation_input_tokens not input_tokens → metric changed to cost-primary + processed-tokens (spec corrected)

## 2. Task builder (`bench/agent_ab/build_tasks.py`)

- [x] 2.1 Comprehension generator: seeded 70/30 clean/hot sampling, rg-derived ground truth
- [x] 2.2 Localization generator: PR→issue via merged-PR "fixes #N" parse; PR .go files (1–10, ≥60% existing at pin); cached API responses
- [x] 2.3 Emit `tasks/tasks.json` — 24 tasks (16 comprehension, 8 localization; gin+prometheus) with header carrying seed, pins, and thresholds
- [x] 2.4 Selftest passes (RouterGroup defs/refs)

## 3. Arm-B treatment prompt (`bench/agent_ab/prompts/arm_b_tools.md`)

- [x] 3.1 Tool documentation written (binary path, query syntax, when-to-use, caveats)
- [x] 3.2 One worked example (gin `CreateTestContext`, real output)
- [x] 3.3 486 tokens (< 600); counted in README

## 4. Runner (`bench/agent_ab/run_ab.py`)

- [x] 4.1 Run matrix with seeded shuffle, serial-per-repo, git checkout+clean(-e .codeindex) reset, resume-by-skip
- [x] 4.2 `claude -p` invocation (verified flags); stream-json archived; tokens/cost/turns/tool counts/codeindex_calls parsed; crash-safe rows
- [x] 4.3 `--smoke` (default), `--full`, `--tasks`, `--reps`, `--model`, `--timeout` (no `--max-turns` in CLI), `--budget-usd` hard stop
- [x] 4.4 Smoke run clean after fix — KEY FIX: bypassPermissions ignores --allowedTools, so sub-agents (Agent/Task) must be HARD-DENIED via --disallowedTools (they broke accounting + buried the answer). Re-ran: 4/4 parseable, 4/4 success

## 5. Grader (`bench/agent_ab/grade.py`)

- [x] 5.1 Markdown-tolerant, path-normalizing parsers; unit-tested on messy fixtures
- [x] 5.2 Comprehension (±2-line def match + file F1) and localization (file F1) scoring; unparseable flag
- [x] 5.3 Smoke graded and hand-checked against answers

## 6. Report (`bench/agent_ab/report.py`)

- [x] 6.1 Paired per-task stats: medians over reps, paired deltas, seeded bootstrap 95% CI, win rate, success deltas
- [x] 6.2 Adoption + per-protocol re-analysis + ITT-vs-PP gap callout
- [x] 6.3 Verdict vs pre-registered thresholds; emits `report.md` with table, provenance, cost, caveats
- [x] 6.4 Determinism check — report.py byte-identical across two runs

## 7. The experiment

- [x] 7.1 Froze + committed `tasks.json` (thresholds in header) before the run
- [x] 7.2 Ran `--full` ($7.89, 96 runs, within $40 budget); monitored throughout
- [x] 7.3 Report generated (`results/report.md`, `FINDINGS.md`); results committed
- [x] 7.4 VERDICT: **RED** — codeindex +17% cost (ITT) / +26% (per-protocol), success parity, 81% adoption. Additive round-trip vs already-cheap native grep. Roadmap: STOP downstream breadth; redesign task set around expensive/insufficient-grep tasks before any further engineering. Recorded in proposal.md + FINDINGS.md

## 8. Verification

- [x] 8.1 `openspec validate agent-ab-efficacy` passes; README + FINDINGS document reproduction from a clean checkout

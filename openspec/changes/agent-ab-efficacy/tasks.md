## 1. Environment verification (do this first — nothing else depends on guesses)

- [ ] 1.1 Verify the toolchain: `go build -o /tmp/codeindex ./cmd/codeindex` succeeds; `claude --version` and `claude --help` work — record the exact flags available for: print mode, output format, allowed tools, appended system prompt, max turns (names may differ from the design's sketch; adapt and document in `bench/agent_ab/README.md`)
- [ ] 1.2 Clone/refresh pinned repos via the `ensure_repo` recipe in `bench/token_bench.py` (gin, prometheus); run `codeindex build` on each and confirm `codeindex query <repo> Labels --limit 5` prints defs + callers in the documented format
- [ ] 1.3 One manual end-to-end probe: run `claude -p` with a trivial prompt in the gin clone with restricted tools and stream-json output; confirm you can extract tokens, cost, num_turns, and tool_use counts from the output (archive this probe under `bench/agent_ab/results/probe/`)

## 2. Task builder (`bench/agent_ab/build_tasks.py`)

- [ ] 2.1 Comprehension generator: seeded symbol sampling per the spec's filters (70% clean / 30% hot mix), rg-derived ground truth (definition file:line via definition regex; referencing files via `rg -w -l`), emit task records
- [ ] 2.2 Localization generator: GitHub API — closed issues with linked merged PRs, PR changed-files filtered to 1–10 non-vendor `.go` files with ≥60% existing at the pin; cache raw API responses to `bench/agent_ab/cache/`; emit task records
- [ ] 2.3 Emit `bench/agent_ab/tasks/tasks.json` (≥20 tasks, both types, both repos) with a header carrying: seed, generation date, repo pins, and the pre-registered GREEN/YELLOW/RED thresholds copied verbatim from the design
- [ ] 2.4 Unit-test the ground-truth extraction on 3 hand-verified symbols and 2 hand-verified issue/PR pairs

## 3. Arm-B treatment prompt (`bench/agent_ab/prompts/arm_b_tools.md`)

- [ ] 3.1 Write the tool documentation: absolute binary path, `query` syntax, the exact output format (copy from a real invocation), when to use it (any "where is X defined / who calls or references X / what would this change affect" moment — before reaching for Grep/Read), the `--limit` flag, and its Go-only + name-based-ambiguity caveats
- [ ] 3.2 Include exactly one worked example (real symbol from gin, real output, one-sentence interpretation)
- [ ] 3.3 Keep it under ~600 tokens; record its token count in the README (it is part of the treatment cost)

## 4. Runner (`bench/agent_ab/run_ab.py`)

- [ ] 4.1 Implement the run matrix (task × arm × rep) with recorded-seed shuffling, serial execution per repo clone, per-run `git checkout -- .` + clean verification, and resume-by-skipping-completed keys from `runs.jsonl`
- [ ] 4.2 Implement the `claude -p` invocation per the verified flags; capture stream-json to `bench/agent_ab/results/transcripts/<task>_<arm>_<rep>.jsonl`; parse tokens/cost/turns/tool counts/codeindex_calls; write result rows crash-safely
- [ ] 4.3 Implement `--smoke` (default; 2 tasks × 2 arms × 1 rep), `--full`, `--tasks N`, `--reps R`, `--model`, `--max-turns`, `--budget-usd` with hard stop, and per-run timeout (kill + flag)
- [ ] 4.4 Run smoke; manually read both transcripts end-to-end; fix prompt/parsing issues; re-run smoke until clean (commit the smoke transcripts)

## 5. Grader (`bench/agent_ab/grade.py`)

- [ ] 5.1 Answer parsers for DEFINITIONS/FILES sections (markdown-tolerant, path-normalizing); unit tests with messy fixtures (absolute paths, backticks, prose preamble)
- [ ] 5.2 Comprehension scoring (±2-line definition match; file-set F1; task F1 = mean; success ≥ 0.5) and localization scoring (file-set F1; success ≥ 0.5); `unparseable` flag path
- [ ] 5.3 Grade the smoke runs and hand-check every score against the transcripts

## 6. Report (`bench/agent_ab/report.py`)

- [ ] 6.1 Paired per-task stats: per-arm medians over reps, paired deltas, median % token reduction with seeded bootstrap 95% CI (≥5,000 resamples), win rate, success deltas, turns
- [ ] 6.2 Adoption rate + per-protocol re-analysis + explicit intent-to-treat vs per-protocol gap callout
- [ ] 6.3 Verdict evaluation against the pre-registered thresholds read from the task-file header; emit `report.md` with the full paired table, provenance block, cost totals, and caveats
- [ ] 6.4 Determinism check: two invocations of `report.py` on the same `runs.jsonl` produce byte-identical reports

## 7. The experiment

- [ ] 7.1 Freeze and commit `tasks.json` (thresholds included) BEFORE the full run
- [ ] 7.2 Run `--full` within budget; monitor the first few runs' transcripts
- [ ] 7.3 Generate the report; commit `runs.jsonl` (transcripts archived, large ones gitignored if needed), `report.md`, and a summary entry in `bench/agent_ab/README.md`
- [ ] 7.4 Record the verdict and its roadmap consequence in `openspec/changes/agent-ab-efficacy/` (update this file) and surface the result to the maintainer — GREEN unblocks `core-indexing-engine` breadth + plugin; YELLOW triggers one registered iteration on the arm-B prompt; RED stops downstream work pending a consumption-model rethink

## 8. Verification

- [ ] 8.1 `openspec validate agent-ab-efficacy` passes; README documents every command needed to reproduce from a clean checkout (build binary → clone repos → build tasks → smoke → full → report)

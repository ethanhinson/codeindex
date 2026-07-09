# agent-ab-efficacy harness

A/B measurement of real Claude Code agents on real tasks, **with** vs **without**
the `codeindex` tool. Implements the OpenSpec change `agent-ab-efficacy`.

> This README records **verified environment reality** (task group 1). Where it
> differs from the change's `design.md`, reality wins and the spec was corrected.

## Verified environment (as of build)

- **codeindex binary:** built by `go build -o bench/agent_ab/.bin/codeindex ./cmd/codeindex`
  (Go 1.26.5, CGO). Lives OUTSIDE any target repo tree (arm A cannot discover it).
  Repo build hash recorded per run via `git rev-parse HEAD`.
- **claude CLI:** version **2.1.193**. Verified flags:
  - `-p/--print` — headless print mode.
  - `--output-format stream-json --verbose` — required together in print mode;
    emits one JSON object per line (events).
  - `--allowedTools "Bash Read Grep Glob"` — comma OR space separated; supports
    scoping like `Bash(git *)`. Arm policy: allow `Bash Read Grep Glob`, and
    additionally deny `Edit Write` via `--disallowedTools`.
  - `--permission-mode bypassPermissions` — needed for non-interactive tool use
    (no approval prompts). Safe here: tasks are read-only and run on throwaway
    clones that are `git checkout`-reset after every run.
  - `--append-system-prompt "<text>"` — inline text (arm B only). We read
    `prompts/arm_b_tools.md` and pass its contents.
  - `--model <alias>` — e.g. `claude-sonnet-4-6`.
  - **`--max-turns` DOES NOT EXIST in 2.1.193.** (design.md assumed it.) We bound
    runs with a per-run subprocess **timeout** (kill + flag) plus the global
    `--budget-usd` guard instead.

## Metrics (verified from a real stream-json probe)

The final `type:"result"` event carries everything:
- `total_cost_usd` — **PRIMARY metric.** Prices input/output/cache correctly.
- `num_turns`, `duration_ms`, `is_error`, `subtype`, `result` (final answer text).
- `usage`: `input_tokens`, `output_tokens`, `cache_creation_input_tokens`,
  `cache_read_input_tokens`, plus `modelUsage` per-model.

**Measurement correction (important):** the spec originally paired on
`input+output tokens`. That is WRONG for this experiment — when the agent reads a
file, the content lands in `cache_creation_input_tokens`, not `input_tokens`
(probe showed input=4 while a file read is thousands of cache-creation tokens).
So we report:
- **Primary:** `total_cost_usd` (correct, cache-aware).
- **Secondary "processed tokens":** `input_tokens + output_tokens +
  cache_creation_input_tokens` (where file-reading cost actually shows up;
  excludes `cache_read` which is dominated by the fixed ~42k-token Claude Code
  system prompt replayed each turn and would dilute the signal).

**Tool calls / adoption:** count `tool_use` blocks in `type:"assistant"` events'
`message.content`, keyed by `name`. codeindex adoption = count of `Bash`
tool_use whose `input.command` contains the codeindex binary path.

A verified probe transcript is archived at `results/probe/probe1.jsonl`.

## Layout (as built)

```
.bin/codeindex            built binary (gitignored)
build_tasks.py            task generator (comprehension + localization)
prompts/arm_b_tools.md    arm-B appended system prompt (the treatment)
run_ab.py                 the A/B runner (matrix, isolation, capture)
grade.py                  arm-blind script grader
report.py                 paired stats + verdict
tasks/tasks.json          frozen task set + pre-registered thresholds
results/                  runs.jsonl, transcripts/, report.md, probe/
cache/                    cached GitHub API responses
```

## Run (once built)

```
# 1. build binary + indexes, generate tasks
go build -o bench/agent_ab/.bin/codeindex ./cmd/codeindex
python3 bench/agent_ab/build_tasks.py            # writes tasks/tasks.json
# 2. smoke first (2 tasks x 2 arms x 1 rep) — inspect transcripts
python3 bench/agent_ab/run_ab.py --smoke
# 3. full run within budget, then analyze
python3 bench/agent_ab/run_ab.py --full --budget-usd 60
python3 bench/agent_ab/report.py                 # writes results/report.md
```

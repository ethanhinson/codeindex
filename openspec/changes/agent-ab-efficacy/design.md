## Context

This document is written to be **self-contained for an implementer with no
prior session context**. Read this and the spec, and you can build the harness.

### What exists already (verify each before building)

- **The `codeindex` binary** — Go, built with `go build -o <path> ./cmd/codeindex`
  from the repo root (requires Go 1.24+ and a C toolchain; CGO). Subcommands:
  - `codeindex build <repo-root>` — indexes all `.go` files (skips `.git`,
    `vendor`, `node_modules`, `testdata`, `.codeindex`) into
    `<repo-root>/.codeindex/graph.db`. Prints
    `built <db>: N files, M symbols in T`.
  - `codeindex query <repo-root> <symbol> [--limit N]` — prints, exactly:

    ```
    def  model/labels/labels.go:220  func (ls Labels) HasDuplicateLabelNames() (string, bool)
    def  <path>:<line>  <signature>            (one line per definition)
    callers (11):
      storage/remote/write_handler.go:246  write  [ambiguous]
      <path>:<line>  <caller-name>[  [ambiguous]]
      ... (+K more; use --limit)              (when caller count exceeds limit)
    ```

    If the symbol has no definition: `def  <symbol>: (not found in index)`.
    IMPORTANT: `query` does NOT auto-build or refresh the index — the harness
    must run `codeindex build` beforehand and must not edit indexed files
    afterward.
  - Go-only, name-based resolution (a caller list for a common method name
    includes same-named matches, flagged `[ambiguous]`). Tasks must therefore
    target Go repositories.
- **Pinned repo clones** — `bench/repos.json` pins gin (`v1.10.0`),
  prometheus (`v3.1.0`), kubernetes (`v1.32.0`) with URLs; `bench/token_bench.py`
  contains `ensure_repo()` showing the shallow-clone-at-pin recipe. Reuse it
  (`import token_bench`) or replicate it.
- **`claude` CLI** — installed locally. Verify flags with `claude --help`
  before relying on them; the intended invocation is (adjust to the installed
  version's actual flags):

  ```
  claude -p "<task prompt>" \
    --model <model> \
    --max-turns <N> \
    --allowedTools "Bash,Read,Grep,Glob" \
    --append-system-prompt "$(cat <arm prompt file>)" \   # arm B only
    --output-format stream-json --verbose
  ```

  The final message of `--output-format json` (or the `result` event of
  `stream-json`) carries `total_cost_usd`, `usage` (input/output/cache token
  counts), and `num_turns`. Tool-call counts come from parsing the stream-json
  events: count `tool_use` blocks in assistant messages, keyed by tool name;
  count arm-B codeindex adoption by matching Bash `tool_use` inputs containing
  the codeindex binary path.
- **GitHub REST API** — unauthenticated works at low volume; support
  `GITHUB_TOKEN` from the environment or `bench/.env` (see
  `token_bench.load_dotenv()`) to avoid rate limits.

### Why this experiment exists

Static analysis proved index answers are 6–363× smaller than the files an agent
*might* read. It cannot prove (a) **adoption** — that an agent chooses the tool
mid-task, or (b) **sufficiency** — that reference-only answers don't trigger
compensating file reads. Both are behavioral. This harness measures them.

## Goals / Non-Goals

**Goals**

- Produce paired A/B measurements of real agent runs on a fixed task set:
  total tokens, cost, turns, tool-call mix, codeindex adoption, task score.
- Arm-neutral, script-computable ground truth (no LLM judging in v1).
- Pre-registered decision thresholds recorded before the first full run.
- Reproducible: pinned repos, pinned task file, archived transcripts, one
  command to re-run the analysis from stored results.

**Non-Goals**

- Engine changes, MCP server, edit/fix tasks, LLM-judge grading, Windows.

## Decisions

**D1 — Headless Claude Code (`claude -p`), not the raw API.** The goal is to
measure Claude *as agents actually run* — with its real tool loop, system
prompt, and Grep/Read habits. A hand-rolled API loop would measure a strawman.
*Alternative:* Agent SDK — acceptable, but `claude -p` is already installed and
matches production behavior exactly.

**D2 — Arm B gets the CLI via Bash + appended system prompt; no MCP yet.**
Cheapest faithful integration: document the binary (absolute path) and when to
use it in an appended system prompt; the agent calls it through Bash. This
under-sells codeindex slightly versus a native MCP tool (harder discovery) —
acceptable: it biases *against* the treatment, so a positive result is robust.
*Alternatives:* MCP server (build cost, later change); CLAUDE.md injection
(pollutes the repo checkout; system-prompt append is cleaner and per-arm).

**D3 — Two task types, both with arm-neutral ground truth.**
- *Comprehension* (symbol navigation): ground truth from **ripgrep**, not from
  codeindex — definition location verified by an rg pattern match on the
  definition line, referencing files from `rg -w -l`. Using the index to grade
  would bias toward arm B; rg is equally available to both arms.
- *Localization* (issue → files to change): ground truth = files changed by the
  **merged PR that closed the issue** (GitHub API), filtered to `.go` files in
  the pinned tree. Fully external to both arms.
*Alternative rejected:* grading with an LLM judge — subjective, and the whole
project ethos is script-verifiable evidence.

**D4 — Paired design with repetitions.** Every task runs in both arms (paired),
R times each (default R=2), with per-task paired deltas as the unit of
analysis. Agent runs are stochastic; medians over reps + bootstrap CIs over
tasks handle it. Order randomized per rep with a fixed seed.

**D5 — Isolation and reset between runs.** Tasks are read-only by tool policy
(no Edit/Write in `--allowedTools`), but defense-in-depth: run each task with
cwd = the pinned clone, and after each run `git -C <repo> checkout -- . `
plus verify `git status --porcelain` is empty (excluding `.codeindex/`, which
is gitignored and must persist for arm B). The codeindex binary lives OUTSIDE
the repo tree at an absolute path so arm A cannot discover it by listing files.

**D6 — Intent-to-treat AND per-protocol analysis.** Report both: (a) all arm-B
runs regardless of whether the agent used codeindex (measures the product as
deployed — skill discoverability included), and (b) only arm-B runs with ≥1
codeindex invocation (measures the tool's effect when used). A large gap
between them is itself a finding: the tool works but the prompt/skill doesn't.

**D7 — Pre-registered thresholds (record in tasks.json header before the full
run; do not change after). Savings = median paired reduction in `total_cost_usd`
(primary metric — see D-metric below):**
- **GREEN** — proceed to MVP + plugin: savings ≥ 30%, task-success delta
  ≥ −5 pp, arm-B adoption ≥ 70%.
- **YELLOW** — iterate on the arm-B prompt / answer format and re-run once:
  savings 10–30%, or adoption 40–70% with per-protocol savings ≥ 30%.
- **RED** — stop and rethink the consumption model before building more:
  savings < 10% intent-to-treat, or success delta < −5 pp, or adoption < 40%
  with per-protocol savings also < 30%.

**D-metric — cost is primary, processed-tokens secondary (verified by probe).**
The naive "input+output tokens" metric misses the effect: reading a file lands
in `cache_creation_input_tokens`, not `input_tokens`. So the headline is
`total_cost_usd`; the secondary token metric is
`input+output+cache_creation_input_tokens`; `cache_read` is recorded but excluded
(fixed system-prompt overhead). See `bench/agent_ab/README.md`.

**D-turns — no `--max-turns` in claude 2.1.193.** Run length is bounded by a
per-run wall-clock timeout (kill + flag) plus the global `--budget-usd` guard.

**D8 — Cost controls.** Default model `claude-sonnet-4-6` (configurable);
`--max-turns 15`; task cap via `--tasks N`; reps via `--reps`; **mandatory
smoke mode** (`--smoke` = 2 tasks × both arms × 1 rep) that must be run and
eyeballed before the full matrix; a hard `--budget-usd` that aborts when the
summed `total_cost_usd` exceeds it. Full-matrix estimate at defaults:
24 tasks × 2 arms × 2 reps ≈ 96 runs; assume $0.15–$0.50/run ⇒ ~$15–50.

**D9 — Repos: gin + prometheus for the main matrix; kubernetes excluded from
v1** (giant checkouts make agent Grep/Glob slow and would dominate wall-time;
add as a stretch tier once the harness is proven).

## Risks / Trade-offs

- **Arm A "wins" by answering from priors without reading code** → the prompt
  demands file:line evidence; grading requires exact paths that exist in the
  pinned tree; unsupported answers score 0.
- **Comprehension tasks favor whichever arm greps better, not codeindex
  specifically** → that is the honest comparison: codeindex must beat the
  agent's native workflow, not a strawman.
- **Symbol selection bias** → symbol picker is scripted with fixed seed and
  published filters (see spec); includes hot/ambiguous symbols, not only clean
  ones.
- **`claude` CLI flag drift across versions** → task 1 verifies flags against
  `claude --help` and records the CLI version in every result row.
- **Localization ground truth is fuzzy** (PRs sometimes touch incidental files)
  → grade with F1 (not exact match), keep only issues whose PR changed 1–10
  `.go` files, and require ≥ 60% of PR-changed files to still exist at the
  pinned commit (else drop the task).
- **Transcript/token accounting differences between arms** (system prompt of
  arm B is larger) → report the appended-prompt token count separately; it is
  part of the treatment's true cost and stays in the totals.

## Migration Plan

Additive. New `bench/agent_ab/` directory; nothing existing changes. Delete the
directory to roll back.

## Open Questions

- Whether to add an edit-task phase (grade by compiling/tests) after v1 — keep
  as stretch; do not block on it.
- Whether arm B should also get a worked example in the appended prompt
  (few-shot) — v1 says yes, one example, because tool docs without an example
  measured poorly in prior agent-tooling experience; the example is part of the
  registered treatment.

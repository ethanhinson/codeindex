## Context

Engine status: `codeindex build|query/callers|callees|bench` works on real repos
(Go, name-based, incremental==full proven), but queries read a **static** index
— no re-check on the query path yet. The A/B harness (`bench/agent_ab/`) is
built, validated, and tagged (`--tag`), with claude CLI 2.1.193 flags verified
(`--plugin-dir` exists; no `--max-turns`). Evidence base: v1 RED (+17% on
locate), v2 GREEN (−73% on branch-out), full findings in `bench/agent_ab/`.

Full design rationale: `docs/superpowers/specs/2026-07-09-skill-and-ide-integration-design.md`.

## Goals / Non-Goals

**Goals**

- Ship the anchor rule as a Claude Code plugin (skill + commands + edit-hook)
  and prove with a pre-registered v3 gate that it captures branch-out savings
  without locate regressions.
- Make queries always-fresh (Phase 0) so refactoring workflows are correct.
- Expose the same queries to IDEs via a concurrency-safe MCP server.

**Non-Goals**

- New languages, precise resolution, dependents edges (other changes).
- LSP/native IDE extensions; Windows.

## Decisions

**D1 — The anchor rule is the product.** The skill's primary content is trigger
discipline, stated positively (known anchor → branch out) and negatively (never
for locating). Basis: v1/v2 measured both sides of the boundary. The same rule
text is embedded in MCP tool descriptions so IDE agents inherit it.

**D2 — Phase 0 freshness is a blocking prerequisite.** Refactoring = querying
while editing; a static index gives wrong answers mid-refactor. Queries run the
incremental patch first (auto-build if missing). Measured cost: 3.9–119 ms per
single-file patch — affordable on every query. *Alternative:* document "run
build first" — rejected; agents will forget, and wrong answers destroy trust.

**D3 — Edit-hook via git-diff hunks → `enclosing` lookup.** The Edit tool's
hook payload carries the file path but not line numbers, so the hook script
derives changed ranges from `git diff -U0` and asks the engine which symbols
enclose them. The query path's freshness (D2) guarantees the index already
reflects the edit. *Alternative:* parse old_string/new_string offsets from the
hook payload — brittle across tools (Write, MultiEdit) and editors.

**D4 — Hook noise controls are requirements, not niceties.** Injection only
when the enclosing symbol has ≥1 caller outside the edited file; once per
symbol per session (dedup state under `.codeindex/`); ≤150-token cap; plugin
setting to disable. A noisy hook would recreate the v1 overhead in a form the
agent can't even decline.

**D5 — `/impact` composes; primitives stay.** `/impact <symbol>` = callers +
callees (+ dependents when the engine grows them), counts-first bounded
summary. Primitives (`/callers`, `/callees`) remain for targeted use. Commands
are thin wrappers over the CLI so the plugin has no logic to drift out of sync.

**D6 — v3 gate runs the real artifact.** Arm B uses `--plugin-dir` with the
actual plugin — not a simulated system prompt — so packaging, skill wording,
and hook behavior are what's measured. Thresholds pre-registered in the task
file header (same discipline as v1/v2): locate regression ≤10%, branch-out
savings ≥50%, hook fire-rate ≥80% on symbol edits / 0 on non-symbol edits.

**D7 — MCP server is a thin adapter with a write mutex.** `codeindex mcp`
(official Go MCP SDK, stdio) maps tools 1:1 onto existing query functions; a
single in-process mutex serializes re-check writes because the server is
long-lived while SQLite is single-writer. *Alternative:* WAL + busy-timeout
tuning — heavier than needed for a per-repo local server.

## Risks / Trade-offs

- **Skill wording fails silently** (agent ignores or over-triggers) → that is
  exactly what the v3 gate measures; iterate wording once (registered) if
  YELLOW.
- **Hook latency on huge repos** → enclosing query includes the lazy patch;
  measured ≤119 ms on kubernetes-scale — acceptable inside a hook. If a repo
  exceeds budget, the hook's off switch is the escape hatch.
- **`/impact` without dependents edges under-reports blast radius** → the
  summary labels what it covers (calls in/out) and says dependents are pending;
  no false completeness.
- **Untracked files break git-diff hunk detection** → hook skips silently
  (no injection is always safe).
- **MCP SDK churn** → pin the SDK version; the adapter surface is 3 small tools.

## Migration Plan

Additive throughout. Phase 0 changes query behavior (static → fresh) — strictly
more correct; `codeindex bench` keeps its own explicit build calls so benchmark
comparability is unaffected. Rollback = remove `plugin/` and the `mcp`
subcommand.

## Open Questions

- Should the hook also fire on file *creation* (Write of a new .go file)?
  Default v1: yes if the new file's symbols are called elsewhere (rare), else
  silent.
- Whether `/impact` should accept `file:line` anchors in addition to symbol
  names — deferred until the `enclosing` lookup proves itself in the hook.

---
id: 6
slug: delta-impact-query-mode
title: Delta impact query mode — what changed in the blast radius since X
status: proposed
priority: high
type: feat
created: 2026-08-04
updated: 2026-08-04
depends_on: []
related: [5]
discovered_from: []
adrs: []
spec:
plan:
results:
trivial: false
auto_groomable:
branch:
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
<!-- docket:artifacts:end -->

## Why

An agent doing a refactor runs the same "who calls this / what breaks?" query repeatedly as it edits. Today each query returns the full impact set, and the agent has to diff two large lists in-context to see what its last edit actually moved. That is exactly the re-read the tool exists to eliminate, paid back on every loop iteration.

Idea surfaced while mining the LLM-research vault: the ReAct edit→re-check loop (Change2Task, SWE-Touch on agents editing mid-task) and the "analytic memory / delta over observations" framing both point at returning the *change* in state rather than the whole state. This stays fully on-brand — deterministic, references-only — because codeindex already has the incremental engine and per-file hashes to compute a before/after cheaply.

## What changes

Add a query mode that returns the **diff** of a symbol's impact set between two index states — e.g. "since commit X", "since the last time I asked", or "since the working tree was clean":
- Added dependents (new callers now in the blast radius), removed dependents, and unchanged.
- Expressed in the same compact `file:line` + signature contract as today's output.
- Exposed across the surfaces where the edit loop lives (graph API, MCP; `/impact` variant in the plugin).

## Out of scope

- Persisting agent session state server-side (that is the separate session-cache change, 0007) — the first cut can take an explicit baseline ref.
- Semantic "why did it change" interpretation — this reports structural add/remove only.

## Open questions

- Baseline addressing: git ref, a saved index snapshot id, or both?
- Does the MCP surface hold two index versions, or recompute the baseline on demand from the incremental engine?
- How to present a symbol that was renamed vs. one that was genuinely added/removed.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

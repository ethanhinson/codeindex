---
id: 16
slug: discovery-pins-separate-manifest-from-mined-corpus
title: Member-discovery pins live in a separate manifest from the mined bench corpus
status: Accepted
date: 2026-08-22
supersedes: []
reverses: []
relates_to: [15]
change: 10
---

## Context

The workspace bench has two distinct concerns that both want a list of repository members:

1. **The mined task corpus.** `bench/workspace/corpus.json` names the members that
   `build_tasks_ws.py` mines cross-repo tasks from. Its output,
   `bench/workspace/tasks/tasks_ws.json`, is FROZEN, and a pre-registered gate (bars B1–B5 in
   `bench/workspace/README.md`) states per-shape and per-language task counts computed from that
   exact freeze. Bar B5 is explicitly a freeze-discipline bar: per-shape n and any excluded shape
   are pinned at freeze and never revised after mining.

2. **Member-discovery coverage.** Change 0010's secondary goal is to show that
   `init-workspace --scan` recognizes every supported declaration format (go.work,
   pnpm-workspace.yaml, npm/yarn workspaces, composer path repositories, lerna.json, Python
   multi-member). This wants organically-declared monorepo repos pinned as evidence, and has
   nothing to do with task mining.

The obvious move — adding the discovery pins to `corpus.json` — is a trap. `build_tasks_ws.py`
mines every member in `corpus.json`, so a new member silently changes the candidate set, the
emitted tasks, the per-shape counts, and therefore the numbers in a gate that was already
registered. That would invalidate the freeze without anyone editing the freeze, and would do it as
a side effect of work explicitly scoped as secondary and non-blocking.

## Decision

Discovery pins live in their own manifest, `bench/workspace/discovery_corpus.json`, documented as
discovery-only and NOT read by `build_tasks_ws.py`. The mined corpus manifest `corpus.json` is
changed only by a deliberate act that also re-freezes the task set and re-registers the affected
bar numbers.

## Consequences

- **Enables:** phase-2 discovery coverage can grow, and can land after the gate registration and
  even after a scored run, without touching the frozen corpus or any registered number. Verified in
  practice: adding four discovery pins left the mined corpus byte-identical.
- **Costs / gives up:** two member manifests now exist, so a reader must know which one governs
  what; they can drift in the sense of describing overlapping repo sets for different purposes. The
  separation is intentional and each file states its role.
- A future change that promotes a discovery pin into the mined corpus MUST re-freeze the task set
  and update the registered bars in the same act — it is never a one-line manifest edit.
- Recorded outcome of the first pass: 4 of 6 formats covered (lerna.json, composer path repos,
  pnpm-workspace.yaml, npm/yarn workspaces); go.work UNCOVERED after exhausting its registered
  three-candidate bound; Python multi-member UNCOVERED for a structural reason worth carrying —
  `internal/workspace/members.go` reads five declaration sources and none is Python
  (`pyproject.toml` / `setup.py` / `setup.cfg` appear only as member CONFIRMATION markers, never as
  declarations), so no Python monorepo can be discovered by `--scan` without an engine change.

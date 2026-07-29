---
id: note-01KYR17XECYXPGDZPKC4HQDSEA
title: docket (danielhanold) comparison — gaps both directions, adoptable ideas
date: 2026-07-29
tags: [competitive-analysis, roadmap]
refs:
    - url: https://github.com/danielhanold/docket
---
docket is the closest existing system to our lore engine: in-repo Markdown
work tracking + skills for Claude Code/Cursor/Codex, explicitly positioned
against OpenSpec (validates our replace-openspec bet). Key structural
difference: docket is CLI-less (bash scripts + skill prose, no DB, no MCP);
we are an engine (Go binary, SQLite index, MCP tools, symbol graph).

## What docket has that we lack

- Claim/lease protocol for multi-agent draining: CAS on status via git push
  (non-FF → re-read → re-claim), claimed_at heartbeat re-stamped at phase
  boundaries, reclaim of expired leases with no branch (self-healing queue).
- Reconcile step: just-in-time rewrite of a stale work item against current
  code/decisions/archive after claim, before build; dated reconcile log;
  kill-if-obsolete / escalate-if-invalidated escape hatches. We only detect
  staleness; docket repairs it.
- Learnings promotion pipeline: findings carry hook (searchable one-liner),
  topics, provenance (change ids), war-story appendix; promotion_state
  retained → candidate → promoted into AGENTS.md/CLAUDE.md; cap flags
  needs-curation, never auto-merges.
- discovered_from provenance field; spec/plan/results/branch/pr links on items.
- Generated BOARD.md with readiness cells ("waiting on #N — needs your merge").
- Two-branch metadata mode: orphan docket branch for planning churn,
  terminal-publish file-copy of done records onto the code branch. NOTE:
  conflicts with our ratification-by-merge (presence on default branch =
  ratified) — only relevant if we adopt high-churn execution state.
- Per-skill model/effort pinning with four-layer config (local > committed >
  global > built-in) and generated per-harness wrappers.

## What we have that docket lacks

- Symbol-graph anchors: code-aware records, staleness detection, lore-for-
  path/symbol queries, related_lore on impact. docket has zero code awareness.
- Ranked search with decay/status/confidence; docket's read contract is
  "load the index file, pick findings whose hook matches" — model-side.
- MCP tool surface (docket is skills-only prose contracts).
- Session/ambient capture layer with temporal decay; private overlay
  (docket is all-shared, harvest-at-close-out only).
- Decision evidence (survival/churn confidence) and external-ref search.

## Adoption candidates (roadmap discussion, not yet committed)

1. Claim/lease fields + reclaim on items (enables parallel-agent draining).
2. Reconcile skill in Plan 2/3 — powered by our anchor-staleness data,
   which makes docket's reconcile cheaper and more targeted.
3. promotion_state on notes + /lore-review graduation into AGENTS.md.
4. discovered_from on items; branch/pr fields (Plan 3 already adds refs).
5. `lore board` rendered view (derived, never source of truth).

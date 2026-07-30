---
id: note-01KYR17XECYXPGDZPKC4HQDSEA
title: docket (danielhanold) comparison — gaps both directions, adoptable ideas
date: 2026-07-29
tags: [competitive-analysis, roadmap]
related: [dec-01KYTG2C8BPFS0GV787Y8AA4QM]
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

## Update 2026-07-30 — drift check after the knowledge-graph-edges work (PR #1)

Re-verified this note against the current backlog and code.

**Still accurate — every "what docket has that we lack" gap remains open** (backlog
confirms each is `ready`): claim/lease (itm-…RHT), reconcile skill (itm-…41J),
promotion pipeline / promotion_state (itm-…1VY; the `promote` command is
private→committed promotion, not notes-graduation), `lore board` (itm-…9SC),
per-skill model/effort pinning. "We only detect staleness; docket repairs it"
is still true — `lore doctor` now adds orphan/dangling-link/density detection,
but still no auto-repair.

**Drifted (undersells us) — "what we have that docket lacks" is now bigger.**
Since this note was written we shipped a record↔record knowledge graph docket has
no analog for (ref: [[dec-01KYTG2C8BPFS0GV787Y8AA4QM]], PR #1):
free-form `related:` edges + backlinks (`lore related`, "Referenced by:" in
`lore show`), a full-trace cycle-safe `Trace` walk, graph-health metrics
(orphans / dangling-links / edges-per-record) in `lore doctor`, and `related_lore`
now surfaced in the CLI `impact` command, not just MCP. We went from "code-aware
records" to "code-aware AND a connected, health-measured knowledge graph."

**Minor:** adoption candidate #4's branch/pr links are now demonstrably usable via
typed refs (`gh-pr: owner/repo#N`, recorded on itm-…HVD for PR #1); only
`discovered_from` remains unbuilt from that item. Nothing in the original analysis
is false — the drift is omission, not error.

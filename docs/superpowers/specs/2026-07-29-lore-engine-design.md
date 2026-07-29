# Lore Engine — Design

**Date:** 2026-07-29
**Status:** Approved design, pre-implementation
**Approach:** Extend codeindex (same binary, same MCP server, same plugin)

## Problem

Agent sessions constantly rediscover things the project already knows: why the
code is shaped the way it is, what was tried and rejected, what work is already
known to be needed. Existing agent memory systems (studied in depth via the
Grok Build source tree) store *lore* but not *decisions*: they keep summaries
in a per-user home directory, discard rejected alternatives during
consolidation, have no provenance linking knowledge to code changes or issues,
and cannot be shared through the repository. Meanwhile the code itself is
versioned, branched, merged, and reviewed through git — decisions and project
knowledge should be too.

## Goal

A decision/lore/backlog layer for codeindex that is:

1. **Git-native** — team knowledge lives in the repo, versioned and merged like
   code; knowledge enters the shared record via diff review (PRs).
2. **Symbol-anchored** — records attach to files and symbols in the existing
   code graph, so knowledge surfaces during impact analysis and goes stale
   detectably when anchored code changes.
3. **Host-portable** — adopted by Claude Code, Cursor, and Codex through their
   standard extension surfaces (MCP tools + skills/rules + hooks), with no new
   protocol. The engine is a sidecar to the host's agent, never a replacement.

**v1 success criterion:** dogfood. Daily use across the author's own repos in
Claude Code/Cursor measurably reduces re-explanation and gets recorded
decisions respected in later sessions.

## Non-goals (v1)

- No embeddings/vector search (BM25 over structured records; embeddings are a
  v2 flag if recall proves insufficient).
- No background daemons, watchers, or webhooks (lazy reindex on query, matching
  codeindex's freshness model).
- No bidirectional tracker sync service; no Jira/Linear API adapters (links +
  explicit `gh`-based reconcile only; full `SyncProvider` adapters are v2).
- No ACP agent mode (the engine is a sidecar; ACP only becomes relevant if the
  product later becomes an agent itself).
- No multi-user identity/approval chains.

## Architecture overview

```
.lore/  (committed Markdown)          ~/.codeindex/lore/<repo-id>/  (private overlay)
        \                                    /
         └── lazy reindex (content hashes) ─┘
                        │
        .codeindex/graph.db  (existing SQLite; new lore tables + FTS5)
                        │            ↑ joins against existing symbol tables
        ┌───────────────┼─────────────────────┐
   CLI subcommands   MCP tools (existing   plugin skills/hooks
   (codeindex lore *)  server, new lore_*   (Claude Code) + rules
                       family; related_lore  snippets (Cursor, Codex)
                       on impact/callers)
```

New internal packages: `internal/lore` (records, layers, lifecycle),
`internal/lore/index` (tables, FTS, ranking), `internal/lore/signals`
(git-derived evidence), `internal/lore/sync` (gh reconcile). CLI and MCP wiring
extends `cmd/codeindex` and `internal/mcpserver`.

## Data model

Three record types. All are Markdown files with YAML frontmatter, **one file
per record** so parallel branches/agents rarely conflict on merge.

```
.lore/
  decisions/2026-07-29-use-go-runtime.md
  items/2026-07-29-migrate-resolver-tests.md
  notes/tree-sitter-cgo-gotcha.md
```

### decision

```yaml
---
id: dec-01K1B2M3N4P5Q6R7S8T9V0W1X2   # ULID: sortable, collision-free across branches
title: Use Go for the engine
status: active                        # active | superseded | rejected
date: 2026-07-29
supersedes: dec-01J9X4...             # optional; superseded record gains superseded_by on write-back
anchors:
  - path: internal/engine/
  - symbol: ResolveImports
refs:
  - gh-issue: ethanhinson/codeindex#12
  - commit: b5b38ba
---
Rationale prose.

## Alternatives considered
Rust — rejected because ...
```

The `## Alternatives considered` section is a first-class convention: rejected
options and why are exactly what memory-consolidation systems discard and what
future sessions need most.

### item (backlog entry)

Same shape, plus backlog fields; status is `open | done | dropped`:

```yaml
priority: p2          # p0..p3
blocked_by: [itm-01K...]
tags: [tech-debt]
```

### note

Informal lore: gotchas, conventions, context. No status machine; evergreen.

Record IDs are ULIDs prefixed by type: `dec-`, `itm-`, `note-`.

### Layers

| Layer | Location | Shared | Decay |
|---|---|---|---|
| Committed | `<repo>/.lore/` | via git | none (evergreen) |
| Private | `~/.codeindex/lore/<repo-id>/` (same subdirs) | no | none for curated notes |
| Sessions | `~/.codeindex/lore/<repo-id>/sessions/*.md` | no | 7-day half-life in ranking |

`<repo-id>` is a hash of the origin remote (normalized `org/repo`), falling
back to the filesystem path for non-git or remote-less repos — all clones and
worktrees of a repo share one overlay.

`codeindex lore promote <id>` moves a private record into `.lore/` (deletes
from overlay, writes to repo, leaves committing to the user/agent). Promotion
is the curation gate: knowledge enters the shared record as a reviewable diff.

## Indexing & search

Lore rides the existing `.codeindex/graph.db`:

- **Tables:** `lore_records` (id, type, status, title, file path, date, layer,
  stale, confidence), `lore_anchors` (record → path/symbol, resolved against
  existing symbol tables), `lore_refs` (typed external refs), `lore_events`
  (external evidence feed), and `lore_fts` (FTS5 over title+body, chunked on
  `##` headings so long records stay findable in pieces).
- **Freshness:** every lore query first diffs `.lore/` + overlay against
  stored per-file content hashes (the same flat-hash approach validated for
  code indexing) and re-indexes only changed records. No daemon.
- **Ranking:** FTS5/BM25, modified by: layer decay (sessions half-life 7 days;
  committed and curated notes evergreen), status (`superseded`/`done` rank
  below `active`/`open` and are labeled), and the confidence score from
  lifecycle signals (below).
- **Staleness:** during reindex, symbol anchors are re-resolved; records whose
  anchored symbols no longer exist get `stale: true`, surfaced in results and
  by `lore doctor`.

## Query surface

### CLI (all support `--json`; output is compact references, matching existing codeindex contract)

```
codeindex lore add <type>                 create record (flags or stdin frontmatter)
codeindex lore search <query>             FTS across layers, ranked, labeled (layer/status/stale/unratified)
codeindex lore show <id>                  full record
codeindex lore for <path|symbol>          records anchored to this code (the graph join)
codeindex lore backlog [--for <anchor>]   open items: p0 first, unblocked first, then age
codeindex lore promote <id>               private → committed
codeindex lore supersede <old-id>         create replacing decision; back-links both ways
codeindex lore capture --stdin            hook entry point: ingest session context into sessions/
codeindex lore event --type <t> ...       CI/deploy evidence ingestion
codeindex lore sync github                explicit reconcile via gh CLI (pull state; push via `lore push <id>`)
codeindex lore doctor                     stale anchors, broken refs, orphaned supersedes, unratified records
codeindex lore init                       scaffold .lore/, rules snippets, optional git hooks, per-host setup
```

### MCP (existing server, new tool family)

`lore_search`, `lore_add`, `lore_show`, `lore_for_symbol`, `lore_backlog`,
`lore_promote`.

**Flagship join:** `impact` and `callers` responses gain a `related_lore`
field — active decisions, relevant notes, and open items anchored to the
queried symbols/files. Knowledge reaches the agent at the moment it is about
to change the code the knowledge concerns, without the agent asking.

## Capture

Two channels, both shipping in v1.

**Deliberate (skills/rules).** A `lore` skill in the existing Claude Code
plugin instructs the host agent to record: after an architectural choice,
after a debugging session resolves with a non-obvious root cause, when the
user corrects an assumption, when the user says "remember this"/"we decided".
Records are written while the agent has full context, anchored to the symbols
just worked on, including alternatives rejected. Equivalent rules ship as
`.cursor/rules/lore.mdc` and an `AGENTS.md` snippet — one behavioral contract,
three dialects. Slash commands: `/decide`, `/lore`, `/lore-review`.

**Ambient (hooks).** Every hook is a one-line shell call into the binary;
per-host adapters are config only:

- Claude Code `Stop` → `codeindex lore capture --stdin`: cheap metadata
  summary of the turn into `sessions/` (no LLM call).
- Claude Code prompt-context hook (extends the plugin's existing
  `prompt_context.py`) → inject top-ranked lore for files in context on the
  first relevant turn.
- git `post-commit` (optional, installed by `lore init`) → link commit SHAs to
  records referenced by the commit.

Cursor and Codex get the deliberate channel fully; ambient capture only via
git hooks (their hook systems are weaker). Accepted v1 asymmetry.

**Promotion review.** `/lore-review` (or periodic skill prompting) proposes
promoting recurring session themes into committed records — consolidation
whose output is a PR-reviewable file rather than a silent merge.

## Lifecycle signals & evidence

Signals split into **durable transitions** (written back to frontmatter — the
change is itself a git-auditable edit) and **derived evidence** (index-only,
recomputed from git history during lazy reindex). No daemons or webhooks; all
git-derived signals come from `git log` since the last indexed commit.

- **Ratification by merge (structural, free):** a record file present on the
  default branch is ratified — it arrived via a merged PR or direct commit.
  Records existing only on a branch are labeled `unratified` in results.
  A PR closed unmerged never ratifies its records.
- **Commit references → item transitions (durable):** commit messages
  containing `closes <item-id>` (idiom of `fixes #12`) mark the item
  `status: done` and append the commit to `refs`, written back to the file.
- **Survival → confidence up (derived):** merged commits touching anchored
  code while the decision stands (not edited/superseded) increase confidence.
  Output shows the evidence: `confidence: high (survived 14 merges)`.
- **Churn → staleness suspicion (derived):** heavy modification of anchored
  code since the record date (default threshold: >60% of anchored lines
  changed; tunable) lowers confidence and is flagged by `doctor`.
- **External events (explicit ingestion):** CI runs
  `codeindex lore event --type deploy --status ok --commit <sha>`; events
  append to `lore_events` and attach evidence to records whose refs/anchors
  trail through that commit. Richer PR-metadata ingestion is v2.

## Backlog & third-party services

The backlog is a query view over items, not a separate store. Committed items
are the team backlog (work discovered on a branch arrives with its PR);
overlay items are a personal queue, promotable. `lore backlog --for <anchor>`
answers "what is already known to need doing in the code I am touching" —
a question no external tracker can answer by code-anchor.

Integration tiers:

1. **Links (core, zero API):** typed `refs` (`gh-issue:`, `jira:`, `url:`),
   searchable both directions (`lore search gh-issue:12` → the decisions and
   items behind a ticket).
2. **Explicit reconcile via `gh` (v1):** `lore sync github` on demand — pulls
   issue/PR state for GitHub refs (closed issue ⇒ item `done`, durable
   write-back with provenance); `lore push <id>` creates a GitHub issue from
   an item and writes the ref back. Uses `gh`'s existing auth; no token
   management.
3. **Zero-integration integration (v1, behavioral):** host agents already have
   GitHub/Jira/Linear MCP tools; the lore skill instructs: when filing or
   referencing an external ticket, add its ref to the related record.

Full provider adapters behind a Go `SyncProvider` interface: v2, only if tier
3 proves insufficient.

## Host integration & packaging

- **Claude Code:** extend existing `plugin/` — skill, two hooks, slash
  commands.
- **Cursor:** `.cursor/mcp.json` entry pointing at `codeindex mcp` + rules
  file; git hooks for ambient capture.
- **Codex:** MCP registration in `config.toml` + `AGENTS.md` snippet.
- `codeindex lore init` scaffolds all of the above per repo, interactively.

One binary, one MCP server per host — lore tools appear beside navigation
tools with no additional host configuration.

## Error handling

Fail-open, never block the host agent:

- Index locked/corrupt during capture → append raw Markdown to the overlay;
  reindexed on next query. Injection failures return nothing, silently.
- Malformed frontmatter → record indexed body-only, reported by `doctor`,
  never fatal.
- Merge conflicts in `.lore/` are ordinary git conflicts on small files;
  one-file-per-record makes them rare.
- `lore sync github` failures (offline, auth) report and leave records
  untouched; reconcile is idempotent.

## Testing

- Unit: frontmatter parse/serialize round-trip, ranking (decay, status,
  confidence weighting), anchor resolution against a fixture graph, signal
  extraction from fixture git histories (merge, `closes`, churn).
- Golden files: CLI text and `--json` output, matching existing codeindex
  test style.
- End-to-end: scripted add → search → supersede → promote → backlog →
  `closes` commit → doctor on a fixture repo with real git history.
- Efficacy (later, matching bench culture): A/B harness measuring whether
  lore injection reduces re-explanation tokens and increases
  decision-adherence across sessions — the lore analog of `bench/efficacy.py`.

## Key references (Grok Build study, 2026-07-29)

Patterns adopted: layered memory with repo-identity keying, temporal decay for
session-layer only, Markdown chunked on headings into FTS, fail-open hooks,
promotion/consolidation gate, session-end metadata capture without LLM calls.
Gaps deliberately filled (Grok has none of these): decision provenance with
alternatives preserved, issue linking, structured status/supersedes lifecycle,
team sharing via git, symbol-anchored staleness detection, merge-derived
evidence.

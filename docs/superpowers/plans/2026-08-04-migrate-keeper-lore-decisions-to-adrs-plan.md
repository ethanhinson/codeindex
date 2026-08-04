# Migrate keeper lore decisions to docket ADRs — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve the durable *engine* decisions from `.lore/decisions/*` as docket ADRs (plus one reversal ADR for the openspec→lore→docket lineage) before change 0004 deletes `.lore/`.

**Architecture:** Markdown-only. Each ADR is authored through the `docket-adr` skill/subagent, which owns id assignment, the `docs/adrs/README.md` index, immutability, and the commit on `origin/docket` (the metadata branch). This change produces **no code** — the only artifact on the feature branch `feat/migrate-keeper-lore-decisions-to-adrs` is this plan file. The eight ADRs land on the `docket` branch, not on `main`, and therefore are NOT part of the feature-branch diff.

**Tech Stack:** docket (`docket-adr` skill), markdown. No Go, no tests, no build.

## Global Constraints

- ADR files land on the `docket` (metadata) branch via `docket-adr` — **never** committed to the feature branch as code. Copied verbatim from the field-write / branch-model rules.
- ADR body sections are exactly `## Context`, `## Decision`, `## Consequences` (docket ADR format). A reversal ADR additionally sets `reverses: [<id>]` in frontmatter and sets the reversed ADR's `status:` to `Reversed by ADR-NN`.
- Each ADR's `change:` back-link = `1` (this change).
- Cite live code anchors verbatim from the source decision, confirmed to exist on `origin/main`: `internal/adapter/`, `internal/graph/`, `cmd/codeindex/`, `internal/config/filter.go` (`WalkWith`), `internal/query/`, `internal/merkle/`.
- Migrate only the 7 engine decisions + 1 lineage/reversal decision. The 6 lore-/UI-specific decisions are intentionally dropped (die with lore).
- Every ADR authored via `docket-adr` so the index (`docs/adrs/README.md`) stays valid; after each returns its number, append it to change 0001's `adrs:` field (metadata worktree).

---

## Source decision → ADR mapping

Confirmed present in `.lore/decisions/` at reconcile:

| # | Source `.lore/decisions/` file | ADR title | Anchor |
|---|---|---|---|
| 1 | `2026-07-30-parsing-via-tree-sitter-with-our-own-edge-resolv.md` | Parsing via tree-sitter with our own edge resolver | `internal/adapter/` |
| 2 | `2026-07-30-storage-is-sqlite-codeindex-graph-db-transaction.md` | Storage is SQLite (`.codeindex/graph.db`), transactional incremental updates | `internal/graph/` |
| 3 | `2026-07-30-engine-implementation-language-is-go-single-stat.md` | Engine implementation language is Go (single static binary) | `cmd/codeindex/` |
| 4 | `2026-07-31-config-driven-index-include-exclude-with-built-i.md` | Config-driven index include/exclude with built-in vendor defaults | `internal/config/filter.go` / `WalkWith` |
| 5 | `2026-07-30-freshness-is-on-demand-build-lazy-re-check-per-q.md` | Freshness is on-demand build + lazy per-query re-check, no daemon | `internal/query/` |
| 6 | `2026-07-30-change-detection-uses-flat-per-file-content-hash.md` | Change detection uses flat per-file content hashes, not a Merkle tree | `internal/merkle/` |
| 7 | `2026-07-30-output-contract-references-only-path-line-signat.md` | Output contract: references only (path:line + signature), never source | `internal/query/` |
| 8 (reversal) | `2026-07-29-lore-replaces-openspec.md` | Docket replaces lore (openspec→lore→docket lineage) | — reverses the lore-replaces-openspec decision's lineage |

**Dropped (not migrated):** graph-UI smoothness/aggregation · graph-UI v3 two-state · lore-is-a-sidecar · lore free-form records · in-repo records + private overlay · Go-side scoring + separate lore.db.

**Note on the reversal ADR:** The source lineage decision (`lore-replaces-openspec`) lived only in `.lore/`, so there is no prior *docket ADR* to point `reverses:` at by id. ADR-8 therefore records the full `openspec → lore → docket` lineage in its `## Context` and states in `## Decision` that "lore replaces openspec" is now itself superseded by "docket replaces lore." Its `reverses:` stays empty (no docket ADR predecessor exists); the reversal is semantic/historical, captured in prose. This is the correct docket shape — a reversal ADR is always a new ADR, and `reverses:` links only *docket* ADR ids.

---

### Task 1: ADR — tree-sitter parsing + own edge resolver

**Files:**
- Create (via `docket-adr`, on `docket` branch): `docs/adrs/<NNNN>-parsing-tree-sitter-own-edge-resolver.md`
- Source: `.lore/decisions/2026-07-30-parsing-via-tree-sitter-with-our-own-edge-resolv.md`

**Interfaces:**
- Produces: ADR number appended to change 0001 `adrs:`; this ADR is a peer to Tasks 2–7 (independent engine decisions, no ordering dependency between them).

- [ ] **Step 1: Dispatch the `docket-adr` subagent** to author the ADR.

Content to author:
- **Title:** Parsing via tree-sitter with our own edge resolver
- **`change:`** `1`
- **`## Context`:** codeindex must extract symbols and call/dependency edges across languages. Grammars parse syntax but do not resolve cross-file references. (Origin: openspec Key decisions, decided 2026-07-08; migrated to lore 2026-07-30.)
- **`## Decision`:** One tree-sitter grammar per language for parsing; edges (calls, deps) are resolved by our own logic, not the grammar. Start name-based, upgrade to import/scope-aware resolution as precision data demands (oracle-measured first). Anchor: `internal/adapter/` (tree-sitter adapters) + the resolver.
- **`## Consequences`:** Enables multi-language support with a uniform pipeline and lets resolution precision improve independently of parsing; costs owning edge-resolution logic and accepting name-based ambiguity until precise resolution lands (flagged via `resolved_confidence`).

- [ ] **Step 2: Record the returned ADR number** and append it to change 0001's `adrs:` field in the metadata worktree (re-sync `.docket/` first; regenerate the `## Artifacts` block via `render-change-links`; commit + push on `docket`).

---

### Task 2: ADR — SQLite `.codeindex/graph.db` transactional storage

**Files:**
- Create (via `docket-adr`, on `docket`): `docs/adrs/<NNNN>-storage-sqlite-graph-db-transactional.md`
- Source: `.lore/decisions/2026-07-30-storage-is-sqlite-codeindex-graph-db-transaction.md`

**Interfaces:**
- Produces: ADR number → change 0001 `adrs:`.

- [ ] **Step 1: Dispatch `docket-adr`** to author:
- **Title:** Storage is SQLite (`.codeindex/graph.db`), transactional incremental updates
- **`change:`** `1`
- **`## Context`:** The symbol graph needs durable storage supporting incremental updates and both-direction edge traversal (callers and callees) without full scans. (Origin: openspec Key decisions 2026-07-08.)
- **`## Decision`:** The symbol graph lives in a single SQLite file (`.codeindex/graph.db`) with transactional incremental updates and indexed both-direction edge traversal. Anchor: `internal/graph/`.
- **`## Consequences`:** Enables trivial distribution (one file), atomic incremental patches, and fast bidirectional traversal; a single-file store bounds concurrency to SQLite's model. (Historical note: lore formerly kept a separate `lore.db` that must not couple to `graph.db` — that lore constraint dies with lore and is not carried forward.)

- [ ] **Step 2: Append returned number to change 0001 `adrs:`** (metadata worktree, as Task 1 Step 2).

---

### Task 3: ADR — Go single static binary engine

**Files:**
- Create (via `docket-adr`, on `docket`): `docs/adrs/<NNNN>-engine-language-go-single-static-binary.md`
- Source: `.lore/decisions/2026-07-30-engine-implementation-language-is-go-single-stat.md`

**Interfaces:**
- Produces: ADR number → change 0001 `adrs:`.

- [ ] **Step 1: Dispatch `docket-adr`** to author:
- **Title:** Engine implementation language is Go (single static binary)
- **`change:`** `1`
- **`## Context`:** codeindex ships as a CLI/engine that must be trivial to distribute and fast at parallel parsing. (Origin: openspec Key decisions 2026-07-08.)
- **`## Decision`:** codeindex is written in Go — a single static binary with fast parallel parsing, trivial distribution (one file, no runtime), and good tree-sitter bindings. Anchor: `cmd/codeindex/`.
- **`## Consequences`:** Enables one-file distribution and parallelism; commits the project to Go's ecosystem and tree-sitter cgo bindings.

- [ ] **Step 2: Append returned number to change 0001 `adrs:`.**

---

### Task 4: ADR — config-driven index include/exclude

**Files:**
- Create (via `docket-adr`, on `docket`): `docs/adrs/<NNNN>-config-driven-index-include-exclude.md`
- Source: `.lore/decisions/2026-07-31-config-driven-index-include-exclude-with-built-i.md`

**Interfaces:**
- Produces: ADR number → change 0001 `adrs:`.

- [ ] **Step 1: Dispatch `docket-adr`** to author:
- **Title:** Config-driven index include/exclude with built-in vendor defaults
- **`change:`** `1`
- **`## Context`:** The repo was indexing its own committed minified SPA bundle (`internal/webserver/dist`) — 1377 garbage symbols (~60% of the index). Indexing scope must be configurable and prune vendored/compiled/VCS dirs by default. (Decided 2026-07-31.)
- **`## Decision`:** Indexing honors a repo Filter (`internal/config` Filter, built from `.codeindex.json`). Built-in defaults prune vendored/compiled/VCS dirs (`node_modules`, `vendor`, `dist`, `build`, `out`, `target`, `.git`, `.next`, `.svelte-kit`, `testdata`, `.codeindex`, …) and `*.min.js/css`. Repos add `exclude` globs/prefixes and `include` overrides; precedence is **include > exclude > defaults**. The Filter is applied at the single walk choke point (`merkle.WalkWith`), so `build`, `patch`, `grep`, and `depmap` all inherit it. Wildcard-free entries are path prefixes; `*`/`**`/`?` entries are globs (`**` spans separators); `include` can re-admit a file inside a default-skip dir while siblings stay skipped. Anchor: `internal/config/filter.go` / symbol `WalkWith`.
- **`## Consequences`:** Removes index bloat and centralizes filtering at one choke point so consumers never re-filter; rejected alternatives: filtering only at the read/UI layer (leaves the index bloated) and a hardcoded ignore list in the walk (not configurable, no re-include).

- [ ] **Step 2: Append returned number to change 0001 `adrs:`.**

---

### Task 5: ADR — on-demand build + lazy per-query freshness

**Files:**
- Create (via `docket-adr`, on `docket`): `docs/adrs/<NNNN>-freshness-on-demand-lazy-per-query.md`
- Source: `.lore/decisions/2026-07-30-freshness-is-on-demand-build-lazy-re-check-per-q.md`

**Interfaces:**
- Produces: ADR number → change 0001 `adrs:`.

- [ ] **Step 1: Dispatch `docket-adr`** to author:
- **Title:** Freshness is on-demand build + lazy per-query re-check, no daemon
- **`change:`** `1`
- **`## Context`:** Answers must stay correct as files change, without the operational cost of a background daemon. (Origin: openspec Key decisions 2026-07-08.)
- **`## Decision`:** No background daemon. The index is built on demand (`build`); every query does a lazy re-check of file hashes before answering, patching anything stale. Anchor: `internal/query/` (`query.Fresh`).
- **`## Consequences`:** Always-correct answers with minimal per-query overhead and no daemon to run/monitor; the cost is a small per-query staleness check instead of amortizing it in a watcher.

- [ ] **Step 2: Append returned number to change 0001 `adrs:`.**

---

### Task 6: ADR — flat per-file content-hash change detection

**Files:**
- Create (via `docket-adr`, on `docket`): `docs/adrs/<NNNN>-change-detection-flat-per-file-hash.md`
- Source: `.lore/decisions/2026-07-30-change-detection-uses-flat-per-file-content-hash.md`

**Interfaces:**
- Produces: ADR number → change 0001 `adrs:`.

- [ ] **Step 1: Dispatch `docket-adr`** to author:
- **Title:** Change detection uses flat per-file content hashes, not a Merkle tree
- **`change:`** `1`
- **`## Context`:** Freshness detection must find changed files cheaply and correctly at this repo's scale. (Origin: openspec Key decisions 2026-07-08.)
- **`## Decision`:** Freshness is detected with flat per-file content hashes plus a size/mtime fast path — NOT the relationship graph and NOT a Merkle tree. Diff vs stored state → re-parse only changed files → patch affected edges. Anchor: `internal/merkle/` (named for historical reasons; implements flat hashing).
- **`## Consequences`:** Simpler and provably correct; rejected: Merkle tree with interior nodes (measured unnecessary at this scale) and dir-mtime subtree skipping (provably misses edits — a changed file need not bump its dir's mtime). Cost: hashing every file each check rather than skipping subtrees.

- [ ] **Step 2: Append returned number to change 0001 `adrs:`.**

---

### Task 7: ADR — references-only output contract

**Files:**
- Create (via `docket-adr`, on `docket`): `docs/adrs/<NNNN>-output-contract-references-only.md`
- Source: `.lore/decisions/2026-07-30-output-contract-references-only-path-line-signat.md`

**Interfaces:**
- Produces: ADR number → change 0001 `adrs:`.

- [ ] **Step 1: Dispatch `docket-adr`** to author:
- **Title:** Output contract: references only (path:line + signature), never source
- **`change:`** `1`
- **`## Context`:** The tool's value premise is token savings for a model consumer — shipping compact references instead of file contents. (Origin: openspec Key decisions 2026-07-08.)
- **`## Decision`:** Query results are references — `path:line` plus signature — never full source. `--json` gives structured output; edges carry `resolved_confidence` so name-only matches are flagged as ambiguous (`[ambiguous]`). Anchor: `internal/query/`.
- **`## Consequences`:** Delivers the token-savings premise and makes ambiguity explicit to consumers; the cost is that callers who want source must fetch it themselves from the returned reference.

- [ ] **Step 2: Append returned number to change 0001 `adrs:`.**

---

### Task 8: Reversal ADR — docket replaces lore (openspec→lore→docket lineage)

**Files:**
- Create (via `docket-adr`, on `docket`): `docs/adrs/<NNNN>-docket-replaces-lore-lineage.md`
- Source: `.lore/decisions/2026-07-29-lore-replaces-openspec.md`

**Interfaces:**
- Consumes: nothing (independent of Tasks 1–7).
- Produces: ADR number → change 0001 `adrs:`.

- [ ] **Step 1: Dispatch `docket-adr`** to author the reversal ADR:
- **Title:** Docket replaces lore (openspec → lore → docket lineage)
- **`change:`** `1`
- **`reverses:`** empty — no *docket* ADR predecessor exists; the reversed decision (`lore replaces openspec`) lived only in `.lore/`. The reversal is semantic/historical, captured in prose (see the mapping note above). Do NOT invent an id to point at.
- **`## Context`:** This repo's decision workflow evolved openspec → lore → docket. openspec (`openspec/` changes + a "Key decisions" block in `openspec/config.yaml`) was replaced by lore on 2026-07-29 (`.lore/decisions/` records, `.lore/items/` backlog, specs staying in `docs/superpowers/specs/`); openspec was retired 2026-07-30. Now lore itself is being backed out (the lore→docket pivot) and work-tracking + decisions move to docket. Without this record, the "lore replaces openspec" decision would be silently lost when `.lore/` is deleted (change 0004).
- **`## Decision`:** "lore replaces openspec" is now itself superseded by "**docket replaces lore**." Decisions become docket ADRs (`docs/adrs/`), planned work becomes docket changes (`docs/changes/`), design docs stay in `docs/superpowers/specs/`. The engine decisions lore migrated from openspec are preserved as ADRs 1–7 (this change); everything lore-/graph-UI-specific dies with lore and is intentionally not preserved.
- **`## Consequences`:** The full lineage survives the `.lore/` deletion; git history retains the retired openspec and lore content for provenance. Cost: a one-time migration (this change) and acceptance that lore-specific decisions are dropped by design.

- [ ] **Step 2: Append returned number to change 0001 `adrs:`.**

---

### Task 9: Verify the ADR index + final gate

**Files:**
- Validate (on `docket`): `docs/adrs/README.md` (generated index) and `docs/adrs/*.md`

**Interfaces:**
- Consumes: all 8 ADR numbers from Tasks 1–8.

- [ ] **Step 1: Re-sync `.docket/`** and confirm all 8 ADR files exist in `docs/adrs/` with contiguous ids and valid frontmatter (`status: Accepted`, `change: 1`, correct `## Context`/`## Decision`/`## Consequences` sections).

- [ ] **Step 2: Confirm the ADR index** `docs/adrs/README.md` was regenerated by `docket-adr` and lists all 8 ADRs (run the docket-adr index-validation path if the skill exposes one).

- [ ] **Step 3: Confirm change 0001's `adrs:` field** lists all 8 numbers and its `## Artifacts` block was regenerated.

- [ ] **Step 4: Confirm no `.lore/` content was deleted** by this change (deletion is change 0004's job) and no code landed on the feature branch (only this plan file).

---

## Self-Review

**1. Spec coverage.** The spec's ".lore/ migration (Phase 0 detail)" section lists exactly 7 keeper engine decisions + 1 reversal ADR. Tasks 1–7 map to the 7 keepers (verified 1:1 against files on disk); Task 8 is the reversal. The spec's "Dropped" list (6 items) is explicitly excluded and documented in the mapping table. Covered.

**2. Placeholder scan.** Each ADR task carries its full Context/Decision/Consequences prose copied/adapted from the source decision — no "TBD"/"add appropriate…" placeholders. The one deliberate open item (reversal `reverses:` id) is resolved explicitly: leave empty, capture in prose.

**3. Type consistency.** No code types. Cross-task consistency checks: every task appends to the same field (change 0001 `adrs:`); anchors match the reconcile-verified paths; ADR ids are assigned by `docket-adr` (not hardcoded), so `<NNNN>` placeholders in filenames are correct — the skill owns numbering.

**Deviation note for the builder:** This is a docs migration with no test cycle. "TDD" does not apply; the reviewer gate per task is "does the ADR faithfully capture the source decision, cite a live anchor, and follow docket ADR format." The `docket-adr` skill is the authoring mechanism and the sole writer of `docs/adrs/` + its index on the `docket` branch.

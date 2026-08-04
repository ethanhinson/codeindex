---
id: 4
slug: cleanup-delete-lore-rewrite-readme
title: Cleanup — delete .lore/, drop lore config, rewrite README
status: in-progress
priority: medium
type: chore
created: 2026-08-03
updated: 2026-08-04
depends_on: [1, 3]
related: [1, 2, 3]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md
plan: docs/superpowers/plans/2026-08-04-cleanup-delete-lore-rewrite-readme-plan.md
results:
trivial: false
auto_groomable:
branch: feat/cleanup-delete-lore-rewrite-readme
pr:
blocked_by:
reconciled: true
claimed_at: 2026-08-04T17:36:16Z
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-03-back-out-lore-lean-into-docket-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md) |
| Plan | [2026-08-04-cleanup-delete-lore-rewrite-readme-plan.md](https://github.com/ethanhinson/codeindex/blob/feat/cleanup-delete-lore-rewrite-readme/docs/superpowers/plans/2026-08-04-cleanup-delete-lore-rewrite-readme-plan.md) |
<!-- docket:artifacts:end -->

## Why

The final Phase 3 cleanup after lore code and the graph UI are gone: remove the
`.lore/` data (its keepers are preserved as ADRs by change 0001), drop lore-specific
config, and rewrite the README to the pure blast-radius positioning so the repo no
longer advertises a product that no longer exists.

## What changes

- Delete the `.lore/` directory (guarded by change 0001 having migrated the keeper
  decisions to ADRs).
- Clean the stale `web/` / `internal/webserver/dist` example strings that 0003 left
  deferred in `internal/config` (the `config.go` Exclude doc-comment + `filter_test.go`
  fixtures) and `internal/merkle` (`walk_test.go` fixtures) — these are illustrative
  exclude-path examples that point at directories 0002/0003 already deleted; retarget
  them to a live example path (`vendor/…`) so no test or comment references a dead tree.
  (No lore/web indexing excludes or `.codeindex/lore.db` handling actually exist in the
  config **code** — `.codeindex.json` only excludes `.claude`/`.superpowers`, and the
  only remaining `lore.db` mentions live in historical plan/spec docs, left untouched.)
- Purge the remaining lore product surface the earlier phases left behind:
  - `plugin/hooks/lore_capture.py` + `plugin/hooks/lore_context.py` (dead lore hooks)
    and their two wirings in `plugin/hooks/hooks.json`.
  - lore descriptions in `.claude-plugin/marketplace.json` and `plugin/README.md`.
  - the stale "and SPA" doc-comment in `cmd/codeindex/serve.go` (the SPA is gone).
- Rewrite `README.md`: remove the lore engine sections (Lore, Host setup, Third-party
  sync), drop the deleted `tree`/`attach` CLI lines, describe codeindex as a
  blast-radius/impact tool with a decoupled, versioned symbol-graph API
  (`serve` → `/api/health`, `/api/graph`, `/api/graph/full`, `schemaVersion`; see
  `docs/graph-api.md`) + CLI; refresh the repository-layout block.

## Out of scope

- Any behavioral change to the impact engine or the graph API (changes 0002/0003).
- Removing docket itself — docket is now the work-tracking system.
- The historical `docs/superpowers/{plans,specs}/*lore*` design docs — they are the
  archived record of a shipped-then-removed product, not live surface.

## Open questions

- (Resolved during reconcile) `bench/` carries no real lore reference — the only grep
  hit is the substring `explore`/`Explore` inside a captured session-log fixture. No
  trim needed.

## Reconcile log

### 2026-08-04
Reconciled against current `origin/main` (Phase 1/2 = changes 0002/0003, both merged).
Findings:
- **`.lore/` is still committed** (42 tracked files) — deletion is now safe; change 0001
  migrated the keeper decisions to ADRs 0001–0008. Delete it.
- **No lore/web indexing excludes or `lore.db` handling live in config code.**
  `.codeindex.json` excludes only `.claude`/`.superpowers`; `internal/config` +
  `internal/merkle` have zero lore code. The genuine 0003-deferred cleanup is the stale
  **example strings** for now-deleted paths: `internal/config/config.go`'s Exclude
  doc-comment (`"internal/webserver/dist"`), `internal/config/filter_test.go`
  (`internal/webserver/dist`, `web/node_modules`), and `internal/merkle/walk_test.go`
  (`internal/webserver/dist/...`, `web/node_modules/...`). Retarget to a live path
  (`vendor/…`) so the suite exercises the same prefix/glob logic without naming a dead
  tree. All remaining `.codeindex/lore.db` mentions are inside historical
  `docs/superpowers/{plans,specs}/*lore*` — left as archived record.
- **Scope folded in (same "de-advertise the removed product" intent):** the earlier
  phases removed the lore CLI/MCP/engine and the `decide.md`/`lore.md` skills but left
  the plugin's lore **hooks** (`plugin/hooks/lore_capture.py`, `lore_context.py`) wired
  in `plugin/hooks/hooks.json`, plus lore copy in `.claude-plugin/marketplace.json` and
  `plugin/README.md`, and a stale "and SPA" doc-comment in `cmd/codeindex/serve.go`.
  These are pure dead-lore surface and belong to this cleanup.
- **`bench/` — no real lore reference.** The lone grep hit is `explore`/`Explore`
  (substring) in a captured session-log JSONL fixture. Nothing to trim.
- No new ADRs cited; `related: [1, 2, 3]` unchanged. Auto-capture disabled; no stubs
  minted.

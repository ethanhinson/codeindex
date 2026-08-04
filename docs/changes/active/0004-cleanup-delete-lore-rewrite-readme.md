---
id: 4
slug: cleanup-delete-lore-rewrite-readme
title: Cleanup — delete .lore/, drop lore config, rewrite README
status: proposed
priority: medium
type: chore
created: 2026-08-03
updated: 2026-08-03
depends_on: [1, 3]
related: [1, 2, 3]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md
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
| Artifact | Link |
|---|---|
| Spec | [2026-08-03-back-out-lore-lean-into-docket-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-03-back-out-lore-lean-into-docket-design.md) |
<!-- docket:artifacts:end -->

## Why

The final Phase 3 cleanup after lore code and the graph UI are gone: remove the
`.lore/` data (its keepers are preserved as ADRs by change 0001), drop lore-specific
config, and rewrite the README to the pure blast-radius positioning so the repo no
longer advertises a product that no longer exists.

## What changes

- Delete the `.lore/` directory (guarded by change 0001 having migrated the keeper
  decisions to ADRs).
- Drop lore/web indexing excludes and `lore.db` handling from config
  (`.codeindex*`, `internal/config`, and any `.codeindex/lore.db` references).
- Rewrite `README.md`: remove the lore engine and graph-UI sections; describe
  codeindex as a blast-radius/impact tool with a decoupled symbol-graph API + CLI.

## Out of scope

- Any behavioral change to the impact engine or the graph API (changes 0002/0003).
- Removing docket itself — docket is now the work-tracking system.

## Open questions

- Whether any `bench/` fixtures reference lore and need trimming — audit during
  reconcile.

## Reconcile log

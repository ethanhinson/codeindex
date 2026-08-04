---
id: 8
slug: docket-replaces-lore
title: "Docket replaces lore (openspec → lore → docket lineage)"
status: Accepted
date: 2026-08-03
supersedes: []
reverses: []
relates_to: []
change: 1
---

## Context

This repo's spec-and-decision workflow has evolved through three systems:

1. **openspec** — `openspec/` changes (proposal/design/tasks) plus a "Key decisions" block in `openspec/config.yaml`.
2. **lore** — on 2026-07-29, openspec was replaced by lore: decisions became `.lore/decisions/` records (rationale + rejected alternatives), planned work became `.lore/items/` backlog entries, and design docs stayed in `docs/superpowers/specs/`. openspec was fully retired 2026-07-30 (the CLI tooling under `.claude/` removed too).
3. **docket** — now lore itself is being backed out (the lore→docket pivot). Work-tracking and decisions move to docket: this repo adopts docket as part of the change.

Without a record of this lineage, the "lore replaces openspec" decision — which lived only in `.lore/decisions/` — would be silently lost when `.lore/` is deleted (change 0004).

## Decision

The decision "lore replaces openspec" is now itself superseded by "**docket replaces lore**." Going forward: decisions become docket ADRs (`docs/adrs/`), planned work becomes docket changes (`docs/changes/`), and design docs continue to live in `docs/superpowers/specs/`. The durable *engine* decisions that lore had migrated from openspec are preserved as docket ADRs 1-7 (authored by this same change, 0001). Everything lore-/graph-UI-specific dies with lore and is intentionally NOT preserved.

## Consequences

The full openspec → lore → docket lineage survives the deletion of `.lore/` (change 0004); git history retains the retired openspec and lore content for provenance. The cost is a one-time migration (this change) plus the deliberate acceptance that lore-specific decisions (graph-UI aggregation, v3 two-state model, lore-is-a-sidecar, free-form records, in-repo records + private overlay, separate lore.db) are dropped by design. Note: this ADR's `reverses:` is empty because the reversed decision was a `.lore/` record, not a prior docket ADR — the reversal is captured here in prose.

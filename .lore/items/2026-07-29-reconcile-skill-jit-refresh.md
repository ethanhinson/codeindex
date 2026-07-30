---
id: itm-01KYR5Z1KB7N1PPEQ46FQMA41J
title: Reconcile skill — JIT refresh of stale items before implementation
status: open
date: 2026-07-29
priority: p1
blocked_by: [itm-01KYR17XECTSCDR5DZX5DXAWTJ]
refs:
    - url: https://github.com/danielhanold/docket
    - url: .lore/notes/docket-comparison-and-adoptable-ideas.md
---
Adopted from docket ("plans rot; refresh just-in-time"). Before an agent
implements an item, a reconcile pass rewrites it against what is true NOW:
drops work already done elsewhere, folds in newer decisions, adjusts scope
for interface drift. Appends a dated `## Reconcile log` entry. Escape
hatches: obsolete → status dropped with reason; fundamentally invalidated →
stop and escalate to the human.

Our edge over docket's version: the engine's anchor-staleness and churn
data tell the reconcile pass exactly which anchors drifted and which
decisions were superseded since the item's date — targeted refresh instead
of a full re-read. Ship as a skill (Plan 2 packaging) + `lore reconcile
<id>` engine support surfacing the drift evidence.

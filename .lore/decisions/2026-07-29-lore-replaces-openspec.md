---
id: dec-01KYR17XEC208KMPSEGKBFT6Y7
title: The lore workflow replaces OpenSpec for this repo
status: active
date: 2026-07-29
anchors:
    - path: .lore/
---
This repo's spec-driven workflow (openspec/ changes with proposal/design/
tasks, plus the "Key decisions" context block in openspec/config.yaml)
is replaced by lore itself: decisions become `.lore/decisions/` records with
rationale and rejected alternatives; planned work becomes `.lore/items/`
backlog entries; design docs stay in docs/superpowers/specs/. This decision
is itself the first record of the new cycle — the dogfood starts here.

Migration of existing openspec content and retirement of the directory is
tracked as a backlog item; nothing is deleted until the loop has proven
itself through at least Plan 1.

Done 2026-07-30 (itm-01KYR17XECZJ5DYEV1VXQQ3TRD): openspec/ retired. Its Key
decisions were migrated into individual engine decision records (Go,
tree-sitter parsing, SQLite graph.db, flat-hash change detection, on-demand
freshness, references-only output) related to
note-01KYTPDHJH59PG2M0FFTEYC92Z; the agent-A/B value boundary became
note-01KYTPPJQ8P164D382G97WWB99; completed changes/specs are preserved in git
history. Anchor re-pointed from openspec/ (removed) to .lore/. The OpenSpec
CLI tooling under .claude/ (opsx commands, openspec-* skills) is a separate
concern, tracked for removal as a follow-up.

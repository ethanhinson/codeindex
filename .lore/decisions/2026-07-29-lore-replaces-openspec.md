---
id: dec-01KYR17XEC208KMPSEGKBFT6Y7
title: The lore workflow replaces OpenSpec for this repo
status: active
date: 2026-07-29
anchors:
    - path: openspec/
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

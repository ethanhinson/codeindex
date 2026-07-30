---
id: itm-01KYTPTJACEDBW6X47TTXV3YM0
title: Remove OpenSpec CLI tooling under .claude/ (opsx commands, openspec-* skills)
status: open
date: "2026-07-30"
priority: p3
related: [dec-01KYR17XEC208KMPSEGKBFT6Y7]
tags: [cleanup]
anchors:
    - path: .claude/commands/opsx/
    - path: .claude/skills/
---

Follow-up from retiring openspec/ (itm-01KYR17XECZJ5DYEV1VXQQ3TRD). The
directory is gone but the OpenSpec workflow tooling remains git-tracked:
.claude/commands/opsx/{apply,archive,explore,propose,sync}.md and
.claude/skills/openspec-{apply-change,archive-change,explore,propose,sync-specs}/.
These operate on an openspec/ root that no longer exists. Retiring them fully
aligns the repo with the lore workflow (dec-01KYR17XEC208KMPSEGKBFT6Y7). Kept
separate from the directory-retirement item because removing installed
slash-command/skill tooling is a distinct, opt-in change — confirm whether any
are still wanted (e.g. for driving external OpenSpec stores) before deleting.

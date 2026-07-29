---
id: dec-01KYR17XEC92B4WWESXCHZ5XD6
title: Records live in-repo (.lore/), layered with a private per-user overlay
status: active
date: 2026-07-29
anchors:
    - path: .lore/
refs:
    - url: docs/superpowers/specs/2026-07-29-lore-engine-design.md
---
Team knowledge is committed Markdown in `.lore/` — versioned, branched,
merged, and reviewed exactly like code. Git is the sync protocol: remote and
cloud workers get lore with their clone and return decisions via PR merge.
A private overlay (`~/.codeindex/lore/<repo-id>/`) holds personal notes and
decaying session captures; `promote` moves a record into the committed layer
as a reviewable diff. One file per record keeps merges near-conflict-free.

## Alternatives considered

**User-home store keyed by repo identity (Grok Build's model).** Private by
default and works on read-only repos, but no team sharing, no branch/merge
semantics, and knowledge dies with the machine. This is the studied gap the
project exists to fill.

**In-repo only.** Loses the scratch layer and session decay; every stray
thought would demand a commit.

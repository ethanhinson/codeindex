---
id: 6
slug: change-detection-flat-per-file-hashes-not-merkle-tree
title: Change detection uses flat per-file content hashes, not a Merkle tree
status: Accepted
date: 2026-07-30
supersedes: []
reverses: []
relates_to: []
change: 1
---

## Context

Detecting which files changed between index builds must be cheap and provably correct at this repo's scale. Origin: decided 2026-07-08 in the openspec "Key decisions" block, migrated to `.lore/decisions/` on 2026-07-30, now migrated to a docket ADR as part of backing lore out (change 0001, before `.lore/` is deleted by change 0004).

## Decision

Freshness is detected with flat per-file content hashes plus a size/mtime fast path — NOT the relationship graph itself and NOT a Merkle tree. The flow is: diff vs stored state, re-parse only changed files, patch affected edges. Anchor: `internal/merkle/` (named for historical reasons but implements flat hashing).

## Consequences

Simpler and provably correct at this scale. Rejected alternatives: (a) a Merkle tree with interior nodes — the interior nodes were measured unnecessary at this scale; (b) dir-mtime subtree skipping — provably misses edits, since a changed file need not bump its directory's mtime. The cost is hashing every candidate file each check rather than skipping whole subtrees; the package name `internal/merkle` is a historical artifact and does not imply a tree structure.

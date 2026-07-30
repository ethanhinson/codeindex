---
id: dec-01KYTPMMAEEA2VBTNQE1ZBD523
title: Change detection uses flat per-file content hashes, not a Merkle tree
status: active
date: "2026-07-30"
related: [note-01KYTPDHJH59PG2M0FFTEYC92Z]
tags: [engine]
anchors:
    - path: internal/merkle/
refs:
    - url: docs/superpowers/specs/2026-07-08-codeindex-design.md
---

Freshness is detected with flat per-file content hashes plus a size/mtime fast
path — NOT the relationship graph itself and NOT a Merkle tree. Diff vs stored
state → re-parse only changed files → patch affected edges.

## Alternatives considered

**Merkle tree with interior nodes / dir-mtime subtree skipping.** Rejected:
interior nodes were measured unnecessary at this scale, and dir-mtime subtree
skipping provably misses edits (a changed file need not bump its dir's mtime).
Flat per-file hashes are simpler and correct.

Migrated 2026-07-30 from openspec/config.yaml Key decisions (decided 2026-07-08),
per dec-01KYR17XEC208KMPSEGKBFT6Y7. (The package is named internal/merkle for
historical reasons but implements flat hashing.)

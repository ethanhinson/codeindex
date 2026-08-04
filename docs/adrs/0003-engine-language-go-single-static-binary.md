---
id: 3
slug: engine-language-go-single-static-binary
title: Engine implementation language is Go (single static binary)
status: Accepted
date: 2026-07-30
supersedes: []
reverses: []
relates_to: []
change: 1
---

## Context

codeindex ships as a CLI/engine that must be trivial to distribute and fast at parallel parsing across large repos.

Origin: decided 2026-07-08 in the openspec "Key decisions" block, migrated to `.lore/decisions/` on 2026-07-30, now migrated to a docket ADR as part of backing lore out (change 0001, before `.lore/` is deleted by change 0004). The `date:` above preserves the `.lore/decisions/` provenance date rather than the docket authoring date.

## Decision

codeindex is written in Go — a single static binary with fast parallel parsing, trivial distribution (one file, no runtime dependency), and good tree-sitter bindings. Anchor: `cmd/codeindex/`.

## Consequences

Enables one-file distribution (no runtime to install) and CPU-parallel parsing. Commits the project to Go's ecosystem and to tree-sitter's cgo bindings (a cgo build dependency).

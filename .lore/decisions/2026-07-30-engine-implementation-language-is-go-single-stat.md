---
id: dec-01KYTPM3VB25CNARDBTC8Z8XNJ
title: Engine implementation language is Go (single static binary)
status: active
date: "2026-07-30"
related: [note-01KYTPDHJH59PG2M0FFTEYC92Z]
tags: [engine]
anchors:
    - path: cmd/codeindex/
refs:
    - url: docs/superpowers/specs/2026-07-08-codeindex-design.md
---

codeindex is written in Go: a single static binary with fast parallel parsing,
trivial distribution (one file, no runtime), and good tree-sitter bindings.

Migrated 2026-07-30 from openspec/config.yaml Key decisions (decided 2026-07-08),
per dec-01KYR17XEC208KMPSEGKBFT6Y7. Still active — the whole engine is Go.

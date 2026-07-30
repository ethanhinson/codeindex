---
id: dec-01KYTPM3VYRGAGH8QHCWRCZR14
title: Parsing via tree-sitter with our own edge resolver (name-based first, precise later)
status: active
date: "2026-07-30"
related: [note-01KYTPDHJH59PG2M0FFTEYC92Z]
tags: [engine]
anchors:
    - path: internal/adapter/
refs:
    - url: docs/superpowers/specs/2026-07-08-codeindex-design.md
---

One tree-sitter grammar per language for parsing; edges (calls, deps) are
resolved by our own logic, not the grammar. Start name-based, upgrade to
import/scope-aware resolution as precision data demands (oracle-measured first).

Migrated 2026-07-30 from openspec/config.yaml Key decisions (decided 2026-07-08),
per dec-01KYR17XEC208KMPSEGKBFT6Y7. Active — see internal/adapter/ (tree-sitter
adapters) and the resolver.

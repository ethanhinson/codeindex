---
id: note-01KYTPDHJH59PG2M0FFTEYC92Z
title: codeindex engine architecture overview (migrated from openspec)
date: "2026-07-30"
tags: [engine]
anchors:
    - path: cmd/codeindex/
    - path: internal/engine/
refs:
    - url: docs/superpowers/specs/2026-07-08-codeindex-design.md
---

Migrated from openspec/config.yaml (context block) when the openspec/ directory
was retired (2026-07-30), per dec-01KYR17XEC208KMPSEGKBFT6Y7.

codeindex is a Claude Code plugin that cuts token usage when navigating a
codebase. It indexes source into a symbol relationship graph (call graph +
dependency graph) so Claude answers "who calls X?" / "what depends on X?" with
compact file:line + signature references instead of grepping and reading whole
files.

Architecture (bottom-up layers): language adapters (tree-sitter) → resolver
(imports/scope → edges) → change-detection index → symbol graph (SQLite) →
query engine → CLI → MCP server → Claude plugin.

Target languages: TypeScript/JS, Python, Go, PHP, .NET/C#.

The individual engine decisions (Go, tree-sitter parsing, SQLite storage,
flat-hash change detection, on-demand freshness, references-only output) are
migrated as their own decision records related to this note. Full design:
docs/superpowers/specs/2026-07-08-codeindex-design.md.

---
id: 1
slug: parsing-via-tree-sitter-with-edge-resolver
title: Parsing via tree-sitter with our own edge resolver
status: Accepted
date: 2026-07-30
supersedes: []
reverses: []
relates_to: []
change: 1
---

## Context

codeindex must extract symbols and call/dependency edges across languages. tree-sitter grammars parse syntax but do not resolve cross-file references (which function calls which, module deps).

Origin: this decision was made 2026-07-08 in the openspec "Key decisions" block, migrated into `.lore/decisions/` on 2026-07-30, and is now migrated to a docket ADR as part of backing lore out of the repo (change 0001, before `.lore/` is deleted by change 0004). The `date:` above preserves the `.lore/decisions/` provenance date rather than the docket authoring date.

## Decision

One tree-sitter grammar per language for parsing; edges (calls, deps) are resolved by our own logic, not the grammar. Start name-based, upgrade to import/scope-aware resolution as precision data demands (oracle-measured first). Anchor: `internal/adapter/` (the tree-sitter adapters) plus the edge resolver.

## Consequences

Enables multi-language support through a uniform parse-then-resolve pipeline and lets resolution precision improve independently of the grammars. Costs: we own the edge-resolution logic, and name-based resolution accepts ambiguity until precise (import/scope-aware) resolution lands — ambiguity is surfaced via `resolved_confidence` on edges.

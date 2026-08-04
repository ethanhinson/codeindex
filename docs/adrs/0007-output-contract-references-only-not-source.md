---
id: 7
slug: output-contract-references-only-not-source
title: "Output contract: references only (path:line + signature), never source"
status: Accepted
date: 2026-07-30
supersedes: []
reverses: []
relates_to: []
change: 1
---

## Context

The tool's core value premise is token savings for a model consumer: return compact references instead of shipping file contents to the model. Origin: decided 2026-07-08 in the openspec "Key decisions" block, migrated to `.lore/decisions/` on 2026-07-30, now migrated to a docket ADR as part of backing lore out (change 0001, before `.lore/` is deleted by change 0004).

## Decision

Query results are references — `path:line` plus signature — never full source. `--json` gives structured output; edges carry `resolved_confidence` so name-only matches are flagged as ambiguous (the `[ambiguous]` flag). This is the whole token-savings premise: compact references instead of shipping file contents to the model. Anchor: `internal/query/` (the `[ambiguous]` flag and `--json` across the query surface implement it).

## Consequences

Delivers the token-savings premise and makes match ambiguity explicit to consumers via `resolved_confidence`/`[ambiguous]`. The cost is that a caller who needs actual source must fetch it themselves using the returned `path:line` reference — the tool deliberately never ships file contents.

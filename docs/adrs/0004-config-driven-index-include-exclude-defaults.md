---
id: 4
slug: config-driven-index-include-exclude-defaults
title: Config-driven index include/exclude with built-in vendor defaults
status: Accepted
date: 2026-07-31
supersedes: []
reverses: []
relates_to: []
change: 1
---

## Context

The repo was indexing its own committed minified SPA bundle (`internal/webserver/dist`) — 1377 garbage symbols, roughly 60% of the index. Indexing scope must be configurable and must prune vendored/compiled/VCS directories by default so consumers do not re-filter.

Decided 2026-07-31; migrated to a docket ADR as part of backing lore out (change 0001, before `.lore/` is deleted by change 0004). The `date:` above preserves the original `.lore/decisions/` decision date rather than the docket authoring date.

## Decision

codeindex indexing honors a repo Filter (`internal/config` Filter, built from `.codeindex.json`). Built-in defaults prune vendored/compiled/VCS dirs (`node_modules`, `vendor`, `dist`, `build`, `out`, `target`, `.git`, `.next`, `.svelte-kit`, `testdata`, `.codeindex`, and similar) plus `*.min.js`/`*.min.css`. Repos add `exclude` globs/prefixes and `include` overrides; precedence is include > exclude > defaults.

The Filter is applied at the single walk choke point (`merkle.WalkWith`), so build, patch, grep, and depmap all inherit it. Wildcard-free entries are path prefixes; entries with `*`/`**`/`?` are globs (`**` spans separators); an `include` can re-admit a file inside a default-skip dir while its siblings stay skipped. Anchor: `internal/config/filter.go` (symbol `WalkWith`).

## Consequences

Removes index bloat (the ~60% garbage from the committed SPA bundle) and centralizes filtering at one choke point so every consumer inherits it without re-filtering.

Rejected alternatives: (a) filter only at the read/UI layer — leaves the index bloated and forces every consumer to re-filter; (b) hardcode a fixed ignore list in the walk — not configurable, with no way to re-include a vendored path.

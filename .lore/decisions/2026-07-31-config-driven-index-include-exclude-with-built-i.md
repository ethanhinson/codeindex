---
id: dec-01KYV5EZP3NVC6ME7XVG6K1K7Y
title: Config-driven index include/exclude with built-in vendor defaults
status: active
date: "2026-07-31"
anchors:
    - symbol: WalkWith
    - path: internal/config/filter.go
---

codeindex indexing now honors a repo Filter (internal/config Filter, built from .codeindex.json). Built-in defaults prune vendored/compiled/VCS dirs (node_modules, vendor, dist, build, out, target, .git, .next, .svelte-kit, testdata, .codeindex, …) and *.min.js/css. Repos add 'exclude' globs/prefixes and 'include' overrides; precedence is include > exclude > defaults. The Filter is applied at the single walk choke point (merkle.WalkWith), so build, patch, grep, and depmap all inherit it. Motivation: the repo was indexing its own committed minified SPA bundle (internal/webserver/dist), 1377 garbage symbols (~60% of the index). Alternatives rejected: (a) filter only at the read/UI layer — leaves the index bloated and every consumer must re-filter; (b) hardcode a fixed ignore list in the walk — not configurable, and no way to re-include a vendored path. Patterns: wildcard-free entries are path prefixes; entries with */**/? are globs ('**' spans separators). Include can re-admit a file inside a default-skip dir while its siblings stay skipped.

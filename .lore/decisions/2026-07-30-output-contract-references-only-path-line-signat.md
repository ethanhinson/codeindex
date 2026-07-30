---
id: dec-01KYTPMMBQ6V10EZF8V0BWB7WD
title: 'Output contract: references only (path:line + signature), never source'
status: active
date: "2026-07-30"
related: [note-01KYTPDHJH59PG2M0FFTEYC92Z]
tags: [engine]
anchors:
    - path: internal/query/
refs:
    - url: docs/superpowers/specs/2026-07-08-codeindex-design.md
---

Query results are references — path:line plus signature — never full source.
`--json` gives structured output; edges carry resolved_confidence so name-only
matches are flagged as ambiguous. This is the whole token-savings premise:
compact references instead of shipping file contents to the model.

Migrated 2026-07-30 from openspec/config.yaml Key decisions (decided 2026-07-08),
per dec-01KYR17XEC208KMPSEGKBFT6Y7. Active — the [ambiguous] flag and --json
across the query surface implement it.

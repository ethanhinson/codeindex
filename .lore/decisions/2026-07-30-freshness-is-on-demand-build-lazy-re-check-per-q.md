---
id: dec-01KYTPMMB36PN31RAGV9XMJE9Q
title: Freshness is on-demand build + lazy re-check per query, no daemon
status: active
date: "2026-07-30"
related: [note-01KYTPDHJH59PG2M0FFTEYC92Z]
tags: [engine]
anchors:
    - path: internal/query/
refs:
    - url: docs/superpowers/specs/2026-07-08-codeindex-design.md
---

No background daemon. The index is built on demand (`build`) and every query
does a lazy re-check of file hashes before answering, patching anything stale.
Always-correct answers with minimal per-query overhead.

Migrated 2026-07-30 from openspec/config.yaml Key decisions (decided 2026-07-08),
per dec-01KYR17XEC208KMPSEGKBFT6Y7. Active — see query.Fresh / the query layer.

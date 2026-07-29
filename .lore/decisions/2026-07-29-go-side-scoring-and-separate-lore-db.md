---
id: dec-01KYR17XECDN2T35W7ERZ932Y8
title: Go-side BM25-lite scoring and a separate lore.db (no FTS5, no graph.db coupling)
status: active
date: 2026-07-29
anchors:
    - path: internal/search/
    - path: internal/graph/
refs:
    - url: docs/superpowers/specs/2026-07-29-lore-engine-design.md
    - url: docs/superpowers/plans/2026-07-29-lore-engine-core.md
---
Lore search is an in-memory scan scoring `##`-heading chunks with the
existing `internal/search` tokenizer; the derived index lives in
`.codeindex/lore.db`, not `graph.db`.

## Alternatives considered

**SQLite FTS5.** mattn/go-sqlite3 only compiles FTS5 behind a `sqlite_fts5`
build tag, which every build/test/CI invocation would have to carry — a
standing footgun that fails at runtime when forgotten. At lore scale
(hundreds of records) it buys nothing; `internal/search` validated in-memory
scanning at Kubernetes scale in tens of milliseconds.

**Lore tables inside graph.db.** Rejected because `export`/`import` ship
graph.db as a shareable artifact — lore is already shared via git and must
not ride along — and because it couples lore schema changes to code-index
rebuilds.

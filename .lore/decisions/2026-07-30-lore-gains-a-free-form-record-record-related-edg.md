---
id: dec-01KYTG2C8BPFS0GV787Y8AA4QM
title: Lore gains a free-form record→record 'related' edge with full-trace graph traversal
status: active
date: "2026-07-30"
tags: [knowledge-graph]
anchors:
    - path: internal/lore/record.go
    - path: internal/lore/index/
    - path: cmd/codeindex/lore.go
refs:
    - url: docs/superpowers/specs/2026-07-30-lore-knowledge-graph-edges-design.md
---

To make codeindex dogfood-able as a knowledge graph of its own codebase, lore
gets a general associative record→record edge: a `related: [id-or-slug]`
frontmatter field, indexed in a new `lore_links` table in lore.db, with
backlinks and a shared BFS `Trace` traversal used by both `lore related` and
the `impact` Related-lore block (now brought to the CLI, not just MCP).

## Alternatives considered

**[[wikilink]] body parsing (Obsidian-native).** Deferred, not rejected —
frontmatter `related:` is the source of truth for v1; body-link sugar can layer
on later only if authoring friction proves real (YAGNI).

**Fixed one-hop `related` expansion.** Rejected. One hop is not enough — the
traversal is full-trace with configurable/inferred depth (`--depth N | all`),
cycle-safe via a visited set, distance/edge-annotated, bounded only by a
total-reached safety cap.

**Lore tables / links inside graph.db.** Rejected, consistent with
dec-01KYR17XECDN2T35W7ERZ932Y8 (no graph.db coupling): the record graph lives
in lore.db; the code→knowledge bridge in `impact` stays a query-time join across
the anchor, never a schema link.

The qualitative friction log that measures this dogfooding is a **convention,
not a feature**: friction is recorded as ordinary notes `related:`-linked to the
surface — no `dogfooding` record type or capture path is built. The only built
measurement surface is graph-health (orphans / dangling links / density) in
`lore doctor`.

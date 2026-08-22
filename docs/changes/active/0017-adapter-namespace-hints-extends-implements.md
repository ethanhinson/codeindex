---
id: 17
slug: adapter-namespace-hints-extends-implements
title: Attach namespace hints to extends/implements references in the language adapters
status: proposed
priority: high
type: fix
created: 2026-08-22
updated: 2026-08-22
depends_on: []
related: [13, 10]
discovered_from: [16]
adrs: []
spec:
plan:
results:
trivial: false
auto_groomable: true
branch:
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
<!-- docket:artifacts:end -->

## Why

The D7 gate runs that killed change 0016 isolated one structural signal:
on `xsubtypes` tasks ("what extends/implements X across repos") the
workspace index lifted haiku +14pp over the grep control — the only
task shape where it helped — yet scored just 0.43 absolute, capped by a
long-recorded adapter gap: **extends/implements references carry no
namespace hint** (the Go adapter's `addDep` never sets `Source`; other
languages unverified at reference level). Without a hint, the resolution
ladder's import-mediated rung can never fire for subtype edges, so the
index is structurally missing the links its best task shape needs. This
gap also degrades single-repo depmap resolution wherever hints gate
tier-1 attachment.

Fixing it is a prerequisite for the pivot campaign (structural corpus,
change 0010, new gate): without hints, subtype tasks measure the gap,
not the feature. Record in
`bench/engine/FINDINGS-workspace-graph.md` (2026-08-22 entry).

## What changes

- For each language adapter (Go, TS/JS, Python, PHP): where an
  extends/implements/embeds reference is emitted, attach the same
  namespace hint (`Source`/`dst_ns`) the adapter already attaches to
  import and call references, derived from the import binding in scope.
- Verify per language against the bench corpus members (nest for TS
  `extends`/`implements`, symfony/drupal for PHP, prometheus/
  client_golang for Go interface embedding, werkzeug/flask for Python
  subclassing) — the hint must appear on real unresolved subtype edges
  in each member's graph.db.
- Single-repo behavior: hints on unresolved edges are metadata; goldens
  must stay byte-identical (measured, not assumed).

## Out of scope

- Resolver/ladder changes — rung 1 already consumes hints; this change
  only makes subtype edges carry them.
- Workspace query surfaces (killed 0016; revival is the new gate's
  outcome).
- Corpus growth (change 0010).

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

---
id: 18
slug: aliased-import-resolution
title: Resolve references made through import aliases — the name-vs-alias schema decision
status: proposed
priority: medium
type: fix
created: 2026-08-22
updated: 2026-08-22
depends_on: []
related: [17, 10, 13]
discovered_from: [17]
adrs: []
spec:
plan:
results:
trivial: false
auto_groomable: false
branch:
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
<!-- docket:artifacts:end -->

## Why

Split out of change 0017 by owner ruling (Option C, 2026-08-22). When
code references a symbol through an import alias (`use ...Model as
Eloquent; class X extends Eloquent`), the stored reference name is the
alias — and resolution looks up symbols *by name*, so it finds nothing.
Attaching a namespace hint cannot fix this (proven inert by the 0017
groom's critic): lookup still runs on the alias in both the in-repo
resolver (`boundIDs`) and the workspace ladder's rung 1. Measured
addressable populations: drupal 202, laravel 215, symfony 59, werkzeug
8, flask 4, nest 0 aliased references currently unresolvable.

## What changes

The load-bearing decision this change exists to make (recorded options
from the 0017 abstain, full analysis in that change's git history):

- **Option A** — rewrite the edge's `dst_name` to the original symbol
  name at extraction time. Resolution works everywhere with no resolver
  change; but the edge no longer matches the source text, `dst_name` is
  part of edge identity (add+delete in delta gates), and every consumer
  displaying reference names sees the rewritten value.
- **Option B** — keep `dst_name` as written; add an original-name
  column plus resolver and `wsresolve` lookup changes to try it.
  Faithful to source; widens scope to schema + resolver + ladder.

Either option deserves an ADR. Adjacent missing-EDGE parse gaps found
by the same groom belong here or in a sibling: Python `class X(mod.Y)`
and TS `extends ns.Foo` emit no subtype edge at all; PHP group `use
A\{…}` emits no import dep (zero occurrences in the current corpus
members, so corpus-invisible today).

## Out of scope

- The Go qualifier/Source fix — change 0017.
- Corpus/task mining — change 0010 (its alias-task shape depends on
  this change landing).

## Open questions

- A vs B (the schema/altitude call — owner decision, with ADR).
- Whether the missing-edge parse gaps ride along or split again.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

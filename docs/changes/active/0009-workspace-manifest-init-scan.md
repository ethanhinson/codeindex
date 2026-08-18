---
id: 9
slug: workspace-manifest-init-scan
title: Workspace manifest load/validate + init-workspace --scan
status: proposed
priority: high
type: feat
created: 2026-08-17
updated: 2026-08-18
depends_on: []
related: []
discovered_from: []
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

The workspace-graph campaign (query across a workspace of repos as if it
were a single graph) registered its GO on 2026-08-17 and its bench-first
phase is fully done: frozen 65-task OSS corpus across all four indexer
languages, arm-A control harness wired and smoked, four-class leak audit
PASS (`bench/workspace/`). The evidence gate (design D7) now waits on the
engine — arm B refuses to run until workspace support exists. This change
is the first engine slice: without a manifest and a way to create one,
nothing downstream (overlay store, resolution ladder, union-graph queries)
can start.

The full design is settled and frozen in
`openspec/changes/workspace-graph/design.md` (owner sign-off 2026-08-17,
open questions: none). This change implements task §3.1 of
`openspec/changes/workspace-graph/tasks.md`.

## What changes

Per design D1 (identity) and D5 (surfaces):

- Load + validate `<workspace-root>/.codeindex/workspace.json` (version,
  member id/root/namespaces/optional deps; roots relative to the
  workspace root, may point outside it).
- New verb `init-workspace --scan`: namespace auto-discovery per language
  (go.mod / package.json / composer.json / Python top-level modules) and
  monorepo member discovery via go.work, pnpm-workspace.yaml, and
  composer path repositories. Manifest overrides always win over
  scanned values.
- Root-kind detection groundwork: a root containing `workspace.json` is a
  workspace; a root with neither manifest nor indexable source errors
  naming both possibilities. Single-repo mode is the absence of a
  manifest, not a flag — existing single-repo behavior stays
  byte-identical.

## Out of scope

- Overlay store (`workspace.db`: registry, cross-edges, stamps) — task
  §3.2.
- Cross-repo resolution ladder — task §3.3 (import-mediated exact only,
  per design D3).
- Workspace freshen + `workspace-status` verb — task §3.4.
- Union-graph query paths, CLI/MCP surface changes beyond the new verb —
  tasks §4.x.
- The evidence gate run — task §5.x.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

## Groom context (owner decisions 2026-08-18)

The autonomous groom's first pass abstained with three residual items
(full abstain record in git history); the owner answered all three on
2026-08-18:

1. **Member-discovery over-collection — apply all three guards:** glob
   expansion restricted to directories only; the language marker must
   sit at the candidate's own top level (not one level down); and a
   root candidate whose namespace set is a subset of a declared
   member's is suppressed. Together these fix all three spurious
   members on the frozen bench corpus.
2. **Root-kind detection — short-circuit helper:** add a small walk
   helper (explicit filter, aborts on the first indexable hit) rather
   than reusing `merkle.WalkWith` — no full-tree walk per detection,
   no process-global `adapter.SetAssociations` mutation against a live
   MCP server. Placement per the prior groom round: `internal/engine`.
3. **Merge gate — this slice may merge ahead of the D7 gate:** the
   frozen SHALL (`specs/workspace-graph/spec.md:125`) is read as
   gating query-behavior slices; this change's PR must carry a dated
   amendment to the openspec change recording that interpretation, and
   the D7 gate still hard-blocks §3.3+/§4 from merging.

### Settled design from the prior groom rounds (survived two critic rounds)

Manifest types in `internal/config` (`Workspace`/`Member`, D1 shape);
`LoadWorkspace` validates **shape only** and never stats a member root,
with a separate `Resolve` reporting missing members (this is what keeps
the frozen "coverage clause names the missing member" scenario,
`specs/workspace-graph/spec.md:117-122`, reachable); `SaveWorkspace`
preserves manifest order (D4 orders answers "by manifest"), sorting only
newly appended members; `version` absent or `!= 1` is an error
(`internal/runtime/runtime.go:70` precedent); unknown JSON fields
ignored; ids restricted to `[A-Za-z0-9._-]` because D4 gives them an
anchor-prefix role (`api:HandleLogin`); absolute `root` rejected, `../`
accepted; `--scan` required, existing manifest refused without
`--force`, and `--force` **merges** — a non-empty authored field is an
override, an **empty `namespaces` is a gap the scan fills**, which is
what makes the D1 dedicated-workspace-directory bootstrap (the shape
`bench/workspace/corpus.json` uses for the empty `bench/repos/oss-ws`
root) actually produce the corpus manifest; usage errors go through
`fatal()` → exit 1 (exit 2 is only `main.go:38-42`'s arg-count guard);
PHP namespaces = composer `name` **plus** `autoload.psr-4` keys, decoded
as string-**or-array** (laravel-framework's `Illuminate\Support\` maps
to three paths, so a `map[string]string` decode fails outright);
Python namespaces probe `src/` first (both corpus Python members are
src-layout, so a root-only rule silently zeroes 12 of the 65 tasks);
`DetectRootKind` has **no call site** in this slice (§4.2 owns CLI
wiring with the byte-identical golden gate); no new ADR.

The prior round's full residual-item analysis (member-candidate rule
variants, the `merkle.WalkWith` signature constraints, the D7 SHALL
reading) lives in the abstain record in git history and is superseded
by the owner decisions above.

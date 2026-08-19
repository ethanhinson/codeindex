---
id: 9
slug: workspace-manifest-init-scan
title: Workspace manifest load/validate + init-workspace --scan
status: done
priority: high
type: feat
created: 2026-08-17
updated: 2026-08-19
depends_on: []
related: [10]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-18-workspace-manifest-init-scan-design.md
plan: docs/superpowers/plans/2026-08-18-workspace-manifest-init-scan-plan.md
results: docs/results/2026-08-18-workspace-manifest-init-scan-results.md
trivial: false
auto_groomable: true
branch: feat/workspace-manifest-init-scan
claimed_at: 
pr: https://github.com/ethanhinson/codeindex/pull/8
blocked_by:
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-18-workspace-manifest-init-scan-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-18-workspace-manifest-init-scan-design.md) |
| Plan | [2026-08-18-workspace-manifest-init-scan-plan.md](https://github.com/ethanhinson/codeindex/blob/feat/workspace-manifest-init-scan/docs/superpowers/plans/2026-08-18-workspace-manifest-init-scan-plan.md) |
| Results | [2026-08-18-workspace-manifest-init-scan-results.md](https://github.com/ethanhinson/codeindex/blob/feat/workspace-manifest-init-scan/docs/results/2026-08-18-workspace-manifest-init-scan-results.md) |
| PR | [#8](https://github.com/ethanhinson/codeindex/pull/8) |
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
- New verb `init-workspace --scan [--force]`. The scan is **two passes**:
  monorepo member discovery at the workspace root (go.work,
  pnpm-workspace.yaml, composer path repositories, lerna.json, and
  package.json `workspaces` — the last two are a dated amendment to the
  frozen design, owner decision 2026-08-18), then namespace
  auto-discovery per language (go.mod / package.json / composer.json +
  psr-4 / Python top-level modules) run over **every** member root,
  authored ones included. Manifest overrides always win over scanned
  values; an empty `namespaces` is a gap the scan fills.
- Bench manifest bootstrap (in-slice): hand-author a skeleton manifest
  for `bench/repos/oss-ws` (member ids + roots, empty namespaces), then
  `init-workspace --scan --force` fills the namespaces; verify by
  prefix-containment against the `namespaces` field of each member in
  `bench/workspace/corpus.json`. This exercises the `--force` merge path
  end-to-end and hands bench arm B a real manifest.
- Two dated amendments to
  `openspec/changes/workspace-graph/design.md`: the D1 declaration-source
  addition above, and the D7 merge-gate interpretation.
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

### 2026-08-18 — claimed and reconciled against `origin/main` (bb076aa)

**Scope unchanged; the design is intact.** Every "Codebase facts" claim in
the spec was re-verified against the current tree and all hold verbatim —
`config.Load`/`config.FileName`, `config.LoadFilter`, `merkle.WalkWith`'s
no-early-exit signature, `adapter.Indexable` as a process-global read,
`main.go`'s arg-count guard + usage string + `fatal()`, the
`runtime.go:66-71` version-check precedent (including the fusion §1 declines
to copy), and the three named goldens. None of the four deliverables exists
yet (`internal/config/workspace.go`, `internal/workspace/`,
`internal/engine/rootkind.go` all absent), and `go.mod` still requires
neither `golang.org/x/mod` nor a YAML module — so no work has been done
elsewhere and nothing is dropped.

**Two new constraints folded into the spec (new §4a), both mechanical:**

1. **`bench/repos/` is gitignored** (`.gitignore:53`), so the filled bench
   manifest could not be committed as the acceptance checklist requires. The
   ignore rule's own stated reason — "bench working clones; re-clone to
   regenerate" — does not cover `oss-ws`, which is authored, not cloned. The
   spec now specifies a **narrow** negation for the single manifest path, and
   the checklist gained an item asserting no member checkout becomes
   committable as a side effect. Consistent with the owner's standing rule
   that only pinned-OSS bench data reaches git: the manifest names OSS member
   roots only.
2. **`bench/workspace/corpus.json` is not on `origin/main`** — the four
   `bench(workspace)` §2 commits are unpushed local commits on `main`, and the
   feature branch is cut from `origin/main`. `corpus.json` is a *verification
   input*, never a deliverable, so this is not a blocker: the spec now pins
   all 10 `{id, root, namespaces}` triples in a table (read from the local
   `main` tree at reconcile), which doubles as the skeleton's authoring source
   and is quoted into the results file so the prefix-containment check is
   auditable from the PR alone.

All 10 corpus checkouts are present on disk, so the §4 bootstrap's
precondition holds. Owner decisions in `## Groom context` were treated as
binding input and not re-litigated. `related: [10]` remains informational.

## Groom context (owner decisions 2026-08-18)

> **Groomed to build-ready 2026-08-18.** Every decision below is distilled
> into the linked spec, which is now the authoritative design for this
> slice. The autonomous groom's critic gate cleared it after one bounded
> revision round (fixes: the scan's namespace pass must run over authored
> member roots or the bench bootstrap fills nothing; the empty-discovery
> error must not fire on the D1 dedicated-workspace-directory shape; bench
> verification must be prefix-containment, since neither symfony nor drupal
> declares a bare `Symfony\`/`Drupal\` psr-4 key). This section is retained
> as the provenance record; it is not a second source of truth.

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

### Owner decisions, round 2 (2026-08-18)

The second abstain's two blockers and five fixable items were answered
by the owner on 2026-08-18; the full abstain record is in git history
(commit fac8f28):

1. **Monorepo discovery sources — amend the frozen design:** add
   `lerna.json` and `package.json` `workspaces` to the source list, as
   a dated amendment to `openspec/changes/workspace-graph/design.md`
   (the corpus's only monorepo, nest, declares members solely via
   `lerna.json`). Separately, the owner wants the bench corpus grown
   significantly — monorepo declaration examples in every supported
   language, then measured; that is filed as change 0010 and is not
   part of this slice.
2. **Bench manifest bootstrap is in-slice — skeleton + scan:**
   hand-author a skeleton `bench/repos/oss-ws/.codeindex/workspace.json`
   (member ids + roots, empty namespaces); `init-workspace --scan
   --force` fills the namespaces; verify the result against
   `bench/workspace/corpus.json` pins. This makes the `--force`
   empty-namespaces merge path real.
3. **Parsing strategy — add both dependencies:** `golang.org/x/mod` for
   go.work / go.mod parsing and a YAML module (e.g. `gopkg.in/yaml.v3`)
   for pnpm-workspace.yaml. Both pure Go; the single-static-binary ADR
   is unaffected.

Binding fixes carried forward from the abstain's fixable list:

- The "preserve manifest order, sorting only newly appended members"
  rule belongs to the `--force` merge step, not `SaveWorkspace` (whose
  signature only receives the final slice).
- The root-kind short-circuit helper must reuse the real filter
  semantics — `config.LoadFilter` + `adapter.Indexable`, passed
  explicitly — never a diverging hard-coded extension list.
- The D7 merge-ahead interpretation (round-1 decision 3) targets a
  named file — the dated amendment section of
  `openspec/changes/workspace-graph/design.md` — and must appear on the
  spec's acceptance checklist.
- The regression bar is the whole single-repo golden suite, not
  `TestCallersTextGolden` alone.

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

## Auto-groom blocked

**2026-08-18** — autonomous groom abstained after two adversarial critic
rounds. No decision needed *owner* context: every open item is
mechanically fixable from the frozen design plus the repo. What it ran
out of was the protocol's single bounded revision round — round 2 still
returned three "wrong but fixable" items, and a spec may only be emitted
when every item survives. Two rounds of design work are summarized below
so a human can finish this in one short interactive pass rather than
starting over.

### What was settled and survived both rounds

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

### The three residual items a human should close

1. **Member-discovery over-collection (critic items 6 / S6).** The
   revised rule — union of {workspace root if it carries a marker} ∪
   {glob-expanded declared members from go.work, pnpm-workspace.yaml,
   npm/yarn `workspaces`, `lerna.json`, composer `type: path`
   repositories} ∪ {depth-1 marker-bearing subdirs, only when no
   declaration file exists} — correctly rescues symfony (whose
   `composer.json` declares two `type: path` repos, so a
   declarations-exclusive rule dropped symfony itself) and nest (which
   carries **only** `lerna.json`). But as written it also mints three
   spurious members on the frozen corpus: `bench/repos/nest`'s root
   `package.json` is itself `@nestjs/core`, duplicating the namespace of
   `packages/core`; `lerna.json`'s `packages/*` glob matches the *files*
   `packages/index.ts`, `tsconfig.json`, `tsconfig.build.json`; and the
   Python marker "an `__init__.py`-bearing directory at the root" makes
   `flask/src` a member duplicating namespace `flask`. Derivable fixes,
   all in-slice: restrict glob expansion to **directories**; require the
   marker at the candidate's **own** top level rather than one level
   down; and add a duplicate-namespace rule (suppress the root candidate
   when its namespace set is a subset of a declared member's, or reject
   at load). Needs a human only to pick which of the three, since each
   is a different notion of "what is a member".
2. **Root-kind detection cannot be wired as specified (critic S4).**
   `merkle.WalkWith(root string, extra func(rel string, d fs.DirEntry)
   bool) ([]string, error)` loads its own filter internally via
   `config.LoadFilter(root)` and accepts no `*Filter`, so the specified
   `config.Load → config.NewFilter → WalkWith` composition is not
   expressible; it has no abort mechanism, so the specified
   "short-circuit on the first indexable hit" is unattainable; and it
   invokes `extra` **only** for files `adapter.Indexable` already
   rejected (`merkle.go:52`), so `extra` cannot observe the first hit
   either. The mechanical fix is `adapter.SetAssociations(cfg.Associations)`
   then `merkle.WalkWith(root, nil)` and test `len(files) > 0`, dropping
   the short-circuit claim — but that walks the entire tree on every
   detection, which is a cost decision on a hot path, and
   `adapter.SetAssociations` mutates **process-global** registry state,
   so `DetectRootKind` is not side-effect-free against a live MCP
   server. Placement is fine: `internal/engine` is the only non-test
   package that blank-imports the adapters (`engine.go:15-18`), and
   `merkle` imports only `adapter`/`config`/`graph`, so no import cycle.
3. **Whether this slice may merge ahead of the D7 gate.** Not a defect —
   flagged so it is not missed. `specs/workspace-graph/spec.md:125` says
   "Implementation SHALL NOT merge before the pre-registered gate
   passes." This slice ships no query behavior the gate can measure, and
   §3.2–§3.5 cannot start without it. An autonomous groom must not
   reinterpret a frozen SHALL, so the call stays with the owner at the
   human merge gate.

### Recommendation

Groom this interactively (`docket-groom-next`), not by re-arming the
autonomous queue — item 1 is a genuine "what counts as a member" choice
and item 2 trades walk cost against global-state mutation, both of which
are one-question conversations. Neither kill nor defer is warranted:
this is the first engine slice of a GO-registered campaign whose
bench-first phase is fully done, and the work above is close to
complete. To re-arm autonomously instead, answer items 1 and 2, flip
`auto_groomable` back to `true`, and delete this section.

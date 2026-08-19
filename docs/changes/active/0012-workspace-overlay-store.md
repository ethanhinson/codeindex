---
id: 12
slug: workspace-overlay-store
title: Workspace overlay store — member registry, cross-edges by stable key, freshness stamps
status: implemented
priority: high
type: feat
created: 2026-08-19
updated: 2026-08-19
depends_on: [9]
related: [9, 10]
discovered_from: []
adrs: []
spec: docs/superpowers/specs/2026-08-19-workspace-overlay-store-design.md
plan: docs/superpowers/plans/2026-08-19-workspace-overlay-store-plan.md
results: docs/results/2026-08-19-workspace-overlay-store-results.md
trivial: false
auto_groomable: true
branch: feat/workspace-overlay-store
claimed_at: 2026-08-19T09:17:22Z
pr: https://github.com/ethanhinson/codeindex/pull/10
blocked_by:
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-19-workspace-overlay-store-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-19-workspace-overlay-store-design.md) |
| Plan | [2026-08-19-workspace-overlay-store-plan.md](https://github.com/ethanhinson/codeindex/blob/feat/workspace-overlay-store/docs/superpowers/plans/2026-08-19-workspace-overlay-store-plan.md) |
| Results | [2026-08-19-workspace-overlay-store-results.md](https://github.com/ethanhinson/codeindex/blob/feat/workspace-overlay-store/docs/results/2026-08-19-workspace-overlay-store-results.md) |
| PR | [#10](https://github.com/ethanhinson/codeindex/pull/10) |
<!-- docket:artifacts:end -->

## Why

§3.1 (change 0009, merged 2026-08-19) gave the workspace-graph campaign
its identity layer: manifest load/validate and `init-workspace --scan`.
The next engine slice is the storage substrate everything downstream
sits on: the cross-repo resolution ladder (§3.3) needs somewhere to
write cross-edges, workspace freshen (§3.4) needs per-member stamps to
gate re-resolution, and the union-graph query paths (§4) read all of it.
Per design D2 (`openspec/changes/workspace-graph/design.md`), the
overlay is the depfiles hash-overlay pattern promoted to repo
granularity — per-repo `graph.db` files stay untouched, individually
buildable, patchable, and artifact-importable.

This change implements task §3.2 of
`openspec/changes/workspace-graph/tasks.md`.

## What changes

A new flat package `internal/overlay` (sibling to `internal/graph` and
`internal/depmap`, which owns the same shape for dependency maps) holding
`<workspace-root>/.codeindex/workspace.db`, opened by path with an exported
`Path(wsRoot)` helper and the delete-and-rebuild version discipline
`graph.Open` already uses.

Per design D2 (storage — overlay DB, not copy-merge, not query-time
re-resolution) the database holds exactly three things:

- **Member registry** — a mirror of the manifest as-built.
- **Cross-repo edges** — referenced by stable key (member id + file path
  + qualified name), never per-DB rowid; member rebuilds renumber symbol
  ids and the overlay must survive them. Keys re-map to ids at query
  time via the member's own DB. Schema must carry what design D3's
  ladder will write: provenance mechanism (`cross_repo_import` /
  `cross_repo_name`), confidence, ambiguity candidates with counts, and
  the member-over-dep suppression record (for `workspace-status` skew
  reporting later).
- **Per-member freshness stamps** — the member's merkle root at last
  overlay resolution.

The overlay schema carries its own version, independent of the graph.db
schema version; an overlay version bump rebuilds the overlay only
(cheap), never member indexes.

The slice ships the API unwired, as change 0009 shipped `config.Resolve`
and `DetectRootKind` — §3.2 has no CLI verb of its own, and the first
callers belong to §3.3/§3.4. Confidence is stored in the frozen spec's
own vocabulary (`exact` / `inferred`); reconciling that with
`graph.Confidence` at the surface is §4.1's.

## Out of scope

- The resolution ladder itself (§3.3) — this slice provides storage the
  ladder writes into, not the resolver.
- Workspace freshen logic and the `workspace-status` verb (§3.4) — the
  stamps are stored here; the stamp-gated re-resolution policy is §3.4.
- Union-graph query paths and surfaces (§4.x).
- The evidence gate run (§5.x).
- Member discovery changes (change 0010 owns corpus growth; the
  out-of-root member design question raised in 0009's results belongs to
  discovery, not the overlay).

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

### 2026-08-19

Reconciled at claim time against `origin/main`, `origin/docket`, the frozen
campaign design, and change 0009 (merged and archived the same day).

**Verdict: no scope change.** All 22 assumptions and every "Codebase facts
this design is built on" claim in the spec re-verified against the current
working tree and held:

- `internal/config/workspace.go` — `Workspace{Version int, Members []Member}`,
  `Member{ID, Root string, Namespaces, Deps []string}`;
  `func (w *Workspace) Resolve(wsRoot string) (present []ResolvedMember,
  missing []string, err error)` unchanged. `validate()` still inspects only
  `ID` and `Root`, never `Namespaces`/`Deps` — so decision 16's Go-side
  de-duplication is still load-bearing, not defensive.
- Member id charset still `A-Z a-z 0-9 . _ -` (no `:`, no `/`) — decision 4's
  three-column stable key still the only unambiguous representation.
- `internal/graph/store.go` — `const schemaVersion = 9`; `Open(path string)
  (*Store, error)`, `OpenRaw(path string) (*sql.DB, error)`,
  `SchemaVersion() int`, `FileSchemaVersion(path string) (int, error)`;
  rebuild warning `"codeindex: index schema v%d -> v%d, rebuilding\n"`. The
  clone target for §2 is intact.
- `internal/graph/types.go` — `Confidence` is still `unambiguous` /
  `ambiguous` / `unresolved`, so decision 8's `exact`/`inferred` divergence
  and its §4.1 hand-off both still stand.
- `internal/overlay` does **not** exist — this is a greenfield package, no
  merge surface.
- `internal/workspace` still imports neither `database/sql` nor
  `internal/graph` (decision 1's premise); `internal/depmap` still the flat
  sibling precedent importing `internal/graph`.
- `go.mod` — module `codeindex`, Go 1.26.5, `github.com/mattn/go-sqlite3
  v1.14.47` already direct. No new module dependency.
- `openspec/changes/workspace-graph/tasks.md` — §3.1 is `[x]`, §3.2 is the
  next unchecked engine task and matches this change's scope verbatim.

**Dependency:** `depends_on: [9]` satisfied — 0009 archived as
`docs/changes/archive/2026-08-19-0009-workspace-manifest-init-scan.md`.

**Nothing dropped as done-elsewhere; no new constraint folded in.** Test gate
remains `go test -tags nollama ./...` per `.docket.local.yml` (decision 15).

<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0014 — Workspace freshen internals — per-member freshen + stamp-gated re-resolution](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0014-workspace-freshen-internals.md)**
<!-- docket:backlink:end -->

# Workspace freshen internals — implementation plan

**Change:** 0014 `workspace-freshen-internals` · **Spec:**
`docs/superpowers/specs/2026-08-20-workspace-freshen-internals-design.md` (on `origin/docket`,
including its **Reconcile addendum — 2026-08-20**, which is binding) · **Depends on:** 0013 (merged)

**Suite (honest):** `go test -tags nollama ./...` — fully green on `origin/main`. Zero new failures
allowed. Plain `go test ./...` fails 10 packages on every ref (environmental, not ours) — never use
it as the gate.

**Plan authored by the implementer directly:** the configured `plan` role skill
(`superpowers:writing-plans`) is not invocable on this machine, so the role degraded to `auto` per
the convention's missing-skill rule.

---

## What is being built

One new unwired internal entry point, `internal/wsfresh.Freshen(wsRoot) (Report, error)`, plus one
small additive export on `internal/overlay`. No verb, no CLI/MCP surface, no non-test caller. The
`workspace-status` verb is **out** — gated at verb wiring by the D7 second amendment.

The pass, in order: detect root kind → load manifest → split present/missing → for each present
member, availability = `graph.OpenExisting` succeeds → per-member `query.Fresh` → re-fold
`MemberMerkleRoot` → compare against `overlay.Stamp` (absent = dirty) → compare registry drift on
whole normalized member records → **gate**: no dirty member and no drift ⇒ zero overlay content
writes; otherwise exactly one whole-pass `wsresolve.Resolve`.

## Invariants that must not be broken

These are load-bearing; a task that finds itself wanting to relax one is a task that has gone wrong.

- **One availability predicate at both sites.** `graph.OpenExisting` success, nothing else — no
  file-exists check. A looser predicate in `Freshen` than in `Resolve` leaves a version-mismatched
  member permanently dirty and re-resolves the whole workspace forever (spec Assumption 10, test 7).
- **Never cold-build.** Availability is established *before* `query.Fresh` is called, so `Fresh`
  always takes its `engine.Patch` + `depmap.VerifyOverlay` branch and never the cold-build branch at
  `query.go:75-83`. A freshness check must not silently index an arbitrarily large member.
- **Reuse the canonical fold.** `(*graph.Store).MemberMerkleRoot()` — never forked, never
  re-implemented. Its value is **opaque**: equality only, never parsed, ordered, or split.
- **Absent stamp ⇒ dirty.** That is the crash-self-healing signal 0013's stamps-last ordering leaves.
- **Dirty ⇒ one whole-pass `wsresolve.Resolve`.** No per-member scoped re-resolution — explicitly
  deferred by the spec. Do not attempt it. `ReplaceMemberEdges` deletes on either endpoint, so the
  naive per-member loop is self-clobbering (learnings
  `symmetric-replace-makes-per-entity-loops-self-clobbering`).
- **Manifest order, single-threaded.** `query.Fresh` holds a package-level mutex for its whole body,
  so parallelism serializes to nothing and determinism is a stated bar.
- **`internal/wsresolve` is frozen.** Its rung order, never-thin semantic, clear→put→stamp-last write
  order, and `Resolve(wsRoot string) (Stats, error)` signature are not touched.

## Verified API surface

Confirmed against `origin/main` at reconcile; build against exactly these.

```go
wsresolve.Resolve(wsRoot string) (Stats, error)   // Stats{MembersResolved, MembersUnavailable, CrossEdges, Ambiguities, Suppressions int}
graph.OpenExisting(path string) (*graph.Store, error)
(*graph.Store).MemberMerkleRoot() (string, error) // sha256 hex, opaque
overlay.Open(path string) (*overlay.Store, error)
overlay.Path(wsRoot string) string
(*overlay.Store).Registry() ([]config.Member, error)          // manifest order
(*overlay.Store).ReplaceRegistry(ws *config.Workspace) error
(*overlay.Store).Stamp(memberID string) (overlay.Stamp, bool, error)  // (Stamp{}, false, nil) when absent
(*overlay.Store).Stamps() ([]overlay.Stamp, error)
(*overlay.Store).MemberEdges(memberID string) ([]overlay.CrossEdge, error)
(*overlay.Store).AmbiguitiesFor(memberID string) ([]overlay.Ambiguity, error)
(*overlay.Store).Suppressions() ([]overlay.Suppression, error)
config.LoadWorkspace(wsRoot string) (*config.Workspace, error)
(*config.Workspace).Resolve(wsRoot string) (present []config.ResolvedMember, missing []string, err error)
config.Member{ID, Root string; Namespaces, Deps []string}
engine.DetectRootKind(root string) (engine.RootKind, error)  // constant engine.RootWorkspace
query.Fresh(root string) (query.FreshInfo, error)
```

Member index path is `<memberAbsRoot>/.codeindex/graph.db` (mirrors `wsresolve.memberIndexPath`).

---

## Task 1 — export one registry normalizer from `internal/overlay`

**Why (addendum R1).** Step 6 requires normalizing the manifest side with *exactly*
`ReplaceRegistry`'s transforms. Those are the unexported `dedupe()` applied to `Namespaces` and
`Deps` inside the unexported `insertMembers`; `dedupe` returns `nil` for empty input, which is where
the spec's nil-coalescing comes from. `wsfresh` cannot reach either. Copying the three lines into
`wsfresh` is **rejected** — it is the `one-invariant-many-sites-drifts` shape, and its divergence
mode is exactly the permanent-drift non-convergence test 5's sibling exists to catch.

**Do:**

1. Add to `internal/overlay/registry.go` an exported function returning the normalized form of a
   member list exactly as `ReplaceRegistry` stores it — for each member, `Namespaces` and `Deps`
   replaced by `dedupe(...)` (first-occurrence order, `nil` when empty), `ID`/`Root` untouched,
   member order preserved. Name it for what it is (e.g. `NormalizeMembers`). It must not mutate its
   input — return fresh slices.
2. **Route the writer through it**: change `insertMembers` to consume that function's output instead
   of calling `dedupe` inline, so there is one implementation with two callers. This is the whole
   point of the task; an export that leaves `insertMembers` calling `dedupe` separately has
   re-created the second site in a different place.
3. Doc comment states the contract explicitly: *this is what `Registry()` will return, so a drift
   comparison normalizes the manifest side with this before comparing.*

**Verify:** additive and behavior-preserving — no schema change, no change to what `ReplaceRegistry`
writes. Existing `internal/overlay` tests must pass untouched. Add a focused unit test: a member with
a duplicate namespace and one with `Deps: []string{}` normalize to first-occurrence-deduped and `nil`
respectively, and the input slice is not mutated.

---

## Task 2 — the `wsfresh` package skeleton and `Report`

**Do:** create `internal/wsfresh/wsfresh.go`, package doc stating it is unwired, has no verb and no
non-test caller, and citing the D7 second amendment for why `workspace-status` is not here.

```go
type Report struct {
    MembersFreshened int             // available members whose per-repo freshen ran
    MembersUnindexed int             // present on disk, graph.OpenExisting failed
    MembersMissing   []string        // declared, absent from disk — manifest order
    Dirty            []string        // member ids whose stamp was absent or moved — manifest order
    Resolved         bool            // whether wsresolve.Resolve ran
    Stats            wsresolve.Stats // MEANINGFUL ONLY WHEN Resolved is true; zero value otherwise
}
```

`MembersUnindexed` is deliberately **not** `MembersUnavailable`: the embedded
`Stats.MembersUnavailable` counts declared-but-unusable members *including missing* ones, under
0013's invariant `MembersResolved + MembersUnavailable == len(declared)`. Two denominators under one
name is the drift shape this repo has already paid for twice. Both the field's doc comment and
`Stats`' must say so, and they must not contradict each other — check them against each other, not
just against the spec.

`Report` is plain data: no coverage clause, no confidence vocabulary, no status string. §4.1 owns
naming stale/missing members to a user.

---

## Task 3 — `Freshen`: steps 1–5 (root kind, manifest, availability, freshen, fold, stamp gate)

**Do:** implement `func Freshen(wsRoot string) (Report, error)` through the per-member dirty
determination.

1. `engine.DetectRootKind(wsRoot)`; error unless `engine.RootWorkspace`. Checked **before**
   `LoadWorkspace` so a repo root reports what it actually is rather than a bare `fs.ErrNotExist`
   (mirrors `wsresolve.Resolve` steps 1–3).
2. `config.LoadWorkspace(wsRoot)`, then `ws.Resolve(wsRoot)` → `present`, `missing`.
   `Report.MembersMissing = missing` (manifest order, as returned).
3. For each `present` member **in manifest order**: `graph.OpenExisting(<absRoot>/.codeindex/graph.db)`.
   On error → `MembersUnindexed++`, continue; the member is **not** built and its overlay rows are
   left alone. On success, close the handle before step 4 — `query.Fresh` writes to that same
   `graph.db` via `engine.Patch`, so holding an open handle across it is asking for lock contention.
   Re-open after the freshen for the fold.
4. `query.Fresh(memberAbsRoot)`. A per-member freshen **error is not fatal**: count the member
   unindexed, do not add it to the freshened set, and keep going — the workspace must keep answering
   (spec Assumption 6, D2's present/missing split). On success `MembersFreshened++`.
5. Re-open with `graph.OpenExisting` and call `MemberMerkleRoot()`; then `ov.Stamp(memberID)`.
   **Dirty** iff the stamp is absent (`ok == false`) **or** present and unequal to the fold. Append
   dirty ids in manifest order.

Open the overlay with `overlay.Open(overlay.Path(wsRoot))` and `defer ov.Close()`. Opening is
permitted on the clean path — `overlay.Open` re-executes the schema and `PRAGMA user_version` every
time, so the file is never a no-write witness; what the clean path must not do is write **content**.

**Do not** call `ReplaceRegistry` here. That is the trap: it is a content write, and calling it
"just to be safe" before the gate silently defeats the whole change and would be caught only by
test 1's `Registry()` conjunct.

---

## Task 4 — `Freshen`: step 6 (registry drift) and step 7 (the gate)

**Do:**

1. **Drift.** `ov.Registry()` → `[]config.Member`. Normalize the loaded manifest's `ws.Members` with
   Task 1's exported normalizer. Compare **whole records** — id, root, ordered namespaces, ordered
   deps, and member order — with `reflect.DeepEqual` over the normalized slices. Comparing raw
   `ws.Members` reports drift forever on a legal manifest carrying a duplicate namespace or
   `"deps": []`; comparing only the id set lets a `namespaces:`/`deps:` edit change resolution while
   every root and stamp stays put, which is the silent staleness D7 hard-fails.
2. **Gate.** `len(dirty) == 0 && !drift` ⇒ return the Report with `Resolved: false` and **no overlay
   content write of any kind** — no registry, edge, ambiguity, suppression or stamp row.
   Otherwise ⇒ exactly one `wsresolve.Resolve(wsRoot)`; set `Resolved: true` and carry its `Stats`
   into the Report. An unindexed or missing member does **not** by itself make the pass dirty.

Note `Resolve` re-derives root kind, manifest, presence and the member opens that `Freshen` just did
— accepted, per Assumption 11, because `Resolve(wsRoot string)` is frozen and widening it is 0013
re-litigation. The duplicated opens cost only on the dirty path. This does plant a second
enforcement site for the present/available/manifest-order invariants, which is why tests 6 and 7
assert the two sites agree.

---

## Task 5 — test fixtures for `internal/wsfresh`

**This task is the one with a real trap in it.** `internal/wsresolve`'s fixtures write synthetic
member indexes directly through `graph.Open` (`writeIndex` in `wsresolve_test.go`). Those are
**unusable here**: `Freshen` runs `query.Fresh`, which runs `engine.Patch` over the member's real
working tree, and `MemberMerkleRoot` folds real per-file content hashes. A synthetic index has no
source behind it, so the fold cannot move when "source is edited" and tests 2 and 4 would be
untestable — or, worse, vacuously green.

**Do:** build fixtures from **real source files** parsed by the real engine.

1. Lay out `<wsRoot>/<memberID>/` with actual source files in a language the engine parses, then
   build each member's index the way the product does (`query.Fresh` on the member root, or
   `engine.Build`) — not by hand-writing rows.
2. Write the manifest with `config.SaveWorkspace`, declaring members in a fixed order.
3. Reuse `wsresolve_test.go`'s `memberState` vocabulary — indexed / no-index / bad-version / absent —
   for the unavailability cases; the bad-version fixture must produce an index that exists but fails
   `graph.OpenExisting`.
4. **Every fixture has ≥2 members with a real cross-edge between them**, except where a test's own
   subject requires otherwise. A single-member fixture cannot reproduce the bugs this area has —
   0012's either-end incidence and 0013's self-clobbering loop are both invisible with one member.
5. Provide the **content-equality helper** (addendum R2): `MemberEdges` and `AmbiguitiesFor` are
   scoped by `memberID`, so the helper iterates the registry's members **in manifest order**,
   concatenates each member's edges and ambiguities, adds the whole-store `Registry()`, `Stamps()`
   and `Suppressions()`, and sorts on a key **total over every field** — for `CrossEdge` the
   `Src`/`Dst` triples plus `Kind`, `Provenance`, `Confidence`, `Line`; for `Ambiguity` the `Src`,
   `RefName`, `RefNS`, `Kind`, `Line`, `Count` and the ordered candidate list. A prefix key passes
   vacuously (learnings `determinism-tests-need-a-total-sort-key`). Iterate over the **manifest's**
   members, not the registry's, so a registry write that *drops* a member cannot compare equal.
6. Do **not** use `resolved_at` as a witness — it is 1-second-granular. Do not use file bytes or a
   file hash — `overlay.Open` rewrites schema pragmas on every open. Compare **content**.

---

## Task 6 — the eight tests

All in `internal/wsfresh`, all under `-tags nollama`.

1. **Clean workspace writes no overlay content.** Freshen twice; the second reports `Resolved: false`
   and `Dirty` empty, and the full content tuple from Task 5 is unchanged. `Registry()` is in the
   tuple because drift is a gate input, so a spurious `ReplaceRegistry` on the clean path must not
   pass unseen.
2. **Moved root re-resolves.** Edit a member's **source** so its fold changes; the next Freshen marks
   exactly that member dirty, `Resolved: true`, and the overlay reflects the edit.
3. **Crash self-healing.** Delete one member's stamp row directly (simulating a pass that died before
   0013's stamps-last step); the next Freshen marks that member dirty, re-resolves, and the restored
   content equals a from-scratch pass's.
4. **Cross-member freshness.** Change dirty member `M` so clean member `S`'s edge into `M` must
   change; assert the `S → M` row is correct afterwards. **This is the test that goes red under naive
   incident-only scoping** — it is why the pass is whole. Verify it bites: temporarily scope
   re-resolution to the dirty member and watch it fail, then restore. A test in this family that has
   not been broken on purpose very often asserts something the wrong code also satisfies.
5. **Manifest-only drift.** Edit only a member's `namespaces:` — no source change, every root
   unchanged, every stamp matching — and the next Freshen must report `Resolved: true`. Repeat for
   `deps:`.
   **Sibling — drift comparison converges.** A manifest carrying a duplicate `namespaces:` entry, and
   one carrying `"deps": []`, must **each** report `Resolved: false` on the second Freshen. Without
   Task 1's normalization these re-resolve forever. Both cases are required; they are the critic's
   binding A5 additions.
6. **Unindexed / missing members.** A declared member absent from disk lands in `MembersMissing`; a
   present member with no index counts unindexed, **is not built** (assert no `graph.db` is created
   under it), and neither alone makes the pass dirty.
7. **Convergence with a version-mismatched member.** Two consecutive Freshens over a workspace with
   one member whose index exists but fails `graph.OpenExisting`; the second must report
   `Resolved: false`. A predicate looser than the resolver's leaves that member perpetually dirty.
8. **Determinism.** Two consecutive dirty passes over identical content produce identical overlay
   content on the total sort key.

---

## Task 7 — correct the three stale sites in the change stub

Spec Assumption 12: three sites describe the rejected per-member-scoped shape and would otherwise
leave the stub arguing with its own spec. **These live on `metadata_branch`, not the feature branch**
— the feature branch never modifies docket metadata. Hand them back to the implementer to apply as a
metadata commit rather than editing them from the feature worktree.

The three: the change `title:`; the "stamp-gated incident re-resolution … scoped to incident edges"
bullet; and the `## Why` paragraph, which carries both rejected claims independently (edges "incident
to any member whose merkle root moved", and "unchanged members cost one stamp comparison" — rejected
as literal by Assumption 9). Restate all three to the whole-pass decision, naming the scoping
deferral and the real standing cost (a clean member still pays a per-repo freshen and a full merkle
fold; what the clean branch guarantees is zero overlay *content* work).

---

## Gate

`go test -tags nollama ./...` green across the whole repo — not just `internal/wsfresh`. Task 1
touches `internal/overlay`, so its existing suite is part of the evidence.

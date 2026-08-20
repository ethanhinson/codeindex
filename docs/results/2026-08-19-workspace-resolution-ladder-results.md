<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0013 — Cross-repo resolution ladder — import-mediated exact, bare-name inferred, ambiguity, suppression](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0013-workspace-resolution-ladder.md)**
<!-- docket:backlink:end -->

# Cross-repo resolution ladder — results

Change: #0013 · Branch: feat/workspace-resolution-ladder · PR: (opened at close of this run) ·
Plan: docs/superpowers/plans/2026-08-19-workspace-resolution-ladder-plan.md · ADRs: none

## Verify (human)

Nothing here is a substitute for the suite — `go test -tags nollama ./...` is green at
`28f4853`. These are the two things no automated test on this branch can reach:

- [ ] **Confirm the deferred `.codeindex/graph.db` path consolidation is wanted.** This slice added
      a fifth open-coded copy of the `<root>/.codeindex/graph.db` join (`wsresolve.memberIndexPath`),
      alongside `internal/webserver/graphstore.go`, `internal/readmodel/graph.go`,
      `internal/query/query.go`, and `internal/engine/artifact.go`. The reviewer explicitly scoped
      the consolidation out of this slice and it was left as a `TODO` naming all five sites. Decide
      whether that becomes its own change.
- [ ] **Confirm the §4.1 hand-off wording is what you want a future slice bound to.** The
      obligation is prose in `internal/wsresolve`'s package doc and nothing mechanically enforces it
      (see Findings). If you would rather it were an ADR or a test fixture, now is the moment.

## Findings

**No ADR was minted, deliberately.** Every non-obvious decision on this slice — the `graph.Open`
ban, the forced clear-then-write order, the rung-2 reading, member-over-dep re-pointing — was made
at *design* time and is recorded with full rationale in the frozen spec's 17 assumptions, in the
shipped doc comments, and in the plan file (which merges with the code). The implementation made
only local shape choices: a `Pass` resolver struct, a typed `graph.VersionMismatchError`, and the
`MembersUnavailable` accounting. An ADR here would duplicate an existing record, not create a
missing one. Note that `terminal_publish` is `false` in this repo, so the *spec* stays on the
`docket` branch — the durable on-`main` record of these rules is the code's own doc comments plus
the merged plan.

**Two important review findings, both fixed in-branch.** Both are worth remembering because
neither was a test failure — the suite was green before and after:

1. **The recorded cross-slice obligation was over-broad** (`746340a`). The package doc asserted that
   a suppression is always accompanied by a cross-edge, so a future §4.1 implementing the obligation
   literally would delete a consumer's still-correct tier-1 edge and put nothing in its place. The
   re-point path disproves the premise — `TestRepointedEdgeFallsThroughWhenOwnerLacksTheName`
   already asserted one suppression record with zero cross-edges. The obligation is now conditioned
   on a cross-edge existing for the same call site (same source key, kind, line). This is the
   `one-invariant-many-sites-drifts` shape exactly: the defect was visible as two doc comments
   disagreeing, and no test could have caught it.
2. **The new readers' `ORDER BY` was not a total order** (`89545f6`). `dst_qualifier` and `dst_ns`
   were selected but not ordered on, and `edges_t` has no unique constraint, so two calls to the
   same name on one line differing only in qualifier tied on every sort key. Worse, the two
   determinism tests compared two reads of the *same* store — the assertion a non-total sort key
   passes vacuously. Fixed by appending `dst_qualifier, dst_ns, id`, and the fixtures now carry a
   deliberate tie pair so the determinism tests have something to bite on.

**Residual test-guard caveats the workers flagged, none blocking:**

- The "member indexes are never written" byte-snapshot guard compares `graph.db` only. It would not
  catch a stray `-wal`/`-shm` sidecar. `OpenExisting` opens `mode=ro`, so none is created in
  practice, but a future change enabling WAL would need the guard widened.
- The `e.src_symbol_id != 0` predicate in `UnresolvedEdges`/`TierOneEdges` is not independently
  mutation-provable: the inner join to the source symbol already excludes file-level import edges.
  It is kept as explicit intent and index-usability, not as the enforcing mechanism.
- The ladder fixtures are hand-built Go-shaped indexes, so language-specific `ProjectDefs` and
  namespace-derivation behavior is covered only through the shared `nsPrefixCases()` table.
- No test asserts the pass-scoped cache actually reduces query count — `Member` exposes no call
  counter and adding one would mean touching `internal/graph`. The win is argued from the code.

## Follow-ups

- **§4.1 must honor the narrowed suppression filter.** When the union-graph query path lands, it
  reads `dep_suppressions` and filters a consumer's intra-repo tier-1 edge **only when** a cross-edge
  exists for the same call site. Implementing the unconditional version drops correct edges. Stated
  in `internal/wsresolve`'s package doc; nothing enforces it mechanically.
- **§4.1 also owns the `exact`/`inferred` ↔ `graph.Confidence` reconciliation** — the campaign's
  pre-existing recorded hand-off, untouched here.
- **Index-path consolidation** (see Verify above) — five sites, `TODO` left in place.
- **`Stats` counters are "derived", not "written".** `PutCrossEdges` upserts and `putAmbiguities`
  deletes-then-inserts on natural keys, so two records sharing a key collapse to one row while the
  count still counts both. The docs now say so; if a caller ever needs the persisted count, it must
  dedupe by natural key rather than trust `Stats`.
- **Deferred §3.5 bars, unchanged from the spec:** stamp *gating* is §3.4's (this slice writes
  stamps and reads none); the single-member-workspace ≡ single-repo bar is §4.2's (it needs a query
  path). The assertable degenerate part — a one-member workspace writes no cross-edges — is covered
  here.
- **Adapter gap the ladder inherits:** Go `extends`/`implements` edges carry no namespace hint
  (the Go adapter's `addDep` never sets `Source`, so `PutFile`'s `bind` map is always empty for Go),
  so Go interface embedding never reaches rung 1 and falls to rungs 2/3. Out of scope here; noted so
  it is not mistaken for a ladder defect.

## Notable plan deviations

- **The plan role degraded to `auto`.** `superpowers:writing-plans` is not invocable on this
  machine, so the plan was authored directly under the convention's missing-skill rule. Same for
  `superpowers:finishing-a-development-branch` at the PR step.
- **Task 2 landed two commits** rather than one (`8f43111` feature, `301a2b1` a gofmt correction of
  the test file) — a follow-up commit, never an amend. Later tasks were told to run `gofmt -l`
  before committing and none repeated it.

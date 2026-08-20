---
id: 15
slug: wsresolve-stamp-pruning
title: Stamp pruning for unavailable members — close the stale-edges-after-unavailability hole
status: done
priority: high
type: fix
created: 2026-08-20
updated: 2026-08-20
depends_on: [14]
related: [13, 14]
discovered_from: [14]
adrs: []
spec: docs/superpowers/specs/2026-08-20-wsresolve-stamp-pruning-design.md
plan: docs/superpowers/plans/2026-08-20-wsresolve-stamp-pruning-plan.md
results: docs/results/2026-08-20-wsresolve-stamp-pruning-results.md
trivial: false
auto_groomable: true
branch: feat/wsresolve-stamp-pruning
claimed_at: 
pr: https://github.com/ethanhinson/codeindex/pull/13
blocked_by:
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-20-wsresolve-stamp-pruning-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-20-wsresolve-stamp-pruning-design.md) |
| Plan | [2026-08-20-wsresolve-stamp-pruning-plan.md](https://github.com/ethanhinson/codeindex/blob/main/docs/superpowers/plans/2026-08-20-wsresolve-stamp-pruning-plan.md) |
| Results | [2026-08-20-wsresolve-stamp-pruning-results.md](https://github.com/ethanhinson/codeindex/blob/main/docs/results/2026-08-20-wsresolve-stamp-pruning-results.md) |
| PR | [#13](https://github.com/ethanhinson/codeindex/pull/13) |
<!-- docket:artifacts:end -->

## Why

Change 0014 shipped `wsfresh.Freshen` with a recorded limitation
(pinned by a labelled characterization test and `Freshen`'s doc
comment): a member that goes available → unavailable *after* being
stamped leaves its stale cross-edges in the overlay while the gate
reports clean — the member can't be freshened or folded, its stamp
never moves, so nothing looks dirty. The naive fix (treat unavailable
as dirty) loops forever: an unavailable member can never be re-stamped,
so every subsequent freshen re-resolves the whole workspace.

While nothing is wired this is unreachable; but the gated §4 unit wires
queries to the overlay, and D7 makes silent staleness a hard fail —
this hole must close before §4 builds. Owner decision 2026-08-20
(merge-gate review of PR #12): file as its own high-priority change,
sequenced ahead of §4 via `depends_on`.

## What changes

Three coordinated edits; the design is settled in the linked spec.

- `internal/overlay`: add `(*Store).DeleteStamp(memberID)` — the only
  new overlay surface. Additive and idempotent.
- `internal/wsresolve.Resolve`: signature stays frozen. A new step 9a,
  before the clear/write phases, prunes every member in
  `DECLARED ∖ available` (availability still the single
  `graph.OpenExisting` predicate): records first via
  `ReplaceMemberEdges(U, nil, nil, nil)` — never-thin unchanged — then
  the stamp. Records-first/stamp-last is the same stamp-last rule 0013
  records for writes, applied to deletes: a mid-prune crash must leave a
  stamp that contradicts the rows, because that surviving stamp is the
  only remaining trigger.
- `internal/wsfresh.Freshen`: `Report` gains `StaleStamped` — declared
  members unavailable this pass that still carry a stamp — and the gate
  trips on it. Semantics become prune-then-clean: pass 1 resolves and
  prunes, pass 2 onward is clean and writes nothing, and the member's
  later return is caught by 0014's absent-stamp-is-dirty rule.

0014's characterization test flips from pinning the hole to asserting
the fix. `wsresolve.TestMissingMembersLeftAlone`'s seeded orphan row
joining two unavailable members is knowingly reversed — that row is
exactly the class of edge the prune exists to remove. Six doc/test sites
carry the old invariant and all move together.

ADR-0012 stands unamended; no new ADR.

## Out of scope

- Any verb wiring (D7 second amendment — the gate binds at wiring).
- Union-graph queries, coverage clauses (§4.x).
- Distinguishing "member root deleted" from "member DB version
  mismatch" beyond what `graph.OpenExisting` already reports — both are
  unavailable; finer states are §4's coverage-clause vocabulary if ever.

## Owner sign-off (2026-08-20, merge gate)

The build's departure from spec assumption 4 is accepted: the prune is
split 9a (records) + 11a (stamps, after all writes succeed), so a
mid-pass error leaves the stale stamp armed as the `StaleStamped`
trigger instead of a clean-but-empty overlay. Records-first/stamp-last
at pass scale is the ratified semantic.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

### 2026-08-20

Reconciled against `origin/main` @ `16b2f37` — which is exactly the 0014
merge commit the spec's assumption 10 names, so no drift has accumulated
since the design was groomed. Verified rather than assumed:

- `depends_on: [14]` satisfied — 0014 archived `done`
  (`docs/changes/archive/2026-08-20-0014-workspace-freshen-internals.md`).
- All three target packages present and unchanged since the spec was
  written: `internal/overlay` (`stamps.go` carries `PutStamp`/`Stamp`/
  `Stamps` and no `DeleteStamp`, so §1 is genuinely additive),
  `internal/wsresolve`, `internal/wsfresh`.
- All six doc/test sites §4 says must move together still carry the old
  invariant verbatim: `wsresolve.Resolve`'s "gets no stamp, and its
  overlay rows are not cleared" and "Rows joining two unavailable members
  are never touched"; `Stats.MembersUnavailable`'s field comment;
  **both** sections of `wsfresh.Freshen`'s doc comment (`# What a clean
  pass does and does not entail`'s "one known exception" and
  `# Known limitation`, including the "the honest fix is stamp pruning
  inside wsresolve.Resolve" forward reference this change discharges);
  the inline step-8 gate comment's "do not close it here"; and the two
  tests.
- Related changes 13 and 14 are both archived `done`; no in-flight change
  touches these packages, so no scope needs dropping.

No scope change, no new constraints folded in, no ADR. Spec stands as
groomed; the 11 assumptions remain binding, including the two the spec
itself flags as most worth a second look — assumption 6 (a separate
`StaleStamped` field rather than widening `Dirty`, forced by three live
assertions that an unavailable member never appears in `Dirty`) and
assumption 7 (knowingly reversing `TestMissingMembersLeftAlone`'s
orphan-row-survives assertion on the authority of the human-authored
stub, not the sibling test's licence).

Auto-capture is disabled for this repo (`AUTO_CAPTURE_ENABLED=false`), so
no stubs were minted; no follow-up work surfaced during reconcile beyond
what the spec's *Out of scope* already records.

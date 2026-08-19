<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0012 — Workspace overlay store — member registry, cross-edges by stable key, freshness stamps](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0012-workspace-overlay-store.md)**
<!-- docket:backlink:end -->

# Workspace overlay store — results

Change: #0012 · Branch: feat/workspace-overlay-store · PR: (opened at close of this run) ·
Plan: docs/superpowers/plans/2026-08-19-workspace-overlay-store-plan.md · ADRs: none (spec decision 14)

## Verify (human)

- [ ] **Confirm the underspecified candidate rule is what you want.** The frozen spec never said
      what `ReplaceRegistry`'s pruning should do with an ambiguity that merely *names* a dropped
      member among its candidates. Review settled it as **never thin a candidate list — delete the
      parent ambiguity whole**, at both `ReplaceRegistry` and `ReplaceMemberEdges`. This is the one
      semantic the later slices inherit that the spec did not pin down. See Finding 1/2 below.
- [ ] **Confirm no ADR is wanted for that rule.** Spec decision 14 declines an ADR for this slice
      (D2 already froze overlay-vs-copy-merge at campaign level), so none was minted. If you read
      the never-thin rule as an architecture decision rather than an implementation of D3's frozen
      ladder ordering, it wants an ADR before §3.3 builds on it.

## Findings

Six review findings (0 blocker, 3 important, 3 minor), all fixed in-branch before the PR opened;
the suite was green before the fixes and green after. Dispositions are in the PR body.

**The two that matter — one incoherence seen from two sites.**

The frozen spec specifies *either-end* incidence for `cross_ambiguities` in three places, but the
initial build implemented `ReplaceMemberEdges`' ambiguity delete as **source-side only**, and
`ReplaceRegistry`'s prune as source-side plus a **candidate-row thin**. Those two are not merely
inconsistent, they argue against each other: `ReplaceMemberEdges`' own doc comment cited "would
strand a `candidate_count` contradicting its own rows" as the reason *not* to thin, while registry
pruning did exactly that thinning. The observable consequence was a surviving ambiguity reporting
`Count: 2` with one candidate row — indistinguishable from the legitimate upstream-truncation case
the `Count >= len(Candidates)` validation deliberately permits, so §3.4's skew reader would have
mis-reported silently. Both are now either-end and both delete whole (705497d, 8590d2c).

**Scope guards (de7b4ca).** `ReplaceMemberEdges` validated only `sup` against `memberID`. A
cross-edge touching neither end, or an ambiguity incident to neither end, could be written by the
call and never removed by a later call for the same member — a permanently un-refreshable row, and
a `ReplaceMemberEdges` not idempotent over its own input. The accept set now equals the delete set
exactly. No caller existed yet, so this was a trap set for §3.3/§3.4 rather than a live defect.

**Coverage gaps closed.** The rollback path of both transaction sites was previously unexercised —
every error the suite provoked was rejected *before* a transaction opened, so "a bad batch never
half-applies" was proven only for pre-validation (17a562c). The no-rowid schema guard was a bare
`_id$` suffix heuristic that would have missed `symbol_rowid`, `srcid`, or a plainly-named
`dst_symbol INTEGER`; it now also asserts no unexpected `INTEGER` column exists, which is the shape
a rowid reference takes regardless of its name (8c60da9).

**Honest residuals recorded by the workers, not papered over:**

- `ReplaceMemberEdges`' "rejected before any write" property is observably enforced only by
  transaction rollback: moving the validation inside the transaction leaves the suite green. A
  future refactor splitting the deletes across two transactions would not be caught.
- The mid-transaction rollback test for the edges side depends on SQLite allocating
  `INTEGER PRIMARY KEY` as max+1. It is documented in the test and defended by an error-substring
  guard, so rowid-allocation drift fails loudly rather than passing for the wrong reason.
- `assertAmbiguitiesCoherent` asserts `candidate_count == COUNT(candidates)` exactly. That holds for
  every ambiguity the tests seed but is **not** a production invariant — production may legally
  carry `Count > len(Candidates)` from upstream truncation. The helper's doc comment says so.

## Follow-ups

- **The package ships unwired, by design** (spec decision 12) — no CLI verb, no caller. §3.3's
  resolution ladder and §3.4's stamp-gated freshen own its first callers. Nothing in-tree can
  currently refill an emptied overlay, but nothing fills it in the first place either, so the
  delete-and-rebuild window is empty by construction.
- **Merkle-root aggregation is the one real gap handed forward** (spec decision 9). `MerkleRoot` is
  an opaque TEXT token this slice never computes or parses; there is no repo-level merkle root in
  the tree today (the `merkle` table is per-file). §3.4 must define the aggregation, most likely by
  extending `internal/merkle`.
- **Confidence-vocabulary reconciliation is §4.1's** (spec decision 8). The overlay stores the
  frozen spec's `exact`/`inferred`; `graph.Confidence` uses `unambiguous`/`ambiguous`/`unresolved`.
  Mapping them at the surface was deliberately not done here.
- **Plan-role degradation, machine-local.** `skills.plan` (`superpowers:writing-plans`) is not
  invocable on this machine, so the plan was authored inline under the convention's missing-skill
  rule. Same for `skills.finish`. Not a repo condition — no change needed unless it recurs on CI.

## Notable plan deviations

None. All five plan tasks executed as written, in order, one commit each, no escalations and no
stray commits. The five additional commits on the branch are the review fix loop.

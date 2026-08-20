<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0014 — Workspace freshen internals — per-member freshen + stamp-gated re-resolution](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0014-workspace-freshen-internals.md)**
<!-- docket:backlink:end -->

# Workspace freshen internals — results

Change: #0014 · Branch: feat/workspace-freshen-internals · PR: (see change `pr:`) ·
Plan: docs/superpowers/plans/2026-08-20-workspace-freshen-internals-plan.md · ADRs: 12

## Verify (human)

Nothing here requires a manual check. `internal/wsfresh.Freshen` is unwired — no verb, no CLI flag,
no MCP surface, no non-test caller — so there is no runtime surface to exercise by hand. The suite
plus the PR diff are the whole receipt. The one thing worth a human's *reading* eye rather than a
human's hands is Finding 3 below, which records a deliberate limitation a later slice must not
"fix" naively.

## Findings

Eight review findings (0 blocker, 4 important, 4 minor); all eight were repaired in-branch before
the PR opened. Full per-finding disposition is in the PR body's table — recorded here is only what
outlives this change.

**Became an ADR.**

- **ADR-0012 — workspace freshness re-resolves the whole workspace.** The load-bearing design call.
  Design D2's literal "re-resolve only overlay edges incident to that member" is not implementable on
  change 0013's frozen `wsresolve.Resolve(wsRoot)` API, for two independent reasons: the deriving
  call is scoped by edge *source*, not by the dirty member; and a hint that previously resolved
  uniquely in a clean member can become an ambiguity whose row has two clean endpoints, so the
  incident set is not closed under re-derivation. Scoped re-resolution is **deferred, not rejected** —
  it stays available as a measured optimization, and the D7 gate is where its cost would be
  justified. Recorded because §4 and the D7 measurement both depend on knowing this was decided
  rather than overlooked.

**Worth carrying forward, not ADR-shaped.**

- **The honest standing cost of a clean pass.** D2's prose says unchanged members cost "one stamp
  comparison". That is false as stated and the spec already rejected it: a clean member still pays a
  full per-repo `query.Fresh` plus a complete `MemberMerkleRoot` fold, because cleanliness is
  unknowable without folding. What the clean branch *does* guarantee is zero overlay **content**
  writes. The D7 latency measurement should use the real number.

- **Finding 3 — an available→unavailable transition leaves stale cross-edges, reported clean.** If a
  member that was resolved and stamped later loses its index, it is skipped as unindexed, it is not
  dirty (only *available* members' stamps are read), the manifest is unchanged so there is no drift —
  and the gate holds while the overlay keeps serving cross-edges into the vanished member. The
  detection signal exists and is deliberately unread: a stamp surviving for a member that is now
  unavailable. **Do not act on it without first adding stamp pruning to `wsresolve.Resolve`** —
  marking such a member dirty on its own re-resolves the whole workspace on every single pass, which
  is exactly the non-convergence `TestFreshenConvergesWithABadVersionMember` forbids. The limitation
  and its prerequisite are written into `Freshen`'s doc comment, and
  `TestKNOWNLIMITATIONVanishedMemberLeavesStaleEdgesReportedClean` pins today's behavior as a
  characterization test — it asserts what the code does, **not** what it should do.

- **Two content witnesses have named, documented blind spots.** `overlayContent` compares overlay
  *content*, so a perfectly idempotent write — a `ReplaceRegistry` with an unchanged manifest, or a
  `PutStamp` with an unchanged root — remains invisible to it. The clean-path guard was given a tooth
  by planting an orphan `member_stamps` row that only `pruneOrphans` would remove, which makes a
  stray `ReplaceRegistry` observable; that guard is therefore coupled to `ReplaceRegistry` continuing
  to prune unconditionally, and its test comment says so. Catching a genuinely no-op write would need
  a driver-level statement counter, which this slice did not add.

- **Reconcile surfaced a structural seam (R1).** `overlay.dedupe()` and `insertMembers` are
  unexported, so the drift comparison could not reuse `ReplaceRegistry`'s own normalization. Rather
  than copy three lines into `wsfresh` — the `one-invariant-many-sites-drifts` shape, whose
  divergence mode is precisely the permanent drift the convergence tests exist to catch —
  `overlay.NormalizeMembers` was exported and `insertMembers` routed through it, so the store's write
  path and the drift read are one implementation with two callers.

## Follow-ups

None minted. `auto_capture` is disabled for this repo, and every finding was about this branch's own
diff — which is fixed or recorded, never minted. Three items are carried as prose for whoever picks
up §4:

1. **Stamp pruning in `wsresolve.Resolve`** — the prerequisite for ever closing the Finding 3 gap.
   Touching `Resolve` re-opens change 0013's frozen surface, so it is a change of its own.
2. **Scoped (source-closure) re-resolution** — ADR-0012's deferred optimization. Gate it on a D7
   measurement, not on intuition; the whole-pass cost is bounded by the unresolved-edge count, not
   the symbol count.
3. **`memberIndexPath` is now open-coded in six places** across `wsresolve` and `wsfresh`. A
   consolidation was noted and deliberately not taken in this slice; it is a tidy-up, not a defect.

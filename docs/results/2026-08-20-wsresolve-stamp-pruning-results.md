<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0015 — Stamp pruning for unavailable members — close the stale-edges-after-unavailability hole](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0015-wsresolve-stamp-pruning.md)**
<!-- docket:backlink:end -->

# Stamp pruning for unavailable members — results
Change: #0015 · Branch: feat/wsresolve-stamp-pruning · PR: (see change `pr:`) · Plan: docs/superpowers/plans/2026-08-20-wsresolve-stamp-pruning-plan.md · ADRs: none

## Verify (human)

- [ ] Confirm you accept the **reversal of spec assumption 4** recorded under *Findings* below —
      the resolver's stamp deletion moved from step 9a to a new step 11a, after the writes. This
      is the one place the build knowingly departed from the groomed design, and the departure is
      argued from the spec's own safety principle rather than against it. Everything else in the
      spec was implemented as written.

## Findings

Six review findings (0 blocker, 2 important, 4 minor); all six were fixed in-branch. Their
per-finding dispositions and commits are in the PR body's disposition table. Recorded here is only
what outlives the diff.

### The one design departure: stamp-last had to hold at pass scale, not just per member

The spec's load-bearing safety rule is *records first, stamp last* — a partial prune must leave a
stamp that contradicts the rows, because that surviving stamp is the only remaining trigger
`StaleStamped` can fire on. Spec assumption 4 placed the whole of step 9a (records **and** stamp)
before step 9, and asserted the placement was "a readability choice, not a correctness one".

Review established that assertion was **overbroad**. Its supporting argument analysed only
*clobbering* — `Ladder` and `Suppress` draw candidates solely from `available`, so nothing steps 10
and 11 write is incident to a prune-set member — and never analysed *error recovery*. With the
stamp deleted at 9a, an ordinary error return from `PutCrossEdges`, `PutAmbiguities`,
`PutSuppressions` or `MemberMerkleRoot` leaves: the pruned member's rows gone **and** its stamp
gone, every available member's rows cleared by step 9 but not rewritten, and every available member
still carrying its previous stamp — which still matches its unchanged merkle root. So `Dirty` is
empty, `ReplaceRegistry` already ran at step 5 so there is no drift, and `StaleStamped` is empty
because the trigger was destroyed. The next `Freshen` returns a clean gate over an overlay holding
no cross-edges at all, permanently.

Assumption 4's rejection list never considered "stamp-delete after step 11", so this is a gap in
the supporting argument rather than a re-litigation of a weighed decision. The prune is now two
loops over one `prune []string` (built by walking `ws.Members` once, so the manifest-order and
no-concatenation rules are unchanged and single-sourced): `ReplaceMemberEdges` at step 9a, and
`DeleteStamp` at a new step 11a after the stamps are written. Records-first/stamp-last now holds at
**both** scales. The cost is assumption 4's readability preference — deletion is no longer all in
one region of the pass — which is the right trade against a silent permanent staleness.

Consistent with spec assumption 9, this produced **no ADR**: the spec explicitly considered and
rejected recording the delete ordering at ADR altitude, on the grounds that it belongs in
`Resolve`'s doc comment next to the code it constrains. Extending the same rule to pass scale lands
in the same place, and ADR-0012 stands unamended.

### Residual risks the build named and did not close

1. **A pre-existing crash window this change does not reach.** A pass with an **empty** prune set
   that dies between step 9 and the end of step 10 leaves a partly-rewritten overlay under stamps
   that all still match, and the gate stays clean until some member's content moves. This predates
   change 0015 — it is a property of 0013's clear/write split, not of the prune — and closing it
   needs a cross-call transaction the overlay API deliberately does not offer. It is now documented
   in `Resolve`'s `# Crash safety` section rather than left implied; the section previously claimed
   "a pass that dies part-way leaves the affected members stampless", which is true only of a
   first-ever pass.
2. **Never-thin under prune is pinned, not proven for the harder case.** The ambiguity test asserts
   that an ambiguity sourced from an *available* member naming an unavailable member only as a
   candidate is deleted whole and re-derived without it — but that assertion was already satisfied
   before step 9a existed, because step 9 clears and step 10 rewrites anything sourced from an
   available member regardless. No arrangement of the spec's stated scenario can go RED before the
   prune lands, so it stands as a characterization pin carried by a genuinely RED stamp assertion in
   the same test. A future implementation that thinned candidates *instead of* deleting whole would
   be caught here only if the source member were also unavailable — that case is uncovered.
3. **One order assertion is fixture-order dependent.** `StaleStamped`'s manifest-order promise is
   now tested with two members of *different* unavailability kinds (present-but-unopenable declared
   before absent-from-disk), which is what gives it teeth against the forbidden
   `missing`-then-unopenable concatenation. That teeth depends on the fixture declaring them in
   that order; a future edit reordering `twoKindsWS`'s manifest would silently defang the order half
   of the test. The fixture's doc comment says so explicitly.

### Verification quality notes

Three mutation checks were run and reverted, so the new guards are known to have teeth rather than
assumed to: deleting the `missing` seed loop turns the new coverage test RED; replacing the
`ws.Members` walk with the forbidden concatenation turns it RED with the members in reverse order;
and widening the trigger set from `ws.Members` to `ov.Stamps()` — the exact widening the spec
forbids — turns `TestFreshenCleanPassWritesNoOverlayContent` RED, confirming the undeclared `ghost`
orphan-stamp tripwire still guards that boundary. The resolver's new crash-window test induces a
real step-11 error return (dropping a member index's `merkle` table, which `graph.OpenExisting`
still accepts but `MemberMerkleRoot` rejects) rather than a fabricated injection, and guards itself
against vacuity by first failing unless step 10's write actually landed.

## Follow-ups

- **The six open-coded `.codeindex/graph.db` joins.** Still a standing TODO, noted at both
  `wsresolve.memberIndexPath` and `wsfresh.memberIndexPath`. Explicitly out of scope for this
  change; should be consolidated across all sites at once.
- **The empty-prune-set crash window** (residual risk 1). Needs a cross-call overlay transaction, so
  it is a design question rather than a patch.
- Auto-capture is disabled for this repo (`AUTO_CAPTURE_ENABLED=false`), so neither of the above was
  minted as a stub; both are recorded here for a human to file if wanted. No review finding was a
  mint candidate in any case — every one was about this branch's own diff, which is never mintable.

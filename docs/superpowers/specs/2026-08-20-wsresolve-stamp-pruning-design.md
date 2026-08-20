<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0015 — Stamp pruning for unavailable members — close the stale-edges-after-unavailability hole](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0015-wsresolve-stamp-pruning.md)**
<!-- docket:backlink:end -->

# Stamp pruning for unavailable members — design

Change: 0015 `wsresolve-stamp-pruning` (type `fix`, priority `high`, `depends_on: [14]` — satisfied,
0014 archived done 2026-08-20, merge 16b2f37).

Groomed autonomously by `docket-auto-groom` on 2026-08-20. Every decision below was defaulted
without a human; the `## Assumptions` block is the audit trail.

## Problem

`internal/wsfresh.Freshen`'s doc comment and the labelled characterization test
`TestKNOWNLIMITATIONVanishedMemberLeavesStaleEdgesReportedClean`
(`internal/wsfresh/gate_test.go:263`) pin a hole shipped knowingly by change 0014:

A member that goes **available → unavailable after being stamped** leaves its stale cross-edges,
ambiguities and suppressions in the overlay while the freshen gate reports clean. `Freshen` skips an
unavailable member at step 5a *before* reading its stamp, the manifest is untouched so there is no
registry drift, so `Dirty` is empty and the gate holds. The overlay goes on serving `app -> lib`
edges into a member that no longer exists, and `Report` has no field that can say so.

The naive repair — read the stamp at 5a and mark the unavailable member dirty — never converges:
`wsresolve.Resolve` never retires the stamp of a member it could not open, so the member is dirty on
every subsequent pass and the whole workspace re-resolves forever. That non-convergence is
independently forbidden by `TestFreshenConvergesWithABadVersionMember`
(`internal/wsfresh/gate_test.go:173`).

`Freshen`'s own doc comment already names the ordering the fix must follow: prune in
`wsresolve.Resolve` **first**, and only then may the freshen pass treat a surviving stamp for an
unavailable member as a trigger.

## Decision

Two coordinated edits plus one additive overlay method.

### 1. `internal/overlay` — `(*Store).DeleteStamp(memberID string) error`

Additive, single-row delete on `member_stamps`, idempotent (deleting an absent stamp is not an
error). This is the only new overlay surface. Nothing else in `internal/overlay` changes; in
particular `ReplaceMemberEdges`' either-end delete semantics, the never-thin ambiguity rule, and the
`exact`/`inferred` vocabulary are untouched and are not re-litigated here.

### 2. `internal/wsresolve.Resolve` — prune the unavailable set

`Resolve`'s signature stays frozen (`Resolve(wsRoot string) (Stats, error)`). Inside the pass, define

```
prune set = DECLARED members ∖ available
```

where `available` is exactly the existing slice built at step 6 — i.e. availability remains the
single predicate `graph.OpenExisting` succeeding, shared verbatim with `wsfresh`. The prune set is
therefore `missing` (absent from disk, per `config.Workspace.Resolve`) **plus** every present member
whose `graph.OpenExisting` failed. It is exactly the set `Stats.MembersUnavailable` counts.

For each member `U` in the prune set, **in manifest order**, executed as a new step **9a, immediately
before the existing step 9 clear loop and therefore before any of step 10's writes**:

1. `ov.ReplaceMemberEdges(U, nil, nil, nil)` — records first. Cross-edges and ambiguities go by the
   existing either-end delete (never-thin applies unchanged: an ambiguity naming `U` only as a
   candidate is deleted whole, not thinned). **Suppressions are not either-end** — the call deletes
   `dep_suppressions WHERE consumer_member = U` only, leaving rows where `U` is merely the owner
   (`internal/overlay/edges.go:128-132`, decision 21). The prune's outcome is still complete by a
   different argument: every surviving suppression's consumer is either available — cleared at step 9
   and re-derived at step 10 without `U`, since `Suppress` draws only from `available` — or itself
   unavailable, and cleared by its own step 9a call — or, third branch, its consumer is no longer
   declared at all, in which case `ReplaceRegistry`'s `pruneOrphans` already killed it earlier in the
   same pass, deleting `dep_suppressions` on **either** column
   (`internal/overlay/registry.go:170-173`).
2. `ov.DeleteStamp(U)` — stamp last.

**Records-first, stamp-last is load-bearing, and it is the same stamp-last rule 0013 records for
writes, applied to deletes:** any partial state must leave a stamp that contradicts the rows, so something is still
detectable. A crash between the two leaves `U` unavailable and **still stamped**, which is exactly
what `StaleStamped` (§3) fires on — the gate trips on every subsequent pass until the prune
completes.

The reverse (stamp first) is unsafe and was the draft's original error. Absent-stamp-is-dirty
(`internal/wsfresh/freshen.go:201-210`) is only reached by members that passed availability at 5a, so
an unavailable member never enters `Dirty` by that route; and deleting the stamp destroys the only
other signal, `StaleStamped`. A crash after a stamp-first delete would therefore leave stale rows
served with **no signal left that could ever fire again** — silently re-creating the very hole this
change closes. The harm asymmetry points the same way: a records-first crash under-serves
(recoverable, and re-triggering), a stamp-first crash over-serves stale cross-edges, which is D7's
hard-fail condition.

**Placement: before step 10, and before step 9 for readability.** The three loops are in fact
disjoint in effect — `Ladder(m, available, …)` (`internal/wsresolve/ladder.go:88`) draws candidates
only from `available`, and `Suppress(available)` names only available members on both columns — so
nothing step 10 or 11 writes is incident to a member in the prune set, and 9a could not clobber them
wherever it sat after them either. Putting it first keeps all deletion in one region of the pass and
makes the disjointness argument unnecessary rather than load-bearing.

The prune is **unconditional within a pass** (no "does it have rows?" pre-check). Idempotence is
structural: a second `Resolve` over the same unavailable member deletes nothing because there is
nothing left, and — the load-bearing half — a second `Freshen` does not call `Resolve` at all,
because the stamp that triggered it is gone.

`Stats` gains no field. `MembersResolved + MembersUnavailable == len(declared)` holds unchanged, and
both `wsfresh` identities in `Report`'s type comment hold unchanged.

### 3. `internal/wsfresh.Freshen` — trigger on a surviving stamp, in a new `Report` field

`Report` gains:

```go
// StaleStamped lists the ids of DECLARED members that are unavailable this
// pass yet still carry an overlay stamp, in manifest order.
StaleStamped []string
```

`Freshen` populates it by reading `ov.Stamp(id)` for **every declared member not available this
pass** — both the `missing` ids and the present-but-unopenable ones — so the trigger set is the same
`DECLARED ∖ available` set `Resolve` prunes. **Populate by iterating `ws.Members` once**, not by
concatenating the `missing` list with the ids collected in the present loop: those are two disjoint
manifest-ordered subsequences, and their concatenation is not manifest order, which the field's doc
comment promises. This needs one small mechanical addition: the present-member loop must **retain
the ids** that failed `graph.OpenExisting` (today it only increments `MembersUnindexed`,
`internal/wsfresh/freshen.go:157-161`), so the `ws.Members` pass has a set to filter against. The
gate becomes:

```go
if len(rep.Dirty) == 0 && len(rep.StaleStamped) == 0 && !drift { return rep, nil }
```

A stamp **read** is not a content write, so the clean path still writes no overlay content and
`TestFreshenCleanPassWritesNoOverlayContent` continues to hold.

Resulting semantics for the hole — **prune-then-clean**:

| pass | `lib` state | `StaleStamped` | `Resolved` | overlay |
|---|---|---|---|---|
| 1 (after `lib` vanishes) | unavailable, stamped | `[lib]` | `true` | pruned: no `app -> lib` edges, no `lib` stamp |
| 2 | unavailable, unstamped | `[]` | `false` | unchanged — nothing written |
| 3+ | unavailable, unstamped | `[]` | `false` | unchanged |

`lib`'s later **return** to availability is caught by 0014's absent-stamp-is-dirty rule at step 6 and
re-derives cleanly. Convergence holds in both directions, and
`TestFreshenConvergesWithABadVersionMember` (`gate_test.go:173`) is unaffected: its member is
`stateBadVersion` from construction, so it is never stamped and `StaleStamped` is empty from **pass
1** onward.

### 4. Docs and tests that must move together

The invariant "unavailable members' overlay rows are not cleared" is asserted at **six** sites today
(site 3 spans two sections of one doc comment, so seven passages in all). All of them move in this
change (the `one-invariant-many-sites-drifts` learning: check the sites against
each other, drift shows up as doc comments arguing):

1. `wsresolve.Resolve`'s **"Missing and unavailable members"** doc section — "gets no stamp, and its
   overlay rows are not cleared" and "Rows joining two unavailable members are never touched" both
   become false and are rewritten to state the prune, including the mid-prune crash state (`U`
   stamped, rows gone) and the `StaleStamped` re-trigger that recovers it.
2. `wsresolve.Stats.MembersUnavailable`'s field comment (`internal/wsresolve/wsresolve.go:50-52`) —
   "left untouched" becomes false.
3. `wsfresh.Freshen`'s doc comment — **both** the `# What a clean pass does and does not entail`
   section (`internal/wsfresh/freshen.go:37-41`, whose "there is one known exception, and it is the
   available -> unavailable transition" no longer holds) and the `# Known limitation` section, which is
   replaced by a statement of the
   prune-then-clean semantics and the records-first/stamp-last ordering; the forward reference to
   "the honest fix is stamp pruning inside `wsresolve.Resolve`" is discharged.
4. `wsfresh.Freshen`'s **inline step-8 gate comment** (`internal/wsfresh/freshen.go:242-249`), which
   restates the limitation and instructs "do not close it here" — the site a reader of the code hits
   first, and the one most likely to be left arguing with its own function's doc comment.
5. `TestKNOWNLIMITATIONVanishedMemberLeavesStaleEdgesReportedClean` — **inverted and renamed** (e.g.
   `TestVanishedMemberIsPrunedThenReportsClean`): pass 1 asserts `StaleStamped == [lib]`,
   `Resolved == true`, the `app -> lib` edge **gone** and `lib`'s stamp **absent**; pass 2 asserts
   `Resolved == false` and byte-stable overlay content. Its own comment already authorizes exactly
   this inversion, conditional on the prune landing.
6. `wsresolve.TestMissingMembersLeftAlone` (`internal/wsresolve/wsresolve_test.go:386`) — **the
   seeded `gone1 -> gone2` orphan row must now be pruned, not survive.** This is the one place the
   change knowingly reverses a 0013 assertion, and the reversal is the point: an edge joining two
   members the resolver could not see is precisely an edge the overlay must not carry. Unlike site 5,
   this test's own comment contains **no** authorization to invert it — the authorization comes from
   the human-authored stub ("so the overlay never carries edges for members the resolver could not
   see", `0015-wsresolve-stamp-pruning.md:53-54`), and the rewritten comment must cite that, not lean
   on the sibling test's licence. Note also that the stub's line 51 phrasing — "remove the member's
   overlay stamp and its incident … records" — enumerates *what* to remove, not a sequence; it does
   not re-mandate stamp-first against §2's ordering. The test keeps its stamp assertions and flips its edge assertion;
   the rename should say so (`TestMissingMembersArePruned`).

**Tripwire to preserve.** `TestFreshenCleanPassWritesNoOverlayContent` (`gate_test.go:58`) plants an
orphan `member_stamps` row for the **undeclared** id `ghost`. The prune set is `DECLARED ∖ available`,
so `ghost` is out of scope and the tripwire survives — but any implementation that widens the set to
"every stamped member not available this pass" erases it and breaks that test. Undeclared stamps stay
`ReplaceRegistry`/`pruneOrphans` territory (`internal/overlay/registry.go:165-167`), untouched here.

New tests:

- `wsresolve`: a member stamped by pass 1, made unavailable, then a pass 2 that leaves its stamp
  absent and its incident rows gone while available members' rows survive.
- `wsresolve`: never-thin under prune — an ambiguity sourced from available `S` naming unavailable
  `U` only as a candidate is deleted **whole**, and `S` re-derives it without `U`.
- `wsfresh`: the three-pass convergence table above, with pass 2's "wrote nothing" witnessed
  **structurally** (`Resolved == false` ⇒ `Resolve` never ran) rather than by a content compare —
  per the `content-witnesses-cannot-see-idempotent-writes` learning, a content witness passes
  vacuously for a write that rewrites the same bytes. The existing `overlayContent` helper is used as
  the corroborating, not the primary, assertion.
- `overlay`: `DeleteStamp` removes exactly one row and is a no-op on an absent stamp.

The test-only raw-SQL `deleteStamp` helper in `internal/wsfresh/fixtures_test.go` is **left as raw
SQL** and is not switched to the new production method: it is the independent witness the fixture
meta-test `TestDeleteStampRemovesExactlyOneStamp` relies on, and routing it through the code under
test would make that test circular.

## Out of scope

- Any verb, CLI flag or MCP surface (the D7 wiring split — no verb in this slice).
- Union-graph queries and coverage clauses (§4.x).
- Incident-scoped or per-member re-resolution — ADR-0012 stands; the prune rides the existing
  whole-pass `Resolve`.
- Distinguishing "member root deleted" from "member DB version mismatch" — both are unavailable.
- Consolidating the six open-coded `.codeindex/graph.db` joins (standing TODO, all sites at once).

## ADR

No new ADR. ADR-0012 (whole-pass freshen) is unchanged and is the frame this rides in; the prune adds
no new decision at ADR altitude — it discharges a limitation ADR-0012's consequences section already
anticipated ("crash self-healing … stamps are written last"). Recording the records-first, stamp-last *delete*
ordering as an ADR was considered and rejected: it is the same stamp-last rule 0013 already records
for writes, applied to deletes, and lives correctly in `Resolve`'s doc comment next to the code it
constrains.

## Verification

`go test -tags nollama -count=1 ./...` (the pinned honest suite, `.docket.local.yml`) must be green.
Plain `go test ./...` fails 10 packages on every ref for environmental reasons and is not the bar.

## Assumptions

Every decision an interactive brainstorm would have raised, the default committed to, and the
alternatives rejected.

1. **Where the prune lives.** *Chosen:* inside `wsresolve.Resolve`, as a new step 9a before the clear
   loop. *Rejected:* a new exported `wsresolve.Prune(wsRoot)` — a second entry point over the same
   manifest/presence/open work, and a second site that could drift from the availability predicate;
   *rejected:* pruning inside `wsfresh` — the change file and `Freshen`'s own doc comment both
   explicitly forbid closing this in the freshen pass alone.
2. **Prune-set definition.** *Chosen:* `DECLARED ∖ available`, i.e. exactly what
   `Stats.MembersUnavailable` counts, with availability still the single `graph.OpenExisting`
   predicate. *Rejected:* pruning only present-but-unopenable members (leaves the deleted-root case,
   the characterization test's own scenario, unfixed); *rejected:* any `os.Stat`-based widening (the
   two-predicates drift the code warns about twice).
3. **Delete ordering within a member: records first, stamp last.** *Chosen* for the crash-convergence
   argument above — a mid-prune crash must leave the stamp contradicting the rows, because the
   surviving stamp is the only thing `StaleStamped` can fire on. *Rejected:* stamp-first — the
   draft's original choice, corrected in this revision after the critic pass showed its convergence
   argument was false: absent-stamp-is-dirty (`freshen.go:201-210`) is unreachable for an unavailable
   member, so a stamp-first crash destroys the last signal and permanently restores the hole, with
   over-serving (D7 hard-fail) rather than under-serving as the failure mode. *Rejected:* one
   transaction spanning both — the overlay API offers no cross-call transaction and 0013 deliberately
   declined to add one; the ordering carries the safety instead.
4. **Prune placement relative to steps 9/10.** *Chosen:* before step 9 — all deletion in one region
   of the pass. *Rejected:* interleaved into the step 9 loop (mixes two operations under one loop's
   rationale). The placement is a readability choice, not a correctness one: steps 10 and 11 write
   nothing incident to a member in the prune set, since `Ladder` and `Suppress` both draw only from
   `available`.
5. **New overlay method vs. reuse.** *Chosen:* add `DeleteStamp`. *Rejected:* `PutStamp(id, "")` as a
   tombstone — invents a second absent-ness encoding the freshen gate would have to learn;
   *rejected:* a composite `overlay.PruneMember(id)` doing both deletes — it would fix the ordering
   inside the overlay, away from the `Resolve` doc comment that argues for it, and adds a method with
   exactly one caller.
6. **How `Freshen` signals the trigger: new `StaleStamped` field vs. widening `Dirty`.** *Chosen:* a
   new field. `Dirty`'s stated definition is "stamp absent or moved away from the member's re-folded
   merkle root" — an unavailable member has no re-fold, so putting it in `Dirty` makes that doc false
   and breaks `Dirty ⊆ freshened members`. This package has already paid twice for one name over two
   denominators (see `MembersUnindexed`'s comment), and this repo's discipline is to split.
   *Rejected:* widening `Dirty` — which is literally the phrasing `Freshen`'s doc comment forecasts
   ("treat a surviving stamp for an unavailable member as dirty"), so this is the assumption most
   worth a human's second look; the forecast is read here as naming the *trigger semantics*, not
   mandating the field, and widening `Dirty` would additionally collide with three live assertions
   that an unavailable member must not appear in it (`freshen_test.go:179`, `fixtures_test.go:505`,
   `gate_test.go:194-196`). *Rejected:* no new field, deriving the trigger inline in the gate — hides a
   user-visible state transition from `Report`, which is the type whose whole job is to describe the
   pass.
7. **Reversing `TestMissingMembersLeftAlone`'s orphan-row assertion.** *Chosen:* the `gone1 -> gone2`
   row is now pruned. *Rejected:* excluding rows joining two unavailable members from the prune to
   preserve the 0013 assertion — it would keep exactly the class of stale edge this change exists to
   remove, and would need a second, narrower incidence rule inside `wsresolve` that
   `ReplaceMemberEdges` does not offer.
8. **`Stats` gains no counter.** *Chosen:* no field; `wsresolve` tests assert the prune directly
   against overlay content. *Rejected:* `Stats.MembersPruned` — a per-pass count is a weak witness
   (it is nonzero on every pass with an unavailable member, pruned or already-pruned) and widening
   `Stats` re-touches a type 0013 documented as a strict two-way partition.
9. **No ADR.** As argued above; ADR-0012 stands unamended.
10. **Dependency state.** `depends_on: [14]` is satisfied — 0014 archived `done` 2026-08-20
    (merge 16b2f37) — so no design-ahead caveat applies; all named code is on `origin/main` at that
    commit.
11. **Unwired stays unwired.** No verb, no caller. The change remains unreachable in production until
    §4/D7 wiring lands, so the risk of the reversed assertions is confined to the test suite.

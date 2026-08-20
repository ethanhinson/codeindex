<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0015 — Stamp pruning for unavailable members — close the stale-edges-after-unavailability hole](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0015-wsresolve-stamp-pruning.md)**
<!-- docket:backlink:end -->

# Stamp pruning for unavailable members — implementation plan

Change 0015 `wsresolve-stamp-pruning` (type `fix`, priority `high`).
Spec: `docs/superpowers/specs/2026-08-20-wsresolve-stamp-pruning-design.md` on
`origin/docket` — **the spec is the authority; this plan is its task
breakdown.** Base: `origin/main` @ `16b2f37`.

> **Plan-role degrade.** `superpowers:writing-plans` is not invocable on this
> machine, so the plan role fell back to `auto` (docket's missing-skill rule)
> and this file was authored directly by the implementer. Content obligations
> are unchanged.

## Ground rules for every task

- **Suite.** The honest suite is `go test -tags nollama -count=1 ./...` (pinned
  in `.docket.local.yml`). Plain `go test ./...` fails 10 packages on every
  ref for environmental reasons and is **not** the bar — never "fix" those.
- **Frozen surfaces.** `wsresolve.Resolve(wsRoot string) (Stats, error)` keeps
  its signature. `Stats` gains **no** field. `overlay.DeleteStamp` is the
  **only** new overlay surface — `ReplaceMemberEdges`' either-end delete
  semantics, the never-thin ambiguity rule and the `exact`/`inferred`
  vocabulary are untouched and are not re-litigated.
- **Availability has exactly one predicate:** `graph.OpenExisting` succeeding.
  No `os.Stat`, no file-exists check, no widening. Two sites (`wsresolve`,
  `wsfresh`) must keep agreeing.
- **Tripwire that must survive.** `TestFreshenCleanPassWritesNoOverlayContent`
  (`internal/wsfresh/gate_test.go:58`) plants an orphan `member_stamps` row for
  the **undeclared** id `ghost`. The prune set is `DECLARED ∖ available`, so
  `ghost` is out of scope and the tripwire lives. Any implementation that
  widens the set to "every stamped member not available this pass" erases it.
  Undeclared stamps stay `ReplaceRegistry`/`pruneOrphans` territory.
- **Do not** switch the test-only raw-SQL `deleteStamp` helper in
  `internal/wsfresh/fixtures_test.go` to the new production method — it is the
  independent witness `TestDeleteStampRemovesExactlyOneStamp` relies on, and
  routing it through the code under test makes that test circular.
- **Learnings that bear on this change** (read before touching the code):
  - `one-invariant-many-sites-drifts` — six doc/test sites carry the old
    invariant; read them **against each other**, not just against the spec.
    Drift shows up as doc comments arguing.
  - `content-witnesses-cannot-see-idempotent-writes` — a "wrote nothing" guard
    that compares stored content passes vacuously. Witness pass 2
    **structurally** (`Resolved == false` ⇒ `Resolve` never ran); use a content
    compare only as corroboration.
  - `known-limitations-need-a-characterization-test` — this change is the
    prerequisite that 0014's characterization test named; the test flips from
    pinning the hole to asserting the fix.
  - `symmetric-replace-makes-per-entity-loops-self-clobbering` — why the prune
    loop is safe: it passes empty inputs, so there is no write for a later
    iteration to clobber, and it runs entirely before step 9/10.

---

## Task 1 — `overlay.(*Store).DeleteStamp`

**Profile:** economy. Fully specified, single-statement, pattern-following
alongside the existing `PutStamp`/`Stamp`/`Stamps` in the same file.

**File:** `internal/overlay/stamps.go`, plus `internal/overlay/stamps_test.go`.

Add, next to `PutStamp`:

```go
// DeleteStamp removes memberID's stamp. Deleting an absent stamp is not an
// error: the call is idempotent, which is what lets wsresolve.Resolve prune
// unconditionally without a "does it have one?" pre-check.
func (s *Store) DeleteStamp(memberID string) error {
	_, err := s.db.Exec(`DELETE FROM member_stamps WHERE member_id = ?`, memberID)
	return err
}
```

**Tests** (`stamps_test.go`, following the file's existing fixture style):

1. `DeleteStamp` removes **exactly one** row — seed at least **two** stamps,
   delete one, assert the other survives byte-for-byte (id, merkle root,
   resolved-at). A single-row fixture cannot catch an over-broad `DELETE`.
2. `DeleteStamp` on an **absent** member id returns `nil` and leaves every
   existing row untouched.

**Done when:** `go test -tags nollama -count=1 ./internal/overlay/...` is green
and the full suite is no worse than base.

---

## Task 2 — `wsresolve.Resolve` step 9a: prune `DECLARED ∖ available`

**Profile:** premium. Named risk: this is the crash-ordering decision, it
reverses a live 0013 assertion, and it rewrites two doc sites whose current
text is load-bearing for readers of the next slice. Correctable, not
irreversible.

**File:** `internal/wsresolve/wsresolve.go`, plus
`internal/wsresolve/wsresolve_test.go`.

### 2a. The prune loop

Insert a new **step 9a immediately before the existing step 9 clear loop** (and
therefore before every step-10 write). Build the prune set by iterating
`ws.Members` **once** — this is what makes the order manifest order — filtering
out the ids in `available`:

```go
// 9a. Prune every DECLARED member that is not available this pass ...
avail := make(map[string]bool, len(available))
for _, m := range available {
	avail[m.ID] = true
}
for _, dm := range ws.Members {
	if avail[dm.ID] {
		continue
	}
	// records first ...
	if err := ov.ReplaceMemberEdges(dm.ID, nil, nil, nil); err != nil {
		return stats, fmt.Errorf("wsresolve: pruning member %q: %w", dm.ID, err)
	}
	// ... stamp last.
	if err := ov.DeleteStamp(dm.ID); err != nil {
		return stats, fmt.Errorf("wsresolve: pruning member %q: %w", dm.ID, err)
	}
}
```

Constraints on this loop, all load-bearing:

- The prune set is exactly what `Stats.MembersUnavailable` counts: `missing`
  (absent from disk) **plus** every present member whose `graph.OpenExisting`
  failed. Derive it from `ws.Members ∖ available`, **not** by concatenating
  `missing` with a separately-collected unopenable list — those are two
  disjoint manifest-ordered subsequences and their concatenation is not
  manifest order.
- **Records first, stamp last**, per member. This is the same stamp-last rule
  0013 records for writes, applied to deletes. A mid-prune crash must leave the
  member unavailable and **still stamped**, because that surviving stamp is the
  only thing `StaleStamped` can fire on. Stamp-first is unsafe: absent-stamp-is-dirty
  (`internal/wsfresh/freshen.go:201-210`) is only reachable by members that
  passed availability, so an unavailable member never enters `Dirty` by that
  route — deleting the stamp first destroys the last remaining signal and
  silently restores the hole, with over-serving (D7 hard-fail) as the failure
  mode instead of under-serving.
- **Unconditional within a pass** — no "does it have rows?" pre-check.
  Idempotence is structural: a second `Resolve` deletes nothing because nothing
  is left, and a second `Freshen` does not call `Resolve` at all, because the
  stamp that triggered it is gone.
- `Stats` gains no field. `MembersResolved + MembersUnavailable == len(declared)`
  is unchanged.

Note on suppressions, which is **not** a gap: `ReplaceMemberEdges` deletes
`dep_suppressions WHERE consumer_member = U` only (not either-end,
`internal/overlay/edges.go:128-132`), leaving rows where `U` is merely the
owner. The prune is still complete by a different argument — every surviving
suppression's consumer is either available (cleared at step 9 and re-derived at
step 10 without `U`, since `Suppress` draws only from `available`), or itself
unavailable and cleared by its own 9a call, or no longer declared at all, in
which case `ReplaceRegistry`'s `pruneOrphans` already killed it earlier in the
same pass, deleting on **either** column (`internal/overlay/registry.go:170-173`).
State this in a comment so a later reader does not "fix" it.

### 2b. Doc sites 1 and 2 (they move with the code)

- **Site 1** — `Resolve`'s **"Missing and unavailable members"** doc section.
  "gets no stamp, and its overlay rows are not cleared" and "Rows joining two
  unavailable members are never touched" are both now **false**. Rewrite the
  section to state the prune, and to state the mid-prune crash state
  (`U` unstamped rows gone / `U` still stamped with rows gone) and the
  `StaleStamped` re-trigger that recovers it. Add step 9a to the numbered pass
  list in the same comment.
- **Site 2** — `Stats.MembersUnavailable`'s field comment
  (`internal/wsresolve/wsresolve.go:50-52`): "left untouched" is now false.
  Rewrite to say these members are pruned from the overlay.

### 2c. Site 6 — flip `TestMissingMembersLeftAlone`

`internal/wsresolve/wsresolve_test.go:386`. Rename to
`TestMissingMembersArePruned`. It **keeps its stamp assertions** and **flips
its edge assertion**: the seeded `gone1 -> gone2` orphan row joining two
unavailable members must now be **gone**, not survive.

The rewritten comment must cite **the change stub's own authority** — "so the
overlay never carries edges for members the resolver could not see"
(`0015-wsresolve-stamp-pruning.md`) — and must **not** lean on the sibling
`wsfresh` characterization test's licence, because unlike that test this one's
own comment contains no authorization to invert it. This is the one place the
change knowingly reverses a 0013 assertion, and the comment should say so
plainly.

### 2d. New `wsresolve` tests

1. **Prune across passes.** A member stamped by pass 1, made unavailable, then
   a pass 2 that leaves its stamp **absent** and its incident rows **gone**,
   while available members' rows survive.
2. **Never-thin under prune.** An ambiguity sourced from available `S` naming
   unavailable `U` only as a **candidate** is deleted **whole** (not thinned),
   and `S` re-derives it without `U`. This pins that the prune rides the
   existing never-thin rule rather than inventing a second incidence rule.

**Done when:** `go test -tags nollama -count=1 ./internal/wsresolve/... ./internal/overlay/...`
is green and the full suite is no worse than base.

---

## Task 3 — `wsfresh`: `Report.StaleStamped` and the gate

**Profile:** premium. Named risk: it changes the gate's trip condition, and
getting the trigger set or the ordering wrong reintroduces the
non-convergence that `TestFreshenConvergesWithABadVersionMember` forbids.

**Depends on tasks 1 and 2** (it observes the prune).

**Files:** `internal/wsfresh/wsfresh.go`, `internal/wsfresh/freshen.go`,
`internal/wsfresh/gate_test.go`.

### 3a. The `Report` field

Add to `Report` in `wsfresh.go`:

```go
// StaleStamped lists the ids of DECLARED members that are unavailable this
// pass yet still carry an overlay stamp, in manifest order.
StaleStamped []string
```

Its comment should also say **why it is not folded into `Dirty`**: `Dirty`'s
stated definition is "stamp absent or moved away from the member's re-folded
merkle root", and an unavailable member has no re-fold — putting it in `Dirty`
would make that doc false and break `Dirty ⊆ freshened members`. Three live
assertions require that an unavailable member never appears in `Dirty`
(`freshen_test.go:179`, `fixtures_test.go:505`, `gate_test.go:194-196`); do not
touch them. Both identities in `Report`'s type comment are unchanged and
`Stats` gains nothing, so that comment needs no edit beyond mentioning the new
field if the file's style calls for it.

### 3b. Populating it, and the gate

In `Freshen`:

- The **present-member loop must retain the ids** that failed
  `graph.OpenExisting` at step 5a (today it only increments `MembersUnindexed`,
  `freshen.go:157-161`), so the manifest pass has a set to filter against.
- Populate `StaleStamped` by iterating **`ws.Members` once**, taking every
  declared member not available this pass — both the `missing` ids and the
  present-but-unopenable ones — and appending it when `ov.Stamp(id)` reports a
  stamp present. **Do not** concatenate `missing` with the collected unopenable
  ids: their concatenation is not manifest order, which the field's doc comment
  promises.
- The gate becomes:

```go
if len(rep.Dirty) == 0 && len(rep.StaleStamped) == 0 && !drift {
	return rep, nil
}
```

A stamp **read** is not a content write, so the clean path still writes no
overlay content and `TestFreshenCleanPassWritesNoOverlayContent` continues to
hold.

### 3c. Doc sites 3 and 4

- **Site 3** — `Freshen`'s doc comment, **both sections**:
  - `# What a clean pass does and does not entail` (`freshen.go:37-41`): "There
    is one known exception, and it is the available -> unavailable transition"
    no longer holds. Rewrite.
  - `# Known limitation: a member that goes from available to unavailable`:
    replace the whole section with a statement of the **prune-then-clean**
    semantics and the records-first/stamp-last ordering. The forward reference
    "the honest fix is stamp pruning inside wsresolve.Resolve" is **discharged**
    — say that it has landed, not that it is pending.
  - Add `StaleStamped` to the numbered pass list / step 6 description as
    appropriate.
- **Site 4** — the inline **step-8 gate comment** (`freshen.go:242-249`), which
  currently restates the limitation and instructs "do not close it here". This
  is the site a reader of the code hits first and the one most likely to end up
  arguing with its own function's doc comment. Rewrite it to describe the
  trip on `StaleStamped`.

Read sites 3 and 4 **against each other and against task 2's sites 1–2** before
committing (`one-invariant-many-sites-drifts`).

### 3d. Site 5 — flip the characterization test

`TestKNOWNLIMITATIONVanishedMemberLeavesStaleEdgesReportedClean`
(`gate_test.go:263`) is **inverted and renamed** to
`TestVanishedMemberIsPrunedThenReportsClean`. Its own comment already
authorizes exactly this inversion, conditional on the prune landing — cite it.

- **Pass 1** asserts `StaleStamped == [lib]`, `Resolved == true`, the
  `app -> lib` edge **gone**, and `lib`'s stamp **absent**.
- **Pass 2** asserts `Resolved == false` and byte-stable overlay content.

### 3e. New `wsfresh` test — the three-pass convergence table

| pass | `lib` state | `StaleStamped` | `Resolved` | overlay |
|---|---|---|---|---|
| 1 (after `lib` vanishes) | unavailable, stamped | `[lib]` | `true` | pruned: no `app -> lib` edges, no `lib` stamp |
| 2 | unavailable, unstamped | `[]` | `false` | unchanged — nothing written |
| 3+ | unavailable, unstamped | `[]` | `false` | unchanged |

Pass 2's "wrote nothing" must be witnessed **structurally** —
`Resolved == false` ⇒ `Resolve` never ran — as the **primary** assertion, per
`content-witnesses-cannot-see-idempotent-writes`: a content witness passes
vacuously for a write that rewrites the same bytes. Use the existing
`overlayContent` helper as the **corroborating** assertion only, and say so in
the test's comment.

Also confirm (no code change needed) that
`TestFreshenConvergesWithABadVersionMember` (`gate_test.go:173`) still passes
unchanged: its member is `stateBadVersion` from construction, so it is never
stamped and `StaleStamped` is empty from pass 1 onward.

**Done when:** the full honest suite `go test -tags nollama -count=1 ./...` is
green with no new failures relative to base.

---

## Gate

One full-suite run at the end: `go test -tags nollama -count=1 ./...`, executed
in the foreground, from the feature worktree. Compare against the base failure
set — the bar is **zero new failures**; the suite is fully green on
`origin/main` for the `nollama` tag.

## Out of scope (do not drift into these)

- Any verb, CLI flag or MCP surface (the D7 wiring split — no verb in this slice).
- Union-graph queries and coverage clauses (§4.x).
- Incident-scoped or per-member re-resolution — ADR-0012 stands.
- Distinguishing "member root deleted" from "member DB version mismatch".
- Consolidating the six open-coded `.codeindex/graph.db` joins (standing TODO,
  to be done at all sites at once).
- No new ADR. ADR-0012 is unchanged and is the frame this rides in.

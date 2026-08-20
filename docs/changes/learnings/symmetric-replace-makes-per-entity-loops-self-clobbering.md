---
slug: symmetric-replace-makes-per-entity-loops-self-clobbering
hook: "If replace-for-X deletes rows incident to X on EITHER end, the obvious per-X write loop deletes what the previous iteration just wrote."
topics: [data-integrity, transactions, sql, api-design]
changes: [13]
created: 2026-08-20
updated: 2026-08-20
promotion_state: candidate
promoted_to:
---

## Apply

A `Replace<Thing>For(X, rows)` whose delete scope is **symmetric** — every row touching X, on either
endpoint — does not compose with the loop everyone writes:

```
for each member m:            # WRONG
    rows = derive(m)
    ReplaceFor(m, rows)       # deletes the m1→m2 rows iteration 1 just wrote
```

Iteration 1 writes `1 → 2`; iteration 2's replace deletes everything incident to member 2, which
includes that row. The result is a store holding only the last member's output, and the bug scales
with member count — with one member it is invisible, with two it silently drops half the data.

The fix is to split the phases rather than to weaken the delete:

1. **Clear every entity first** — call the replace with empty input for each X.
2. **Write the whole derived set** with non-deleting `Put*` calls.
3. **Stamp / mark-complete last**, so a crash between 2 and 3 leaves an unstamped store the next run
   rebuilds rather than a stamped partial one it trusts.

Do not add a "delete-source-side-only" variant to make the naive loop work — that reintroduces the
un-refreshable-row bug the symmetric delete exists to prevent. The loop is wrong, not the API.

**The general rule:** whenever a per-entity mutation's *scope* is broader than its *key*, per-entity
iteration is unsound. Before writing the loop, ask what iteration N does to iteration N−1's output.

**Test it by actually breaking it.** Collapse the two phases back into one loop and watch the
regression test go red. A test written against this shape without that check very often asserts
something the naive code also satisfies.

## Why it bites

Every individual call is correct, the API's contract is correct, and the loop is the shape the API's
name invites. There is no error, no constraint violation, no partial-write warning — just fewer rows
than expected, in a store nobody diffs. And with a single-entity fixture, which is what a first test
uses, the bug cannot reproduce at all: you need at least two entities with a row *between* them.

## Provenance

- **#0013, PR #11** — cross-repo resolution ladder, first caller of the `internal/overlay` store
  shipped unwired by #0012. `ReplaceMemberEdges` deletes on either end (deliberately — see
  `one-invariant-many-sites-drifts`), so the derive-then-write-per-member loop had member 2's call
  deleting the `1 → 2` edges member 1's call had just written. Restructured to clear-all → write-all
  (`Put*`) → stamp-last; no new overlay method was needed. The regression test was verified by
  collapsing the two steps and watching it go red.

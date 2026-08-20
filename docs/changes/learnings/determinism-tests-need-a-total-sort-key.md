---
slug: determinism-tests-need-a-total-sort-key
hook: "A determinism test that reads the same store twice proves nothing — a non-total ORDER BY passes it vacuously."
topics: [testing, determinism, sql, coverage]
changes: [13]
created: 2026-08-20
updated: 2026-08-20
promotion_state: candidate
promoted_to:
---

## Apply

Two things have to be true for "this query returns a stable order" to be tested, and the easy test
supplies neither:

**The sort key must be total over the rows the query can return.** Selecting a column is not
ordering on it. If the table has no unique constraint, then any two rows agreeing on every
`ORDER BY` term are returned in whatever order the engine happens to produce, and the query is
non-deterministic no matter how many columns the `SELECT` list carries. Audit it by asking: which
columns could two distinct rows share entirely? If the answer is "all the ones in the `ORDER BY`",
append discriminators until it is not — ending at the row id is the reliable terminator.

**The fixture must contain an actual tie.** A determinism test over data where no two rows collide
on the sort key cannot fail, because the ordering is forced by the data rather than by the query.
Deliberately seed a tie pair — two rows differing *only* in the columns you suspect are unordered —
so the assertion has something to bite on.

And the failure mode that hides both: **comparing two reads of the same store in the same process is
not a determinism test.** SQLite will very likely hand back the same physical scan order twice in a
row, so the assertion passes on a query with no total order at all. If you want a real signal,
compare against a *rebuilt* store, an independently-computed expected sequence, or at minimum a
hardcoded expected slice — something that cannot co-vary with the bug.

## Why it bites

This defect class is invisible in every ordinary way. The suite is green. The tests are named after
the property and read as if they test it. Coverage is full — the query executes. The only tell is
reading the `ORDER BY` against the table's uniqueness constraints, which nobody does while the tests
are passing.

It also fails *late* and *elsewhere*: order instability surfaces as a flaky downstream snapshot, or
as a diff that churns between runs, on a different machine or a different SQLite build, long after
the query was reviewed. The cost of the audit is one minute per query; the cost of the miss is a
flake hunt in a component that is not the buggy one.

## Provenance

- **#0013, PR #11** — cross-repo resolution ladder. `UnresolvedEdges` and `TierOneEdges` selected
  `dst_qualifier` and `dst_ns` but did not order on them, and `edges_t` has no unique constraint, so
  two calls to the same name on one line differing only in qualifier tied on every sort key. Both
  determinism tests compared two reads of the same store and so passed vacuously. Fixed in-branch
  (89545f6) by appending `dst_qualifier, dst_ns, id`, plus fixtures carrying a deliberate tie pair.
  Found by review, not by the suite.

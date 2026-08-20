---
slug: content-witnesses-cannot-see-idempotent-writes
hook: "A 'nothing was written' guard that compares stored CONTENT passes vacuously for any write that rewrites the same bytes — plant a row only the write would disturb."
topics: [testing, coverage, invariants, sql]
changes: [14]
created: 2026-08-20
updated: 2026-08-20
promotion_state: candidate
promoted_to:
---

## Apply

"The fast path performs no write" is a claim about **calls**, and the obvious test witnesses
**state**: snapshot the store, run the pass, compare. That test cannot distinguish "no write
happened" from "a write happened and produced identical bytes" — and the second is the common case,
because the fast path is exactly the path where the data has not changed. So the guard passes on day
one and keeps passing after someone reintroduces the write it exists to forbid.

Two ways out, in increasing cost:

**Give the witness a tooth.** Find a side effect of the forbidden write that is *not* content-equal,
and plant it. Here: the registry write unconditionally prunes orphaned rows, so seeding an orphan row
that only that prune would remove makes a stray call observable through a content comparison. Cheap
and effective — but it couples the guard to that side effect continuing to exist, so **say so in the
test's comment**. A test whose tooth silently falls out is worse than one that never had one, because
the name still promises coverage.

**Count the calls.** A driver-level statement or call counter witnesses the claim directly and has no
coupling. It is more machinery, and it is the honest answer when the forbidden write has no
observable side effect at all.

Whichever you pick, write down which writes remain invisible. In 0014 a `PutStamp` with an unchanged
root stays undetectable by the content witness, and that is recorded rather than papered over.

## Why it bites

The general shape: **witnessing an absence through a state comparison only works if the forbidden
action is guaranteed to change state.** For idempotent actions it is guaranteed *not* to, so the
witness and the claim are orthogonal, and the test is decorative. This is the same family as a
determinism test that reads a store twice and gets the same order for reasons unrelated to the
`ORDER BY` — the assertion holds for the wrong reason, so it never fails, so it is never revisited.

The tell to grep for: a test asserting "X did not happen" whose only mechanism is comparing a
before/after dump. Ask what the dump would look like if X *had* happened. If the answer is "the
same", the test is asserting nothing.

## Provenance

- **#0014, PR #12** — workspace freshen internals. The clean-path guard asserts the freshness pass
  writes no overlay content when no member is dirty. The `overlayContent` witness compares overlay
  content, so an idempotent `ReplaceRegistry` (unchanged manifest) or `PutStamp` (unchanged root)
  would have been invisible to it. Given a tooth by planting an orphan `member_stamps` row that only
  `ReplaceRegistry`'s unconditional prune removes; the coupling is stated in the test's comment, and
  the residual blind spot (a no-op `PutStamp`) is recorded in the results file as needing a
  driver-level statement counter, which the slice did not add.

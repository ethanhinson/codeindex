---
slug: rollback-untested-when-errors-precede-the-transaction
hook: "If every error your suite provokes is rejected BEFORE the transaction opens, 'never half-applies' is unproven — you tested validation, not atomicity."
topics: [testing, transactions, coverage, sqlite]
changes: [12]
created: 2026-08-19
updated: 2026-08-19
promotion_state: candidate
promoted_to:
---

## Apply

"A bad batch never half-applies" is a claim about **rollback**. A suite proves it only if at least
one test drives a failure that occurs *after* the transaction is open and *after* at least one
statement inside it has executed. Pre-validation tests — the ones that are easy to write, because
you just pass a malformed input — return before `BEGIN` and exercise none of that path.

The tell: batch-rejection tests all pass, and you have never seen the rollback branch execute. Grep
your own tests for one that reaches a mid-transaction error. If there is none, the atomicity
guarantee is asserted in a doc comment and nowhere else, and a later refactor — splitting the work
across two transactions, say, or moving validation inside the transaction — changes the guarantee
without reddening anything.

Corollary worth stating plainly: **moving the validation inside the transaction should break a
test.** If you can move it and the suite stays green, that is the proof the property was never
covered. That mutation is a two-minute check and a much stronger signal than coverage percentage.

## Constructing the mid-transaction failure

You usually have to reach for an implementation detail to provoke one, and that is acceptable if
the dependency is **documented and defended**. In 0012 the edges-side rollback test relies on
SQLite allocating `INTEGER PRIMARY KEY` as max+1 to collide a row mid-batch. That is a real coupling
to allocator behavior, so the test carries a comment saying so plus an error-substring guard: if
rowid allocation ever drifts, the test fails loudly on the wrong error rather than silently passing
for the wrong reason. A fragile test that announces its fragility is worth far more than an absent
one.

Related trap from the same review: a schema guard written as a **name heuristic** proves nothing. A
"no rowid references" check matching a bare `_id$` suffix misses `symbol_rowid`, `srcid`, and a
plainly-named `dst_symbol INTEGER`. Assert the *shape* the thing actually takes — here, that no
unexpected `INTEGER` column exists at all — because the shape holds regardless of what someone
names the column later.

## Provenance

- **#0012, PR #10** — workspace overlay store. Both transaction sites had entirely unexercised
  rollback paths (closed by 17a562c); the `_id$` schema heuristic was widened to an INTEGER-column
  shape assertion (8c60da9). Both surfaced in review, not in the build, and both were coverage gaps
  behind a green suite.

---
slug: one-invariant-many-sites-drifts
hook: "When one spec'd invariant must be enforced at several sites, check the sites against EACH OTHER — drift shows up as their doc comments arguing."
topics: [review, invariants, spec-fidelity, data-integrity]
changes: [12]
created: 2026-08-19
updated: 2026-08-19
promotion_state: candidate
promoted_to:
---

## Apply

A spec that states one rule in three places, enforced by two or three different functions, is the
setup for silent divergence. Each function is written (and reviewed) against the spec sentence
nearest to it, so each looks locally correct while the set is incoherent. Reviewing site-by-site
against the spec **cannot** find this; you have to read the sites against one another.

The cheap tell is that **the sites' own doc comments contradict each other**. In change 0012,
`ReplaceMemberEdges`' comment justified *not* thinning a candidate list — "would strand a
`candidate_count` contradicting its own rows" — while `ReplaceRegistry`'s prune, in the same
package, did exactly that thinning. Neither comment was wrong about its own code. The pair was the
defect. When two functions in a package explain opposite treatments of the same data, one of them
is a bug, regardless of how well each reads alone.

Two concrete sub-shapes worth carrying:

**Either-end incidence.** A rule phrased "the row is incident to X" over a row with two endpoints
(source and destination, parent and child) is very easy to implement as source-side-only, because
the source is the side you already have in hand. Whenever a delete/prune is scoped by one endpoint,
ask explicitly what happens to a row incident only via the other one — the answer is usually a
permanently un-refreshable row, since no later call for either member will match it.

**Accept set must equal delete set.** A "replace everything for member M" function is idempotent
over its own input only if every row it will *accept* on write is a row it would *delete* on the
next call. Validate the write side against exactly the predicate the delete side uses. A function
that accepts a row it can never remove has set a trap for the first caller, which may not exist yet
— 0012's was found with zero callers in tree, which is the cheapest possible moment.

## Why it bites

The observable damage is worse than "inconsistent." Thinning a candidate list while leaving its
`count` field alone produced an ambiguity reporting `Count: 2` with one candidate row — which is
**indistinguishable from a legal state**, because upstream truncation legitimately yields
`Count > len(Candidates)` and the validation deliberately permits it. The corruption is invisible
to every consumer, and the downstream reader mis-reports silently forever rather than failing.

Generalize that: any denormalized counter alongside its rows has a legal skew window. Any bug that
lands *inside* that window is undetectable by validation, by definition. So the counter/rows pair
deserves review attention out of proportion to its size — the failure mode is not a crash, it is a
permanent quiet lie.

## Provenance

- **#0012, PR #10** — workspace overlay store. Two review findings (both `important`, fixed
  in-branch as 705497d and 8590d2c, scope guard de7b4ca) were one incoherence seen from two sites:
  either-end incidence specified in three spec places, implemented source-side-only at two sites,
  one of which additionally thinned candidates. Suite was green before the fixes and after — no
  test would have caught it, because each site passed its own tests.

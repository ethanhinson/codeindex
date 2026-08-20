---
slug: one-invariant-many-sites-drifts
hook: "When one spec'd invariant must be enforced at several sites, check the sites against EACH OTHER — drift shows up as their doc comments arguing."
topics: [review, invariants, spec-fidelity, data-integrity]
changes: [12, 13, 14]
created: 2026-08-19
updated: 2026-08-20
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

**A recorded obligation is a site.** When a slice hands work to a future slice as prose — a package
doc saying "§4.1 must filter X" — that sentence is an enforcement site with no compiler and no test
behind it, and it drifts exactly like a doc comment. Check it against the code that already exists:
if any current test demonstrates a case the obligation's premise excludes, the obligation is
over-broad and the future implementer will faithfully write a bug. Read hand-offs against the
fixtures, not just against the spec.

**An unreachable normalizer manufactures a second site.** The drift above is usually diagnosed after
the fact; there is one case where you can see it coming at *write* time. When a new caller must
compare against data that some existing writer normalizes, and that normalization lives in an
**unexported** helper, the language itself is pushing you toward a copy — three transplanted lines
that are correct on the day they are written and are a second enforcement site forever. Export the
one normalizer and route the original writer through it instead. The test for whether this applies
is not "is the copy small"; it is "would a later edit to the writer's normalization silently stop
applying here?" If yes, it is a site, however few lines it is. Note the divergence mode is
especially nasty when the invariant is a *convergence* property — two normalizations that disagree
never agree that nothing changed, so the loop re-does its work on every pass, forever, with no error
anywhere.

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
- **#0013, PR #11** — cross-repo resolution ladder. The same shape, one layer out: `internal/wsresolve`'s
  package doc recorded a hand-off to §4.1 asserting that a suppression record is always accompanied by
  a cross-edge, so a literal implementation would delete a consumer's still-correct tier-1 edge and put
  nothing in its place. An **already-passing test on the same branch**
  (`TestRepointedEdgeFallsThroughWhenOwnerLacksTheName`) disproved the premise — one suppression, zero
  cross-edges. Fixed in-branch (746340a) by conditioning the obligation on a cross-edge existing for the
  same call site. Suite green before and after; the defect was again visible only as two doc comments
  disagreeing.
- **#0014, PR #12** — workspace freshen internals. First time the shape was caught *before* the
  second site existed, at reconcile rather than at review. The freshen pass had to compare a manifest
  against the overlay registry using exactly `ReplaceRegistry`'s own transforms, which lived in the
  unexported `overlay.dedupe()` reached only through the unexported `insertMembers` — so the obvious
  move was to copy three lines into `internal/wsfresh`. Rejected on this finding by name; instead
  `overlay.NormalizeMembers` was exported and `insertMembers` routed through it, leaving the store's
  write path and the drift read as one implementation with two callers. The avoided failure mode was
  the convergence one above: a drift comparison normalizing differently from the writer reports drift
  on every pass, re-resolving the whole workspace forever — which is exactly what that slice's
  `TestFreshenConvergesWithABadVersionMember` exists to forbid.

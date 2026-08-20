---
slug: known-limitations-need-a-characterization-test
hook: "Ship a known gap as a test that asserts what the code DOES, plus the prerequisite that must land first — prose alone gets 'fixed' into a regression."
topics: [testing, hand-offs, convergence, spec-fidelity]
changes: [14]
created: 2026-08-20
updated: 2026-08-20
promotion_state: candidate
promoted_to:
---

## Apply

Sometimes the right call is to ship a slice with a known, bounded gap. The failure mode is not the
gap — it is what the *next* implementer does with it. They find the gap, see that the detection
signal is already sitting right there in the data, wire it up in four lines, and ship a regression
that the suite welcomes, because nothing in the suite ever said what the old behavior was.

Record a deliberate limitation in three places, not one:

1. **A characterization test**, named so that it cannot be misread as an aspiration — the shape used
   here was `TestKNOWNLIMITATION<Behavior>`. Its assertions state what the code **does today**, and
   a comment says so explicitly. Someone who "fixes" the gap will now break a test on the way, which
   is the entire point: the conversation happens at the right moment instead of at the incident.
2. **The doc comment of the entry point**, so a reader of the API sees the boundary without reading
   the tests.
3. **The prerequisite**, stated as the thing that must land *first* — not merely "this is a
   limitation" but "closing this requires X, and doing it without X causes Y."

Point 3 is the load-bearing one and the one usually skipped. A gap that is cheap to close is
normally closed; a gap that survives a review is usually surviving because the naive close is
actively worse than the gap, and that reasoning is invisible in the code.

## Why it bites

The naive close is attractive precisely because the signal is already there. In change 0014 an
available→unavailable member leaves stale cross-edges in the overlay while the freshness gate
reports clean, because only *available* members' stamps are read. The detection signal — a stamp
surviving for a member that is no longer available — is present and deliberately unread. Wiring it
to "mark dirty" reads as a one-line fix and is not: a permanently unavailable member is dirty on
every pass forever, so the gate never holds, and every freshness check re-resolves the entire
workspace. That is unbounded work, silently, with a green suite — and the actual prerequisite is
stamp pruning in a function that a *previous* change deliberately froze, which makes it a change of
its own rather than a line in this one.

Note the asymmetry that makes prose insufficient: the limitation is a **silent** wrong answer and
the naive fix is a **silent** performance collapse. Neither one announces itself, so the only thing
that can carry the reasoning forward is an executable artifact that fails when someone crosses the
line.

## Provenance

- **#0014, PR #12** — workspace freshen internals. Review finding 3 (`important`). Resolved not by
  fixing the gap but by pinning it: `TestKNOWNLIMITATIONVanishedMemberLeavesStaleEdgesReportedClean`
  as a characterization test, the limitation and its prerequisite written into `Freshen`'s doc
  comment, and the prerequisite (stamp pruning in `wsresolve.Resolve`) carried into the results file
  as named follow-up work — subsequently filed as its own change rather than left as prose.

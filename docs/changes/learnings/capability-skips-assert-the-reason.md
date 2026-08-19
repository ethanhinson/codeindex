---
slug: capability-skips-assert-the-reason
hook: "A capability skip must key on the degradation REASON, never on the presence of a disclosure marker."
topics: [testing, degradation, sentinels]
changes: [11]
created: 2026-08-19
updated: 2026-08-19
promotion_state: candidate
promoted_to:
---

## Apply

When a test branches on whether the build under test has some capability, detect it from the
product's **own runtime disclosure** rather than from a build tag — a build tag asserts how the
binary was compiled, the disclosure asserts what the binary can actually do, and only the second
also covers an otherwise-capable build whose dependency is missing or broken at runtime.

But detect it from the disclosure's **reason**, not its presence. A marker like
`[lexical-only: <reason>]` is emitted for *several* distinct reasons, and only some of them mean
"this environment legitimately cannot do the thing." The others mean "the thing is broken" — which
is the exact regression the test exists to catch. Keying the skip on the marker's presence makes
the test absorb its own failure condition and report green.

Concretely, the shape that works:

- Enumerate the expected degradation reasons explicitly, and skip only on those.
- Make any **unrecognized** reason **fail** the test — never fall through to the skip.
- Keep the degraded branch asserting something build-independent; a skip that asserts nothing is a
  hole, not a branch.
- Mutation-test the classifier itself. The classifier is the guard, and an unexercised guard is
  decoration — flip each reason and confirm the test actually reddens.

Known cost, worth stating at the call site: substring matching on reason text means a future
degradation site whose wording happens to contain an enumerated phrase silently re-opens the skip.
Enumerate the current sites in a comment next to the classifier so that coupling stays visible.

## War story

- 2026-08-19 (#11, PR #9) — `TestSearchToolAndPrompt` asserted that the conceptual query
  "helper increment function" surfaces the symbol `Helper`, which only succeeds through semantic
  search. Under the repo's honest local gate (`go test -tags nollama ./...`, no embedding provider)
  it failed on `origin/main` too, reddening the finalize merge gate for **every** change, not just
  its own. Root-cause first: lexical-only search was working correctly — `matchQuality` lowercases
  and stems both sides and the ladder is conjunctive, so a 3-token phrase legitimately does not
  match a symbol named `Helper`. No product bug; the test had simply never declared its dependency
  on an embedding-capable build.

  The fix chose runtime detection over a `//go:build nollama` test split. **Review then caught that
  the first cut keyed on the mere presence of the `[lexical-only: ` marker** — which would have
  silently swallowed the `"query embedding failed: <err>"` case, i.e. an embedding-capable build
  whose embed call broke at runtime, into the "honest degradation, skip and log" path. That is
  precisely the regression the test protects. Fixed in `dbcdd1f` by gating on the reason and failing
  on any unexpected one, proven by mutation. Recorded as ADR-0011.

  The general lesson is the ordering as much as the rule: root-cause *before* adapting the test, or
  the adaptation papers over a real defect — and when the adaptation is a skip, the skip's predicate
  is itself load-bearing code that needs its own adversarial pass.

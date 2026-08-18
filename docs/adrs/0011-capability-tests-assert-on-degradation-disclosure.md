---
id: 11
slug: capability-tests-assert-on-degradation-disclosure
title: Capability-dependent tests assert against the product's own degradation disclosure
status: Accepted
date: 2026-08-18
supersedes: []
reverses: []
relates_to: []
change: 11
---

## Context

`codeindex` search is a hybrid of a lexical lane and a vector (embedding) lane. Under the `nollama`
build tag — which this machine must use, because the vendored llama.cpp headers are absent and plain
`go test ./...` cannot build ~10 packages on any ref — `embed.NewLocal` returns `ErrUnavailable`,
`search.Semantic` receives a nil provider, and the tool degrades to lexical-only. It discloses that
in its own output header as `[lexical-only: <reason>]`.

`TestSearchToolAndPrompt` asserted that the conceptual query "helper increment function" surfaces the
symbol `Helper`. That outcome is reachable only through the vector lane, so the test silently assumed
an embedding-capable build and failed on every ref under the honest local gate.

Root cause was investigated and discharged: lexical matching is NOT broken. `matchQuality` in
`internal/search/find.go` lowercases and stems both sides, and `find <repo> helper` returns
`Helper [exact]`. The lexical ladder is deliberately **conjunctive** — a candidate must carry every
query stem — so a 3-token conceptual phrase correctly does not match a symbol named `Helper`. This is
the intended division of labour between `find` (names) and `search` (concepts).

## Decision

Capability-dependent tests detect capability **at runtime, from the product's own disclosure marker**,
rather than from the build tag that happens to remove the capability. Concretely, the test branches on
the presence of `[lexical-only: ` in the tool output.

Two refinements are part of the decision and are rules, not commentary:

1. **The disclosure's REASON is load-bearing, not just its presence.** `internal/search/semantic.go`
   sets the degradation reason at three sites. Two mean capability is genuinely absent ("no embedding
   provider in this build"; "index has no embeddings for the active model") and legitimately license
   skipping the capability-dependent assertion. The third — "query embedding failed: <err>" — means the
   build IS capable and the embedding call broke at runtime; that is a real regression and must FAIL the
   test, not be absorbed as honest degradation. A capability check keyed on the marker's mere presence is
   a false negative that hides exactly the breakage the test exists to catch.
2. **A degraded branch must still assert something build-independent.** Asserting only that the tool
   disclosed a degradation proves nothing about the surviving lane, so the test additionally asserts a
   bare-identifier query (`Helper`) surfaces `Helper` — the `exact` rung of the lexical ladder,
   independent of embeddings. Without it, a genuine lexical-fallback regression would pass unnoticed on
   lexical-only builds.

### Alternatives considered

- **Split the test with `//go:build nollama` / `//go:build !nollama` files.** Rejected: it keys on the
  compiled-in capability rather than the actual one, so it stays blind to an embedding-capable build
  whose index carries no vectors yet (the same marker, a different reason). It also adds build-tagged
  test files and duplicated assertions.
- **`t.Skip` the whole test on lexical-only builds.** Rejected: it surrenders the tool-listing,
  routing-law, prompt, and lexical assertions that are all perfectly valid without embeddings, shrinking
  coverage far beyond the capability actually missing.

## Consequences

- **Enables:** one test body that is honest on every build shape; the semantic assertion runs unchanged
  and unweakened wherever embeddings are live; `go test -tags nollama ./...` is fully green, unblocking
  the finalize merge gate for every change.
- **Costs:** the test now depends on the stability of a human-readable output marker and on substring
  matches over two reason strings. A reworded disclosure, or a fourth `degraded =` site whose wording
  happens to contain "no embedding provider" or "index has no embeddings", would silently re-open the
  skip. The three current sites are named in a code comment to keep that visible.
- **Gives up:** compile-time certainty about which branch runs. The embedding-capable branch cannot be
  exercised in this environment at all, so it is verified only by inspection and by mutation testing of
  the reason classifier.

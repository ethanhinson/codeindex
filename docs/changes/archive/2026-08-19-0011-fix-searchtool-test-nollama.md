---
id: 11
slug: fix-searchtool-test-nollama
title: Make TestSearchToolAndPrompt honest under nollama builds
status: done
priority: high
type: fix
created: 2026-08-18
updated: 2026-08-19
depends_on: []
related: [9]
discovered_from: [9]
adrs: [11]
spec:
plan: docs/superpowers/plans/2026-08-18-fix-searchtool-test-nollama-plan.md
results: docs/results/2026-08-18-fix-searchtool-test-nollama-results.md
trivial: true
auto_groomable:
branch: feat/fix-searchtool-test-nollama
claimed_at: 
pr: https://github.com/ethanhinson/codeindex/pull/9
blocked_by:
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Plan | [2026-08-18-fix-searchtool-test-nollama-plan.md](https://github.com/ethanhinson/codeindex/blob/main/docs/superpowers/plans/2026-08-18-fix-searchtool-test-nollama-plan.md) |
| Results | [2026-08-18-fix-searchtool-test-nollama-results.md](https://github.com/ethanhinson/codeindex/blob/main/docs/results/2026-08-18-fix-searchtool-test-nollama-results.md) |
| PR | [#9](https://github.com/ethanhinson/codeindex/pull/9) |
| ADRs | [ADR-0011](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0011-capability-tests-assert-on-degradation-disclosure.md) |
<!-- docket:artifacts:end -->

## Why

The repo's honest local test gate is `go test -tags nollama ./...`
(pinned in `.docket.local.yml`, owner decision 2026-08-18 — the vendored
llama.cpp headers are absent on this machine, so CGO embed builds fail on
every ref). Under that tag exactly one test fails, on `origin/main` too:
`TestSearchToolAndPrompt` (`internal/mcpserver/search_test.go:52`). Root
cause: with no embedding provider the search tool degrades to
lexical-only, and the test's conceptual query ("helper increment
function" surfacing symbol `Helper`) only succeeds through semantic
search. The test silently assumes an embedding-capable build. Until it
passes, the finalize merge gate is red for every change, including 0009.

## What changes

Make the test honest about build capabilities: when the build has no
embedding provider (nollama), either skip with an explicit reason or
assert the documented lexical-only behavior instead of the semantic
expectation. The semantic assertion must keep running unchanged on
embedding-capable builds — do not weaken what the test protects there.

If root-causing shows lexical-only search *should* match `helper` →
`Helper` and doesn't (a real fallback bug, not a test-environment gap),
stop and surface that instead of papering over it — that fix is its own
change.

## Out of scope

- Restoring the vendored llama.cpp headers / fixing the CGO embed build.
- Improving lexical-fallback ranking or matching (separate change if the
  root-cause points there).

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

### 2026-08-18 — reconcile (docket-implement-next)

Scope unchanged; the change is still accurate and still needed. Failure
reproduced on the claim ref exactly as described:

```
search "helper increment function" (0 symbols, 0 clusters) [lexical-only: no embedding provider in this build]:
  (no matches — try different concept words, or add hints)
```

**Root-cause discharged — the abort condition does NOT fire.** The change
required stopping if lexical-only search *should* match `helper` → `Helper`
and doesn't. It does match. Verified empirically against the same fixture
with a `-tags nollama` binary:

- `find <repo> helper` → `Helper  func  a.go:2  callers=2  [exact]`
- `find <repo> "helper increment function"` → 0 matches, plus the built-in
  guidance *"this looks like a concept query; use `search` for feature/topic
  questions"*

Matching is fully case-insensitive (`internal/search/find.go`
`matchQuality` lowercases both sides and stems). The ladder is
**conjunctive**: a candidate must carry *all* query stems, so the 3-token
conceptual phrase correctly fails to match a symbol named `Helper`. That is
the documented division of labour between `find` (names) and `search`
(concepts) — the test was simply asserting a semantic-only outcome without
declaring that dependency. No product bug; proceed with the test fix.

Related **0009** (`implemented`, PR #8) is workspace-manifest work and does
not touch `internal/mcpserver` or the search lanes — no interaction.

**Adjacent observation, deliberately NOT folded in** (stays out of scope
per "Improving lexical-fallback ranking or matching"): `search.Semantic`
joins `hints` into one space-separated query
(`hintQ := strings.Join(opts.Hints, " ")`, `internal/search/semantic.go`)
and feeds it through the same conjunctive ladder, so a multi-hint call only
matches symbols carrying *every* hint token — hints behave conjunctively
rather than as independent identifier guesses. `auto_capture` is disabled in
this repo, so this is reported in prose rather than minted as a stub.

**Implementation direction chosen:** detect capability at *runtime* from the
tool's own `[lexical-only: …]` disclosure rather than from the `nollama`
build tag. Runtime detection is honest about actual capability (it also
covers an embedding-capable build whose index carries no vectors yet), and
it keeps the semantic `Helper` assertion running unchanged wherever
embeddings are live.

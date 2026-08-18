---
id: 11
slug: fix-searchtool-test-nollama
title: Make TestSearchToolAndPrompt honest under nollama builds
status: proposed
priority: high
type: fix
created: 2026-08-18
updated: 2026-08-18
depends_on: []
related: [9]
discovered_from: [9]
adrs: []
spec:
plan:
results:
trivial: true
auto_groomable:
branch:
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
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

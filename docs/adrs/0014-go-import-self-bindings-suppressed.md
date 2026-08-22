---
id: 14
slug: go-import-self-bindings-suppressed
title: Go import self-bindings carry no namespace information and are suppressed at both hint sites
status: Accepted
date: 2026-08-22
supersedes: []
reverses: []
relates_to: []
change: 17
---

## Context

Change 0017 makes Go subtype (struct-embedding) edges carry a namespace hint, so the resolution
ladder's import-mediated rung can fire for them. Three edits: the Go adapter's `import_spec` sets
`RawDep.Source`; `embeddedTypeName` stops discarding the `qualified_type` package operand and
resolves it through the file's `aliases` map into an import path; and `internal/graph/store.go`'s
dep loop prefers that edge-local `Source` over the file-level `bind` map (mirroring the calls
path).

Setting `Source` on Go import deps had a consequence nobody had to decide before: it newly
populated the file-level `bind` map for Go files. Two hint sites read from that world — the call
loop (`bind[c.Callee]`) and the dep loop.

This was MEASURED, not theorized, and both halves were real:

1. **Call collision (measured).** Go's `calleeName` yields selector/method names, so `x.log()` in
   a file importing `"log"` produces callee `log`, and the newly-populated `bind["log"] = "log"`
   captured it. `nsMatch` widens the match by suffix, so candidate namespace `internal/log`
   matched hint `log`. Critically, in `resolve()` the `nsHint != ""` `boundIDs` steps run BEFORE
   the `srcNS` same-scope step — so the hint did not merely narrow the candidates, it PREEMPTED
   the rung that had been deciding the edge. Measured effect: an `x.log()` call moved from the
   in-package `Server.log` to an unrelated `internal/log/log.go`. Lowercase single-segment stdlib
   import paths are exactly the shape of unexported Go method names, so this surface is wide.

2. **Import-edge movement (measured).** A single-segment import (`import "log"`, no `/` in the
   target, so `resolve()` is actually called on it) moved from `app/server.go` to
   `internal/log/log.go` — resolving `unambiguous` to an unrelated symbol, where `unambiguous` is
   the token every downstream consumer trusts.

## Decision

A **self-binding** — an import dep whose normalized hint equals its target — carries no
name-to-namespace information: it says only "the name X lives in namespace X". For Go this is true
by construction (a Go namespace is a repo-relative DIRECTORY and a Go import's `Target` is the
whole slash-bearing path, so `normalizeHint` is a pass-through and `Source == Target` for every Go
import). Such a hint is therefore suppressed.

The rule is expressed as ONE predicate, `goImportSelfHint(fromFile, hint, target string) bool` in
`internal/graph/store.go` (returns `hint == target && strings.HasSuffix(fromFile, ".go")`), and is
applied at BOTH hint sites — the `bind`-population loop and the dep loop. One predicate, two call
sites, deliberately: an earlier iteration applied it at only one site, and the two sites' doc
comments immediately began contradicting each other.

**The rule is scoped to Go, and the scoping is the load-bearing part.** For Python and TypeScript
the namespace IS the module path, so a self-binding is semantically correct and load-bearing:
`from app import app` yields `bind["app"] = "app"`, which `nsMatch` resolves via its `.`-separator
and `CutPrefix` legs. All three non-Go adapters emit `KindExtends`/`KindImplements` with
`Source == ""`, so `bind` is their ONLY hint channel. A language-agnostic version of this rule
silently deletes those hints — it was written that way first, and review caught it before merge.

The gate is on the importing FILE being Go, not on the target's shape. Target shape cannot work:
the hazard case is `import "log"` — bare, non-slash-bearing, lowercase, single-segment —
indistinguishable from a Python module named `log`. Any target-shape test that catches it also
catches `from log import log`. A related shape-based filter (filter `bind` by whether the target
looks like a path) was considered and REJECTED for the inverse reason: the colliding entry is
itself a non-path target, so a non-path filter would preserve exactly the wrong entries.

## Consequences

- Go's `bind` map returns to being empty in effect, which is what it was before this change — Go
  gains nothing from `bind`, because an import's `Target` (a full path) never equals a bare
  embedded type name. The live channel for Go is the edge-local `Source`.
- The import-edge movement class is closed BY CONSTRUCTION rather than argued empty. It had
  measured size 0 on the prometheus bench member, but that was a property of prometheus's
  directory layout, not evidence the class was closed; the structural fix removes it on any repo.
- Python/TS/PHP `bind` population is bit-for-bit unchanged, and is now covered by tests that did
  not previously exist (including one for PHP's `use A\B\C;` backslash leg, which nothing had
  exercised end-to-end).
- Cost: the rule is a special case keyed on a file extension inside otherwise language-neutral
  store code. It is justified because the underlying fact — whether a namespace is a directory or
  a module path — genuinely is language-specific, but it is a place future language work must
  look.
- Guarded in-suite by `TestGoImportBindDoesNotCaptureMethodCall`,
  `TestGoImportEdgeKeepsItsPreChangeResolution`, and `TestSelfBindingImportStillBindsOutsideGo` in
  `internal/graph`. This matters because the repo has NO committed graph goldens: every
  `DumpNormalized` consumer is an incremental-vs-full rebuild-equivalence check under the same
  code, so both sides move identically and none can detect a resolution regression.

Relevant commits on branch `feat/adapter-namespace-hints-extends-implements`: `54fedc4` (first,
Go-only-by-intent version), `8e9ee63` (scoped to Go after review found it was deleting
Python/TS/PHP hints), `9d84218` (applied at the second site, restoring symmetry).

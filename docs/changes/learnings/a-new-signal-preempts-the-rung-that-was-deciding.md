---
slug: a-new-signal-preempts-the-rung-that-was-deciding
hook: "Adding a signal to an ordered resolution ladder is not narrowing — a newly non-empty input preempts every rung below it, including the one that was already deciding correctly."
topics: [invariants, resolution, review, measurement]
changes: [17]
created: 2026-08-22
updated: 2026-08-22
promotion_state: candidate
promoted_to:
---

## Apply

"We're only adding a hint; worst case it doesn't help" is the wrong model for any resolver shaped as
an **ordered ladder of rungs**, where the first rung whose precondition holds returns the answer.
A hint's precondition is usually `hint != ""`. Populating a field that used to be empty therefore
does not narrow the winning rung's candidate set — it **changes which rung wins**, for every edge
where the hint is now non-empty, including the edges that were already resolving correctly via a
lower rung.

So the review question for a "we now also set `Source`/`ns`/`hint`" change is never "does the hint
point at the right thing?" It is:

1. **Which rungs sit above the previously-deciding one**, and what did the new field just make
   reachable? In 0017's `resolve()`, the `nsHint != ""` `boundIDs` steps run *before* the `srcNS`
   same-scope step — so a newly-hinted call in a file that imports a same-named package left its
   in-package method and landed on an unrelated package's symbol.
2. **Which edge kinds share the field?** The change intended to hint *subtype* edges; the same map
   fed *call* edges and *import* edges, which is where both regressions actually landed. A hint
   channel is rarely per-kind.
3. **Is the new value ever tautological?** A hint that resolves to the thing it is a hint *for* (an
   import edge hinted by its own import) is not a narrowing at all — it is a rung-jump on no
   information. Suppress it at the source; see `dialect-specific-remedies-need-a-language-gate` for
   the gating trap that suppression walks into.

**Measure both directions.** A win count alone will not surface this: the wins are real and the
regressions are elsewhere in the corpus. Score `resolved → differently-resolved` as its own class,
not as noise, and treat `unambiguous → unambiguous (different target)` as the highest-suspicion
bucket — a confidently wrong answer is worse than the ambiguity it replaced.

## Why it bites

The failure mode is a *confident* wrong answer. Before the change the edge was right; after it, it
points somewhere plausible and reports high confidence, so no downstream consumer and no ambiguity
report flags it. And in a repo whose graph tests are **rebuild-equivalence** checks — incremental
vs. full under the same code — both sides move identically, so the suite is structurally incapable
of seeing a resolution regression. Only a before/after measurement across two binaries can.

## Provenance

- **#0017, PR #15** — Go subtype namespace hints. Setting `RawDep.Source` on `KindImports` deps
  newly populated the file-level `bind` map for Go files, and two regressions followed, neither
  visible to a green suite. (a) A call collision: `x.log()` in a file importing `"log"` moved from
  the in-package `Server.log` to `internal/log/log.go` — found by a plan task dedicated to measuring
  the *non-target* edge kinds, not by review. (b) An import-edge movement the spec had asserted
  could not exist: `import "log"` in `app/server.go` resolved `unambiguous` to
  `internal/log/log.go`. That second one was initially pinned as "measured delta, class size 0 on
  this bench member"; whole-branch review refused a zero-observed-instances class as a merge basis
  and it was closed **by construction** instead (`9d84218`). Both are now suppressed by one
  predicate applied at both hint sites — ADR-0014.

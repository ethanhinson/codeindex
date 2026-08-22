---
slug: dialect-specific-remedies-need-a-language-gate
hook: "A remedy whose correctness argument cites one language's semantics must be gated to that language — in shared multi-language code it silently deletes the other dialects' signal."
topics: [review, multi-language, invariants, spec-fidelity]
changes: [17]
created: 2026-08-22
updated: 2026-08-22
promotion_state: candidate
promoted_to:
---

## Apply

When a fix is justified by a sentence of the form "this case is degenerate **because** in
`<language>`, X and Y are the same thing," check where the fix actually executes. If it lands in a
shared, language-agnostic path — a common normalizer, a store loop, a `bind`/hint map every adapter
feeds — the justification covers one caller and the code covers all of them. For every other
language the "degenerate" case is ordinary, load-bearing data, and the remedy is a silent deletion.

The tell is that the remedy's own comment is a language-specific claim while its location has no
language in scope. Read them against each other: *is the predicate that makes this a no-op derivable
from anything the function can see?* If the answer requires knowing which adapter produced the row,
the gate must be explicit — pass the language through, or apply the predicate at the per-adapter
site rather than the shared one.

Two aggravating factors make this class hard to catch:

**A suppression is invisible to a green suite.** Deleting a hint does not fail anything; it degrades
a resolution from "narrow" to "broad," which no assertion in a suite built around the *fixed*
language is looking at. The blocker in 0017 was green before and after.

**The unaffected languages may have no other channel.** Check what the deleted signal was competing
with. If the fixed language has a second, richer hint path (0017's Go edges carry an edge-local
`Source`), the shared map is optional *there* and mandatory everywhere else — precisely the
asymmetry that makes the author feel the map is expendable.

## Why it bites

The damage is a confidence downgrade in an unrelated language, discovered by nobody: no error, no
red test, no user-visible failure — just a resolver that returns three candidates where it used to
return one, in a codebase whose benchmarks all target the language the fix was written for. The
measurement that justified the change cannot see it either, because the bench member is a repo in
the fixed language.

## Provenance

- **#0017, PR #15** — Go namespace hints for subtype edges. Populating the file-level `bind` map for
  Go newly collided with in-package call resolution (`x.log()` in a file importing `"log"` moved to
  an unrelated `internal/log/log.go`), so a self-binding skip was added to suppress it. The skip was
  written **without a language gate**. It is genuinely empty for Go — a Go namespace is a
  repo-relative directory and a Go import's `Target` is the whole slash-bearing path, so
  `bind[name] == name` never carries information — but for Python and TypeScript the namespace *is*
  the module path, making `from app import app` → `bind["app"] = "app"` a correct hint. All three
  non-Go adapters emit subtype deps with `Source == ""`, so `bind` is their **only** hint channel:
  the skip deleted it outright. Caught at whole-branch review as the run's single `blocker`, fixed
  in-branch as `8e9ee63` by scoping the predicate to Go (`goImportSelfHint`). Suite green
  throughout; no test in tree covered it.

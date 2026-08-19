<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0011 — Make TestSearchToolAndPrompt honest under nollama builds](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0011-fix-searchtool-test-nollama.md)**
<!-- docket:backlink:end -->

# Plan — Make TestSearchToolAndPrompt honest under nollama builds

Change: 0011 `fix-searchtool-test-nollama` (trivial, high, fix)

> Authored inline: the configured plan skill (`superpowers:writing-plans`)
> was unavailable in this harness, so the `plan` role degraded to `auto`
> per the docket Skill-layer missing-skill rule.

## Problem

`go test -tags nollama ./...` — the repo's honest local gate, pinned in
`.docket.local.yml` — fails on exactly one test, on `origin/main` too:

```
--- FAIL: TestSearchToolAndPrompt
    search_test.go:52: search did not surface Helper:
        search "helper increment function" (0 symbols, 0 clusters) [lexical-only: no embedding provider in this build]:
          (no matches — try different concept words, or add hints)
```

`TestSearchToolAndPrompt` asserts that the conceptual query
`"helper increment function"` surfaces the symbol `Helper`. That outcome is
reachable only through the vector lane. Under `-tags nollama`,
`embed.NewLocal` returns `ErrUnavailable` (`internal/embed/local_stub.go`),
`search.Semantic` receives a nil provider, and the tool degrades to the
lexical lane — disclosing it in its own output header as
`[lexical-only: no embedding provider in this build]`.

## Root cause is discharged — this is a test-honesty gap, not a product bug

The change file directed an abort if lexical-only search *should* match
`helper` → `Helper` and doesn't. It does match. Verified with a
`-tags nollama` binary against the same fixture:

- `find <repo> helper` → `Helper  func  a.go:2  callers=2  [exact]`
- `find <repo> "helper increment function"` → 0 matches, plus the built-in
  guidance *"this looks like a concept query; use `search` for
  feature/topic questions"*

`matchQuality` (`internal/search/find.go`) lowercases and stems both sides,
so matching is case-insensitive; the ladder is **conjunctive** — a candidate
must carry every query stem. A three-token conceptual phrase therefore
correctly fails to match a symbol named `Helper`. The test was asserting a
semantic-only outcome without declaring that dependency.

## Approach

Detect embedding capability **at runtime**, from the search tool's own
`[lexical-only: ` disclosure, rather than from the `nollama` build tag.

Why runtime over a build-tag split:

- It is honest about *actual* capability, not just the compiled-in one — it
  also covers an embedding-capable build whose index carries no vectors yet
  (`search.Semantic` emits the same marker with a different reason).
- It needs no new build-tagged files or exported test hooks.
- The marker is the product's own documented disclosure, so the test keys on
  a contract rather than on an implementation detail.

The semantic assertion stays **byte-identical** on embedding-capable builds.
The degraded branch asserts the documented lexical-only behaviour rather
than skipping, so the test keeps protecting something in both modes.

## Tasks

### Task 1 — Make the search assertion capability-aware

File: `internal/mcpserver/search_test.go`

Everything before and after the search call — tool listing, routing-law
description checks, the `explore-feature` prompt assertions — stays
unchanged and unconditional.

Around the search-result assertions (currently lines ~47-53):

1. Keep the header assertion unconditional: the output must contain
   `search "helper increment function"` in every mode.
2. Branch on `strings.Contains(out, "[lexical-only: ")`:
   - **Embedding-capable** (marker absent): assert `Helper` surfaces —
     the existing assertion, unweakened, with its existing failure message.
   - **Lexical-only** (marker present): assert the documented degraded
     contract — the disclosure names a reason (the text after
     `[lexical-only: ` is non-empty) and the no-match guidance
     `(no matches — try different concept words, or add hints)` is
     rendered. Log one `t.Logf` line naming why the semantic assertion did
     not run, so a green run is self-explaining rather than silently
     narrower.
3. Add a comment stating the capability dependency explicitly, so the next
   reader does not re-derive it.

Verification: `go test -tags nollama ./internal/mcpserver/ -run TestSearchToolAndPrompt -v`
passes and its log line names the lexical-only reason.

### Task 2 — Guard the lexical contract that *is* build-independent

Same file. The degraded branch above proves the tool disclosed a
degradation; it does not prove lexical search still works. Add a small
assertion that holds in **both** modes: a second `search` call whose query
is the bare identifier `Helper` must surface `Helper`. That path is the
`exact` rung of the lexical ladder, is independent of embeddings, and
would have caught a genuine lexical-fallback regression — the failure mode
the change file was worried about.

Verification: passes under `-tags nollama`.

### Task 3 — Full-suite gate

Run `go test -tags nollama ./...` and confirm it is **fully green** — the
change's stated success criterion.

Note: plain `go test ./...` fails ~10 packages on every ref from missing
vendored llama.cpp headers. That is environmental and explicitly out of
scope; the `nollama` tag is the honest gate.

## Out of scope

- Restoring the vendored llama.cpp headers / fixing the CGO embed build.
- Improving lexical-fallback ranking or matching — including the
  observation that `search.Semantic` joins `hints` into one
  space-separated query and runs it through the conjunctive ladder, so
  multi-hint calls match only symbols carrying every hint token.

## Success

`go test -tags nollama ./...` fully green on `feat/fix-searchtool-test-nollama`,
with the semantic `Helper` assertion still running unchanged wherever
embeddings are live.

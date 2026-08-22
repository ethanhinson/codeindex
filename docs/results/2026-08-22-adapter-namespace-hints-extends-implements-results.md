<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0017 — Go subtype references carry namespace hints — fix the qualifier discard and KindImports Source](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0017-adapter-namespace-hints-extends-implements.md)**
<!-- docket:backlink:end -->

# Go subtype references carry namespace hints — results

Change: #0017 · Branch: feat/adapter-namespace-hints-extends-implements · PR: (see change `pr:`) · Plan: docs/superpowers/plans/2026-08-22-adapter-namespace-hints-extends-implements-plan.md · ADRs: 14

## Verify (human)

- [ ] **Confirm the acceptance bar is read as met, and read it narrowly.** The bar was
      `ambiguous → unambiguous` with a verified-correct target over the 23 addressable
      qualified-embed edges. Measured on the `prometheus` bench member: **14 wins of 23**, with
      both pinned exemplars moving — `chunkenc.Chunk` (3 edges, wrong package
      `prompb/types.pb.go` → correct `tsdb/chunkenc/chunk.go`) and `refresh.Discovery`
      (exactly 4 edges, 22 candidates → 1 correct). Go extends aggregate: unambiguous 79→92,
      ambiguous 25→12, **unresolved 15→15**. That last figure is the honest one:
      **zero `unresolved → resolved`**, which is what the bar always said would happen.
      `storage.Appender` is **PARTIAL** — right package, still ambiguous among 3 in-package
      same-name symbols — and is excluded from the win count.
- [ ] **Accept one confidence downgrade.** `promql/engine_test.go:294`'s `hintRecordingQuerier`
      embedding `storage.Querier` goes `unambiguous → ambiguous`. Before, it pointed *confidently*
      at a same-file method (wrong); now it is honestly ambiguous among 5 `Querier` symbols in the
      right package. This is the only confidence downgrade in the whole rebuild diff. It is the
      same root cause as the `storage.Appender` limitation and is gated behind the same
      prerequisite (in-package disambiguation).
- [ ] **Expect an add/add merge conflict on `bench/engine/FINDINGS-workspace-graph.md`.** The file
      does not exist on `origin/main` @ `2c8b9c3`, but local unpushed `main` @ `263b954` has a
      123-line version. This branch creates it with the 0017 entry under the same H1. Resolution is
      concatenation — no content loss.
- [ ] **Read ADR-0014** (`docs/adrs/0014-go-import-self-bindings-suppressed.md`). It records the
      one genuinely non-obvious decision, and it is the thing a future language-adapter change must
      not walk into blind.

## Findings

Five review findings (1 blocker, 2 important, 2 minor); all five fixed in-branch. Per-finding
dispositions and commits are in the PR body's disposition table. Recorded here is only what
outlives the diff.

### The blocker: a Go-only remedy was written language-agnostically

This is the finding worth carrying forward, because the bug was introduced *by a correct fix to a
correctly-identified regression* and was invisible to the entire suite.

Setting `Source` on Go import deps (edit 1) newly populated the file-level `bind` map for Go files.
That was a named risk in the spec, and it was **real**: measured, a `x.log()` call in a file
importing `"log"` moved from the in-package `Server.log` to an unrelated `internal/log/log.go`.
The mechanism matters — in `resolve()` the `nsHint != ""` `boundIDs` steps run *before* the `srcNS`
same-scope step, so a newly non-empty hint does not narrow the candidate set, it **preempts the
rung that had been deciding the edge**.

The remedy — stop letting Go import deps populate `bind` — was implemented as skipping a
*self-binding* (`normalizeHint(...) == Target`). For Go that predicate is exactly right and total
by construction. But it was written without a language gate, and **it is only true for Go**: a Go
namespace is a repo-relative directory and a Go import's `Target` is the whole slash-bearing path,
so the tautology is genuinely empty; for Python and TypeScript the namespace *is* the module path,
so `from app import app` → `bind["app"] = "app"` is a semantically correct, load-bearing hint.
All three non-Go adapters emit `KindExtends`/`KindImplements` with `Source == ""`, so `bind` is
their **only** hint channel. The language-agnostic version silently deleted it.

Nothing caught this. The full suite was green across the defect. The nearest test
(`dephint_test.go`'s fallback half) is annotated "the behavior every non-Go adapter depends on" but
its fixture is a *non*-self-binding shape no adapter emits, so it passed regardless.

Fixed by scoping to the importing file being Go (`goImportSelfHint`). The gate is on the **file**,
not the target's shape, and that is deliberate: the hazard case `import "log"` is bare, lowercase
and single-segment — indistinguishable from a Python module named `log`, so any target-shape test
that catches it also catches `from log import log`. A related shape filter had already been
rejected in the spec for the inverse reason (the colliding entry is *itself* a non-path target, so
a non-path filter preserves exactly the wrong entries).

### The second finding closed a delta the spec expected not to exist

The spec's named-risk section asked for verification that import edges do not change which symbol
they resolve to. Measurement said they **do**: `import "log"` moved from `app/server.go` to
`internal/log/log.go`, resolving `unambiguous` to an unrelated symbol — and `unambiguous` is the
token every downstream consumer trusts.

The build's first response was honest but incomplete: it pinned the movement as a
`TestMEASUREDDELTA…` characterization test rather than faking a no-regression guard, and bench
measurement then found the class size was **0 on prometheus**. Review correctly refused that as a
merge basis — zero on one member whose directory layout happens not to produce the shape is not
evidence the class is closed, and `kubernetes`/`laravel`/`nest`/`flask`/`gin` were unmeasured.

The real defect was an **asymmetry**: the self-binding rule was enforced at the `bind` site but not
at the dep site, so the codebase asserted in one place that a tautological hint is hazardous and in
the other place applied exactly that hint. Applying the same predicate at both sites closed the
class **by construction** rather than by argument, at provably zero cost to the acceptance bar (a
qualified embed's `Source` is a full import path and its `Target` a bare type name, so the two are
never equal and the skip cannot fire). The pinned delta test became the genuine no-regression guard
plan task 6b originally asked for and could not deliver.

This is the ledger's `one-invariant-many-sites-drifts` shape, caught at birth: the two sites' doc
comments had already started contradicting each other.

## Notable plan deviations

- **Tasks 9 and 10 were executed as one worker, not two.** Task 9's stop-rule is defined *in terms
  of* task 10's table, and both need the same pair of before/after bench indexes; splitting them
  would have rebuilt the indexes twice and left task 9 without its accounting reference.
- **Review finding 3 was discharged by finding 2's fix rather than its own task.** Finding 3's
  stated remedy *was* finding 2's fix; a separate dispatch would have been a no-op.
- **Plan task 6a's remedy branch fired.** The plan wrote it as a conditional ("if it does capture
  it…"). It did.
- **Two plan priors were corrected by measurement**, recorded in the bench write-up: (a) the 2
  non-addressable edges of the 25 are not "bare generic `T`" — one is a qualified `*testing.T`
  whose package is not in the index, the other a bare unqualified `Postings` (the denominator of 23
  is unaffected); (b) 21 further `refresh.Discovery` embeds were `unambiguous`-but-**wrong** before
  — 14 of them resolving the embed to the embedding struct itself — and are now correct. They sit
  outside the table by design, since the bar is stated over `ambiguous → unambiguous`, and are
  **not** counted as wins.
- **Several tests could not do a literal pre-implementation RED** because they characterize
  existing behavior. They used mutation evidence instead — revert the edit, watch the test go red
  for the intended reason, restore. Review independently checked these and found none vacuous.

## Follow-ups (not filed as changes — `auto_capture` is disabled in this repo)

- **In-package disambiguation** is the named prerequisite behind both `storage.Appender` (PARTIAL)
  and the `promql` `storage.Querier` downgrade. It is recorded three ways per the ledger's
  `known-limitations-need-a-characterization-test`: a `TestKNOWNLIMITATION…` test, the `boundIDs`
  entry-point doc comment, and the prerequisite stated as what must land *first*. The two naive
  closes — relabel `>1` as unambiguous, or break the tie on ordering — each convert an honest
  `ambiguous` into a confident wrong answer no consumer can detect.
- **`graph.RawDep.Source`'s doc comment** (`internal/graph/types.go`) still reads "import origin:
  TS specifier, Py module, PHP use-path" and no longer enumerates Go. Cosmetic, untouched.
- **The import-resolution measurement protocol** (`bench/engine/FINDINGS-import-resolution.md`) has
  a six-repo protocol; change 0017 altered the `bind` population rule and re-measured **prometheus
  only**. The blocker fix restores non-Go `bind` population bit-for-bit, so a re-measurement is not
  believed necessary — but the reasoning is analytic, not measured.
- **Go interface embedding and `implements` remain at ZERO coverage**, not partial. Go emits no
  `KindImplements` dep anywhere and interface embedding parses as `type_elem`, never
  `field_declaration`, so it emits no dep at all. Despite this change's title, it adds no implements
  coverage; adding a `type_elem` emit site is a missing-EDGE change that would move the denominator
  this change's acceptance bar is stated over.

## Evidence

Suite `go test -tags nollama -count=1 ./...` green at `be46b6b` (the pinned `FINALIZE_TEST_COMMAND`;
plain `go test ./...` fails 10 packages environmentally and is not the gate). Two full-suite runs
total: one at the end of the build, one after the fix loop.

The rebuild-diff check was **not** an empty-diff rule — `DumpNormalized`'s edge string embeds the
confidence token and the resolved dst, so every win necessarily shows as a diff line and an
empty-diff stop-rule would have fired on success. The rule was that every diff line be accounted
for: **45 changed edge lines, 0 symbol lines, all accounted for**, plus a re-implementation of the
resolution ladder validated against both databases at **0 mismatches in 256 checks**.

Worth stating plainly, because it shaped every testing decision here: **this repo has no committed
graph goldens.** Every `DumpNormalized` consumer is an incremental-vs-full rebuild-equivalence check
under the same code, so both sides move identically and none can detect a resolution regression —
and `DumpNormalized` does not select `dst_ns` at all. The in-suite regression protection is
entirely the new `internal/graph` tests.

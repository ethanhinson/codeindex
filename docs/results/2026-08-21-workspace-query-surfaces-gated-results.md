<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0016 — Workspace query surfaces — union-graph verbs, CLI/MCP wiring, workspace-status; merge gated on the D7 evidence run](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0016-workspace-query-surfaces-gated.md)**
<!-- docket:backlink:end -->

# Workspace query surfaces — results

Change 0016 `workspace-query-surfaces-gated`. Base `origin/main` @ `2c8b9c3`.
Plan: `docs/superpowers/plans/2026-08-20-workspace-query-surfaces-gated-plan.md`.
Spec: `docs/superpowers/specs/2026-08-20-workspace-query-surfaces-gated-design.md`
on `origin/docket` (amended 2026-08-21 — see ADR-0013 below).

Local suite green: `go test -tags nollama -count=1 ./...`, 24 packages ok, at
`1767e2b`. Plain `go test ./...` fails 10 packages on **every** ref
(absent vendored llama.cpp headers) and is not the bar.

## Verify (human)

1. **The D7 evidence gate is yours and is not run here.** §9's arm A vs arm B
   run on the frozen 65-task corpus is owner-attended: the bench harness and
   corpus live in local-main-only unpushed commits, so it runs from the local
   main tree against this branch's binary. Order is fixed: `leak_audit_ws.py`
   over the campaign transcripts first (a standing pre-verdict gate that
   refuses a verdict on non-zero exit), then the two arms, then the bars read
   **verbatim from `bench/workspace/README.md`**, never from the gate script's
   source. **The kill condition is live**: if B does not beat A on recall or
   efficiency, the control wins, the `bench/engine/FINDINGS-workspace-graph.md`
   entry is the deliverable, and this change closes. That is a legitimate
   outcome, not something to work around.
2. **Read the disposition table in the PR body against the diff.** Eleven
   review findings were fixed in-branch; there is deliberately no re-review
   round, so the human reading the fix commits is the remaining check.
3. **Spot-check a real workspace by hand** if you have one: `codeindex
   workspace-status <ws>`, then `codeindex callers <ws> api:SomeSymbol`, then
   `codeindex status <ws> --json | jq .` (that last one was broken and is
   finding #4 below).

## Findings

### ADR-0013 — an unindexed member is named stale on every pass, not just the transition

The one decision from this run that outlived the branch, and it was a review
catch, not a build one.

Spec §4.3 defined `members_stale` as a **four-way** union and deliberately
excluded `MembersUnindexed`, on the stated ground that such a member is
"covered by `StaleStamped` when it previously contributed rows, and by
`boundary` when it never did." **Both halves are false.** `StaleStamped` is
one-shot by its own doc comment — "non-empty for at most ONE pass per
transition" — because the `wsresolve.Resolve` the transition triggers prunes
the very stamp that fired it; and `boundary` is a fixed constant string that
says nothing about a declared member *inside* the workspace whose rows were
omitted.

The steady state was therefore a present, declared, unbuilt member silently
missing from every answer while `members_stale` read `(none)`. A freshly cloned
member that has never been built reaches that on the **first query**. That is
the silent staleness the change's own D7 gate hard-fails — the slice would have
shipped, under its own gate, the exact condition that gate refuses.

The build had **characterized** it (`TestBYDESIGNAnUnindexedMemberIsNamedStale
OnlyOnTheTransitionPass`) rather than closing it. On review that disposition was
judged wrong: the spec's exclusion rested on a demonstrably false supporting
argument, which per `ordering-claims-must-survive-the-error-return` makes it a
**gap** — fair to reopen — not a weighed decision being re-litigated. The
prerequisite the characterization test itself named was also one line and
already precedented in the same branch (`MembersFreshenFailedIDs`, commit
`843b0a7`).

Fixed in `ca22ac8`: `members_stale` is a **five-way** union, and
`wsfresh.Report` gained `MembersUnindexedIDs` written additively at the same
site as its count. The D7 property now queries **twice** and asserts staleness
on both the transition and the steady-state pass — a single-pass test cannot
catch a one-shot disclosure, which is precisely how the gap survived.

Spec §4.3 and assumption 6 carry a dated amendment pointing at ADR-0013.

### Findings the review caught that the green suite did not

All eleven were fixed in-branch; the suite was green before and after, which is
the recurring shape in this campaign.

- **`nav`'s `*Total` fields were recounted from truncated per-member lists**
  (blocker, `baf588e`). `query.Nav` truncates `Files` to `limit` while reporting
  the untruncated count; `navWorkspace` iterated the truncated list and then
  recomputed the total from it, so a one-member workspace under-reported and the
  `... (+N more)` tail could never fire — a direct break of D7's frozen §7.4
  bar. The in-code justification was **inverted**: the per-member-limit argument
  is correct for `findWorkspace` (where `FindAnswer.Total` is genuinely
  post-truncation in repo mode) and was wrongly carried over to `nav`, whose
  totals are pre-truncation. Worth noting the worker **rejected the reviewer's
  suggested fix** with a concrete counter-argument — going unlimited would have
  changed the `Files` head ordering and the definitions fallback, breaking §7.4
  in the other direction — and accumulated the inner totals instead.
  No test caught it because every fixture used `limit 10` over ≤4 files, so the
  truncation branch was never entered.
- **`status <workspace-root> --json` emitted unparseable output** (important,
  `e601922`) — interleaved prose headers plus N+1 top-level JSON documents. A
  documented flag producing garbage on a newly supported input. Now one
  document; repo-mode byte-identity was **measured** against a `HEAD~1` binary,
  not assumed.
- **`wsfresh`'s package doc still declared the package UNWIRED** and forbade the
  wiring this branch performed (important, `b64c797`). Textbook
  `one-invariant-many-sites-drifts`: `session.go` carefully explained *why* it
  calls `Freshen` while `wsfresh.go` said nothing may call it. A second stale
  claim about `workspace-status` was found and corrected in the same pass.
- **An ambiguous stable-key re-map discarded its candidates** (minor,
  `1767e2b`), rendering the *recorded* path — the very datum the re-map exists
  because it may have moved — plus `DefLine: 0`. The load-bearing property
  (never manufacture an exact-looking answer) held; the disclosure did not.
- **The read-only query path opened the overlay through the mutating
  `overlay.Open`** (minor, `1767e2b`), which `workspace-status` had gone to
  lengths to avoid. On the §4.2 degrade path a schema-mismatched overlay could
  be deleted and recreated **empty by the query itself** — silent staleness by
  another road. Now version-gated with a `overlay_unreadable` clause
  disclosure.
- Also fixed: the `repo:` prefix was skipped on pathless rows (unresolved
  callees/deps — the common case); the `wsquery` package doc overclaimed
  *structural* byte-identity that §7.1 explicitly forbids claiming; a
  `not wired yet` scaffold sentinel survived into the merged surface; and
  `find`'s summed `Total` gained the named characterization test its recorded
  exception wanted.

## Residual risks the build named and did not close

- **Every workspace query entry — including all eight MCP tool handlers — runs
  a whole-manifest `Freshen`.** So an MCP session pays one full-workspace
  freshen per tool call. This is §4.1's argued design (assumption 4), not a
  defect, and the D7 run is what measures it. The reviewer flagged it as **the
  residual most likely to decide the gate**. The escape hatch, if latency bites,
  is the artifact-import path the risk note already sanctions — not a subset
  freshen, which would be a second enforcement site for ADR-0012's whole-pass
  rule.
- **`workspace-status` prints the unindexed *count* but ids for freshen-failed
  and missing.** A deliberate asymmetry left after ADR-0013 added
  `MembersUnindexedIDs`; the ids are now available and printing them is a small
  follow-up.
- **An ambiguous cross-edge now contributes N rows instead of 1**, so
  `CallersTotal`/`Total` grow on that path. Intended disclosure, not a count
  regression — but it is a visible behaviour change if anyone has expectations
  pinned to the old single-row shape.
- **Per-member fan-out line format and the `refresh` summary text are
  unpinned.** No golden covers them, so they can drift without a test noticing.
- **`grep`'s backend-disagreement join is driven through a seam**, not two real
  backends — the environment cannot reach both. The agreement case is real.
- **`nav`'s cross-member path dedup** decrements `filesTotal` on collision,
  making the total a best-effort upper bound when one member's root nests inside
  another's. Unreachable for a single member, so §7.4 stays exact.
- **MCP byte-identity is measured against `internal/query` as it stands today**,
  not against a frozen pre-branch capture.

## Verification quality notes

- **Real RED/GREEN**: tasks 2, 4, 5, 7, 10, and the fixes for findings #1, #2,
  #4, and three of the five in the minor batch. Several are worth trusting
  specifically — finding #2's RED reproduced the `d7SilentlyStale` hard fail
  itself, and finding #10's RED printed the query destroying the overlay
  (`workspace overlay schema v2 -> v1, rebuilding`).
- **Mutation-only evidence** (tests authored against stubs, RED not read;
  guards proven by reverted mutation instead): tasks 6, 8, 9, 11, 12. Task 6
  reproduced the vendored-snapshot re-admission verbatim under mutation, which
  is the strongest of these.
- **No RED available by construction**: task 1 (goldens — proven to bite by
  perturbing a renderer and reverting), task 13 (docs), the comment-only minor
  batch, and finding #11's characterization test.
- Two suite runs total in the fix phase's budget; both green, zero FAIL lines.

## Plan deviations

- **The §4.5 workspace `--json` clause sibling had a seam but no caller** after
  tasks 7 and 9 — flagged independently by both workers. Not a distinct plan
  task, so it was folded into task 11 as an explicit second deliverable rather
  than left for the reviewer to find.
- **Task 12 was directed to fix-or-characterize and chose characterize**; review
  reversed that call (ADR-0013). The direction was right — the finding surfaced
  loudly instead of being worked around — but the disposition needed the second
  opinion.
- `wsquery.Fresh` and `ErrWorkspaceNotWired` were **removed** rather than
  completed; no non-test caller existed.

## Follow-ups

None minted — `auto_capture` is disabled for this repo. Recorded here instead:

1. Print `MembersUnindexedIDs` in `workspace-status` alongside the
   freshen-failed and missing ids (closes the asymmetry ADR-0013 left).
2. Decide the freshen-per-MCP-call cost once D7 has measured it; the
   artifact-import path is the sanctioned lever.
3. Pin the fan-out line format and `refresh` summary text with goldens if they
   are to be treated as a surface.

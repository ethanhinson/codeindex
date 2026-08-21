---
id: 16
slug: workspace-query-surfaces-gated
title: Workspace query surfaces — union-graph verbs, CLI/MCP wiring, workspace-status; merge gated on the D7 evidence run
status: implemented
priority: high
type: feat
created: 2026-08-20
updated: 2026-08-21
depends_on: [15]
related: [9, 12, 13, 14, 15, 10]
discovered_from: []
adrs: [12, 13]
spec: docs/superpowers/specs/2026-08-20-workspace-query-surfaces-gated-design.md
plan: docs/superpowers/plans/2026-08-20-workspace-query-surfaces-gated-plan.md
results: docs/results/2026-08-21-workspace-query-surfaces-gated-results.md
trivial: false
auto_groomable: true
branch: feat/workspace-query-surfaces-gated
claimed_at: 2026-08-21T02:15:05Z
pr: https://github.com/ethanhinson/codeindex/pull/14
blocked_by:
reconciled: true
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| Spec | [2026-08-20-workspace-query-surfaces-gated-design.md](https://github.com/ethanhinson/codeindex/blob/docket/docs/superpowers/specs/2026-08-20-workspace-query-surfaces-gated-design.md) |
| Plan | [2026-08-20-workspace-query-surfaces-gated-plan.md](https://github.com/ethanhinson/codeindex/blob/feat/workspace-query-surfaces-gated/docs/superpowers/plans/2026-08-20-workspace-query-surfaces-gated-plan.md) |
| Results | [2026-08-21-workspace-query-surfaces-gated-results.md](https://github.com/ethanhinson/codeindex/blob/feat/workspace-query-surfaces-gated/docs/results/2026-08-21-workspace-query-surfaces-gated-results.md) |
| PR | [#14](https://github.com/ethanhinson/codeindex/pull/14) |
| ADRs | [ADR-0012](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0012-workspace-freshness-re-resolves-whole-workspace.md), [ADR-0013](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0013-unindexed-member-named-stale-every-pass.md) |
<!-- docket:artifacts:end -->

## Why

The workspace engine is complete and merged (changes 0009, 0012–0015)
but deliberately unwired: no verb reads the overlay, so a user still
cannot ask a single cross-repo question. This change is the wiring —
the last slice of the workspace-graph campaign
(`openspec/changes/workspace-graph/tasks.md` §4.1–§4.4 + the
`workspace-status` half of §3.4, plus hosting the §5 gate run).

Per the D7 second amendment (design.md Amendments, owner ruling
2026-08-19), this is exactly the slice the merge block binds to: **its
PR merges only if the pre-registered D7 evidence gate passes.** If the
grep-across control wins, the verdict is recorded in
`bench/engine/FINDINGS-workspace-graph.md` and the change closes —
that is a legitimate outcome (kill condition, frozen in D7).

## What changes

Per frozen design D4 (query semantics) and D5 (surfaces):

- **New package `internal/wsquery`** — the union layer. An import cycle
  forces it (`wsfresh` already imports `query`); its `RootRepo` branch
  tail-calls `internal/query` verbatim, which is what makes the
  non-regression bar mechanical. It is also the single root-kind
  detection site the whole binary routes through.
- **Union-graph verbs (§4.1):** `callers`/`callees`/`impact`/`nav`, plus
  `dependents`/`deps` (forced: `impact` embeds the dependents block, so
  a per-repo `dependents` would contradict it), over member graph +
  overlay edges; `impact` stays depth-1 and simply includes cross-edges;
  `find`/`grep` fan out (complete sets, no rank-merge; manifest order).
  Workspace-relative paths; every reference carries `repo:
  <member-id>`; anchors accept an optional `<member-id>:` prefix. Two
  recorded obligations are discharged here: the `dep_suppressions`
  filter, conditioned on a same-call-site cross-edge exactly as
  `wsresolve`'s package doc narrows it; and the overlay's
  `exact`/`inferred` vocabulary, reconciled onto the surface's existing
  `Ambiguous` flag plus one new `Inferred` flag (the surface carries no
  `graph.Confidence` at all — that framing was wrong).
- **CLI root-kind wiring (§4.2):** every verb's root argument accepts a
  workspace root via `engine.DetectRootKind` (first real call site);
  single-repo goldens stay byte-identical — **measured, not structural**,
  since the nine shared `Text()` renderers are being edited, so goldens
  are extended from five to all nine first. The single-member-workspace
  ≡ single-repo bar (deferred from §3.5) lands here, so **§3.5 is ticked
  by this slice too**.
- **MCP (§4.3):** `codeindex mcp <workspace-root>` serves the union
  graph; `repo` provenance as a workspace-mode-only text prefix per
  owner ruling 2; no new tools, no schema or description edits; plugin
  note untouched (house rule).
- **Refusals:** `export`/`import`/`ingest`/`depmap`/`serve` error on a
  workspace root per owner ruling 1, and `search` joins them — cross-
  workspace semantic search is a frozen non-goal, not a gap.
- **`workspace-status` verb** (§3.4's gated half): per-member
  build/stamp state, member/vendor version skew from suppression
  records.
- **Workspace goldens + freshness property (§4.4):** workspace answers
  pinned; the D7 freshness scenario (mutate one member, query from
  another, no explicit rebuild → answer reflects it or the coverage
  clause names the member stale) as an executable property test.
  Freshen wiring calls `wsfresh.Freshen` on query entry, **whole
  workspace** — argued from `Freshen`'s subset-free signature and the
  unknowable-without-folding cost, *not* from ADR-0012, which governs
  re-resolution scope only. A `Freshen` error degrades and discloses
  rather than failing the query, naming every declared member stale.

**Merge gate (§5, runs on this PR before merge):** re-run the
four-class leak audit over campaign transcripts (standing pre-verdict
gate), then the pre-registered D7 run — arm A (shell + checkouts) vs
arm B (A + workspace MCP pointed at this branch's binary via
`CODEINDEX_WS_MCP_BIN`) on the frozen 65-task corpus; bars read from
`bench/workspace/README.md` verbatim. Verdict + residuals recorded in
`bench/engine/FINDINGS-workspace-graph.md` either way. Note: the bench
harness and corpus live in local-main-only commits (unpushed), so the
gate runs from the local main tree against the branch-built binary —
the run is an owner-attended step, not part of the autonomous build.

## Out of scope

- Corpus growth (change 0010) — the gate runs on the frozen 65-task
  corpus as registered.
- Cross-workspace semantic search / vectors, UI, git-remote identity,
  language expansion, re-ranking (frozen non-goals).
- Scoped incident re-resolution (ADR-0012 stands; a D7-measured
  follow-up if latency demands).

## Owner rulings (2026-08-20) — settled, folded into the spec

Groomed 2026-08-20. Both abstain questions were owner-ruled and are now
carried in the linked spec; the prior groom's verified findings and the
critic's corrections were distilled into it and are no longer restated
at proposal altitude. Design detail lives in the spec — this section
keeps only the rulings themselves and one record correction.

**Record correction (2026-08-20).** Carried finding 6 below said "only
three of the nine [renderers] have goldens today
(`internal/query/query_test.go:43,113,140`)". The tree has **five** —
it missed `TestEnclosingText:128` and `TestGrepTextAndJSON:186`. Both
critic passes confirmed five independently against `origin/main` at
2c8b9c3. The conclusion (extend goldens to all nine) is unchanged.

The first groom pass abstained on two narrowings of the frozen D5
surface paragraph; the owner ruled on both (full abstain record in git
history):

1. **Maintenance verbs on a workspace root — run per member.**
   `build`/`refresh`/`status` fan out across members in manifest order
   (`refresh` on a workspace root is `wsfresh.Freshen`, already merged;
   `status` pairs with the new `workspace-status`). Data/serve verbs
   (`export`/`import`/`ingest`/`depmap`/`serve`) error on a workspace
   root with a clear per-repo message.
2. **MCP `repo` provenance — workspace-mode-only text prefix.** Each
   result line carries the member id (e.g. `api: services/api/u.go:42`)
   in workspace mode only; single-repo text output stays byte-identical;
   no new tools; the plugin note stays untouched. This discharges the
   frozen "`repo` in result schemas" via the text surface the tools
   actually have.

**Verified designer-pass findings** — established against `origin/main` at
2c8b9c3 and confirmed by the critic; safe to build on:

1. **An import cycle forces a new package.** `internal/wsfresh/freshen.go:12`
   imports `internal/query` and calls `query.Fresh` at `freshen.go:218`, so the
   union layer cannot live in `internal/query`. A new `internal/wsquery` with
   `DetectRootKind` routing, whose `RootRepo` branch tail-calls `internal/query`
   verbatim, is the shape.
2. **The `dep_suppressions` filter needs no new `internal/graph` reader.**
   `graph.(*Store).TierOneEdges()` (`internal/graph/wsreaders.go:92`) returns
   exactly the five-part call-site tuple plus `DstNamespace`, and it lines up
   with the overlay's `src` columns because the ladder builds the cross-edge
   source key the same way (`internal/wsresolve/ladder.go:107` →
   `internal/overlay/edges.go:408-422`). The same-call-site narrowing at
   `internal/wsresolve/wsresolve.go:19-29` must be honored verbatim.
3. **`impact` is depth-1 today** (`internal/query/answers.go:320`), so D4's
   "crosses member boundaries by default" means the depth-1 neighbourhood
   includes cross-edges with no flag — not a new closure single-repo lacks.

**Corrections the next groom must carry** — errors the critic caught in the
discarded draft, recorded so they are not re-derived:

4. **The stable-key re-map must use `graph.ProjectDefs`, not `Definitions`.**
   `ProjectDefs`' own doc comment states "Definitions cannot be reused — it
   neither filters tier nor returns Tier/Namespace"; `Definitions`
   (`internal/graph/store.go:656-663`) has no `tier` filter, so using it
   re-admits a vendored snapshot as a cross-repo target — the exact failure
   member-over-dep precedence exists to prevent. The keys being inverted were
   written by `ProjectDefs` (`internal/wsresolve/ladder.go:211,215`). Also: the
   `QName` split must be on the **last** `.`, since `QName()` is
   `Parent + "." + Name` and dotted parents exist in TS/Python.
5. **ADR-0012 does not force whole-workspace freshen-on-query.** The draft
   claimed it did; ADR-0012 governs *re-resolution* scope only, not which
   members receive `query.Fresh`. Whole-workspace freshen still looks like the
   right default — `wsfresh.Freshen`'s signature takes no member subset, and the
   whole-pass `Resolve` reopens every member anyway — but it must be argued on
   those grounds, not by appeal to a frozen ADR.
6. **"Byte-identical by construction" overclaims.** The nine `Text()` renderers
   in `internal/query/answers.go` are shared and would be edited, so repo-mode
   identity is *measured*, not structural — and only three of the nine have
   goldens today (`internal/query/query_test.go:43,113,140`). Extend goldens to
   all nine. Relatedly, `ImpactAnswer.Coverage` (`answers.go:322`, rendered
   unconditionally at `answers.go:336`) already exists; the spec must say what
   it holds in workspace mode alongside the new clause.
7. **`explore-feature` is not a verb.** It is an MCP *prompt*
   (`internal/mcpserver/mcpserver.go:186`) returning a static workflow string;
   it reads no root and cannot "error on a workspace root". `search` is the real
   case, and `wsquery` must state a guard for `mcpserver.go:178`'s
   `query.SearchText`.
8. **Smaller gaps:** `limit` semantics are unstated for the anchor verbs' own ∪
   cross union (`CallersAnswer.Text()` prints `CallersTotal - len(Callers)`);
   `Report.Dirty` is missing from the four-way `members_stale` union and
   `members_consulted` is never defined; a `Freshen` error itself has no stated
   posture; `enclosing`'s longest-prefix match must be on *resolved absolute*
   roots (D1 allows `../api`); and **§3.5 must be ticked too** — its
   single-member-workspace bar is exactly what this slice discharges.

**Recommendation: neither kill nor defer.** The change is well-formed, its
dependency is satisfied, and the campaign's engine work is merged and waiting on
exactly this wiring. The two open questions are narrow rulings, not a redesign.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

### 2026-08-20 — reconcile (docket-implement-next)

**Baseline unmoved.** The spec was authored against `origin/main` at `2c8b9c3`
and `origin/main` is still exactly `2c8b9c3` (change 0015's merge, PR #13). No
intervening code has landed, so nothing in the design is stale by drift. Scope,
`depends_on: [15]` (satisfied, archived `2026-08-20-0015-wsresolve-stamp-pruning.md`),
and the frozen non-goals are unchanged. No work has been done elsewhere; no
constraint has been added. **The change body and spec are carried forward
unedited** apart from the record note below.

**Independent re-verification of the spec's load-bearing code citations.** Every
fact the design rests on was re-checked against the tree, and all of them hold:

- The import cycle forcing `internal/wsquery` is real — `internal/wsfresh/freshen.go:12`
  imports `internal/query` and calls `query.Fresh` at `:218`.
- `internal/wsquery` does **not** exist yet, and no `workspace-status` verb
  exists anywhere — both are genuinely new (`wsfresh.go:8-9` only comments that
  the verb is deliberately unwired). The slice is still unbuilt in full.
- **Nine** `Text()` renderers in `internal/query/answers.go` (Callers:111,
  Callees:139, Find:163, Grep:195, Nav:228, Dependents:273, Deps:299,
  Impact:332, Enclosing:374) — the count the golden-extension task keys on.
- **Five** existing text goldens, exactly as the spec's record correction says
  (`TestCallersTextGolden:43`, `TestCalleesTextGolden:113`, `TestEnclosingText:128`,
  `TestNavTextGolden:140`, `TestGrepTextAndJSON:186`) — so the change body's
  carried finding 6 ("only three") stays the erroneous side and the spec's five
  is operative. Confirmed a third time here.
- `Ambiguous bool` exists on `CallerRef` (`:32`) and `CalleeRef` (`:44`) and on
  **no other** reference struct — so §3.7's reconcile onto `Ambiguous` + a new
  `Inferred` is correct and no answer type carries a `graph.Confidence`.
- `SplitAnchor` (`query.go:31-39`) parses `::` first (`strings.Index`, `:32`)
  then the last `.` (`strings.LastIndex`, `:35`) — §3.4's narrower prefix rule
  is required as written.
- `Impact` is depth-1 (`query.go:523`), `impactCoverage` at `:518-519`,
  `ImpactAnswer.Coverage` is a `string` (`answers.go:322`) rendered
  unconditionally at `:336`.
- `graph.ProjectDefs` (`wsreaders.go:127`) and `TierOneEdges` (`:92`) exist;
  `Definitions` is `store.go:656`. §3.8's "`ProjectDefs`, never `Definitions`"
  binding stands.
- `wsfresh.Report` is exactly as §4.3's blocking note describes:
  `MembersFreshened`/`MembersFreshenFailed`/`MembersUnindexed` are `int`
  (`:52`, `:63`, `:81`); `MembersMissing`/`Dirty`/`StaleStamped` are `[]string`
  (`:88`, `:95`, `:126`); plus `Resolved bool` (`:132`) and `Stats` (`:141`).
  The additive id slice for freshen-failed members is genuinely needed.
- The `Dirty`-with-`Resolved: false` degrade window is real and must be
  honoured: `rep.Dirty` appended at `freshen.go:255`, `rep.Resolved = true` only
  at `:375`, error returns at `:292`, `:318`, `:373` sitting between them.
- `wsresolve.go:19-29` records the `dep_suppressions` obligation verbatim,
  including the load-bearing **ONLY WHEN … same call site** narrowing.
- `fatal()` (`cmd/codeindex/main.go:646-649`) is `os.Exit(1)` unconditionally —
  assumption 14's withdrawal of "exit 2" holds.
- MCP: eight tools plus the `search` handler at `mcpserver.go:178`
  (`query.SearchText`) and the `explore-feature` **prompt** at `:186` returning a
  static string — assumption 16 stands, the prompt is untouched.
- `overlay.Suppressions()` (`:513`), `OutEdges` (`:408`), `InEdges` (`:417`),
  `Suppression{ConsumerMember, Namespace, OwnerMember, SuppressedVersion}`
  (`:47-50`), `CrossEdge.Confidence` `"exact"|"inferred"` (`:26`) — all present.
- `config.LoadWorkspace` (`:44`) and `Workspace.Resolve` (`:106`) returning
  `present` first, with `ResolvedMember{Member{ID, Root}, AbsRoot}` — §2.3's
  absolute longest-prefix match is implementable exactly as specified.

**One non-load-bearing correction.** A few cited line numbers drift by one to
five lines against the tree: the reference structs are at `DefRef:15`,
`CallerRef:26`, `CalleeRef:37`, `DependentRef:50`, `FindRef:58`, `GrepRef:71`,
`DepRef:80`, `EnclosingRef:89` (§5 lists 14/24/36/49/57/68/79/88), and
`ladder.go`'s `ProjectDefs` call is at `:210` with the parent-less fallback at
`:214-217` (§3.8 cites `:211,215` and `:213-218`). Every *identity* is correct
and every rule the numbers illustrate is unaffected, so the spec is **not**
edited for this — line numbers are the least durable part of a design doc and
re-pinning them buys nothing. Recorded here so the builder trusts the names over
the offsets.

**Merge gate unchanged and still owner-attended.** §9's D7 evidence run is not
part of this build (the bench harness and corpus are local-main-only, unpushed).
This run stops at the open PR; the gate and the kill condition remain live and
the owner's to execute.

**Auto-capture:** disabled for this repo (`AUTO_CAPTURE_ENABLED=false`), so no
stubs were minted. Nothing surfaced in this pass that warranted one regardless.

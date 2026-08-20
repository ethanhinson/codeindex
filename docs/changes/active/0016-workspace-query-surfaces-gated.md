---
id: 16
slug: workspace-query-surfaces-gated
title: Workspace query surfaces — union-graph verbs, CLI/MCP wiring, workspace-status; merge gated on the D7 evidence run
status: proposed
priority: high
type: feat
created: 2026-08-20
updated: 2026-08-20
depends_on: [15]
related: [9, 12, 13, 14, 15, 10]
discovered_from: []
adrs: [12]
spec:
plan:
results:
trivial: false
auto_groomable: false
branch:
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
| Artifact | Link |
|---|---|
| ADRs | [ADR-0012](https://github.com/ethanhinson/codeindex/blob/docket/docs/adrs/0012-workspace-freshness-re-resolves-whole-workspace.md) |
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

- **Union-graph verbs (§4.1):** `callers`/`callees`/`impact`/`nav` over
  member graph + overlay in-edges; `impact` crosses member boundaries by
  default; `find`/`grep` fan out across members (complete sets, no
  rank-merge; members in manifest order). Workspace-relative paths;
  every reference carries `repo: <member-id>`; anchors accept an
  optional `<member-id>:` prefix. Two recorded obligations from the
  engine slices are discharged here: filter the surviving intra-repo
  tier-1 edge via `dep_suppressions` (conditioned on a same-call-site
  cross-edge, else double-count), and reconcile the overlay's
  `exact`/`inferred` confidence vocabulary with `graph.Confidence` at
  the surface.
- **CLI root-kind wiring (§4.2):** every verb's root argument accepts a
  workspace root via 0009's `DetectRootKind` (first real call site);
  single-repo goldens stay byte-identical; the
  single-member-workspace ≡ single-repo bar (deferred from §3.5) lands
  here.
- **MCP (§4.3):** `codeindex mcp <workspace-root>` serves the union
  graph; `repo` field in result schemas; no new tools; plugin note
  untouched (house rule).
- **`workspace-status` verb** (§3.4's gated half): per-member
  build/stamp state, member/vendor version skew from suppression
  records.
- **Workspace goldens + freshness property (§4.4):** workspace answers
  pinned; the D7 freshness scenario (mutate one member, query from
  another, no explicit rebuild → answer reflects it or the coverage
  clause names the member stale) as an executable property test.
  Freshen wiring calls `wsfresh.Freshen` on query entry (D2: lazy,
  members consulted by the query where the verb permits).

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

## Auto-groom blocked

### 2026-08-20

`docket-auto-groom` abstained. **The blocker is the adversarial gate, not the
design.** A full designer pass ran and produced a complete draft spec; the
`docket-auto-groom-critic` dispatch was issued twice-over the same draft (one
dispatch plus one collect attempt against the running agent) and **returned no
legible verdict within the run's window**. The host harness resolved the
dispatch asynchronously and exposed no mechanism that makes the parent block on
the child's return, so the protocol's fresh foreground re-dispatch leg could
only have repeated the first. Per the convention's *Dispatch-capability
resolution*, that is **Tier B**: the groom abstains rather than self-review,
because the agent that drafted a spec cannot be its own adversarial gate. The
draft was **discarded uncommitted** — emitting it would mark this change
build-ready, and an ungated spec on the largest, merge-gated slice of the
campaign is exactly what the gate exists to prevent.

**What a human should supply.** Nothing about the design is known to be
missing — re-arm by running `docket-groom-next` interactively (you are the
gate), or by re-running `docket-auto-groom` in a harness where the critic
dispatch returns synchronously. Either way, delete this section on re-arm.

**Designer-pass findings, ungated — verify before trusting any of them.** These
were established by reading `origin/main` at 2c8b9c3 and are recorded so the
work is not lost; none has survived adversarial review:

1. **An import cycle forces a new package.** `internal/wsfresh` already imports
   `internal/query` (`internal/wsfresh/freshen.go:12` — it calls `query.Fresh`
   per member), so the union-graph layer cannot live in `internal/query`. A new
   `internal/wsquery` (importing `query`, `wsfresh`, `overlay`, `config`,
   `graph`, `engine`) is the natural home, with `DetectRootKind` routing and the
   `RootRepo` branch tail-calling `internal/query` verbatim so §4.2's
   byte-identical bar is structural rather than measured.
2. **The `dep_suppressions` filter needs no new `internal/graph` reader.**
   `graph.(*Store).TierOneEdges()` (`internal/graph/wsreaders.go:92`) already
   returns the exact five-part call-site tuple plus the resolved target's
   `DstNamespace`, which is precisely the key the same-call-site condition in
   `internal/wsresolve/wsresolve.go:19-29` is stated against.
3. **MCP result "schemas" are text.** Every `mcpserver` tool returns
   `TextContent` carrying `query.*Text()` byte-for-byte
   (`internal/mcpserver/mcpserver.go:37-232`), so "`repo` in result schemas"
   can only mean the rendered text plus the CLI `--json` shape — a structured
   MCP result would be a new surface, not wiring. Worth an owner ruling.
4. **`impact` is depth-1 today**, not a transitive closure
   (`internal/query/answers.go:320`), so D4's "crosses member boundaries by
   default" should mean the depth-1 neighbourhood includes cross-edges with no
   flag — not a new closure the single-repo verb lacks.
5. **Two decisions look like the ones most needing you.** (a) D2's "members
   freshen lazily (only those consulted)" appears unreachable without the
   incident-scoped re-resolution ADR-0012 rejects by name, which points at
   whole-workspace freshen on every query with latency as a D7-measured
   residual. (b) D5's "every verb's root accepts a workspace root" has no
   frozen answer for the non-query verbs (`build`, `refresh`, `export`,
   `search`, `serve`, …); erroring honestly rather than silently creating a
   stray `graph.db` at the workspace root was the draft's default, but it is a
   scope call.

**Recommendation: neither kill nor defer.** The change is well-formed, its
dependency is satisfied, and the campaign's engine work is merged and waiting on
exactly this wiring.

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->

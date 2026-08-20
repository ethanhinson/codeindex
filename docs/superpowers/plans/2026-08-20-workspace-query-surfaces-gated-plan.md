<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0016 — Workspace query surfaces — union-graph verbs, CLI/MCP wiring, workspace-status; merge gated on the D7 evidence run](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0016-workspace-query-surfaces-gated.md)**
<!-- docket:backlink:end -->

# Workspace query surfaces — implementation plan

Change 0016 `workspace-query-surfaces-gated` (type `feat`, priority `high`).
Spec: `docs/superpowers/specs/2026-08-20-workspace-query-surfaces-gated-design.md`
on `origin/docket` — **the spec is the authority; this plan is its task
breakdown.** Base: `origin/main` @ `2c8b9c3` (the spec's own baseline, unmoved
at reconcile time).

> **Plan-role degrade.** `superpowers:writing-plans` is not invocable on this
> machine, so the plan role fell back to `auto` (docket's missing-skill rule)
> and this file was authored directly by the implementer. Content obligations
> are unchanged.

This is the final and largest slice of the workspace-graph campaign: the union
query layer, CLI and MCP wiring, `workspace-status`, and the executable D7
freshness property. Every engine piece it consumes is already merged and
unwired.

---

## Ground rules for every task

- **Suite.** The honest suite is `go test -tags nollama -count=1 ./...`
  (pinned in `.docket.local.yml`). Plain `go test ./...` fails 10 packages on
  every ref for environmental reasons and is **not** the bar — never "fix"
  those. Run the suite in the **foreground**; never background it.
- **The non-regression bar is measured, not assumed.** Repo-mode text and JSON
  must be byte-identical after this branch. That is why Task 1 pins goldens for
  all nine renderers **before** any renderer is edited. Do not reorder Task 1.
- **`internal/query` is edited only for the additive struct fields and the
  conditional prefix / coverage line.** No workspace logic ever lands in it.
- **One root-kind detection site.** `engine.DetectRootKind` gains exactly one
  new non-test caller: `wsquery.RootKind`. Every root-kind branch in the binary
  — including the eight non-query verbs in `main.go` — reaches it through
  `wsquery.RootKind` / `wsquery.RefuseWorkspaceRoot`. Never call
  `engine.DetectRootKind` from `main.go` or `mcpserver`.
- **Exit codes.** `fatal()` (`cmd/codeindex/main.go:646-649`) is
  `os.Exit(1)` unconditionally. This slice adds **no** second exit mechanism.
  Exit 2 stays the pre-dispatch usage code. Assumption 14.
- **Frozen non-goals.** No transitive `impact` closure (§3.3). No new MCP tool,
  no MCP schema or description edit, and the plugin note is **untouched**
  (house rule). No cross-workspace semantic search. No re-ranking.
- **Line numbers in the spec drift by 1–5 lines** against the tree (reconcile
  log records the exact deltas: reference structs are at `DefRef:15`,
  `CallerRef:26`, `CalleeRef:37`, `DependentRef:50`, `FindRef:58`, `GrepRef:71`,
  `DepRef:80`, `EnclosingRef:89`; `ladder.go`'s `ProjectDefs` call is `:210`
  with the parent-less fallback at `:214-217`). **Trust the names, not the
  offsets.** Every identity in the spec is correct.

### Learnings that bear on this change (read before touching code)

- **`one-invariant-many-sites-drifts`** — this slice is the worst case the
  finding describes: one invariant enforced at nine renderers, two answer
  paths, and a package-doc hand-off. §7.5 makes the cross-site read an explicit
  review obligation. Also note its **recorded-obligation-is-a-site** rule:
  `wsresolve.go:19-29`'s prose is an enforcement site with no compiler behind
  it, and Task 6 discharges it.
- **`known-limitations-need-a-characterization-test`** — §3.6's
  suppression-with-no-cross-edge case ships as a **named characterization
  test**, not prose. `TestRepointedEdgeFallsThroughWhenOwnerLacksTheName`
  already proves the premise the naive reading would violate.
- **`capability-skips-assert-the-reason`** — the coverage clause is a
  degradation disclosure (ADR-0011). Tests must key on the **reason**, and any
  unrecognized reason must **fail**, never skip.
- **`ordering-claims-must-survive-the-error-return`** — §4.2's degrade path is
  exactly an error return. The `Dirty`-nonempty-with-`Resolved: false` window is
  **real** (`freshen.go`: `rep.Dirty` appended at `:255`, `Resolved` set at
  `:375`, error returns at `:292/:318/:373` between them). Any implementation
  that treats it as impossible is wrong.
- **`determinism-tests-need-a-total-sort-key`** — the union's ordering claim
  (§3.5) needs a fixture with an actual tie and an independently-computed
  expected sequence, not two reads of the same store.

---

## Task 1 — Text + JSON goldens for all nine renderers (§7.1)

**Profile:** standard. Test authoring across nine answer types with shared
fixtures; no product code.
**Runs first. No renderer may be edited until this is committed.**

**Files:** `internal/query/query_test.go` (and a new
`internal/query/golden_test.go` if the existing file gets unwieldy).

Today **five** answer types have text goldens: `TestCallersTextGolden:43`,
`TestCalleesTextGolden:113`, `TestEnclosingText:128`, `TestNavTextGolden:140`,
`TestGrepTextAndJSON:186`. Four do not: **Find, Dependents, Deps, Impact**.

1. Add text goldens for the four missing renderers, following the existing
   goldens' fixture style exactly.
2. Pin repo-mode `--json` for **all nine**. `omitempty` is the only thing that
   will keep the JSON keys stable once Tasks 2 adds `Repo` and `Inferred`, so
   an unpinned JSON shape is an unmeasured bar.
3. The Impact golden must cover the `(coverage: %s)` line (`answers.go:336`)
   and at least one `"  ... (+%d more)\n"` truncation, since §3.5 changes what
   the arithmetic counts in workspace mode and the repo-mode string must not
   move.
4. The Callers golden must cover `"  ... (+%d more; raise limit)\n"`
   (`answers.go:118-120`) and the `ambigTag` path (`answers.go:392`).

**Verification:** the honest suite is green, and the nine goldens fail if any
renderer byte changes. Prove the last clause by mutation — perturb one
renderer's format string locally, confirm the matching golden reddens, revert.
An unexercised golden is decoration.

---

## Task 2 — Additive reference fields + conditional renderers (§5, §3.7)

**Profile:** standard. Mechanical and pattern-following, but it edits all nine
shared renderers, which is why Task 1 exists.

**Files:** `internal/query/answers.go`.

1. Add `Repo string \`json:"repo,omitempty"\`` to **all eight** reference
   structs: `DefRef`, `CallerRef`, `CalleeRef`, `DependentRef`, `FindRef`,
   `GrepRef`, `DepRef`, `EnclosingRef`. `DepRef` is included deliberately —
   `deps` is a union verb (§3.1), so omitting it ships a cross-repo answer with
   no provenance on any row.
2. Add `Inferred bool \`json:"inferred,omitempty"\`` to **`CallerRef` and
   `CalleeRef` only** — the only two structs carrying any confidence signal
   today (`Ambiguous bool` at `:32` and `:44`). No other struct gains it, and
   **no answer type ever carries a `graph.Confidence`** (§3.7's correction).
3. In each of the nine `Text()` renderers, print `<member-id>: ` immediately
   before the path on a reference line **only when `Repo != ""`**. In repo mode
   `Repo` is always empty, so the branch never fires.
4. Emit an `[inferred]` tag when `Inferred` is true, on the same line and by
   the same mechanism as `ambigTag` (`answers.go:392`). The renderers are **not**
   taught the overlay's `exact`/`inferred` vocabulary — they key on the boolean.
5. Add the coverage-clause rendering hooks the later tasks fill:
   `ImpactAnswer.Coverage` stays a **`string`** and in workspace mode carries
   the existing sentence **plus** the appended clause (§4.5). Do **not** re-type
   it — that breaks repo-mode JSON, which is a non-regression bar (assumption 8).

**Verification:** all nine Task-1 goldens still pass **byte-identically**, text
and JSON. That is the whole point of the task ordering; if a golden moves, the
conditional branch is wrong, not the golden.

---

## Task 3 — `wsfresh.Report` gains a freshen-failed id slice (§4.3)

**Profile:** economy. Fully specified, additive, single-site.

**Files:** `internal/wsfresh/wsfresh.go`, `internal/wsfresh/freshen.go`.

`Report.MembersFreshenFailed` is an **`int`** (`wsfresh.go:63`) but §4.3's
`members_stale` union needs **ids** for set 4. Add an id slice **alongside** the
existing count (e.g. `MembersFreshenFailedIDs []string`).

- **Additive only.** Do **not** re-type or rename `MembersFreshenFailed`: live
  assertions (`freshen_test.go:501,519-520,523-524`) and this slice's own
  arithmetic depend on the existing denominators, and the package has an
  explicit one-name-one-denominator discipline (`wsfresh.go:38-40,71-80`).
  Assumption 11.
- **Write the count and the slice at the same site** so they cannot disagree.
- Add a test asserting `len(slice) == count` on a fixture with ≥2 failed
  members — a single-failure fixture cannot catch a divergence.

---

## Task 4 — `internal/wsquery` skeleton: routing, `RootKind`, `RefuseWorkspaceRoot` (§1, §2.2)

**Profile:** standard. The architectural spine; mechanical once the shape is
fixed, and the shape is fully specified.

**Files:** new `internal/wsquery/wsquery.go`, `internal/wsquery/wsquery_test.go`.

The package is **forced, not chosen**: `internal/wsfresh` imports
`internal/query` (`freshen.go:12`) and calls `query.Fresh` (`:218`), so the
union layer cannot live in `internal/query` without a cycle.
`wsquery` importing `engine + config + graph + overlay + wsfresh + query` is
acyclic.

1. **Mirror `internal/query`'s exported surface one-for-one:** `Callers`,
   `Callees`, `Impact`, `Nav`, `Find`, `Grep`, `Dependents`, `Deps`,
   `Enclosing`, `SearchText`, `Fresh`, plus each `*Text` twin **that exists
   today**. There is no `query.EnclosingText` — do **not** invent one.
   Identical signatures, identical return types (`*query.CallersAnswer`, …).
2. Every entry point begins with root-kind detection, and the `RootRepo` branch
   is a **verbatim tail-call**:

   ```go
   kind, err := RootKind(root)
   if err != nil { return nil, err }
   if kind == engine.RootRepo { return query.Callers(root, anchor, limit) }
   ```

   Same arguments, same return, **no wrapping, no re-formatting, no error
   decoration**. This is the mechanical basis of the non-regression bar.
3. Export the two helpers the eight non-query verbs route through:
   - `RootKind(root) (engine.RootKind, error)` — the single detection call, and
     the only new non-test caller of `engine.DetectRootKind`.
   - `RefuseWorkspaceRoot(verb, root) error` — the single per-repo refusal
     message, which loads the manifest to list member ids. Fixed shape:

     ```
     codeindex export: <root> is a workspace, not a repo. Run it per member, e.g.
       codeindex export <root>/services/api out.db
     members: api, shared, web
     ```
   - `search` is refused for a **different reason** and its message says so:
     cross-workspace semantic search is a **frozen non-goal** (vectors stay
     per-repo), not a mechanical limitation. `wsquery.SearchText`'s
     `RootWorkspace` branch returns that error, so **neither** call site
     (`main.go:171` CLI, `mcpserver.go:178` MCP handler) tests root kind itself.
4. For this task the `RootWorkspace` branch of the answer functions may return a
   clearly-named not-yet-wired error; Tasks 5–8 replace it. `RootKind`,
   `RefuseWorkspaceRoot` and every `RootRepo` tail-call are complete here.

**Tests:** repo-mode identity — for each entry point, assert the `wsquery`
result equals the `query` result on the same fixture repo (deep-equal on the
answer, byte-equal on `*Text`). Plus `RefuseWorkspaceRoot`'s message shape for
each of the six refusing verbs, with `search`'s distinct wording asserted
separately.

---

## Task 5 — Freshen, degrade posture, and the coverage clause (§4.1–§4.6)

**Profile:** premium. This is the D7 hard-fail zone: silent staleness is the one
outcome the gate refuses. Consequential but correctable.

**Files:** `internal/wsquery/` (freshness + coverage), tests alongside.

1. **Load the manifest explicitly, before `Freshen`.** `wsquery` calls
   `config.LoadWorkspace(wsRoot)` first. An absent, unparseable, or invalid
   manifest is returned as an **error** there — a configuration fault, not a
   staleness condition. This is mandatory, not an optimization: all ten of
   `Freshen`'s error returns are opaque `fmt.Errorf` wraps with no sentinel
   (`freshen.go:152,155,162,166,178,244,252,292,318,373`) and
   `engine.DetectRootKind` only *stats* the manifest
   (`internal/engine/rootkind.go`), so nothing downstream can tell a manifest
   fault from a member fold failure.
2. **Whole-workspace freshen on every `RootWorkspace` entry point:**
   `wsfresh.Freshen(wsRoot)` before reading anything, mirroring the per-repo
   `query.Fresh` contract. The argument is `Freshen`'s subset-free signature
   (`freshen.go:144`), the unknowable-without-folding cost, and the fan-out
   verbs' whole-manifest reach — **explicitly not** ADR-0012, which governs
   re-resolution scope only (assumption 4).
3. **A `Freshen` error degrades and discloses; it never fails the query.** The
   query proceeds against the overlay as it stands, the clause carries
   `freshen_failed: <error>`, and **every declared member is named stale** —
   not just those the partial `Report` lists. Reading the partial report as
   authoritative would reproduce the "quietly WRONG clean verdict" that
   `freshen.go:236-244` says the fold error is deliberately fatal to prevent.
4. **`members_stale` is the four-way union** of `Dirty`, `StaleStamped`,
   `MembersMissing`, and Task 3's freshen-failed ids — computed **after**
   `Freshen` returns, with `Dirty` dropped **only when `Resolved` is true**
   (the pass that just ran retired their staleness).
   **`Dirty` must stay in the union.** The `Dirty`-nonempty-with-`Resolved:
   false` window is real, and it is exactly the degrade path of item 3. Do not
   collapse this to a three-way union.
   `MembersUnindexed` is **not** in the union — it is covered by `StaleStamped`
   (previously contributed rows) or `boundary` (never did), so adding it
   double-counts.
5. **`members_consulted`** = member ids whose `graph.db` was read **to compose
   this answer, excluding the freshen pass**, in manifest order. The exclusion
   is load-bearing: §4.1's freshen opens every present member, so the
   unqualified phrasing makes the field constant and information-free. For an
   anchor verb: the anchor's own member plus every member appearing as a
   `SymKey.Member` on a unioned cross-edge. For a fan-out verb: every member the
   fan-out queried. A member consulted that contributed nothing is **still
   listed** — the clause answers "what did you look at".
6. **`boundary`** is the fixed sentence that symbols outside the workspace are
   unknown.
7. **Rendering (§4.5):** `ImpactAnswer.Coverage` in workspace mode = the
   existing sentence **plus** the appended clause, still a `string`. The other
   eight answer types carry the clause as a **trailing line** in `Text()` and an
   `omitempty` JSON sibling, added at the `wsquery` layer. Repo mode emits
   neither. A structured `workspace: {…}` object is added to the **JSON shape
   only**, `omitempty` in repo mode.
8. **§4.6's per-verb layer policy** is discharged by documenting, at the clause
   construction site, which layer each verb's clause describes: anchor verbs
   **graph**, `find`/`grep` **retrieval**, `enclosing` **graph, single-member**,
   `search` n/a. `nav` is graph-layer despite composing retrieval components;
   its find/grep components inherit the same clause rather than carrying a
   second.

**Tests:** per `capability-skips-assert-the-reason`, assert on the **reason**.
Inject a `Freshen` failure and assert every declared member is named. Inject a
malformed manifest and assert it is an **error**, not a degrade. Construct a
report with `Dirty` non-empty and `Resolved: false` and assert those ids appear
in `members_stale` — this test is what stops a future reader collapsing the
union.

---

## Task 6 — Union primitives: stable-key re-map, confidence, suppression filter (§3.6–§3.8)

**Profile:** premium. Named, correctable risk: the wrong reader here re-admits a
vendored snapshot as a cross-repo target, and the wrong filter deletes a
still-correct edge.

**Files:** `internal/wsquery/`, tests alongside.

1. **Stable-key re-map** — cross-edges store `SymKey{Member, File, QName}` and
   member rebuilds renumber symbol ids, so keys are inverted at query time via
   the member's own DB.
   - Use **`(*Store).ProjectDefs(name, parent)`** (`graph/wsreaders.go:127`),
     **never `Definitions`** (`store.go:656`). `ProjectDefs`' own doc says why:
     `Definitions` neither filters tier nor returns Tier/Namespace, so it
     re-admits a vendored tier-1 snapshot — the exact failure member-over-dep
     precedence exists to prevent. It is also the correct inverse: the keys were
     *written* by `ProjectDefs` (`ladder.go:210`).
   - **Split `QName` on the LAST `.`** — `QName()` is `Parent + "." + Name` and
     dotted parents are ordinary in TS and Python. A first-dot split silently
     mis-parents every nested symbol in two of the four supported languages.
   - **All three cardinalities are defined** (`ProjectDefs` treats `parent == ""`
     as *no parent restriction*, and the write side has a parent-less fallback
     at `ladder.go:214-217`, so a dotless QName can return many):
     - **zero** → drop the reference from the answer **and count it**, surfaced
       as a fourth clause field `keys_unmapped: <n>` (JSON sibling + trailing
       text line). Additive to D6's reserved three, never a substitute.
     - **one** → use it.
     - **many** → filter to symbols whose `Parent` equals the QName's parent
       **exactly** (for a dotless QName, `Parent == ""`). If exactly one
       survives, use it. If several still survive, render the reference as
       **ambiguous** with its candidates (`Ambiguous: true`, the existing
       multi-candidate shape) — **never** pick the first row. Assumption 19.
2. **Confidence reconcile — one exported function**, called by every
   reference-construction site (the alternative is the same two words translated
   at nine renderers):

   | overlay record | `Ambiguous` | `Inferred` |
   |---|---|---|
   | cross-edge, `confidence == "exact"` (rung 1) | false | false |
   | cross-edge, `confidence == "inferred"` (rung 2) | false | **true** |
   | ambiguity record (not an edge) | **true** | false |

   Keeping `inferred` distinguishable is not cosmetic: D3's epistemics rule is
   that a bare-name (rung-2) answer must never be presented as an
   import-mediated one.
3. **The `dep_suppressions` filter — implemented exactly as
   `wsresolve.go:19-29` records it, with no widening:**

   > filter out an intra-repo edge whose resolved target is a tier-1 symbol in a
   > suppressed namespace **ONLY WHEN** the overlay carries a cross-edge from
   > the same call site — same source key (`src_file`, `src_name`,
   > `src_parent`), same kind, same line.

   - **No new `internal/graph` reader is needed.** `(*Store).TierOneEdges()`
     (`wsreaders.go:92`) returns the five-part call-site tuple plus
     `DstNamespace`, and it lines up with the overlay's `src_*` columns because
     the ladder builds the key the same way (`ladder.go:104-107`) and
     `insertCrossEdge` writes it verbatim.
   - Implementation: read `overlay.Suppressions()`, index the consumer member's
     `TierOneEdges()` by `(src_file, src_qname, kind, line)`, drop an edge only
     when its `DstNamespace` is suppressed **and** a cross-edge with the same
     key exists.
   - **The condition is a cross-edge at the same call site, never the
     suppression record alone.** A suppression can exist with **no** cross-edge
     behind it (the re-pointed edge fell through rungs 2–4 —
     `TestRepointedEdgeFallsThroughWhenOwnerLacksTheName` proves it on the
     merged tree). Filtering there deletes a still-correct tier-1 edge and puts
     nothing in its place.
   - **Ship a named characterization test** for that case — one suppression,
     zero cross-edges, the intra-repo edge **survives** — named so it cannot be
     read as an aspiration (per `known-limitations-need-a-characterization-test`).

**Tests:** all three re-map cardinalities including the ambiguous-many case; the
last-dot split against a dotted-parent TS/Python fixture; the confidence table
row by row; the suppression filter's positive case **and** the characterization
test above.

---

## Task 7 — Anchor union verbs + anchor prefixes (§3.1, §3.3–§3.5)

**Profile:** premium. Consequential and correctable; the `limit`/`Total`
arithmetic and the `impact`/`dependents` coherence are both silent-wrongness
shapes.

**Files:** `internal/wsquery/`, tests alongside.

1. **The union:** `callers(X)` = member M's own callers ∪
   `overlay.InEdges(dstKey)` resolved through each source member's DB.
   `callees`/`deps` symmetric via `overlay.OutEdges`. `nav` unions the same way
   over its callers component; its name-search and grep components fan out per
   Task 8.
2. **`dependents` is a union verb — forced.** `ImpactAnswer` embeds
   `DependentsTotal`/`Dependents`, and `impact` crosses member boundaries by
   ruling. Per-repo `dependents` would make `codeindex dependents` and the
   dependents block of `codeindex impact` report different numbers for the same
   anchor from the same root. **`deps` is unioned too** — discretionary, for
   directional symmetry (assumption 1a).
3. **`impact` stays depth-1.** `query.Impact` composes definitions + callers +
   callees + dependents at depth 1. D4's "crosses member boundaries by default"
   means the depth-1 neighbourhood **includes cross-edges with no flag**. It
   does **not** authorize a transitive closure the single-repo path lacks —
   building one blows the non-regression bar for a capability nobody asked for.
4. **`limit` bounds the unioned list; `*Total` counts the unioned set** — own ∪
   cross, **after** Task 6's suppression filter. Applying the limit per member
   and then concatenating makes `CallersTotal - len(Callers)` a lie in both
   directions and is **forbidden**.
   Order within the unioned list: own-member references first in their existing
   order, then cross references in manifest order of the source member. It is
   **not** claimed that a truncated workspace head equals the single-repo head —
   §3.6's filter legitimately removes own-member edges.
5. **Anchor prefixes (§3.4).** An anchor may carry an optional `<member-id>:`
   prefix (`api:HandleLogin`), stripped and used to scope definition lookup; an
   unknown id is an error listing the known ids. A **bare** anchor matching in
   several members returns the same multi-candidate disambiguation answer the
   single-repo path already returns — **no new answer shape**.
   The parse rule must not collide with `query.SplitAnchor`
   (`query.go:31-39`), which parses **`::` first** (`strings.Index`, `:32`) then
   the last `.` (`strings.LastIndex`, `:35`):

   > Strip a leading `<id>:` **only when** the text before the first `:` is a
   > declared member id **and** the character immediately after that `:` is not
   > itself `:`. Otherwise pass the anchor to `SplitAnchor` untouched.

   The second clause is load-bearing: without it `api::HandleLogin` with a
   member `api` becomes `:HandleLogin`, which **neither** `SplitAnchor` branch
   parses — a silently wrong lookup in exactly the PHP/TS anchors the bench
   corpus is full of. `api:Type::method` and `api:Type.method` must both work.

**Tests:** the `dependents`-vs-`impact`-block coherence asserted directly (same
anchor, same root, same numbers) — per `one-invariant-many-sites-drifts`, that
is a two-site check, not two one-site checks. `limit` truncation arithmetic over
a union where own and cross both contribute. Every anchor-prefix case above,
including `api::HandleLogin` and an unknown id. Ordering pinned against an
**independently-computed** expected sequence with a deliberate tie in the
fixture (per `determinism-tests-need-a-total-sort-key`).

---

## Task 8 — Fan-out verbs, `enclosing`, and `search` refusal (§3.2, §2.3)

**Profile:** standard. Normal feature work over a settled shape.

**Files:** `internal/wsquery/`, tests alongside.

1. **`find`/`grep` (and `nav`'s search components)** run the existing per-repo
   query against each present member in manifest order and **concatenate** —
   complete sets, within-member order preserved, **no rank-merge, no scoring**
   (D4 froze it; re-ranking is a frozen non-goal).
2. **Aggregates are defined explicitly**, because concatenation does not define
   them:
   - `FindAnswer.Total` = the **sum** of per-member totals.
   - `GrepAnswer.RawHits` = the **sum** of per-member raw-hit counts.
   - `GrepAnswer.Backend`: when every consulted member reports the same backend,
     that value verbatim. When they differ, the distinct backends joined by `+`
     in manifest order (`ripgrep+sqlite`). **Never blank, never silently the
     first member's** — the `(%s)` in `GrepAnswer.Text()` would become a quiet
     lie about how the answer was obtained.
   - `limit` bounds the **concatenated** list, same reasoning as §3.5.
3. **`enclosing` (§2.3)** is intra-file. On a workspace root the file argument
   is workspace-relative; resolve it to absolute and select the member by
   **longest-prefix match over resolved absolute member roots** —
   `filepath.Abs(filepath.Join(wsRoot, m.Root))`, exactly `Workspace.Resolve`'s
   `AbsRoot`. The absolute form is **mandatory**: D1 sanctions member roots that
   climb out of the workspace (`../api`), so matching declared relative strings
   would fail or match the wrong member. A file matching no member root is an
   error naming the file and listing the member roots. The answer is the
   per-repo `query.Enclosing` on that member with paths re-written
   workspace-relative and `Repo` set.
4. **`search` refuses** on a workspace root from `wsquery.SearchText`'s
   `RootWorkspace` branch (message authored in Task 4). Both call sites inherit
   it; neither tests root kind.
5. **Paths** in workspace mode are workspace-relative
   (`services/api/handlers/u.go`); in repo mode exactly what they are today.

**Tests:** aggregate sums; the `Backend` agreement and disagreement cases (the
disagreement case needs a two-backend fixture — a same-backend fixture cannot
catch a first-member fallback); `enclosing` routing including a `../`-style
member root; a file matching no member.

---

## Task 9 — CLI wiring (§2.1, §2.2)

**Profile:** standard. Integration work across a large dispatch switch.

**Files:** `cmd/codeindex/main.go`.

1. **Query verbs:** substitute `wsquery` for `query` at the existing call sites.
   **No other edit to the dispatch shape.**
2. **`build` and `status` fan out** across `Workspace.Resolve(wsRoot)`'s
   `present` slice in **manifest order**, running the existing per-repo path per
   member and printing a per-member-prefixed line. **Missing members are
   reported by id, not skipped silently.**
3. **`refresh` is the exception and iterates nothing:** it is
   `wsfresh.Freshen(wsRoot)`. `Freshen` already *is* the per-member freshen plus
   the overlay gate; reimplementing the loop would be a second enforcement site
   for ADR-0012's whole-pass rule.
4. **A member whose per-repo call fails does not abort the fan-out.** Report the
   failure against that member's id, continue the loop, and at the end return an
   **aggregate error naming every failed member**, which the existing `fatal`
   prints and exits **1** on. Aborting mid-fan-out leaves the workspace
   half-built with no record of which members made it (assumption 14).
5. **`export`/`import`/`ingest`/`depmap`/`serve`** call
   `wsquery.RefuseWorkspaceRoot(verb, root)` and `fatal` on it. `search` refuses
   through `wsquery.SearchText` (Task 4/8), not through a root-kind test here.
6. **`status <workspace-root>`** prints the per-member fan-out **and then** the
   `workspace-status` block (Task 11), so one command answers both halves.
7. **`init-workspace` is unchanged** (already workspace-only). **`bench` and
   `model` are untouched** — their second arg is not a repo root.
8. **Document the anchor prefix** in the CLI `--help`/usage line — this is the
   only surface that documents it (assumption 18).

**Tests:** the fan-out's continue-on-failure behaviour with an aggregate error
naming **every** failed member (a single-failure fixture cannot prove
"every"); each refusing verb's message and exit code.

---

## Task 10 — MCP wiring (§5)

**Profile:** standard. Small diff, but it sits on a byte-identity bar.

**Files:** `internal/mcpserver/mcpserver.go`.

- Handlers call `wsquery.*Text` instead of `query.*Text`. That is the entire
  functional change.
- **The eight tools are unchanged in name, schema, and description.** No new
  tool. No `repo` JSON field on a tool schema — these tools return
  `TextContent` and have no structured result to add one to; the member id
  arrives **inside the text they already return**. That is what "discharges the
  frozen `repo` in result schemas via the text surface the tools actually have"
  means (owner ruling 2).
- **`explore-feature` is untouched.** It is an MCP **prompt**, not a tool and
  not a verb: it reads no root and returns a static workflow string, so it
  cannot error on a workspace root. Its text is unchanged; it names `search`,
  which refuses and explains itself (assumption 16).
- **The plugin note is untouched** (house rule).
- The anchor prefix rides the existing free-text `symbol` argument — functional
  but **deliberately unadvertised** on MCP, the recorded cost of ruling 2's
  surface freeze (assumption 18). Do not "helpfully" document it here.

**Tests:** single-repo MCP text output is **byte-identical**; workspace-mode
output carries the `<member-id>: ` prefix.

---

## Task 11 — `workspace-status` verb (§6)

**Profile:** standard. A new verb over already-merged readers.

**Files:** `cmd/codeindex/main.go`, plus whatever reader helper `wsquery` needs.

`codeindex workspace-status <workspace-root> [--json]`. It **errors on a repo
root**, naming the repo-mode `status` verb. Report contents:

- **Per member:** id, resolved absolute root, present/missing, indexed/
  unindexed, and **stamp presence plus the stamped merkle root as recorded** —
  explicitly **not** a "currently dirty" verdict. Dirtiness needs a fresh merkle
  re-fold against the stamp; `wsfresh.foldMember` is unexported and no exported
  fold-and-compare exists, so reporting it would mean either doing the expensive
  fold half (falsifying "reads state, does not freshen") or writing a second
  implementation of the dirty predicate — the same second-enforcement-site
  objection §4.1 uses to reject subset freshen. The honest cheap field is the
  recorded stamp; `codeindex refresh` is what turns it into a verdict
  (assumption 12).
- The overlay's **schema version**, cross-edge and ambiguity counts.
- **Member/vendor version skew** from `overlay.Suppressions()`: one line per
  `Suppression{ConsumerMember, Namespace, OwnerMember, SuppressedVersion}` —
  "`drupal` vendors `symfony/http-foundation` at v7.1.0; member `symfony` wins"
  — with `SuppressedVersion == ""` rendered as **"version unknown"**, never
  omitted. This is the D3 reporting obligation the suppression record was
  written for.

**It reads state; it does not freshen.** That keeps it usable as a diagnostic on
a workspace whose freshen is the thing being diagnosed.

**Tests:** the empty-`SuppressedVersion` rendering (a fixture without one cannot
catch a silent omission); the repo-root refusal; `--json` shape.

---

## Task 12 — Workspace goldens, the D7 freshness property, and the single-member bar (§7.2–§7.4)

**Profile:** premium. This is the evidence layer the merge gate reads; silent
staleness is a hard fail.

**Files:** `internal/wsquery/` tests.

1. **Workspace goldens (§7.2)** for each union verb over a small fixture
   workspace — **two members, one cross-edge, one ambiguity, one suppression** —
   covering: the `repo:` prefix, workspace-relative paths, manifest ordering,
   `limit` truncation arithmetic over the union, the coverage clause, and both
   the anchor-prefix and bare-anchor-ambiguity paths.
2. **The D7 freshness property (§7.3), executable:** mutate one member, query
   from another, **no explicit rebuild** → the answer reflects the mutation
   **or** its coverage clause names that member stale. **Silent staleness is a
   hard fail.** Write it as a **property over the union verbs**, not a single
   scripted scenario, so it also covers §4.2's `Freshen`-failed path (inject a
   freshen failure → assert the clause names the member).
3. **The single-member bar (§7.4), held verbatim:** a one-member workspace's
   answers match the single-repo answers **modulo the `repo` field**. The
   earlier draft's "and workspace-relative paths" was a real relaxation of a
   frozen bar and is **withdrawn**. Build the fixture so the bar is literally
   satisfiable: the sole member's `root` is **`.`**, so workspace-relative and
   repo-relative paths coincide and the only difference left is `repo`. A
   fixture whose sole member sits in a subdirectory does **not** meet the frozen
   bar and is not the fixture the bar runs on. This discharges D7's
   non-regression bar **and** §3.5's deferred unit test.

---

## Task 13 — Campaign bookkeeping and the cross-site coherence sweep (§7.5)

**Profile:** economy for the ticks; the sweep is a **review** obligation carried
into Step 6, not a code task.

**Files:** `openspec/changes/workspace-graph/tasks.md`.

Tick **§3.4** (its `workspace-status` half lands here; the freshen half merged
in 0014), **§3.5**, and **§4.1–§4.4**. Leaving §3.5 unticked misreports the
campaign — its single-member-workspace bar is exactly what Task 12 discharges.
Do **not** tick §5.x: the gate is owner-attended and outside this build.

**The §7.5 cross-site read** (carried into review, per
`one-invariant-many-sites-drifts` — read these pairs **against each other**, not
each against the spec):

- `dependents` vs `impact`'s dependents block (§3.1);
- the `RootRepo` tail-call vs the nine renderers' conditional branches (§1, §5);
- `refresh`-on-workspace vs `wsfresh.Freshen`'s own gate (§2.1);
- the suppression filter's condition vs `wsresolve.go:19-29`'s prose (§3.6);
- the `members_stale` union vs each `Report` field's doc comment (§4.3).

---

## Out of scope (do not drift into these)

- Corpus growth (change 0010) — the gate runs the frozen 65-task corpus.
- Cross-workspace semantic search / vectors, UI, git-remote identity, language
  expansion, re-ranking — frozen non-goals.
- Scoped incident re-resolution — ADR-0012 stands.
- A transitive `impact` closure (§3.3).
- Any new MCP tool, and any edit to the plugin note.
- **The §5 / §9 D7 evidence gate itself.** It is owner-attended: the bench
  harness and corpus live in local-main-only unpushed commits, so the run
  happens from the local main tree against this branch's binary. This build
  stops at the open PR.

<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0016 — Workspace query surfaces — union-graph verbs, CLI/MCP wiring, workspace-status; merge gated on the D7 evidence run](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0016-workspace-query-surfaces-gated.md)**
<!-- docket:backlink:end -->

# Design: workspace query surfaces — union-graph verbs, CLI/MCP wiring, workspace-status

Change: 0016 · frozen design: `openspec/changes/workspace-graph/design.md` (D4, D5, D6, D7)
Baseline: `origin/main` at 2c8b9c3 · tasks: §3.4 (`workspace-status` half), §3.5, §4.1–§4.4, hosts §5

This is the wiring slice. Every engine piece it consumes is merged and unwired:
`config.LoadWorkspace`/`Workspace.Resolve`, `engine.DetectRootKind`,
`internal/overlay` (registry, cross-edges, ambiguities, suppressions, stamps),
`internal/wsresolve.Resolve`, `internal/wsfresh.Freshen`. No verb reads any of
it yet. This design says exactly how each verb starts reading it, and what it
must not change while doing so.

## 1. Package shape — `internal/wsquery`

**A new package is forced, not chosen.** `internal/wsfresh` imports
`internal/query` (`internal/wsfresh/freshen.go:12`) and calls `query.Fresh`
(`freshen.go:218`). Any union layer must call *both* `wsfresh.Freshen` and the
per-repo `query.*` answers, so it cannot live inside `internal/query` without a
cycle. `internal/query` already imports `internal/engine`, and `internal/engine`
imports neither, so `internal/wsquery` importing
`engine` + `config` + `graph` + `overlay` + `wsfresh` + `query` is acyclic.

`internal/wsquery` is the **only** new package. Its exported surface mirrors
`internal/query`'s one-for-one — `Callers`, `Callees`, `Impact`, `Nav`, `Find`,
`Grep`, `Dependents`, `Deps`, `Enclosing`, `SearchText`, `Fresh`, plus each
`*Text` twin **that exists today** (there is no `query.EnclosingText`, and
`wsquery` does not invent one) — with identical signatures and identical return
types (`*query.CallersAnswer`, …). Every entry point begins:

```
kind, err := engine.DetectRootKind(root)
if err != nil { return nil, err }
if kind == engine.RootRepo { return query.Callers(root, anchor, limit) }  // verbatim tail-call
```

The `RootRepo` branch is a **verbatim tail-call into `internal/query`** — same
arguments, same return, no wrapping, no re-formatting, no error decoration. That
is the mechanical basis of the non-regression bar, and it is the reason the
routing lives in one package rather than being sprinkled through the CLI and MCP
handlers: two root-kind branches would be two enforcement sites for one rule.

CLI (`cmd/codeindex/main.go`) and MCP (`internal/mcpserver/mcpserver.go`) change
for the **query verbs** by substituting `wsquery` for `query` at their existing
call sites. No other edit to their dispatch shape.

**Where the eight non-query verbs detect root kind.** `build`/`refresh`/`status`
(fan out) and `export`/`import`/`ingest`/`depmap`/`serve` (refuse) also branch on
root kind, and they are not part of `internal/query`'s surface, so they are not
`wsquery` answer functions. They branch in `cmd/codeindex/main.go`, but they do
**not** call `engine.DetectRootKind` themselves: `wsquery` exports two small
helpers that every one of them calls —

- `wsquery.RootKind(root) (engine.RootKind, error)` — the single detection call,
- `wsquery.RefuseWorkspaceRoot(verb, root) error` — the single per-repo refusal
  message (§2.2), which loads the manifest to list member ids.

So the invariant is: **`engine.DetectRootKind` has exactly one non-test caller
outside `wsfresh`/`wsresolve`, namely `wsquery.RootKind`, and every root-kind
branch in the binary reaches it through `wsquery`.** That is the one-site
property §1 needs; the earlier phrasing ("wsquery is the sole call site") was
false as soon as §2's eight verbs are counted.

`internal/query` itself is edited only where §5 requires (a `repo` field on the
reference structs and a workspace clause on `ImpactAnswer`), never with
workspace logic.

## 2. Root-kind behaviour, verb by verb

`engine.DetectRootKind` returns `RootWorkspace` iff `.codeindex/workspace.json`
is stat-able; a root with neither manifest nor indexable source is already an
error naming both possibilities. Nothing about that changes.

| verb | workspace root | rationale |
|---|---|---|
| `build`, `status` | **fan out per member, manifest order** | owner ruling 1 |
| `refresh` | `wsfresh.Freshen(wsRoot)` — the merged whole-workspace pass, which *is* the per-member fan-out plus the overlay gate (§2.1) | owner ruling 1 |
| `export`, `import`, `ingest`, `depmap`, `serve` | **error, per-repo message** | owner ruling 1 |
| `callers`, `callees`, `impact`, `nav`, `dependents`, `deps` | union graph | D4 |
| `find`, `grep` | fan out, concatenate complete sets, manifest order | D4 |
| `enclosing` | route to the owning member | see §2.3 |
| `search` | **error, per-repo message** | frozen non-goal |
| `init-workspace` | unchanged (already workspace-only) | §3.1 |
| `mcp` | serves the union graph | D5, owner ruling 2 |
| `bench`, `model` | untouched — their second arg is not a repo root | — |

### 2.1 Maintenance fan-out

`build` and `status` on a workspace root iterate
`Workspace.Resolve(wsRoot)`'s `present` slice in manifest order, run the
existing per-repo path per member, and print a per-member-prefixed line. Missing
members are reported by id, not skipped silently.

**`refresh` is the exception and does not iterate anything here:** it is
`wsfresh.Freshen(wsRoot)`, not a per-member loop — `Freshen` already *is* the
per-member freshen plus the overlay gate, and reimplementing the loop would be a
second enforcement site for ADR-0012's whole-pass rule. `status` runs
the per-member `runStatus` loop **and then** the new `workspace-status` block
(§6), so one command answers both halves.

A member whose per-repo call fails does not abort the fan-out: the failure is
reported against that member's id and the loop continues; at the end the verb
returns an aggregate error naming every failed member, which `main.go`'s
existing `fatal` (`cmd/codeindex/main.go:646-649`) prints and exits **1** on.
Aborting mid-fan-out would leave the workspace half-built with no record of
which members made it.

**Exit codes.** `fatal` is `os.Exit(1)` unconditionally and this slice does
**not** add a second exit mechanism — every workspace-root refusal and every
fan-out failure exits **1** through `fatal`. (Exit 2 in the binary today is
reserved for usage errors raised before dispatch, `main.go:42-43`; a workspace
root is a well-formed argument, so it does not belong there.)

### 2.2 The refusing verbs

`export`/`import`/`ingest`/`depmap`/`serve`/`search` on a workspace root exit
1 (via `fatal`) with a message of fixed shape, produced by the single
`wsquery.RefuseWorkspaceRoot` helper:

```
codeindex export: <root> is a workspace, not a repo. Run it per member, e.g.
  codeindex export <root>/services/api out.db
members: api, shared, web
```

`search` is refused for a different reason and says so: cross-workspace semantic
search is a **frozen non-goal** (design.md:31–33, vectors stay per-repo), so its
message names that rather than implying a mechanical limitation.

`search` is refused at **two** sites — `cmd/codeindex/main.go:175` and the MCP
`search` handler at `internal/mcpserver/mcpserver.go:178`. Both route through
`wsquery.SearchText`, whose `RootWorkspace` branch returns the error; neither
call site tests the root kind itself. `explore-feature`
(`mcpserver.go:186`) is an MCP **prompt**, not a tool and not a verb: it reads
no root, returns a static workflow string, and is therefore untouched — it
cannot "error on a workspace root" because it never sees one. Its text is
unchanged; it names `search`, which will refuse and explain itself.

### 2.3 `enclosing`

`enclosing` takes a file plus a line range and is purely intra-file. On a
workspace root the file argument is workspace-relative; `wsquery.Enclosing`
resolves it to an absolute path and selects the member by **longest-prefix match
over resolved absolute member roots** — `filepath.Abs` of
`filepath.Join(wsRoot, m.Root)`, exactly `Workspace.Resolve`'s `AbsRoot`. The
absolute form is mandatory, not cosmetic: D1 explicitly sanctions member roots
that climb out of the workspace (`../api`), so a prefix match on the *declared*
relative strings would fail to match, or match the wrong member, for the
dedicated-workspace-directory shape that D1 calls the sanctioned layout for
scattered repos. A file matching no member root is an error naming the file and
listing the member roots. The answer is then the per-repo `query.Enclosing` on
that member, with paths re-written workspace-relative and `repo` set.

## 3. Union semantics

### 3.1 The anchor verbs

`callers(X)` = member M's own callers ∪ `overlay.InEdges(dstKey)` resolved
through each source member's DB. `callees`/`deps` symmetric via
`overlay.OutEdges`. `nav` unions the same way over its callers component; its
name-search and grep components fan out per §3.2.

`dependents` is a union verb even though D4 names only
callers/callees/impact/nav, and this one is **forced**: `ImpactAnswer` embeds
`DependentsTotal`/`Dependents` (`internal/query/answers.go:326-327`), and
`impact` crosses member boundaries by ruling. If `dependents` stayed per-repo,
`codeindex dependents` and the dependents block of `codeindex impact` would
report different numbers for the same anchor from the same root — one invariant,
two sites, guaranteed drift.

`deps` is a **discretionary** call, not a forced one: `ImpactAnswer` contains no
`Deps` block, so no two-sites-disagree argument reaches it. It is unioned
anyway, because it is `dependents`' exact downward twin (same edge table, read
in the other direction) and shipping one direction cross-repo while the other
stops at the member boundary is the kind of asymmetry a user reads as a bug.
Recorded as its own assumption so the weaker footing is visible.

### 3.2 Fan-out verbs

`find`/`grep` (and `nav`'s search components) run the existing per-repo query
against each present member in manifest order and **concatenate** — complete
sets, within-member order preserved, no rank-merge and no scoring. This is not a
simplification; D4 froze it, and the non-goals freeze re-ranking generally.

Their **aggregate fields** are defined explicitly, because concatenation does
not define them:

- `FindAnswer.Total` (`answers.go:157-161`) = the **sum** of the per-member
  totals.
- `GrepAnswer.RawHits` (`answers.go:187-193`) = the **sum** of the per-member
  raw-hit counts.
- `GrepAnswer.Backend` is a single per-store string with no natural union. When
  every consulted member reports the same backend, that value is used verbatim
  (the overwhelmingly common case, and the one the single-member bar of §7.4
  runs through). When they differ, the value is the distinct backends joined by
  `+` in manifest order (`ripgrep+sqlite`). It is never blank and never silently
  the first member's — a blank or arbitrary backend makes the `(%s)` in
  `GrepAnswer.Text()` (`answers.go:197-198`) a quiet lie about how the answer
  was obtained.

`limit` for these two bounds the concatenated list, on the same reasoning as
§3.5.

### 3.3 `impact` is depth-1

`query.Impact` composes definitions + callers + callees + dependents at
**depth 1** (`internal/query/query.go:523-558`) — it is not a
transitive closure today. D4's "crosses member boundaries by default" therefore
means: the depth-1 neighbourhood **includes cross-edges, with no flag**. It does
**not** authorize adding a transitive closure that the single-repo path lacks.
Building one here would silently change the single-repo answer shape and blow
the non-regression bar for a capability nobody asked for.

### 3.4 Anchor prefixes and ambiguity

An anchor may carry an optional `<member-id>:` prefix (`api:HandleLogin`). The
prefix is stripped and used to scope definition lookup to that member; an
unknown id is an error listing the known ids. A **bare** anchor matching in
several members returns the same multi-candidate disambiguation answer the
single-repo path already returns for duplicate names — no new answer shape.

Prefix parsing must not collide with `query.SplitAnchor`
(`internal/query/query.go:31-38`), which parses **`::` first** and only then the
last `.`. The member prefix rule is therefore narrower than "split on the first
`:`":

> Strip a leading `<id>:` **only when** the text before the first `:` is a
> declared member id **and** the character immediately after that `:` is not
> itself `:`. Otherwise the anchor is passed to `SplitAnchor` untouched.

The second clause is the load-bearing one: without it, `api::HandleLogin` with a
member `api` would be shortened to `:HandleLogin`, which neither `SplitAnchor`
branch parses (`Index("::")` misses, `LastIndex(".")` misses) — a silently
wrong lookup in exactly the PHP/TS anchors the bench corpus is full of.
`api:Type::method` and `api:Type.method` both work, because the prefix is
stripped before `SplitAnchor` sees the remainder.

**MCP anchor prefixes ride the existing argument.** D5 also says tool schemas
gain "the optional anchor prefix." No schema change is needed or made: the
`symbol` argument of the anchor tools is already a free-text string, so
`api:HandleLogin` is accepted today by the same field. Tool **descriptions** are
not edited either (owner ruling 2 freezes the tool surface), which means the
prefix is functional but undocumented on the MCP side. That is the deliberate
cost of the ruling and is recorded as an assumption rather than quietly
discharged — the CLI's `--help`/usage line does document it.

### 3.5 `limit` semantics under the union

Every anchor verb's `limit` today bounds the returned list while `*Total` fields
carry the full count, and `Text()` prints `Total - len(list)` — literally
`"  ... (+%d more; raise limit)\n"` for callers (`answers.go:118-120`) and
`"  ... (+%d more)\n"` in the impact blocks (`answers.go:336-354`); the exact
strings matter because §7.1 pins them as goldens. Under the union, **`limit`
bounds the unioned list and `*Total` counts the unioned set** — own ∪ cross,
after the §3.6 filter. Applying the limit per member and then concatenating
would make `CallersTotal - len(Callers)` a lie in both directions and is
forbidden.

Order within the unioned list: own-member references first in their existing
order, then cross references in manifest order of the source member. That is
deterministic, which is the whole claim. It is explicitly **not** claimed that
the head of a truncated workspace list equals the single-repo list's head —
§3.6's suppression filter *removes* own-member intra-repo edges wherever a
same-call-site cross-edge exists, so the two heads can legitimately differ.

### 3.6 The `dep_suppressions` filter — recorded obligation, discharged

`internal/wsresolve`'s package doc (`wsresolve.go:19-29`) records the obligation
verbatim. Its narrowing is load-bearing and is implemented **exactly** as
written, with no widening:

> filter out an intra-repo edge whose resolved target is a tier-1 symbol in a
> suppressed namespace **ONLY WHEN** the overlay carries a cross-edge from the
> same call site — same source key (`src_file`, `src_name`, `src_parent`), same
> kind, same line.

A suppression record can exist with **no** cross-edge behind it (the re-pointed
edge fell through rungs 2–4 because the owner lacks the name — see
`TestRepointedEdgeFallsThroughWhenOwnerLacksTheName`). Filtering there would
delete a still-correct tier-1 edge and put nothing in its place. The condition
is a cross-edge at the same call site, never the suppression record alone.

**No new `internal/graph` reader is needed.** `(*Store).TierOneEdges()`
(`internal/graph/wsreaders.go:92`) returns `TierOneEdge{UnresolvedEdge,
DstNamespace}` — precisely the five-part call-site tuple plus the resolved
namespace — and it lines up with the overlay's `src_*` columns because the
ladder builds the cross-edge source key the same way
(`internal/wsresolve/ladder.go:104-108`) and that key is written verbatim by
`insertCrossEdge` (`internal/overlay/edges.go:53-67`). Note the carried record
cites `edges.go:408-422` here; those lines are `OutEdges`/`InEdges`, the key
*readers*, not the writer — corrected so the builder lands on the right
function. The
filter is: read `overlay.Suppressions()`, index the consumer member's
`TierOneEdges()` by `(src_file, src_qname, kind, line)`, and drop an edge only
when its `DstNamespace` is suppressed **and** a cross-edge with the same key
exists.

A **characterization test** pins the no-cross-edge case: one suppression, zero
cross-edges, the intra-repo edge survives. Named so it cannot be read as an
aspiration, per `known-limitations-need-a-characterization-test`.

### 3.7 Confidence vocabulary reconcile

`overlay.CrossEdge.Confidence` is `"exact" | "inferred"` — the frozen workspace
vocabulary, deliberately not `graph.Confidence`, whose values are
`"unambiguous" | "ambiguous" | "unresolved"` (`internal/graph/types.go:57-64`).
The comment at `overlay/edges.go:23-26` names this slice as the reconcile site.

**Correction to the obvious framing:** the answer surface does not speak
`graph.Confidence` at all, and no answer type carries one. The *only* confidence
signal any reference struct has today is `Ambiguous bool`, on exactly two of
them — `CallerRef` (`answers.go:32`) and `CalleeRef` (`answers.go:44`).
`DefRef`, `DependentRef`, `DepRef`, `FindRef`, `GrepRef`, and `EnclosingRef`
carry none. So the reconcile is **not** overlay-vocabulary → `graph.Confidence`;
it is overlay-vocabulary → the surface's existing boolean plus one new one:

| overlay record | `Ambiguous` | `Inferred` |
|---|---|---|
| cross-edge, `confidence == "exact"` (rung 1) | false | false |
| cross-edge, `confidence == "inferred"` (rung 2) | false | **true** |
| ambiguity record (not an edge) | **true** | false |

`Inferred bool \`json:"inferred,omitempty"\`` is a **new field on `CallerRef`
and `CalleeRef` only**, authorized here and listed in §5 with the `Repo` field
it ships alongside. `omitempty` plus always-false-in-repo-mode keeps repo-mode
JSON byte-identical, exactly as `Repo` does. Its text rendering is a
`[inferred]` tag emitted only when true, on the same line and by the same
mechanism as `ambigTag` (`answers.go:392`).

The mapping lives in **one** exported function in `internal/wsquery` and every
reference-construction site calls it — the alternative is the same two words
being translated at nine renderers. The renderers are not taught the overlay's
vocabulary at all: `ambigTag` continues to key on `Ambiguous`, and the new tag
keys on `Inferred`. Keeping `inferred` distinguishable is not cosmetic — D3's
epistemics rule is that a bare-name (rung-2) answer must never be presented as
an import-mediated one.

### 3.8 Stable-key re-map at query time

Cross-edges store `SymKey{Member, File, QName}`; member rebuilds renumber symbol
ids, so keys are inverted to symbols at query time via the member's own DB (D2).
The re-map **must** use `(*Store).ProjectDefs(name, parent)`
(`internal/graph/wsreaders.go:127`), **not** `Definitions`
(`internal/graph/store.go:656`). `ProjectDefs`' own doc comment says why:
`Definitions` neither filters tier nor returns Tier/Namespace, so using it
re-admits a **vendored tier-1 snapshot** as a cross-repo target — the exact
failure member-over-dep precedence exists to prevent. It is also the correct
inverse: the keys being inverted were *written* by `ProjectDefs`
(`ladder.go:211,215`).

`QName` splits into `(parent, name)` on the **last** `.`, because `QName()` is
`Parent + "." + Name` and dotted parents are ordinary in TS and Python. A
first-dot split silently mis-parents every nested symbol in two of the four
supported languages.

**Re-map result cardinality — all three cases are defined.** `ProjectDefs` is
not a clean inverse: it treats `parent == ""` as *no parent restriction*
(`internal/graph/wsreaders.go:127-137`), and the write side has a deliberate
parent-less fallback (`internal/wsresolve/ladder.go:213-218`), so a dotless
`SymKey.QName` can come back with many symbols.

- **Zero symbols** (member rebuilt, symbol gone): the reference is **dropped
  from the answer and counted**, and the count is surfaced as a fourth
  workspace-clause field `keys_unmapped: <n>` (§4.5's JSON sibling and the
  trailing text line), rather than being silently swallowed. It is additive to
  D6's reserved three fields, not a substitute for any of them.
- **Exactly one:** used.
- **More than one:** filter to symbols whose `Parent` equals the QName's parent
  **exactly** — for a dotless QName that means `Parent == ""`. If exactly one
  survives, use it. If several still survive, the reference is rendered as
  **ambiguous** with its candidates (`Ambiguous: true`, the existing
  multi-candidate shape), never resolved by picking the first row. Picking
  arbitrarily here would manufacture an `exact`-looking answer out of a
  genuinely ambiguous key, which is the one thing D3 forbids end to end.

## 4. Freshness, staleness, and the coverage clause

### 4.1 Freshen on query entry — whole workspace

Every `wsquery` entry point on a `RootWorkspace` calls `wsfresh.Freshen(wsRoot)`
before reading anything, mirroring the per-repo `query.Fresh` contract.

**The freshen is whole-workspace, and ADR-0012 is not the argument for it.**
ADR-0012 governs *re-resolution* scope — it says a dirty workspace runs exactly
one whole-pass `wsresolve.Resolve` — and says nothing about which members
receive `query.Fresh`. The argument is its own:

1. `wsfresh.Freshen(wsRoot string) (Report, error)`
   (`internal/wsfresh/freshen.go:144`) takes **no member subset**. A per-query
   subset would mean a second freshen implementation, i.e. a second enforcement
   site for the gate ADR-0012 defines.
2. Whether a member is clean is **unknowable without folding it** — ADR-0012's
   own consequences section says so. So "only freshen the members this query
   consults" cannot skip the expensive half anyway once any member is dirty,
   because the dirty path is a whole-pass `Resolve` that reopens every member.
3. D4 makes the union potentially span every member (`find`/`grep` fan out by
   definition), so the consulted set is usually the whole manifest regardless.

D2's "members freshen lazily, only those consulted by the query, where the verb
permits" is satisfied vacuously here: no verb permits it, and the risk note
already sanctions the artifact-import path as the answer if latency bites.
That latency is exactly what the D7 run measures.

### 4.2 `Freshen` error posture — degrade, disclose, never fail the query

**Manifest faults are separated out first, by construction.** `wsquery` calls
`config.LoadWorkspace(wsRoot)` **explicitly, before** `Freshen`. A manifest that
is absent, unparseable, or invalid is returned as an error there — a
configuration fault, not a staleness condition. This is not an optimization: all
ten of `Freshen`'s error returns are opaque `fmt.Errorf` wraps with no
sentinel or error type (`freshen.go:152,155,162,166,178,244,252,292,318,373`),
and `engine.DetectRootKind` deliberately only *stats* the manifest, never parses
it (`internal/engine/rootkind.go:32-34`), so nothing downstream can tell a
manifest fault from a member fold failure. Discriminating up front is the only
way the exception is actually discriminable.

**Any remaining `Freshen` error degrades and discloses maximally.** The query
proceeds against the overlay as it stands; the coverage clause carries
`freshen_failed: <error>` **and names every declared member as stale**, not just
the ones the partial `Report` happens to list. The partial-report subtlety is
the whole reason for the "every declared member" rule: `Freshen` returns a
partially-filled `rep` on its error paths, and its own comment at
`freshen.go:236-244` says the mid-flight fold error is *deliberately* fatal
because a racing rebuild "would drop the member out of `Dirty`, leave its stale
stamp unread, and let the gate return a quietly WRONG clean verdict." Reading
that partial report as authoritative would reproduce exactly that quiet wrong
verdict at the query surface — the silent staleness D7 hard-fails. Treating the
whole report as untrustworthy costs an over-broad stale list on a rare path and
buys the guarantee.

Rationale for degrading at all rather than failing: a workspace that refuses to
answer while one member's index is briefly unopenable is strictly worse than one
that answers and says what it could not verify. The disclosure is what makes
that trade honest.

### 4.3 `members_stale` — the four-way union

D6 reserves the clause shape `workspace: {members_consulted, members_stale,
boundary}`. `members_stale` is the union of **four** id sets from
`wsfresh.Report`:

1. `Dirty` — stamp absent or moved from the re-folded merkle root.
2. `StaleStamped` — declared-but-unavailable and still stamped.
3. `MembersMissing` — declared, absent from disk.
4. the ids behind `MembersFreshenFailed` — available, but `query.Fresh` errored.

The clause is computed **after** `Freshen` returns and reads `Resolved`: when
`Resolved` is true, `Dirty` members are dropped from `members_stale`, because
the pass that just ran retired their staleness. Naming them stale there would be
a false alarm, and a permanently-stale-looking workspace teaches the agent to
ignore the field.

**`Dirty` must stay in the union, and the degrade path of §4.2 is exactly why.**
An earlier draft justified this by claiming `Dirty` non-empty with
`Resolved == false` "cannot occur." That is false, and dangerously so: `Freshen`
appends to `rep.Dirty` at `freshen.go:255` but sets `rep.Resolved = true` only
at `freshen.go:375`, and the error returns at `:292`, `:318`, `:373` sit between
them — precisely the paths §4.2 tells the query to proceed through. A builder
who believed the impossibility claim would collapse this to a three-way union
and drop dirty-member disclosure on the degrade path. (On that path §4.2's
every-declared-member rule already covers it; the four-way union is what covers
it on any future path where `Freshen` returns a partial report without an
error.)

**Blocking implementation note:** `Report.MembersFreshenFailed` and
`Report.MembersUnindexed` are **counts (`int`), not id slices**
(`internal/wsfresh/wsfresh.go:52-81`), while `MembersMissing`, `Dirty`, and
`StaleStamped` are `[]string`. The clause needs ids for set 4. The fix is to add
an id slice to `wsfresh.Report` alongside the existing count — an **additive**
field, not a rename, and not a re-typing of the counts, because three live
assertions and this slice's own arithmetic depend on the existing denominators
(see `MembersUnindexed`'s field comment on why this package splits names rather
than overloading them). The count and the slice must be written at the same site
so they cannot disagree.

`MembersUnindexed` (present, index unopenable) is **not** in `members_stale`: an
unindexed member is covered by `StaleStamped` when it previously contributed
rows, and by `boundary` when it never did. Adding it whole would double-count.

### 4.4 `members_consulted` — defined

`members_consulted` is **the set of member ids whose graph.db was read to
compose this answer, excluding the freshen pass**, in manifest order — not the
manifest, and not the present set. The exclusion is not a quibble: §4.1's
whole-workspace freshen opens and patches *every* present member
(`query.Fresh` per member, `freshen.go:218`), so a definition phrased simply as
"whose DB this answer read" would resolve to the present set on every query and
carry no information at all.

For an anchor verb it is the anchor's own member plus every member appearing as
a `SymKey.Member` on a unioned cross-edge. For a fan-out verb it is every member
the fan-out queried (i.e. every present member — the fan-out genuinely reads
them all). A member that was consulted but contributed nothing is still listed:
the clause answers "what did you look at", not "what did you find".

### 4.5 `boundary` and where the clause is rendered

`boundary` is the fixed sentence that symbols outside the workspace are unknown.

`ImpactAnswer.Coverage` is an existing **`string`**
(`answers.go:322`), set from the constant `impactCoverage`
(`query.go:518-519`) and rendered unconditionally at `answers.go:336` as
`(coverage: %s)`. In **repo mode it is byte-unchanged**. In workspace mode it
carries the existing sentence **plus** the workspace clause appended, keeping
the field a string and the renderer a one-liner. A structured
`workspace: {…}` object is added to the JSON shape only, as a sibling field that
is `omitempty` in repo mode — so `--json` consumers get the reserved shape while
the text surface stays a single line and repo-mode JSON keys are unchanged.

The other eight answer types have no coverage field today and do not gain one.
In workspace mode they carry the clause as a trailing line in `Text()` and a
sibling `omitempty` JSON field, added at the `wsquery` layer. Repo mode emits
neither.

### 4.6 D6's per-verb layer policy — which layer the clause describes

D6 reserves the clause shape **and** requires that "the graph-vs-retrieval layer
policy (Critic #5) names which layer a workspace coverage clause describes per
verb" (design.md:169-176). Naming it:

| verb | layer the clause describes |
|---|---|
| `callers`, `callees`, `impact`, `nav`, `dependents`, `deps` | **graph** — the clause is a statement about which members' edge sets were unioned; a stale member means edges may be missing or superseded |
| `find` | **retrieval** — the clause is about which members' symbol indexes were searched; a stale member means its ranked rows may be from an older tree |
| `grep` | **retrieval** — same, over content hits |
| `enclosing` | **graph**, single-member — the clause names the one owning member, and is stale iff that member is |
| `search` | n/a — refuses on a workspace root (§2.2) |

The distinction is the one Critic #5 asked for: a graph-layer clause bounds
*completeness of an edge set*, a retrieval-layer clause bounds *freshness of a
searched index*. `nav` is graph-layer despite composing retrieval components,
because its headline claim ("who calls this") is an edge claim; its find/grep
components inherit the same clause rather than carrying a second one.

## 5. `repo` provenance on the surfaces

Owner ruling 2: `repo` is a **workspace-mode-only text prefix**; single-repo
text stays byte-identical; no new MCP tools; the plugin note is untouched.

- **Reference structs** — all **eight** of them: `DefRef` (`answers.go:14`),
  `CallerRef` (`:24`), `CalleeRef` (`:36`), `DependentRef` (`:49`), `FindRef`
  (`:57`), `GrepRef` (`:68`), `DepRef` (`:79`), `EnclosingRef` (`:88`) — gain a
  `Repo string \`json:"repo,omitempty"\`` field. `omitempty` is what keeps
  repo-mode JSON byte-identical. `DepRef` is included deliberately: `deps` is a
  union verb (§3.1), so omitting it would ship a cross-repo answer with no
  member provenance on any row.
- **`CallerRef` and `CalleeRef` additionally** gain
  `Inferred bool \`json:"inferred,omitempty"\`` (§3.7) — the only two structs
  that already carry a confidence signal, and the only two that need a second.
- **Text renderers** print `<member-id>: ` before the path on each reference
  line — `api: services/api/u.go:42` — **only when `Repo != ""`**. In repo mode
  `Repo` is always empty, so the format string branch never fires and the bytes
  are unchanged.
- **Paths** in workspace mode are workspace-relative
  (`services/api/handlers/u.go`); in repo mode they are exactly what they are
  today.
- **MCP**: the eight tools are unchanged in name, schema, and description. Their
  handlers call `wsquery.*Text` instead of `query.*Text`, and the member id
  arrives inside the text they already return. No `repo` JSON field is added to
  a tool schema, because these tools return `TextContent` and have no structured
  result to add it to — this is what "discharges the frozen `repo` in result
  schemas via the text surface the tools actually have" means.
- **Plugin note**: untouched (house rule).

## 6. `workspace-status`

A new CLI verb, `codeindex workspace-status <workspace-root> [--json]`. It errors
on a repo root, naming the repo-mode `status` verb. Its report:

- per member: id, resolved absolute root, present/missing, indexed/unindexed,
  and **stamp presence plus the stamped merkle root as recorded** — explicitly
  *not* a "currently dirty" verdict. Dirtiness is by definition a fresh merkle
  re-fold compared against the stamp, `wsfresh`'s `foldMember` is unexported and
  no exported fold-and-compare exists, so reporting it would mean either doing
  the expensive fold half (falsifying "reads state, does not freshen" below) or
  writing a second implementation of the dirty predicate — the exact
  second-enforcement-site objection §4.1 uses to reject subset freshen. The
  honest cheap field is the recorded stamp; `codeindex refresh` is what turns it
  into a verdict;
- the overlay's schema version, cross-edge / ambiguity counts;
- **member/vendor version skew** from `overlay.Suppressions()`: one line per
  `Suppression{ConsumerMember, Namespace, OwnerMember, SuppressedVersion}` —
  "`drupal` vendors `symfony/http-foundation` at v7.1.0; member `symfony` wins" —
  with `SuppressedVersion == ""` rendered as "version unknown", never omitted.
  This is the D3 reporting obligation the suppression record was written for.

It reads state; it does not freshen. That keeps `workspace-status` usable as a
diagnostic on a workspace whose freshen is the thing being diagnosed.

`codeindex status <workspace-root>` prints the per-member fan-out (§2.1)
followed by this block.

## 7. Tests

### 7.1 Extend the single-repo goldens to all nine renderers

"Byte-identical by construction" is **false** for this slice and must not be
claimed. The nine `Text()` renderers in `internal/query/answers.go` are shared
between repo and workspace mode and are being **edited** (§5's conditional
prefix, §4.5's coverage line), so repo-mode identity is **measured, not
structural**. Today **five** answer types are covered by golden tests
(`TestCallersTextGolden:43`, `TestCalleesTextGolden:113`, `TestEnclosingText:128`,
`TestNavTextGolden:140`, `TestGrepTextAndJSON:186`) — Find, Dependents, Deps, and
Impact have no text golden.

**Record correction.** The change file's verified finding 6 says "only three of
the nine have goldens today (`query_test.go:43,113,140`)". The tree has five;
the finding missed `TestEnclosingText:128` and `TestGrepTextAndJSON:186`. Both
critic passes independently confirmed five against the tree. The change record is
the erroneous side and is corrected there; this spec's count is the operative
one. The conclusion — extend to all nine — is unchanged either way.

**Add text goldens for all nine before touching a renderer**, so the diff that
adds the prefix is measured against a pinned baseline rather than asserted safe.
Also pin repo-mode `--json` for each, since `omitempty` is the only thing keeping
the JSON keys stable.

### 7.2 Workspace goldens

Workspace answers pinned for each union verb over a small fixture workspace
(two members, one cross-edge, one ambiguity, one suppression), covering: the
`repo:` prefix, workspace-relative paths, manifest ordering, `limit` truncation
arithmetic over the union (§3.5), the coverage clause, and the anchor-prefix and
bare-anchor-ambiguity paths.

### 7.3 The D7 freshness property test (executable)

Mutate one member, query from another, **no explicit rebuild** → the answer
reflects the mutation, **or** its coverage clause names that member stale.
Silent staleness is a hard fail. Written as a property over the union verbs, not
a single scripted scenario, so it also covers the `Freshen`-failed path of §4.2
(inject a freshen failure → assert the clause names the member).

### 7.4 §3.5's deferred bar — single-member workspace ≡ single-repo

D7's bar is frozen and is held **verbatim**: a one-member workspace's answers
match the single-repo answers **modulo the `repo` field** (design.md:193-194).
An earlier draft added "and workspace-relative paths" — that is a real
relaxation of a frozen bar and is withdrawn. Instead the fixture is built so the
bar is literally satisfiable: the sole member's `root` is `.`, so
workspace-relative and repo-relative paths coincide and the only difference left
is the `repo` field, exactly as frozen. (A fixture whose sole member sits in a
subdirectory would not meet the frozen bar and is therefore not the fixture the
bar runs on.) This is D7's non-regression bar and §3.5's deferred unit test;
both are discharged here.
**Tick §3.5 in `tasks.md`** alongside §3.4/§4.x — it is this slice's work, and
leaving it unticked misreports the campaign.

### 7.5 Cross-site coherence review

Per the `one-invariant-many-sites-drifts` finding, the review pass reads these
site pairs **against each other**, not each against the spec: `dependents` vs
`impact`'s dependents block (§3.1); the `RootRepo` tail-call vs the nine
renderers' conditional branches (§1, §5); `refresh`-on-workspace vs
`wsfresh.Freshen`'s own gate (§2.1); the suppression filter's condition vs
`wsresolve.go:19-29`'s prose (§3.6); the `members_stale` union vs each
`Report` field's doc comment (§4.3).

## 8. Out of scope

- Corpus growth (change 0010) — the gate runs the frozen 65-task corpus.
- Cross-workspace semantic search / vectors, UI, git-remote identity, language
  expansion, re-ranking — frozen non-goals.
- Scoped incident re-resolution — ADR-0012 stands; a D7-measured follow-up only.
- Transitive `impact` closure (§3.3).
- Any new MCP tool, and any edit to the plugin note.

## 9. The merge gate (§5 — owner-attended, outside this build)

The PR merges **only if** the pre-registered D7 gate passes (design.md
Amendments, 2026-08-19: the block binds exactly at wiring, which is this slice).
The gate is an **owner-attended step, not part of the autonomous build**: the
bench harness and corpus live in local-main-only unpushed commits, so the run
happens from the local main tree against this branch's built binary.

Order: (1) re-run `leak_audit_ws.py` over the campaign transcripts — a standing
pre-verdict gate that refuses a verdict on non-zero exit; (2) run arm A
(shell + checkouts, PATH shim + `CODEINDEX_DISABLED`) vs arm B (A + workspace
MCP via `CODEINDEX_WS_MCP_BIN`) on the frozen 65-task corpus, isolation
`--setting-sources project,local`; (3) read the bars **verbatim from
`bench/workspace/README.md`**, never from the gate script's own source.

Verdict and residuals are recorded in
`bench/engine/FINDINGS-workspace-graph.md` either way. **The kill condition is
frozen and live:** if B does not beat A on recall or efficiency, the control
wins, the FINDINGS entry is the deliverable, and the change closes. That is a
legitimate outcome, not a failure to work around.

Local honest suite for the build itself: `go test -tags nollama -count=1 ./...`
(pinned), green on `origin/main` at 2c8b9c3.

## Assumptions

Every decision below was defaulted autonomously. Owner rulings 1 and 2, the
frozen D4/D5/D6/D7 semantics, the five recorded obligations, the
`internal/wsquery` package choice, and the one-change/one-PR shape were **given**
and are not re-litigated here.

1. **`dependents` is a union verb** (§3.1) — *forced*: `ImpactAnswer` embeds the
   dependents block and `impact` is union by ruling, so per-repo `dependents`
   would contradict `impact` from the same root. Rejected the narrow reading
   because it manufactures the exact two-sites-disagree defect this repo has
   paid for three times.

1a. **`deps` is also unioned** (§3.1) — *discretionary*, and flagged as the
   weaker of the pair: `ImpactAnswer` has no `Deps` block, so no
   two-sites-disagree argument reaches it. Unioned anyway for directional
   symmetry with `dependents` (same edge table, opposite direction). A reviewer
   who wants `deps` left per-repo would not be contradicting anything frozen.

2. **`search` refuses on a workspace root** (§2.2). Alternatives: fan out across
   members; route to a member via prefix; refuse. Chosen: refuse, citing the
   frozen non-goal — cross-workspace semantic search is explicitly out (vectors
   stay per-repo). Fan-out would ship the non-goal by accident; a prefix route is
   new surface with no D7 bar behind it.

3. **`enclosing` routes by longest-prefix over resolved absolute roots** (§2.3).
   Alternative: prefix-match declared relative roots. Rejected: D1 sanctions
   `../api` member roots, which relative matching mishandles.

4. **Whole-workspace freshen on query entry** (§4.1), argued from `Freshen`'s
   subset-free signature, the unknowable-without-folding cost, and the fan-out
   verbs' whole-manifest reach — **explicitly not** from ADR-0012, which governs
   re-resolution scope only. Alternative (consulted-members-only freshen)
   rejected as a second freshen implementation for no measurable win before D7.

5. **A `Freshen` error degrades and discloses rather than failing the query**
   (§4.2), and on that path **every declared member is named stale**, not just
   the ones the partial `Report` lists. The manifest-fault exception is made
   discriminable by calling `config.LoadWorkspace` explicitly *before* `Freshen`
   — necessary because all ten `Freshen` error returns are opaque
   `fmt.Errorf` wraps with no sentinel, and `DetectRootKind` only stats the
   manifest. Alternative (fail the query) rejected: it converts a transient
   member problem into total unavailability. Alternative (trust the partial
   report) rejected explicitly: `freshen.go:236-244` says the fold error is
   deliberately fatal because a partial report yields a "quietly WRONG clean
   verdict" — the silent staleness D7 hard-fails.

6. **`members_stale` = the four-way union, minus `Dirty` when `Resolved` is
   true** (§4.3). Alternative: include `Dirty` unconditionally — rejected, a
   just-re-resolved workspace would report itself permanently stale. The earlier
   draft's supporting claim that "`Dirty` non-empty with `Resolved == false`
   cannot occur" is **withdrawn as false** (`rep.Dirty` is appended at
   `freshen.go:255`, `Resolved` set at `:375`, error returns at `:292/:318/:373`
   sit between) — the rule survives, the reason is now the degrade path.

7. **`members_consulted` = members whose DB was read *to compose the answer*,
   excluding the freshen pass** (§4.4). The exclusion is load-bearing: §4.1's
   whole-workspace freshen touches every present member, so the unqualified
   phrasing would make the field constant and information-free. Alternatives (the
   manifest; the present set) rejected as not varying with the answer.

8. **The coverage clause rides as appended text + an `omitempty` JSON sibling**
   (§4.5), keeping `ImpactAnswer.Coverage` a `string`. Alternative: re-type
   `Coverage` to a struct. Rejected: it breaks repo-mode JSON, which is a
   non-regression bar.

9. **Confidence maps overlay→(`Ambiguous bool`, new `Inferred bool`) at one
   boundary function** (§3.7). The earlier draft's "the surface speaks
   `graph.Confidence`" is **withdrawn as false** — no answer type carries a
   `graph.Confidence`, and only `CallerRef`/`CalleeRef` carry any confidence
   signal at all. The new `Inferred` field is authorized here and listed in §5;
   without it, a rung-2 bare-name answer would be indistinguishable from a rung-1
   import-mediated one, which D3's epistemics rule forbids. Alternative (teach
   the renderers the overlay vocabulary) rejected as nine sites for one mapping.

10. **`limit` bounds the unioned list; `*Total` counts the unioned set** (§3.5),
    and for `find`/`grep` the aggregates sum across members with `Backend`
    joined on disagreement (§3.2). Alternative: per-member limit then
    concatenate — rejected, makes the `Total - len(list)` arithmetic wrong. The
    earlier draft's claim that the truncated head matches the single-repo head is
    **withdrawn**: §3.6's filter legitimately removes own-member edges.

11. **`wsfresh.Report` gains an additive id slice for freshen-failed members**
    (§4.3). Alternative: re-type `MembersFreshenFailed` from `int` to
    `[]string`. Rejected: live count assertions
    (`internal/wsfresh/freshen_test.go:501,519-520,523-524`) and the package's
    explicit one-name-one-denominator discipline (`wsfresh.go:38-40,71-80`)
    depend on the existing counts.

12. **`status` on a workspace root = per-member fan-out + the
    `workspace-status` block; `workspace-status` reads state and does not
    freshen** (§2.1, §6) — and consequently reports the **recorded stamp**, not
    a "currently dirty" verdict, since the dirty predicate needs a fresh merkle
    fold and `wsfresh.foldMember` is unexported. Alternatives: make it freshen
    (rejected — a diagnostic that mutates what it diagnoses is useless on the
    freshen failure it exists to explain); export a fold-and-compare predicate
    (rejected *for this slice* — it widens a merged package's API for a display
    field, and the honest cheap field plus `codeindex refresh` covers the need).

13. **Text goldens are added for all nine renderers before any renderer is
    edited** (§7.1). The alternative — claim byte-identity by construction — is
    false for this slice and was the critic's finding in the discarded draft.

14. **Fan-out maintenance verbs report per-member failures and continue,
    returning an aggregate error that exits 1 through the existing `fatal`**
    (§2.1). Alternative: abort on first failure — rejected, leaves a half-built
    workspace with no record of which members succeeded. The earlier draft's
    "exit status 2" is **withdrawn**: `fatal` is `os.Exit(1)` unconditionally
    (`cmd/codeindex/main.go:646-649`), exit 2 is the pre-dispatch usage code, and
    this slice adds no third exit mechanism.

15. **Dependency state.** `depends_on: [15]` is satisfied (0015 merged at
    2c8b9c3; §3.4's freshen half and §3.3's ladder are in tree). The design-ahead
    rule was not needed.

16. **`explore-feature` is untouched** (§2.2) — it is an MCP prompt, reads no
    root, and cannot error on one. Recorded because the discarded draft treated
    it as a verb.

17. **Root-kind detection has exactly one call site, `wsquery.RootKind`, which
    the eight non-query verbs also route through** (§1). Alternatives: let
    `main.go` call `engine.DetectRootKind` directly for those eight (rejected —
    nine detection sites and nine refusal messages); grow `wsquery` non-query
    entry points wrapping `build`/`export`/… (rejected — it would make the union
    layer own the build pipeline). Two exported helpers is the smallest thing
    that keeps the one-site property true.

18. **The MCP anchor prefix rides the existing free-text `symbol` argument, with
    no schema and no description change** (§3.4). D5 says tool schemas gain "the
    optional anchor prefix"; owner ruling 2 freezes the tool surface. Chosen:
    make it *work* without advertising it on MCP — the argument is already a
    free string. Cost, recorded rather than hidden: an MCP client is never told
    the prefix exists. The CLI usage line does document it. A human who wants it
    advertised is overriding ruling 2's surface freeze, which is their call.

19. **A stable key re-mapping to several symbols is rendered ambiguous, never
    resolved by picking a row** (§3.8), after an exact-parent filter. Alternative:
    take the first result. Rejected: it manufactures an exact-looking answer from
    an ambiguous key, the one thing D3 forbids end to end.

20. **The D7 single-member bar is held verbatim ("modulo the `repo` field") and
    the fixture's sole member root is `.`** (§7.4). The earlier draft's "and
    workspace-relative paths" is **withdrawn** — it relaxed a frozen bar.

21. **D6's per-verb graph-vs-retrieval layer policy is discharged by the table in
    §4.6** — anchor verbs graph-layer, `find`/`grep` retrieval-layer, `enclosing`
    single-member graph-layer, `search` n/a. It was undischarged in the earlier
    draft.

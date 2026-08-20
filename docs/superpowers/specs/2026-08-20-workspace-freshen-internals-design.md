<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0014 — Workspace freshen internals — per-member freshen + stamp-gated re-resolution](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0014-workspace-freshen-internals.md)**
<!-- docket:backlink:end -->

# Workspace freshen internals — per-member freshen + stamp-gated re-resolution

**Change:** 0014 `workspace-freshen-internals` · **Depends on:** 0013 (merged) · **Related:** 0012, 0013
**Slice:** `openspec/changes/workspace-graph` §3.4 — *internals half only* (the `workspace-status`
verb is OUT, per the D7 second amendment of 2026-08-19).

## Problem

§3.3 (change 0013) shipped `internal/wsresolve.Resolve` — a full-pass, whole-workspace resolution
ladder with no non-test caller. Nothing yet decides *when* that pass should run. Design D2's
freshness contract is:

> workspace `Fresh` = for each member, run the existing per-repo freshen; then for each member whose
> merkle root differs from its stamp, re-resolve only overlay edges incident to that member and
> update the stamp. Unchanged members cost one stamp comparison.

Without it, §4's union-graph answers would serve stale cross-edges, which D7 makes a hard fail.

## Scope

**In:** a workspace freshen pass as an unwired internal Go entry point; the stamp gate that decides
whether re-resolution runs; missing-member reporting surfaced through `config.Resolve` (0009); the
stamp-gating tests 0013 deferred out of the §3.5 bar, including crash self-healing.

**Out:** the `workspace-status` verb and every other verb wiring; union-graph query paths; coverage
clauses; confidence vocabulary (§4.1); the single-member-workspace ≡ single-repo equivalence bar
(§4.2's, per the owner ruling); the D7 evidence-gate run (§5.x); any change to the merkle fold or to
the 0013 ladder's frozen rung order, never-thin semantic, or clear→put→stamp write order.

**Mergeability.** The entry point has no non-test caller and no verb reads its output, so it changes
no answer any verb returns — the exact criterion the 2026-08-19 amendment uses to admit unwired
engine internals ahead of the gate, as §3.2 and §3.3 already were.

## Design

### The pass

New package `internal/wsfresh`, one exported entry point:

```go
// Freshen brings wsRoot's workspace overlay into agreement with its members'
// current content. No verb calls it yet.
func Freshen(wsRoot string) (Report, error)
```

Steps, in order:

1. **Root kind + manifest.** `engine.DetectRootKind` must report a workspace root; then
   `config.LoadWorkspace`, then `(*config.Workspace).Resolve(wsRoot)` to split declared members into
   present-on-disk and missing. Manifest order is preserved throughout — the pass is deterministic.
2. **Availability, one predicate.** A present member is **available** iff
   `graph.OpenExisting(memberIndexPath)` succeeds — absent, version-mismatched, and unopenable are
   all the same answer. This is `wsresolve.Resolve`'s own predicate (`wsresolve.go:164`) and this
   slice adopts it verbatim rather than defining a second one (see Assumption 10).
3. **Per-member freshen** (available members, manifest order): `query.Fresh(memberRoot)` — the
   existing per-repo freshen. Because availability already established that the index exists,
   `query.Fresh` takes its `engine.Patch` + `depmap.VerifyOverlay` branch and never its cold-build
   branch. A member that is present but unavailable is **not** built (Assumption 2).
4. **Root fold.** For each freshened member, re-read `(*graph.Store).MemberMerkleRoot()` — the
   canonical 0013 fold, reused, not forked. The value is opaque: equality comparison only, never
   parsed or ordered.
5. **Stamp gate.** For each freshened member, read `overlay.Stamp(memberID)`. The member is **dirty**
   when its stamp is absent, or present and unequal to the freshly folded root. A member whose stamp
   is present and equal is **clean** and costs exactly this one comparison *on top of* the freshen
   and fold every member pays (Assumption 9).
6. **Registry drift.** The pass is dirty as a whole when `(*overlay.Store).Registry()` does not equal
   the loaded manifest's member list — compared on the **whole record**: id, root, ordered
   namespaces, ordered deps, and manifest order. Not the id set (Assumption 5). The manifest side is
   **normalized first** with exactly `ReplaceRegistry`'s own transforms — first-occurrence `dedupe()`
   of namespaces and deps, and nil-coalescing of empty slices — because `Registry()` returns what
   `ReplaceRegistry` stored, not what the manifest literally said. Comparing raw would report drift
   forever on a legal manifest carrying a duplicate namespace or a `"deps": []`.
7. **Decision.** No dirty member and no registry drift ⇒ **no overlay content write** — no registry,
   edge, ambiguity, suppression or stamp row; return. (`overlay.Open` itself re-executes the schema
   and `PRAGMA user_version` on every open, so the *file* is never a no-write witness.)
   Otherwise ⇒ one call to `wsresolve.Resolve(wsRoot)`, whose `Stats` ride back in the Report.

### Why re-resolution is whole-pass, not per-member scoped

This is the slice's load-bearing decision. D2's "re-resolve only overlay edges incident to that
member" cannot be implemented literally on the frozen 0013 API, for two independent reasons:

1. **The dirty member is not the only member whose derivation changed.** An edge `S → M` is derived
   from source `S`'s unresolved-edge list resolved against `M`'s definitions. When `M` changes, the
   record must be re-derived — but the *deriving* call is `Ladder(S, …)`, scoped by source `S`, not
   by the dirty member. Every member that could source an edge into `M` must therefore re-run.
2. **The blast radius escapes the incident set.** If a hint in `S` that previously resolved uniquely
   in clean member `O` now also resolves in dirty `M`, the ladder's answer for that call site changes
   from an exact `S → O` cross-edge to an ambiguity. The row that must change has endpoints `S` and
   `O` — neither dirty. "Edges incident to the changed member" is not closed under re-derivation.

*Note, not a third reason:* `overlay.ReplaceMemberEdges` deletes rows incident to a member on either
end (`edges.go:171`, `WHERE src_member = ? OR dst_member = ?`, inside `ReplaceMemberEdges` at
`edges.go:144`), so the naive per-dirty-member
derive-and-write loop is the self-clobbering shape learnings
`symmetric-replace-makes-per-entity-loops-self-clobbering` records from 0013. This rules out the
naive loop only — a competently scoped alternative would use the same clear-all → put-all →
stamps-last composition over a computed source-closure, so it is not an argument against scoping.
The two reasons above are.

The stub's own framing is permission-shaped — *"scoped to incident edges rather than a full-workspace
pass **where the design permits**"* — and the design does not permit it on the frozen API. D2's risk
note bounds the cost: *"worst case is bounded by a full overlay rebuild which is itself bounded by the
unresolved-edge count, not the symbol count."* What D2 states as *contract* — unchanged members cost
one stamp comparison — is satisfied at the overlay: step 7's clean branch performs no overlay work at
all. Narrowing the dirty case to a computed source-closure stays available as a measured, later
optimization; the D7 gate is where its cost would be justified, rather than guessed at here.

**Never-thin and crash self-healing hold unchanged**, because the re-resolution path *is* 0013's
pass: stale ambiguity records are deleted whole and re-derived, and stamps are written last, so a
pass that dies mid-write leaves the affected members stampless — which step 5 reads as dirty on the
next `Freshen`. That property now has a caller exercising it for the first time.

### Report

```go
type Report struct {
    MembersFreshened int      // available members whose per-repo freshen ran
    MembersUnindexed int      // present on disk, graph.OpenExisting failed
    MembersMissing   []string // declared, absent from disk — config.Resolve's `missing`, manifest order
    Dirty            []string // member ids whose stamp was absent or moved, manifest order
    Resolved         bool     // whether wsresolve.Resolve ran
    Stats            wsresolve.Stats // MEANINGFUL ONLY WHEN Resolved is true; zero value otherwise
}
```

`MembersUnindexed` is deliberately **not** named `MembersUnavailable`: the embedded
`wsresolve.Stats.MembersUnavailable` counts declared-but-unusable members *including missing* ones,
under 0013's stated invariant `MembersResolved + MembersUnavailable == len(declared)`. Two different
denominators in one struct under one name is the `one-invariant-many-sites-drifts` shape; the rename
plus the `Stats` caveat is the cheapest possible time to avoid it.

`config.Resolve` already has one caller — `wsresolve.go:142` — which collapses `missing` into a
count. This slice is its **second** call site and the first that surfaces the missing ids to its own
caller. The Report is data only: no answer shaping, no coverage clause, no confidence vocabulary;
§4.1 owns naming stale/missing members to a user.

An unindexed or missing member does **not** by itself make the pass dirty and is **never** built by
this pass: absence is a runtime condition, exactly as 0013 already treats it, and its overlay rows are
left alone. A per-member freshen error is likewise not fatal — the member is counted unindexed and
the workspace keeps answering (D2's whole reason for the present/missing split).

## Tests — the stamp-gating item of the §3.5 bar

All in `internal/wsfresh`, under `-tags nollama`. Except where noted, fixtures have ≥2 members with a
real cross-edge between them — the single-member fixture cannot reproduce the bugs this area has
(0012/0013 learnings). §3.5's other items (ladder order, stable-key survival across member rebuild,
single-member equivalence) are not this slice's.

1. **Clean workspace writes no overlay content.** Freshen twice; the second reports
   `Resolved: false`, `Dirty: nil`, and the overlay's *content* is unchanged — `Registry()`,
   `Stamps()`, `MemberEdges()`, `AmbiguitiesFor()`, `Suppressions()` row sets equal on a total sort
   key. `Registry()` is in the tuple because Assumption 5 promotes it to a gate input, so a spurious
   `ReplaceRegistry` on the clean path must not pass unseen. Content, not a file hash:
   `overlay.Open` re-executes the schema and `PRAGMA user_version` on every open, so the bytes are
   not a stable witness. `resolved_at` is 1-second-granular and is likewise unusable as a witness.
2. **Moved root re-resolves.** Edit a member's source so its fold changes; the next Freshen marks
   exactly that member dirty, sets `Resolved: true`, and the overlay reflects the edit.
3. **Crash self-healing.** Delete one member's stamp row directly (a pass that died before 0013's
   step 11); the next Freshen marks that member dirty and re-resolves, and the restored overlay
   content equals a from-scratch pass's.
4. **Cross-member freshness.** Change dirty member `M` so that clean member `S`'s edge into `M` must
   change; assert the `S → M` row is correct afterwards. This is the test that goes red under naive
   incident-only scoping, and it is why the pass is whole.
5. **Manifest-only drift.** Edit only a member's `namespaces:` (no source change, every root
   unchanged, every stamp matching); the next Freshen must report `Resolved: true`. Repeat for
   `deps:`. This is the silent-staleness hole an id-set comparison leaves open.
   **Sibling — drift comparison converges.** A manifest carrying a duplicate `namespaces:` entry, and
   one carrying `"deps": []`, must each report `Resolved: false` on the second Freshen. Without the
   step-6 normalization the comparison reports drift on every pass and re-resolves forever — the same
   non-convergence failure Assumption 10 exists to prevent, reintroduced from the other side.
6. **Unindexed / missing members.** A declared member absent from disk lands in `MembersMissing`; a
   present member with no index counts unindexed, is not built (assert no `graph.db` is created), and
   neither alone makes the pass dirty.
7. **Convergence with a version-mismatched member.** Two consecutive Freshens over a workspace with
   one member whose index exists but fails `graph.OpenExisting`; the second must report
   `Resolved: false`. A freshen predicate looser than the resolver's would leave that member
   perpetually dirty and re-resolve the whole workspace forever.
8. **Determinism.** Two consecutive dirty passes over identical content produce identical overlay
   contents, compared on a total sort key (learnings `determinism-tests-need-a-total-sort-key`).

Honest suite: `go test -tags nollama ./...` (plain `go test ./...` fails 10 packages on every ref,
environmentally).

## Assumptions

Every decision below was defaulted autonomously; each records the rejected alternatives.

1. **Scoped incident re-resolution → whole-pass `wsresolve.Resolve` in the dirty case.**
   *Rejected:* a per-dirty-member `ReplaceMemberEdges` loop (self-clobbering, see the note above); a
   computed dirty-source-closure scoped derive (sound in principle, but it needs a reachability model
   over declared deps that the ladder's later rungs may exceed — new correctness surface for an
   unmeasured win). *Why:* the two reasons in the section above show the incident set is not closed
   under re-derivation on the frozen API; the stub permits scoping only "where the design permits";
   D2's risk note explicitly bounds the worst case at a full overlay rebuild; and the observable
   contract (clean ⇒ no overlay work) is met exactly. The optimization becomes a D7-measured question
   rather than a guess.
2. **Present-but-unindexed member → unindexed, never built.** *Rejected:* calling `query.Fresh`
   unconditionally, which cold-*builds* an absent index (`query.go:75-83`). *Why:* a freshness check
   must not silently perform a full index build of an arbitrarily large member, and it would
   contradict 0013's settled treatment of an unbuilt member as unavailable. Gating on availability
   first keeps the precedent's patch + `depmap.VerifyOverlay` behavior — which the fold's
   dep-reattach sensitivity depends on — without the cold-build surprise. Whether a workspace verb
   should offer to build members is §4's, with a user in front of it.
3. **New package `internal/wsfresh`, not a second export on `wsresolve`.** *Rejected:* adding
   `Freshen` to `wsresolve`. *Why:* `Resolve`'s own doc comment (`wsresolve.go:66`) pins it as the
   package's only exported *entry point* (the package also exports the `Member`/`Pass`/`Suppress`
   machinery it composes), and `internal/query` is the CLI/MCP-facing freshness surface with package-level
   mutable state (a write mutex and the cold-build side channel) that the resolver should not depend
   on. Not an engine-dependency argument — `wsresolve` already imports `internal/engine`. No import
   cycle either way (`internal/query` depends on neither `overlay` nor `wsresolve`); this is a
   layering choice, and cheap to revisit while there are no callers.
4. **Missing stamp ⇒ dirty.** *Rejected:* treating a stampless member as clean, or as an error.
   *Why:* `overlay.Stamp` returns `(_, false, nil)` on absence — a clean signal by design — and
   absence is precisely the crash-self-healing marker 0013's stamps-last ordering leaves.
5. **Registry drift compares whole member records, not the id set.** *Rejected:* comparing declared
   member ids only. *Why:* `Registry()` round-trips id, root, namespaces and deps, and those fields
   are ladder *inputs* (`ladder.go` rung 1 resolves on namespaces; `Suppress` derives precedence from
   deps). An id-set comparison lets a `namespaces:` edit change resolution while every root and stamp
   stays put — zero dirty members, `Resolved: false`, and the overlay serves edges derived from the
   old claims indefinitely. That is exactly the silent staleness D7 hard-fails, produced by the gate
   meant to prevent it. Test 5 pins it. The comparison normalizes the manifest side with
   `ReplaceRegistry`'s own `dedupe()` and nil-coalescing first: `Registry()` returns what was
   *stored*, and a duplicate namespace or a `"deps": []` is legal loader input the store collapses,
   so raw equality would report drift forever. Test 5's sibling pins that convergence.
6. **Unindexed/missing members do not trigger a pass, and are not an error.** *Rejected:* erroring,
   or forcing a pass to purge their rows. *Why:* identical to 0013's settled posture — a member
   checked out elsewhere is a runtime condition and the workspace must keep answering. Purging on
   transient absence would only rebuild on return.
7. **`Report` is plain data; no coverage/confidence vocabulary, and `MembersUnindexed` is named apart
   from `Stats.MembersUnavailable`.** *Rejected:* pre-shaping a status string; reusing the
   "unavailable" name. *Why:* §4.1 owns the vocabulary hand-off; and the two counts have different
   denominators, which is the drift shape 0012/0013 already paid for twice.
8. **Single-threaded, manifest order.** *Rejected:* parallel member freshen. *Why:* `query.Fresh`
   holds a package-level mutex for its whole body, so parallelism serializes to nothing, and
   determinism is a stated bar.
9. **The gate's standing cost is a full per-member freshen plus fold, not literally one comparison.**
   *Rejected:* claiming D2's "one stamp comparison" holds literally. *Why:* a clean member still pays
   `engine.Patch` on its own `graph.db` and a `MemberMerkleRoot()` fold (a full ordered `merkle` scan
   plus a `DepFiles()` read and sort). That is inherent — cleanliness is unknowable without folding —
   but it is exactly the cost D2's "workspace freshen latency" risk cares about, so it is stated here
   for the D7 measurement rather than hidden behind the contract sentence. What the clean branch does
   guarantee is zero overlay *content* work — `overlay.Open` still re-executes the schema and
   `PRAGMA user_version` on every open, so the file itself is not a witness.
10. **One availability predicate, `graph.OpenExisting` success, at both sites.** *Rejected:* a
    file-exists check in freshen alongside the resolver's open-succeeds check. *Why:* a member whose
    db exists but is version-mismatched would then be freshened, folded and marked dirty by
    `Freshen`, and skipped-and-never-stamped by `Resolve` — permanently dirty, re-resolving the whole
    workspace on every call. Test 7 pins convergence.
11. **`Freshen` re-derives root-kind, manifest, presence and the member opens that `Resolve` then
    redoes.** *Rejected:* threading pre-resolved state into `wsresolve`. *Why:* `Resolve(wsRoot
    string)` is frozen and its signature admits nothing else, and widening it is 0013 re-litigation.
    The cost is a duplicated open per member on the dirty path only. It does plant a second
    enforcement site for the present/available/manifest-order invariants — the
    `one-invariant-many-sites-drifts` setup — so tests 6 and 7 assert the two sites agree, and the
    predicate in Assumption 10 is stated once and cited at both.
12. **The stub's own wording is corrected in the same commit.** Three sites describe the rejected
    shape and would otherwise leave the stub arguing with its own spec: the `title:`; the "Stamp-gated
    incident re-resolution … scoped to incident edges" bullet; and the `## Why` paragraph, which
    carries *both* rejected claims independently ("re-resolution of overlay edges incident to any
    member whose merkle root moved" **and** "unchanged members cost one stamp comparison", the latter
    rejected as literal by Assumption 9). All three are restated to the whole-pass decision with the
    scoping deferral and the real standing cost named.
13. **Dependency state.** `depends_on: [13]` is satisfied — 0013 (`85e090b`, PR #11), 0012
    (`f6d4dee`, PR #10) and 0009 (`528fc08`, PR #8) are all `done` and merged on `origin/main`. No
    unmet dependency at design time; the implementer's reconcile re-validates at build time.

## Reconcile addendum — 2026-08-20 (implementer pass, change 0014)

Every API this spec names was re-verified against `origin/main` at reconcile time. The design holds;
the signatures below are the authoritative ones to build against, and three items are refinements
the spec did not settle.

### Verified signatures (unchanged from the design's assumptions)

| Symbol | Signature |
|---|---|
| `wsresolve.Resolve` | `func Resolve(wsRoot string) (Stats, error)` |
| `wsresolve.Stats` | `MembersResolved, MembersUnavailable, CrossEdges, Ambiguities, Suppressions int` |
| `graph.OpenExisting` | `func OpenExisting(path string) (*Store, error)` |
| `(*graph.Store).MemberMerkleRoot` | `func () (string, error)` — sha256 hex, opaque |
| `overlay.Open` | `func Open(path string) (*Store, error)` |
| `(*overlay.Store).Registry` | `func () ([]config.Member, error)` — manifest order |
| `(*overlay.Store).ReplaceRegistry` | `func (ws *config.Workspace) error` |
| `(*overlay.Store).Stamp` | `func (memberID string) (Stamp, bool, error)` — `(Stamp{}, false, nil)` on absence |
| `(*overlay.Store).Stamps` | `func () ([]Stamp, error)` |
| `(*overlay.Store).MemberEdges` | `func (memberID string) ([]CrossEdge, error)` |
| `(*overlay.Store).AmbiguitiesFor` | `func (memberID string) ([]Ambiguity, error)` |
| `(*overlay.Store).Suppressions` | `func () ([]Suppression, error)` |
| `config.LoadWorkspace` | `func LoadWorkspace(wsRoot string) (*Workspace, error)` |
| `(*config.Workspace).Resolve` | `func (wsRoot string) (present []ResolvedMember, missing []string, err error)` |
| `config.Member` | `ID, Root string; Namespaces, Deps []string` |
| `engine.DetectRootKind` | `func DetectRootKind(root string) (RootKind, error)`; constant `engine.RootWorkspace` |
| `query.Fresh` | `func Fresh(root string) (FreshInfo, error)`; cold-build branch confirmed at `query.go:75-83` |

`internal/wsfresh` does not exist and nothing in tree references `Freshen` — a clean slate, as the
design assumed.

### R1 — `dedupe()` is unexported, so step 6's normalization needs one exported seam

**The gap.** Step 6 and Assumption 5 require the manifest side to be normalized with *exactly*
`ReplaceRegistry`'s own transforms. Those transforms live in `internal/overlay/registry.go` as the
unexported `dedupe()`, applied inside the equally unexported `insertMembers` — once to
`m.Namespaces` and once to `m.Deps` — and `dedupe` returns `nil` for an empty input, which is where
the nil-coalescing the spec names actually comes from. `internal/wsfresh` cannot call it.

**Resolution — export one normalizer in `internal/overlay`, and route the writer through it.** Add
an exported function to `internal/overlay` that returns the normalized form of a member list exactly
as `ReplaceRegistry` would store it, and change `insertMembers` to consume that same function rather
than calling `dedupe` inline. Then `wsfresh`'s drift comparison and the store's write path are one
implementation with two callers, not two implementations that agree today.

Re-implementing `dedupe`'s three lines inside `wsfresh` is **rejected**: it is precisely the
`one-invariant-many-sites-drifts` shape — a second enforcement site for a normalization rule with no
compiler and no test tying it to the first, whose divergence mode (drift reported forever, whole
workspace re-resolved on every call) is the exact non-convergence Assumption 5's sibling test exists
to catch. Test 5's sibling pins the behavior; this pins the structure.

The change to `internal/overlay` is additive and behavior-preserving — a refactor of an existing
private path into an exported one, no schema change, no change to what `ReplaceRegistry` writes. It
stays inside this slice's scope; it is not a redesign of the registry.

### R2 — the content-equality tuple is per-member, so the test must iterate

`MemberEdges` and `AmbiguitiesFor` are both scoped by `memberID`; only `Registry()`, `Stamps()` and
`Suppressions()` are whole-store. Test 1 (and test 8's determinism comparison) must therefore build
its comparison tuple by iterating the registry's members **in manifest order**, concatenating each
member's edges and ambiguities, and then sorting the whole thing on a total key — per
`determinism-tests-need-a-total-sort-key`, comparing two reads of one store proves nothing without
a key total over the rows the query can return, so the helper sorts on every field of `CrossEdge`
(`Src`/`Dst` triples, `Kind`, `Provenance`, `Confidence`, `Line`) and of `Ambiguity` (`Src`,
`RefName`, `RefNS`, `Kind`, `Line`, `Count`, then the ordered candidate list) rather than on a
prefix. A member present in the manifest but absent from the registry must still be visited, or a
spurious `ReplaceRegistry` that *drops* a member would compare equal.

### R3 — two ADRs bear on this slice; neither conflicts

- **ADR-0006** (`change-detection-flat-per-file-hashes-not-merkle-tree`) states change detection is
  flat per-file hashing, explicitly *not* a Merkle tree, and records that the package name
  `internal/merkle` "is a historical artifact and does not imply a tree structure." `MemberMerkleRoot`
  inherits that naming. It is a fold over flat per-file hashes plus the dep-file state, not a tree
  root, and this slice treats its value as opaque — equality only, never parsed or ordered — which is
  exactly what keeps the naming harmless. No ADR is contradicted and none is needed.
- **ADR-0005** (`freshness-on-demand-build-lazy-per-query-recheck`) establishes on-demand build plus a
  lazy per-query re-check. Assumption 2 (a present-but-unindexed member is reported unindexed and
  never built) **narrows** that posture at the workspace layer without reversing it: `query.Fresh`
  keeps its cold-build branch for the single-repo path, and `Freshen` simply never reaches that
  branch, because availability is established first. Whether a workspace verb should offer to build
  members is §4's, with a user in front of it. No ADR change.

### Unchanged

Scope, the whole-pass decision and its two reasons, the Report shape, all 13 assumptions, and the
8 tests stand as written. `depends_on: [13]` re-verified satisfied — 0013 is archived `done`
(`2026-08-20-0013-workspace-resolution-ladder.md`), as are 0012 and 0009. Honest suite remains
`go test -tags nollama ./...`.

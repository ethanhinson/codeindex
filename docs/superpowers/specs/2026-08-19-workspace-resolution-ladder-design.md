<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0013 — Cross-repo resolution ladder — import-mediated exact, bare-name inferred, ambiguity, suppression](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0013-workspace-resolution-ladder.md)**
<!-- docket:backlink:end -->

# Design: Cross-repo resolution ladder — import-mediated exact, bare-name inferred, ambiguity, suppression

Change: 0013 `workspace-resolution-ladder` · openspec task §3.3 of
`openspec/changes/workspace-graph/tasks.md` · frozen design
`openspec/changes/workspace-graph/design.md` (D3 ladder; D2 storage; D5 surfaces).

Authored by `docket-auto-groom` (autonomous self-brainstorm). Every decision
below was defaulted without a human; the `## Assumptions` block at the end is
the audit trail.

## Scope

The third engine slice of the workspace-graph campaign, and the first caller of
the overlay store change 0012 deliberately shipped unwired. Four deliverables:

1. **A new package `internal/wsresolve`** holding the D3 ladder: one exported
   entry point that runs a full resolution pass over a workspace.
2. **The four-rung ladder**, in the frozen order, over each available member's
   unresolved edges, writing cross-edges / ambiguity records through the
   `internal/overlay` API.
3. **Member-over-dep precedence** — detection, suppression records, and the
   re-pointing of the suppressed member's affected edges at the owning member.
4. **Five new exported entry points in `internal/graph`** the ladder needs and
   the package does not have: `UnresolvedEdges`, `TierOneEdges`,
   `ProjectDefs`, `MemberMerkleRoot`, and the non-creating `OpenExisting`.

No CLI verb, no MCP surface, no query path, no change to any member's
`graph.db` schema or contents.

## Codebase facts this design is built on

Verified against the working tree at `origin/main` (2026-08-19):

- `internal/overlay` exists and is **unwired** — no non-test caller in tree.
  Its API is exactly as specced in
  `docs/superpowers/specs/2026-08-19-workspace-overlay-store-design.md` §4–§6:
  `Path(wsRoot)`, `Open(path)`, `ReplaceRegistry(*config.Workspace)`,
  `PutCrossEdges([]CrossEdge)`, `PutAmbiguities([]Ambiguity)`,
  `PutSuppressions([]Suppression)`,
  `ReplaceMemberEdges(memberID, edges, ambiguities, suppressions)`,
  `OutEdges/InEdges/MemberEdges/AmbiguitiesFor/Suppressions`,
  `PutStamp(memberID, merkleRoot)` / `Stamp` / `Stamps`.
  `SymKey{Member, File, QName}`; `CrossEdge{Src, Dst, Kind, Provenance,
  Confidence, Line}`; `Ambiguity{Src, RefName, RefNS, Kind, Line, Candidates,
  Count}`; `Suppression{ConsumerMember, Namespace, OwnerMember,
  SuppressedVersion}`.
- **`ReplaceMemberEdges(M, …)` deletes on EITHER end** — every `cross_edges`
  row with `src_member = M` or `dst_member = M`, and every `cross_ambiguities`
  row sourced from M *or naming M among its candidates*, whole (never thinned),
  with its candidate rows. Suppressions are deleted **consumer-side only**.
  Inputs are validated against exactly that scope before the transaction opens.
  This either-end delete is what forces decision 7's write order.
- `internal/config/workspace.go` — `Workspace{Version, Members}`,
  `Member{ID, Root, Namespaces, Deps}`,
  `func (w *Workspace) Resolve(wsRoot string) (present []ResolvedMember,
  missing []string, err error)`, `ResolvedMember{Member, AbsRoot}`. `Resolve`
  still has **no non-test caller** — this change owns its first.
- `internal/engine/rootkind.go` — `DetectRootKind(root) (RootKind, error)`,
  `RootRepo` / `RootWorkspace`; the manifest is stat'ed, never parsed.
- `internal/graph/store.go` — the `edges` VIEW exposes
  `id, src_symbol_id, dst_symbol_id, dst_name, dst_qualifier, dst_ns, kind,
  confidence, line, src_file`. **Unresolved ⇔ `dst_symbol_id = 0`.**
  `dst_ns` is D3's namespace hint H.
- **`dst_ns` is populated for all four languages, but by two different
  mechanisms, and Go's has a hole.** `PutFile` builds a per-file
  `bind[target] → normalizeHint(source, target, path)` map from the file's
  import deps — gated on `d.Source != ""` (`store.go:341-345`) — and uses it
  whenever a call carries no Go alias hint (`store.go:352-355`).
  `normalizeHint` (`store.go:1157`) resolves relative TS specifiers against the
  importing file's dir, strips a PHP `use`-path's bound final segment, and
  passes Go paths / Python modules / bare specifiers through verbatim.
  **PHP / Python / TS get H from `bind`; Go gets it only from
  `RawCall.NsHint`**, set from the alias table at
  `internal/adapter/golang/golang.go:160-161` and carried at `:190`. The Go
  adapter's `addDep` (`golang.go:59-67`) sets `Target` only and never `Source`,
  so `bind` is always empty for a Go file. **Consequence: Go
  `extends`/`implements` edges carry no H at all** (the deps loop reads
  `hint := bind[d.Target]`), so Go interface embedding is a rung-1 blind spot
  that falls to rung 2/3. Not a defect this slice fixes — adapter coverage is
  out of scope — but the ladder must not be written as if H were universal.
- `internal/graph/store.go:1130` — `nsMatch(candNS, hint)` is unexported and is
  **bidirectional** suffix alignment (`HasSuffix(hint, sep+candNS)` **or**
  `HasSuffix(candNS, sep+hint)` over `/ . \`), plus a TS-extension prefix arm
  matching `candNS == hint + ".ts" | "/index.ts" | …`. D3 rung 1 needs a
  different shape (a member's *declared root* namespace as a boundary
  **prefix** of H), so `nsMatch` is not reusable here. See decision 4.
- `internal/graph/store.go:136-174` — **`graph.Open` is destructive.** It
  creates the file when absent and, on `PRAGMA user_version != schemaVersion`,
  `os.Remove`s it and recreates it empty. `OpenRaw(path)` (`store.go:180`) is
  the non-mutating alternative. A member's index lives at
  `<AbsRoot>/.codeindex/graph.db` (`internal/query/query.go:27`,
  `internal/engine/artifact.go:66`). See §6 and decision 16.
- The `edges` VIEW exposes `src_file` but not `src_name`/`src_parent`, so
  `UnresolvedEdges` is a join from `edges` to `symbols` on `src_symbol_id`,
  not a single-table select. The `symbols` VIEW (`store.go:59-66`) does expose
  `tier` and `namespace`, so `ProjectDefs` and `TierOneEdges` are buildable.
- `internal/overlay/edges.go:36` currently reads
  `// RefNS is D3's namespace hint: empty for a rung-2 bare name.` — the
  literal-rung-2 invariant. Decision 6 breaks it (a rung-3 record may now carry
  a non-empty `RefNS` after a rung-1 miss), so that comment is updated by this
  slice. The stored shape does not change; only the comment.
- `internal/workspace/namespaces.go:120-141` — `phpNamespaces` emits **both**
  the composer `name` (`symfony/symfony`) and every PSR-4 autoload key
  (`Symfony\Component\`). §5's suppression match depends on this: it compares a
  `depfiles.namespace` against declared member namespaces, and only the PSR-4
  form shares a string shape with what a depmap records.
- `openspec/changes/workspace-graph/tasks.md` still shows `- [ ] 3.2`
  unchecked although change 0012 is `done`. Bookkeeping, corrected by this
  slice's builder alongside ticking §3.3.
- `internal/graph/types.go:83` — `func (s Symbol) QName() string` already
  exists and is `Parent + "." + Name`, else `Name`. `Caller.QName()` is the
  same rule. Decision 3 adopts it rather than minting a second convention.
- `internal/graph/store.go:656` — `Definitions(name, parent)` does **not**
  filter on `tier` and does not return `Tier` or `Namespace`. It cannot answer
  "does N resolve uniquely inside member M's own code". See decision 5.
- File-level import edges are inserted with `src_symbol_id = 0`
  (`store.go:368-383`, "imports (file-level, `src_symbol_id=0`)"), and Go
  import paths "stay unresolved verbatim — packages are not symbols". Such a
  row is unresolved on both ends. See decision 2.
- `internal/graph/depmaps.go` — `AttachMap(mapPath, prefix)` materializes a
  depmap's symbols as `tier = 1` under the map's `namespace`, and records one
  `depfiles` row per covered file carrying `(path, namespace, version,
  maphash, curhash, size, mtime, modified)`. `DepFiles() ([]DepFileState,
  error)` reads them back — the only in-tree source of a vendored copy's
  version string, and therefore of `Suppression.SuppressedVersion`.
- `internal/workspace/namespaces.go` — declared member namespaces are Go module
  paths (`github.com/prometheus/client_golang`), package.json names
  (`@nestjs/common`), composer names (`symfony/symfony`) **plus PSR-4 keys,
  which keep their trailing backslash** (`Symfony\Component\`), and Python
  top-level module names (`flask`). Decision 4's matcher must tolerate the
  trailing separator.
- `internal/graph/store.go` — the `merkle` table is `(path, hash, size, mtime)`,
  **per file**; nothing in the tree aggregates a repo-level root (0012's
  decision 9 recorded this gap). See decision 9.
- `internal/depmap` is the flat sibling-package precedent
  (`internal/{graph,depmap,overlay}`), each importing `internal/graph` rather
  than growing it.
- Test gate: `go test -tags nollama ./...` (pinned in `.docket.local.yml`).
  Plain `go test ./...` fails 10 packages on every ref for missing vendored
  llama.cpp headers — environmental, not a regression.

## Design

### 1. Package, entry point, and surface

A new flat package `internal/wsresolve`, sibling to `internal/graph`,
`internal/depmap`, and `internal/overlay`:

```
internal/wsresolve/
  wsresolve.go   # Resolve: the full-workspace pass and its orchestration
  ladder.go      # the four rungs over one member's unresolved edges
  nsprefix.go    # the namespace-boundary prefix matcher (decision 4)
  suppress.go    # member-over-dep precedence
  *_test.go
```

```go
// Stats reports what one pass produced, for callers that surface progress.
type Stats struct {
    MembersResolved int // present members whose contribution was rewritten
    MembersUnavailable int // declared members with no usable index, untouched
    CrossEdges      int
    Ambiguities     int
    Suppressions    int
}

// Resolve runs a full cross-repo resolution pass over the workspace rooted at
// wsRoot: it loads the manifest, mirrors it into the overlay registry, runs
// the D3 ladder over every available member's unresolved edges, and writes the
// resulting cross-edges, ambiguity records, suppression records, and member
// stamps. It is the first caller of internal/overlay.
func Resolve(wsRoot string) (Stats, error)
```

**No CLI verb.** D5 names `init-workspace` and `workspace-status` as the only
new verbs, and `workspace-status` is §3.4's. `Resolve` therefore ships as an
internal entry point with no in-tree caller, exactly as §3.1 shipped
`config.Resolve` and §3.2 shipped the whole overlay package. §3.4's freshen
path is its first caller. See decision 1.

`Resolve` calls `engine.DetectRootKind(wsRoot)` first and returns an error
naming the path when the root is `RootRepo`: a resolution pass over a
non-workspace is a programming error, and the manifest load that follows would
otherwise report it as a bare `fs.ErrNotExist`.

### 2. Candidate edges — what the ladder reads

Two new exported readers on `*graph.Store` (in `internal/graph`, because both
are single queries over its own schema and its `strs`-interned views):

```go
// UnresolvedEdge is one edge whose target was never bound to a symbol:
// dst_symbol_id = 0. Src identifies the owning symbol; DstName/DstQualifier/
// DstNS are the resolver's surviving hints (DstNS is the import-derived
// namespace hint).
type UnresolvedEdge struct {
    SrcFile      string
    SrcName      string
    SrcParent    string
    DstName      string
    DstQualifier string
    DstNS        string
    Kind         string
    Line         int
}

// UnresolvedEdges returns every edge with dst_symbol_id = 0 whose source is a
// real symbol (src_symbol_id != 0), in deterministic order. File-level import
// edges carry src_symbol_id = 0 and are excluded: they have no source symbol,
// so no stable key can name their source end.
func (s *Store) UnresolvedEdges() ([]UnresolvedEdge, error)

// ProjectDefs returns the TIER-0 definitions of name (optionally restricted to
// parent), in deterministic order. Tier-1 rows — symbols materialized from an
// attached dependency map — are excluded: a workspace member's own code is the
// live editable source, and admitting a vendored snapshot as a cross-repo
// resolution target is precisely what D3's member-over-dep precedence forbids.
func (s *Store) ProjectDefs(name, parent string) ([]Symbol, error)
```

`ProjectDefs` returns `Symbol`, so `Symbol.QName()` supplies the stable key's
`QName` at both ends (decision 3). `UnresolvedEdges` orders by
`(src_file, src_name, src_parent, dst_name, kind, line)`; `ProjectDefs` by
`(file, start_line, name)` — the project's rebuild-determinism rule.

The ladder additionally reads, per available member, the **suppressed-namespace
candidates** described in §5 — edges that are *resolved* today, but only to a
tier-1 vendored symbol in a namespace a workspace member owns.

### 3. The ladder (frozen order, per unresolved edge in member S)

Let `N = DstName`, `Q = DstQualifier`, `H = DstNS`, and let
`others = available members other than S, in manifest order`. Resolution inside S
already ran and already lost — that is what made the edge unresolved — so S is
never a candidate.

For each member X, `defs(X) = X.store.ProjectDefs(N, Q)`, retried with
`Q = ""` when the qualified form returns nothing (the qualifier is a lexical
hint the intra-repo resolver already treats as narrowing-only; a cross-repo
lookup must not be stricter than the local one it is extending).

1. **Import-mediated (exact).** If `H != ""` and exactly one member
   `M ∈ others` satisfies `nsPrefix(ns, H)` for one of M's declared namespaces
   (decision 4), and `len(defs(M)) == 1` → write
   `CrossEdge{Src: key(S, edge.Src), Dst: key(M, defs(M)[0]), Kind: edge.Kind,
   Provenance: "cross_repo_import", Confidence: "exact", Line: edge.Line}`.
   The only rung that produces `exact`.
2. **Unique bare name (inferred).** Otherwise, let
   `hits = {X ∈ others : len(defs(X)) == 1}` — members in which N resolves
   uniquely. If `len(hits) == 1` → the same `CrossEdge` with
   `Provenance: "cross_repo_name"`, `Confidence: "inferred"`.
3. **Ambiguous.** Otherwise, let `cands` be every `(X, d)` for `X ∈ others`,
   `d ∈ defs(X)`. If `len(cands) >= 2` → write
   `Ambiguity{Src: key(S, edge.Src), RefName: N, RefNS: H, Kind: edge.Kind,
   Line: edge.Line, Candidates: ordered(cands), Count: len(cands)}`, where
   `ordered` puts the candidates of the single member named in S's manifest
   `deps` first (only when `deps` names **exactly one** candidate member — the
   frozen tiebreaker), then the remaining candidates in manifest order, then
   within a member by `(file, start_line, name)`.
4. **Unresolved.** Otherwise (`len(cands) == 0`, or the single remaining
   candidate member offers several definitions and no tiebreaker applies —
   which rung 3 already covers) the edge is left exactly as it is today: no
   overlay row of any kind is written for it.

`Count` is always `len(Candidates)`: this slice never truncates, so recording a
larger number would be a lie the overlay's own validation permits.

Because of the rung-2 reading above, a rung-3 record may now carry a non-empty
`RefNS`. `internal/overlay/edges.go:36`'s comment
("empty for a rung-2 bare name") is updated to say `RefNS` is the hint as
recorded, empty when the reference carried none — a comment change only, no
schema or behavior change in `internal/overlay`.

**Rung 2 is reached even when H is non-empty.** D3 phrases rung 2 as "no H";
this design reads that as "no rung-1 hit". The frozen order is untouched — rung
1 still runs first and still wins — and the reading only settles what happens
on a rung-1 *miss* when H happens to be non-empty. The literal reading is
**non-monotonic**, which is visible from the frozen text alone: rung 3's guard
("N resolves in multiple members") carries no H condition, so a rung-1 miss
with non-empty H still records an *ambiguous* result when N resolves in ≥2
members, while the strictly better-evidenced case — N resolving in exactly one
other member — would be dropped to rung 4 unresolved. A ladder that records the
weaker answer and discards the stronger one is not the frozen order applied; it
is a gap in its phrasing. See decision 6.

### 4. Namespace-boundary prefix matching

```go
// nsPrefix reports whether the import hint h falls inside the declared member
// namespace ns, matching only on a namespace boundary: exact equality, or h
// continuing after ns with one of the separators "/", "\", ".". Trailing
// separators on ns are trimmed first, because PSR-4 autoload keys are declared
// with one ("Symfony\Component\").
func nsPrefix(ns, h string) bool
```

Match is case-sensitive and purely lexical — no toolchain resolution, matching
the epistemics rule that confidence classes are resolver-visibility claims.
`github.com/acme/auth` matches `github.com/acme/auth/token` but not
`github.com/acme/authz`; `Acme\Auth` matches `Acme\Auth\Token`; `flask`
matches `flask.helpers` but not `flasky`.

**One function, two callers.** `nsPrefix` is used by rung 1 *and* by §5's
suppression detection. Per the `one-invariant-many-sites-drifts` learning
(harvested from 0012, whose defect was exactly this shape), the boundary rule
is defined once and both sites call it; neither restates it, and a test asserts
the two sites agree on a shared table of cases.

### 5. Member-over-dep precedence

For each available member C, read `C.store.DepFiles()`. Group by
`DepFileState.Namespace`; for each attached namespace `A`, find the members
`O ∈ present, O != C` such that `nsPrefix(o_ns, A)` for one of O's declared
namespaces — i.e. the vendored namespace falls inside a namespace a live member
owns. The two strings are of different provenance — `A` is a
`depfiles.namespace` (what the depmap recorded), `o_ns` is a manifest-declared
namespace — and they align only because `internal/workspace/namespaces.go`
emits the shapes a depmap uses: Go module paths, and for PHP **both** the
composer name and the PSR-4 roots. A member declaring only a composer name
whose vendored copy is recorded under a PSR-4 root will not match, and is
therefore not suppressed; that is a discovery gap (change 0010's territory),
not a resolution one, and this slice's behavior on it — record nothing, resolve
nothing — is the safe direction. If exactly one such O exists, the member wins:

- Record `Suppression{ConsumerMember: C.ID, Namespace: A, OwnerMember: O.ID,
  SuppressedVersion: <the version on C's depfiles rows for A, "" if unset>}`.
  When C's rows disagree on version (possible across re-attachments), the
  lexicographically smallest non-empty value is taken, deterministically.
- **Re-point C's affected edges.** Every edge in C that resolved to a tier-1
  symbol whose namespace matches A becomes a ladder candidate, entering at
  rung 1 with `H = A` — which, since exactly one member owns A, selects O
  directly. The resulting cross-edge is `cross_repo_import` / `exact` when N
  resolves uniquely in O, and otherwise falls through rungs 2–4 unchanged.

If **more than one** member could own A, nothing is suppressed and nothing is
recorded: "the member wins" presupposes an unambiguous member, and inventing a
winner is a silent lie about which code the agent would edit.

Re-pointing needs one more `internal/graph` reader, symmetric with
`UnresolvedEdges`:

```go
// TierOneEdges returns every edge resolved to a tier-1 symbol, with the
// resolved target's namespace, in deterministic order. Same source-symbol
// requirement as UnresolvedEdges.
func (s *Store) TierOneEdges() ([]TierOneEdge, error)
```

`TierOneEdge` is `UnresolvedEdge` plus `DstNamespace string` — the namespace of
the tier-1 symbol the edge currently points at, which is what the suppression
set is matched against.

**Why re-pointing belongs here and not in §4.** These edges are not "today's
unresolved edges", so admitting them widens D3's stated candidate set — the one
place this design does, named rather than smuggled. It is still this slice's
work for a narrow reason: producing the cross-edge into O requires resolving N
inside O against O's own index, which is *the ladder*. Making §4 do it would
either duplicate the ladder in the query layer or make every query re-resolve
at read time, which D2 explicitly rejected. See decision 8.

Suppression does **not** mutate C's `graph.db`: the tier-1 rows stay, the
intra-repo edge stays, and the overlay simply also carries the cross-edge into
O. Per-repo indexes remain individually buildable and artifact-importable (D2).

**Recorded hand-off to §4.1 — double-edge suppression at read time.** Because
C's intra-repo tier-1 edge survives alongside the new cross-edge, a union-graph
query over C would otherwise count the same call twice: once at the vendored
snapshot, once at the owning member. §4.1 must read `dep_suppressions` and
suppress intra-repo edges whose resolved target is a tier-1 symbol in a
suppressed namespace. This slice writes the record that makes that filter
possible and does not implement the filter — nothing reads the overlay yet, so
no double-count is observable before §4.1. This is a *stated* obligation, not a
side effect: a §4.1 that ignores `dep_suppressions` is wrong.

### 6. Orchestration and write order

```
Resolve(wsRoot):
  1. DetectRootKind(wsRoot) must be RootWorkspace
  2. ws := config.LoadWorkspace(wsRoot)
  3. present, missing := ws.Resolve(wsRoot)
  4. ov := overlay.Open(overlay.Path(wsRoot));  defer ov.Close()
  5. ov.ReplaceRegistry(ws)                       // mirror the manifest as-built
  6. for each present member: graph.OpenExisting(<AbsRoot>/.codeindex/graph.db)
     — NEVER graph.Open; defer close. A member whose index is absent, at the
     wrong schema version, or unopenable is UNAVAILABLE and is skipped
     (present members minus these = the AVAILABLE members)
  7. derive suppressions (§5) over all available members
  8. for each available member S (manifest order): run the ladder (§3) over
     S's UnresolvedEdges + its suppressed TierOneEdges, accumulating records
     in memory
  9. for each available member S: ov.ReplaceMemberEdges(S.ID, nil, nil, nil)
 10. ov.PutCrossEdges(all edges); ov.PutAmbiguities(all ambiguities);
     ov.PutSuppressions(all suppressions)
 11. for each available member S: ov.PutStamp(S.ID, memberMerkleRoot(S))
```

**Steps 9 and 10 are split deliberately, and this is the load-bearing
orchestration decision.** `ReplaceMemberEdges(M, …)` deletes on *either* end,
so the naive loop — derive M's records, write them with
`ReplaceMemberEdges(M, …)`, move to the next member — has the call for M₂
deleting the `S₁ → M₂` edges the call for M₁ just wrote. Clearing every present
member first (step 9, with empty inputs, which the API's own validation
accepts trivially) and then writing the whole derived set with the
non-deleting `Put*` calls (step 10) is the only composition of the existing API
that is correct for a full pass. No new overlay method is added. See decision 7.

**The pass never opens a member index through `graph.Open`.** `graph.Open`
creates the file when absent and deletes-and-recreates it on a version
mismatch, so using it here would let a resolution pass create or wipe a
member's index — violating this design's own "no change to a member's
`graph.db`" invariant while every test still passed. `OpenExisting` is mandatory,
and a present member whose index is absent, unopenable, or at a
`FileSchemaVersion` other than `graph.SchemaVersion()` is **treated exactly
like a missing member**: it contributes no candidates, is not a candidate
target, gets no stamp, and its overlay rows are not cleared. Its id is counted
in `Stats.MembersUnavailable`, and the pass returns no error — an unbuilt member is
a runtime condition (the same class as an absent one), not an authoring fault.
Since `OpenRaw` returns a bare `*sql.DB` and the new readers are `*Store`
methods, `internal/graph` gains one more constructor:

```go
// OpenExisting opens an existing index read-only in the sense that matters: it
// never creates the file and never deletes-and-rebuilds on a version mismatch.
// Absence returns an error wrapping fs.ErrNotExist; a mismatch returns a
// version error naming both versions; an unreadable or corrupt file returns
// the underlying open/pragma error. Open remains the build path.
func OpenExisting(path string) (*Store, error)
```

`wsresolve` uses `OpenExisting` exclusively and treats **every** error it
returns — absent, version mismatch, or unopenable/corrupt — as "member
unavailable" per the paragraph above.

**Missing members are left alone.** Their overlay rows are not re-derivable
(the ladder needs their `graph.db` to map a name to a stable key), and D2/D4
require a workspace to keep answering while a member is unavailable. Step 9
does collaterally delete a missing member's rows that are *incident to a
present member* — unavoidable, since a present member's whole contribution is
being rewritten, and correct, since such an edge could not be re-derived at
this pass anyway. Rows joining two missing members survive untouched.

**The pass is not one transaction, and does not need to be.** Each overlay call
is individually atomic; the overlay holds no primary data and every row is
re-derivable by re-running the pass (D2). Stamps are written **last** (step 11),
so a pass that dies part-way leaves the affected members stampless and §3.4's
stamp gate re-resolves them — the crash-safety property is carried by the stamp
ordering, not by a transaction the API does not offer.

### 7. Stamps

```go
// MemberMerkleRoot folds a member index's content state into one repo-level
// token: sha256 over "path\x00hash\n" for every merkle row in path order,
// then a "\x01depfiles\n" separator, then
// "path\x00namespace\x00version\x00curhash\n" for every depfiles row in path
// order. The value is opaque to the overlay, which compares it for equality
// only.
func (s *Store) MemberMerkleRoot() (string, error)
```

The tree has no repo-level merkle aggregation (0012 decision 9 recorded the
gap), and the overlay treats `MerkleRoot` as an opaque token. This slice needs
*a* real value, because a placeholder would make §3.4's first stamp comparison
meaningless.

**The `depfiles` half is not padding.** `merkle` covers project files only —
`AttachMap` writes `depfiles`, never `merkle`. Since this slice's suppression
and re-pointing output is derived from `depfiles` (§5), a merkle-only fold
would leave the stamp unchanged across a re-attach or a dependency version
bump, and §3.4's gate would then skip a member whose overlay contribution had
in fact moved. Folding both is the smallest fix, and it keeps the stamp a
faithful summary of everything this pass read.

`namespace` and `version` are folded alongside `curhash` for the same reason
one level in: `Suppression.SuppressedVersion` is read from `depfiles.version`,
not from file content, so a re-attach that moves the version string while every
covered file's bytes stay identical changes this slice's output and would
otherwise leave the stamp untouched.

This is **not** a policy commitment: §3.4 owns staleness policy and may replace
the fold. The cost of replacing it is honest and small — every stamp mismatches
once and every member is re-resolved on the next pass. (It is *not* an overlay
schema-version bump: the fold lives in `internal/graph`, and the overlay stores
whatever string it is handed.) See decision 9.

### 8. Tests (the §3.5 bars that belong to this slice)

Fixtures are small hand-built member indexes created through `graph.Open` +
`PutFile` in a temp dir — no repo checkouts, no network.

- **Ladder order** — one fixture workspace exercising each rung and each
  fall-through in turn, asserting provenance/confidence per edge:
  rung 1 hit; rung 1 miss on hint matching two members → falls to 2/3;
  rung 1 miss on N ambiguous inside the hinted member → falls to 2/3;
  rung 2 unique hit; rung 3 with `Count` and candidate order, with and without
  the `deps` tiebreaker, and with `deps` naming two candidates (no reorder);
  rung 4 leaves the overlay empty for that edge. Plus a case asserting rung 1
  beats rung 2 when both could fire, and that a re-run in the other member
  order produces byte-identical overlay content.
- **Stable-key survival across member rebuild** — resolve; rebuild one member's
  `graph.db` from scratch so its symbol rowids are renumbered; assert every
  cross-edge's `SymKey` still names a live symbol in the rebuilt member, and
  that a second `Resolve` produces the identical overlay row set.
- **Namespace-boundary matcher** — a shared table over all four languages
  including the PSR-4 trailing-separator case, the `authz`/`flasky`
  near-miss cases, and asserting rung 1 and suppression detection agree on it.
- **Suppression** — a member vendoring another member's namespace: the record
  carries the right consumer/owner/version; the affected tier-1 edge is
  re-pointed at the owning member; two possible owners suppress nothing; the
  consumer's own `graph.db` is byte-unchanged.
- **Missing member** — a declared member absent from disk contributes nothing,
  gets no stamp, and its rows joining another missing member survive a pass.
- **Whole-pass write order** — the regression this design's decision 7 exists
  for: a workspace where member 1 sources an edge into member 2, asserting the
  edge is present after a full pass (it is absent if steps 9/10 are collapsed
  back into a per-member `ReplaceMemberEdges`).
- **Idempotence** — two consecutive passes over an unchanged workspace produce
  identical overlay content and identical stamps except `ResolvedAt`.
- **Member indexes are never written** — snapshot every member `graph.db`'s
  bytes before a pass and assert them unchanged after, including a member whose
  index is absent (it must stay absent, not be created) and one at a wrong
  `user_version` (it must stay byte-identical, not be rebuilt). This is the
  regression test for the `graph.Open` hazard decision 16 exists for.
- **Stamp covers depmap state** — re-attach a depmap at a new version in one
  member and assert `MemberMerkleRoot` changes even though no project file did.

**The fourth §3.5 bar is deferred, not dropped.** tasks.md §3.5 lists four unit
bars: ladder order, stable-key survival, stamp gating, and
*single-member-workspace ≡ single-repo*. Stamp gating is §3.4's (this slice
writes stamps and reads none). The single-member equivalence bar is **§4.2's**:
it asserts that a workspace answer equals a single-repo answer, and with no
query path reading the overlay there is no workspace answer to compare. What
*is* assertable here — that a one-member workspace produces an empty
cross-edge/ambiguity set, since every rung requires a member other than S — is
included in the ladder-order fixture as the degenerate case.

Gate: `go test -tags nollama ./...` green.

## Acceptance checklist

- [ ] `internal/wsresolve.Resolve(wsRoot) (Stats, error)` exists, is the only
      exported entry point, and has no in-tree caller.
- [ ] No new CLI verb, no MCP change, no plugin-note change.
- [ ] `graph.UnresolvedEdges`, `graph.ProjectDefs`, `graph.TierOneEdges`,
      `graph.MemberMerkleRoot`, `graph.OpenExisting` added; no member
      `graph.db` schema change and no `graph.schemaVersion` bump.
- [ ] No pass creates, truncates, or otherwise writes any member `graph.db`;
      `graph.Open` appears nowhere in `internal/wsresolve`'s NON-TEST code
      (fixtures legitimately build indexes with it; the wrong-version fixture
      uses `OpenRaw` plus a `PRAGMA user_version` write).
- [ ] A present member with an absent, wrong-version, or unopenable index is
      handled as unavailable, counted in `Stats.MembersUnavailable`, and does
      not fail the pass.
- [ ] `MemberMerkleRoot` changes when `depfiles` changes with no project-file
      change.
- [ ] `internal/overlay/edges.go`'s `RefNS` comment updated; no other change to
      `internal/overlay`.
- [ ] `openspec/changes/workspace-graph/tasks.md` §3.2 and §3.3 both ticked.
- [ ] The four rungs fire in the frozen order; `exact` is produced by rung 1
      only; `cross_repo_import` / `cross_repo_name` are the only provenance
      values written.
- [ ] Ambiguity records are never thinned; `Count == len(Candidates)`; the
      `deps` tiebreaker orders first only when it names exactly one candidate
      member.
- [ ] Suppressions are consumer-scoped, recorded only when exactly one member
      owns the namespace, and carry the vendored version from `depfiles`.
- [ ] A full pass writes every cross-edge regardless of member order
      (decision 7's regression test passes).
- [ ] Missing members' overlay rows and stamps are not rewritten.
- [ ] Stamps are written last, one per resolved available member.
- [ ] `graph.Confidence` is untouched; the overlay stores `exact`/`inferred`.
- [ ] `go test -tags nollama ./...` green.

## Out of scope

- Workspace freshen policy, stamp-**gated** incremental re-resolution, repo-level
  merkle policy, and the `workspace-status` verb (§3.4). This slice writes
  stamps; it never reads one to decide whether to skip work.
- Union-graph query paths, the `repo` field, anchor prefixes, coverage clauses
  (§4.x). Nothing reads the overlay yet. Two obligations are **handed to
  §4.1** rather than met here: reconciling `exact`/`inferred` with
  `graph.Confidence` (the campaign's pre-existing recorded hand-off), and
  filtering the consumer-side intra-repo tier-1 edge a suppression overrides
  (§5's recorded hand-off, new in this slice).
- The §3.5 single-member-workspace ≡ single-repo bar, which needs a query path
  (§4.2). See decision 16.
- Adapter coverage gaps the ladder inherits — notably that Go
  `extends`/`implements` edges carry no namespace hint, so Go interface
  embedding never reaches rung 1.
- The evidence gate run (§5.x) and member-discovery changes (change 0010).
- Any change to a member's own `graph.db`, including the tier-1 rows a
  suppression overrides.

## Assumptions

Every decision an interactive brainstorm would have raised, the default taken,
and the alternatives rejected. No human answered any of these.

1. **New package `internal/wsresolve`, not a file in `internal/overlay` or
   `internal/engine`.** Chosen: a flat sibling, matching the
   `graph`/`depmap`/`overlay` precedent. Rejected: putting the ladder in
   `internal/overlay` — 0012 scoped that package to storage and its spec says
   so explicitly, and a store that resolves is the copy-merge shape D2 rejects.
   Rejected: `internal/engine` — engine owns single-repo build/patch and would
   acquire an `overlay` dependency for a path no single-repo run takes.

2. **Candidate edges exclude file-level import edges (`src_symbol_id = 0`).**
   D3 says "candidates are exactly today's unresolved edges", and Go import
   edges are unresolved by design. But an overlay row is keyed by a `SymKey` on
   both ends, and a file-level import has no source symbol — there is no stable
   key to write. Rejected: synthesizing a file-scoped pseudo-key — it would put
   rows in `cross_edges` that no query path can map back to a symbol, and the
   overlay has no representation for a file-level endpoint. The imports these
   edges represent are exactly the hints H that rung 1 consumes, so their
   information is used, not lost.

3. **`SymKey.QName` = `graph.Symbol.QName()` (`Parent.Name`, else `Name`).**
   Chosen because the convention already exists and is already what
   `Caller.QName()` renders. Rejected: a namespace-qualified key — the
   namespace is a per-language derived value (`graph.DeriveNamespace`) that
   changes shape by extension, so it would make the key non-portable across a
   member's languages, and 0012's schema already carries `member` and `file`
   as the disambiguating columns.

4. **Rung 1's match is a boundary *prefix* of a declared member namespace, via
   a new `nsPrefix`, not `graph.nsMatch`.** `nsMatch` is unexported and is
   suffix alignment for intra-repo resolution; declared member namespaces are
   *roots* (`github.com/acme/auth`) while H is a full import path
   (`github.com/acme/auth/token`), which is the prefix direction. Separators
   `/ \ .`, trailing separators trimmed for PSR-4. Rejected: exporting and
   reusing `nsMatch` — its either-direction suffix rule would match
   `github.com/other/auth` against a member declaring `auth`, manufacturing
   `exact` cross-edges from a coincidence. Rejected: per-language separator
   tables — the manifest does not record a member's language, and the union of
   three separators has no observed collision across the four namespace shapes
   the scanner emits.

5. **Cross-repo candidates are tier-0 only, via a new `ProjectDefs`.**
   `Definitions` does not filter tier. Admitting tier-1 would let a member's
   vendored copy of a third library become a cross-repo target, which is the
   exact failure member-over-dep precedence exists to prevent, one level out.
   Rejected: filtering in `wsresolve` after calling `Definitions` — the method
   returns neither `Tier` nor `Namespace`, so the filter is not expressible at
   the caller.

6. **Rung 2 is read as "no rung-1 hit", not "H is literally empty".** The
   *frozen order is untouched* — rung 1 still runs first and still wins; this
   only settles what happens on a rung-1 *miss* when H happens to be non-empty.
   The literal reading is non-monotonic on its own terms: rung 3's guard
   carries no H condition, so a rung-1 miss with non-empty H still records an
   ambiguous answer when N resolves in ≥2 members, while the better-evidenced
   unique-hit case would be discarded as unresolved. Recording the weaker
   answer and dropping the stronger one is a gap in the phrasing, not a rule.
   Rejected: the literal reading. Rejected: dropping H from the ambiguity
   record on a rung-1 miss — the hint is real and recording it costs nothing;
   the consequence is that `internal/overlay/edges.go:36`'s comment ("empty for
   a rung-2 bare name") must be corrected by this slice, which §3 states.

7. **A full pass clears all available members first (empty
   `ReplaceMemberEdges`), then writes the whole derived set with `Put*`.**
   Forced by `ReplaceMemberEdges`' either-end delete, which makes any
   per-member derive-then-write loop order-dependent and lossy. Rejected:
   deleting `workspace.db` and rebuilding — it destroys missing members' rows,
   which are not re-derivable while those members are off disk, contradicting
   D2/D4's "must still answer while a member is unavailable". Rejected: adding
   a `ReplaceAll` method to `internal/overlay` — it would give the whole pass
   one transaction, which is genuinely nicer, but it grows a merged package's
   API for a property the stamp-last ordering already delivers; if §3.4 finds
   it needs one, adding it then is a strictly better-informed call.

8. **Member-over-dep precedence re-points affected tier-1 edges, not just
   records a suppression.** This widens D3's stated candidate set beyond
   "today's unresolved edges" — the one place this design does so, and it is
   named rather than smuggled. The reason it is §3.3's and not §4's is
   mechanical, not rhetorical: producing the cross-edge into the owning member
   means resolving N inside that member's index, which is the ladder itself, so
   deferring it would either duplicate the ladder in the query layer or force
   query-time re-resolution — the option D2 rejected by name. Rejected:
   record-only, which makes the frozen "the member wins" observably inert in
   the slice that owns it. Rejected: deleting the tier-1 rows from the
   consumer's `graph.db` — D2 requires per-repo indexes to stay untouched and
   individually buildable. **Acknowledged cost:** the consumer's intra-repo
   tier-1 edge survives, so §4.1 still has to read `dep_suppressions` and
   filter it out to avoid double-counting. That obligation is recorded in §5
   rather than waved away; re-pointing removes the resolution work from §4.1,
   not the filtering work.

9. **`MemberMerkleRoot` folds the `merkle` rows *and* the `depfiles`
   `curhash` rows, and lives in `internal/graph`.** Rejected: writing an empty
   or constant stamp — §3.4's first comparison would then be against a value
   that never changes, and the gate would silently never fire. Rejected: a
   merkle-only fold — `AttachMap` writes `depfiles`, never `merkle`, so a
   re-attach or dependency version bump would change this slice's suppression
   and re-pointing output while leaving the stamp identical, and §3.4's gate
   would skip a member whose overlay contribution had moved. Rejected:
   designing the aggregation in `internal/merkle` with a policy story — that is
   §3.4's task. Replacing the fold later costs one universal stamp mismatch and
   a full re-resolve; it is *not* an overlay schema bump, since the overlay
   stores whatever opaque string it is handed.

10. **`deps` tiebreaker applies only when `deps` names exactly one candidate
    member**, per D3's literal wording; when it names two or more, candidate
    order falls back to manifest order with no reordering. Rejected:
    ordering all deps-named candidates first — D3 says "names exactly one", and
    a partial ordering would make the first candidate look privileged without
    a rule behind it.

11. **Suppression is recorded only when exactly one member owns the vendored
    namespace**, and nothing is suppressed otherwise. Rejected: picking the
    longest-matching owner — it manufactures a winner in the one situation
    where "which code would the agent edit" has no answer.

12. **`Resolve` runs a full pass every time it is called and reads no stamp.**
    Stamp-*gated* re-resolution is §3.4's, stated in this change's own
    out-of-scope. Rejected: a `since`/incremental parameter — it would be an
    unexercised API shaped by a policy that does not exist yet.

13. **The qualifier `Q` is used as a narrowing retry, not a hard filter** — a
    cross-repo lookup with `Q` that finds nothing retries with `Q = ""`.
    Mirrors `graph.resolve`'s own ladder, where qualified steps precede plain
    ones and fall through totally. Rejected: requiring `Q` to match — it would
    drop every method call whose receiver type lives in a third member.

14. **Dependency state.** `depends_on: [12]` is satisfied — 0012 is `done` and
    archived. `related: [9, 12]` both merged. Nothing in this design is
    designed ahead of an unmet dependency.

15. **Member indexes are opened with a new non-creating `graph.OpenExisting`,
    never `graph.Open`.** `graph.Open` creates an absent file and
    deletes-and-recreates a version-mismatched one, so a resolution pass built
    on it would violate this design's own "never touch a member's `graph.db`"
    invariant while every functional test still passed. An absent or
    wrong-version index is treated as an unavailable member — the same class as
    a root missing from disk, which `config.Resolve` already reports as a
    runtime condition rather than an error. Rejected: `OpenRaw` alone — it
    returns a bare `*sql.DB`, and the new readers are `*Store` methods.
    Rejected: erroring the whole pass on one unbuilt member — D2/D4 require a
    workspace to keep answering while a member is unavailable, and "not indexed
    yet" is the most ordinary form of that.

16. **The §3.5 single-member-workspace bar is deferred to §4.2, explicitly.**
    The bar asserts a workspace answer equals a single-repo answer, and no
    query path reads the overlay in this slice. The degenerate part that *is*
    assertable here — a one-member workspace writes no cross-edges — is
    included. Rejected: silently dropping it, which is how a frozen task list
    loses a bar.

17. **Test gate is `go test -tags nollama ./...`.** Plain `go test ./...` fails
    10 packages on every ref for missing vendored llama.cpp headers;
    `.docket.local.yml` pins the tagged form as `finalize.test_command`.

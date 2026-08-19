<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0012 — Workspace overlay store — member registry, cross-edges by stable key, freshness stamps](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0012-workspace-overlay-store.md)**
<!-- docket:backlink:end -->

# Design: Workspace overlay store — registry, cross-edges by stable key, member stamps

Change: 0012 `workspace-overlay-store` · openspec task §3.2 of
`openspec/changes/workspace-graph/tasks.md` · frozen design
`openspec/changes/workspace-graph/design.md` (D2 storage; schema shaped to
receive D3's ladder output).

## Scope

The second engine slice of the workspace-graph campaign, and the storage
substrate every later slice sits on. Four deliverables:

1. **A new package `internal/overlay`** holding the overlay store:
   `<workspace-root>/.codeindex/workspace.db`.
2. **Schema + open/create/rebuild** — the three D2 things (member registry,
   cross-repo edges by stable key, per-member freshness stamps) plus the two
   D3 side-records the schema must be able to receive (ambiguity candidates
   with counts, member-over-dep suppressions), under an overlay schema
   version independent of `graph.db`'s.
3. **Registry write API** — mirror the manifest as-built in one transaction.
4. **Stable-key read/write API for cross-edges + side-records, and the stamp
   read/write API.**

Nothing else in the campaign moves. Per-repo `graph.db` files are not touched
— not their schema, not their version, not their build path. No CLI verb, no
MCP surface, no query path.

## Codebase facts this design is built on

Verified against the working tree at `origin/main` (2026-08-19):

- `internal/config/workspace.go` — `Workspace{Version, Members []Member}`,
  `Member{ID, Root, Namespaces, Deps}`; `LoadWorkspace` (shape-only
  validation, never stats a member root), `SaveWorkspace`, and the separate
  `Resolve(wsRoot) (present []ResolvedMember, missing []string, err error)`.
  `LoadWorkspace`/`SaveWorkspace` **do** have production callers —
  `cmd/codeindex/initworkspace.go:38` and `:60`. `Resolve`/`ResolvedMember`
  are the pieces with **no non-test caller** (the only other in-tree mention
  is a comment at `internal/workspace/scan.go:37`); §3.2/§3.4 own their first
  callers. Member ids are restricted to `A-Z a-z 0-9 . _ -` (no `:` or `/`),
  and duplicate ids and duplicate cleaned roots are both load-time errors.
- `validate()` in the same file checks `ID` (empty / charset / duplicate) and
  `Root` (empty / absolute / duplicate-after-`Clean`). It **never inspects
  `Namespaces` or `Deps`** — a manifest with a repeated namespace or a
  repeated dep loads cleanly. See decision 16.
- `internal/workspace/` — discovery only (`Members`, `Namespaces`, `Scan`,
  `Merge`). It parses foreign build files, so it pulls
  `golang.org/x/mod/modfile` and `gopkg.in/yaml.v3` alongside `config`/`os`/
  `encoding/json` — it is not a zero-dependency package. What matters here is
  narrower: it imports neither `database/sql` nor `internal/graph`, and
  nothing in it opens a database.
- `internal/graph/store.go:38` — `const schemaVersion = 9`, with the comment
  "The index is a derived artifact: a version mismatch triggers
  delete-and-rebuild, not migration."
- `internal/graph/store.go:137-173` — `Open(path)`: read `PRAGMA
  user_version`; on mismatch count tables, close, `os.Remove` the file, warn
  on stderr **only if the discarded file had tables** (`codeindex: index
  schema v%d -> v%d, rebuilding`), reopen; then `db.Exec(schema)` and
  `PRAGMA user_version = N`. No WAL, no other pragmas. Companions:
  `OpenRaw` (test hook), `SchemaVersion()`, `FileSchemaVersion(path)`.
- `internal/graph/store.go:41-110` — the schema string: `CREATE TABLE IF NOT
  EXISTS` throughout, string interning via `strs` + `_t` base tables +
  reconstructing VIEWs (v7), and the `key TEXT PRIMARY KEY, value TEXT NOT
  NULL` meta-table shape repeated three times (`depmeta`, `vecmeta`,
  `obs_meta`).
- `internal/graph/store.go:1004` — `RefreshMerkle(tx, FileMeta)`; the
  `merkle` table is `(path, hash, size, mtime)` — **per-file**. There *is* a
  dedicated `internal/merkle` package (`Walk`/`WalkWith`, `Detect`/
  `DetectWith`, `hashBytes` = `sha256.Sum256` at `merkle.go:74`), and merkle
  machinery is threaded through `internal/engine`, `cmd/codeindex`,
  `internal/depmap`, and `internal/search`. But every one of those is
  **per-file**: nothing in the tree aggregates a repo-level root, and no
  accessor returns one. That is the gap decision 9 turns on — §3.4 will have
  to define the aggregation, most likely by extending `internal/merkle`.
- `internal/graph/depmaps.go:81` — `AttachMap(mapPath, prefix)`: the existing
  overlay precedent, `ATTACH DATABASE ? AS depmap` + deferred `DETACH`,
  materializing tier-1 symbols into the repo index.
- `internal/depmap/depmap.go` — the flat sibling-package precedent: a
  dependency-map subsystem in its own `internal/` package that imports
  `internal/graph` rather than growing it.
- `cmd/codeindex/main.go:314` — `func dbPath(root string) string {
  filepath.Join(root, ".codeindex", "graph.db") }`. `internal/query/query.go:27`
  is the one verbatim second copy of that function; `internal/webserver/
  graphstore.go:17`, `internal/readmodel/graph.go:65`, `internal/engine/
  artifact.go:66` and `cmd/codeindex/ingest.go:40` are inline
  `filepath.Join` expressions inside other functions (more again in tests).
  Six non-test sites spell the same join.
- `internal/graph/types.go:57-63` — `Confidence` is `unambiguous` /
  `ambiguous` / `unresolved`. The frozen workspace spec
  (`openspec/changes/workspace-graph/specs/workspace-graph/spec.md:65-71`)
  and design D3 use a **different** vocabulary: `exact` / `inferred` /
  ambiguous / unresolved. See decision 8.
- `go.mod` — module `codeindex`, Go 1.26.5; `github.com/mattn/go-sqlite3`
  already a direct dependency. This slice adds no module dependency.
- Test gate: `go test -tags nollama ./...` (pinned in `.docket.local.yml`).
  Plain `go test ./...` fails 10 packages on every ref for missing vendored
  llama.cpp headers — environmental, not a regression.

## Design

### 1. Package and path (`internal/overlay`)

A new flat package `internal/overlay`, sibling to `internal/graph` and
`internal/depmap`:

```
internal/overlay/
  overlay.go        # Path, Open/Close, schema, version
  registry.go       # member registry write/read
  edges.go          # cross-edges, ambiguities, suppressions
  stamps.go         # per-member freshness stamps
  *_test.go
```

```go
// FileName is the overlay database, relative to the workspace root.
const FileName = ".codeindex/workspace.db"

// Path returns wsRoot's overlay database path.
func Path(wsRoot string) string { return filepath.Join(wsRoot, FileName) }

// Open opens (creating if needed) the overlay database at path.
func Open(path string) (*Store, error)
func (s *Store) Close() error

// SchemaVersion is the overlay version this binary writes and requires.
func SchemaVersion() int
// FileSchemaVersion reads a file's overlay version without the enforcing path.
func FileSchemaVersion(path string) (int, error)
// OpenRaw opens without schema/version handling — test hook.
func OpenRaw(path string) (*sql.DB, error)
```

`Open` takes a **path**, matching `graph.Open`, so tests can point at a temp
file; `Path(wsRoot)` is exported alongside it so the six-site `graph.db` path
duplication is not repeated for the overlay.

### 2. Version and rebuild

```go
const schemaVersion = 1 // v1: member registry, cross-edges, stamps
```

Independent of `graph.schemaVersion` by construction — a different constant
in a different package over a different file. `Open` reproduces
`graph.Open`'s sequence exactly: read `PRAGMA user_version`; on mismatch
count tables, close, `os.Remove`, warn on stderr only if the discarded file
had tables (`codeindex: workspace overlay schema v%d -> v%d, rebuilding`),
reopen; `Exec(schema)`; `PRAGMA user_version = 1`.

Delete-and-rebuild is not a shortcut here, it is D2's stated property: "an
overlay version bump rebuilds the overlay only (cheap), never member
indexes." The overlay holds no primary data — every row is re-derivable from
the members' own `graph.db` files by re-running the ladder.

### 3. Schema

No `key`/`value` meta table is created. `graph.db` has three of them
(`depmeta`, `vecmeta`, `obs_meta`) because each has a defined payload; the
overlay v1 has none, and an empty key/value table is how a schema later
acquires a meaning nobody designed. A future slice that needs one adds it
with a version bump — which, per §2, costs an overlay rebuild and nothing
else.

```sql
-- Member registry: the manifest as built, in manifest order.
CREATE TABLE IF NOT EXISTS members (
  id TEXT PRIMARY KEY, root TEXT NOT NULL, ord INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS member_namespaces (
  member_id TEXT NOT NULL, namespace TEXT NOT NULL, ord INTEGER NOT NULL,
  PRIMARY KEY (member_id, namespace));
CREATE INDEX IF NOT EXISTS idx_member_ns ON member_namespaces(namespace);
CREATE TABLE IF NOT EXISTS member_deps (
  member_id TEXT NOT NULL, dep_id TEXT NOT NULL, ord INTEGER NOT NULL,
  PRIMARY KEY (member_id, dep_id));

-- Cross-repo edges, keyed by stable key on BOTH ends.
CREATE TABLE IF NOT EXISTS cross_edges (
  id INTEGER PRIMARY KEY,
  src_member TEXT NOT NULL, src_file TEXT NOT NULL, src_qname TEXT NOT NULL,
  dst_member TEXT NOT NULL, dst_file TEXT NOT NULL, dst_qname TEXT NOT NULL,
  kind TEXT NOT NULL, provenance TEXT NOT NULL, confidence TEXT NOT NULL,
  line INTEGER NOT NULL DEFAULT 0,
  UNIQUE (src_member, src_file, src_qname, dst_member, dst_file, dst_qname,
          kind, line));
CREATE INDEX IF NOT EXISTS idx_cross_src ON cross_edges(src_member, src_file, src_qname);
CREATE INDEX IF NOT EXISTS idx_cross_dst ON cross_edges(dst_member, dst_file, dst_qname);
CREATE INDEX IF NOT EXISTS idx_cross_dst_member ON cross_edges(dst_member);

-- D3 rung 3: ambiguity, keyed by the unresolved reference, with its count.
CREATE TABLE IF NOT EXISTS cross_ambiguities (
  id INTEGER PRIMARY KEY,
  src_member TEXT NOT NULL, src_file TEXT NOT NULL, src_qname TEXT NOT NULL,
  ref_name TEXT NOT NULL, ref_ns TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL, line INTEGER NOT NULL DEFAULT 0,
  candidate_count INTEGER NOT NULL,
  UNIQUE (src_member, src_file, src_qname, ref_name, kind, line));
CREATE INDEX IF NOT EXISTS idx_ambig_src ON cross_ambiguities(src_member, src_file, src_qname);
CREATE TABLE IF NOT EXISTS cross_ambiguity_candidates (
  ambiguity_id INTEGER NOT NULL, rank INTEGER NOT NULL,
  member_id TEXT NOT NULL, file TEXT NOT NULL, qname TEXT NOT NULL,
  PRIMARY KEY (ambiguity_id, rank));

-- D3 member-over-dep precedence: what was suppressed, for skew reporting.
-- consumer_member = the member whose tier-1 depmap attachment was suppressed.
-- owner_member    = the workspace member that claims the namespace and won.
-- suppressed_version = the vendored copy's version as recorded in the
--                      consumer's own depfiles ('' when unknown).
CREATE TABLE IF NOT EXISTS dep_suppressions (
  consumer_member TEXT NOT NULL, namespace TEXT NOT NULL,
  owner_member TEXT NOT NULL, suppressed_version TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (consumer_member, namespace));
CREATE INDEX IF NOT EXISTS idx_supp_owner ON dep_suppressions(owner_member);

-- Per-member freshness stamps: the member's merkle root at last resolution.
CREATE TABLE IF NOT EXISTS member_stamps (
  member_id TEXT PRIMARY KEY, merkle_root TEXT NOT NULL,
  resolved_at INTEGER NOT NULL);
```

`candidate_count` is stored on the ambiguity row rather than derived by
`COUNT(*)` over the candidates table because D3's contract is that the count
is reported *as recorded at resolution time*; a derived count would silently
change meaning if a future slice ever truncated the candidate list for
display.

`ref_ns` carries D3's namespace hint H (empty for a rung-2 bare name) so the
suppression/skew story and any later re-resolution can tell rung 1 from rung
2 without re-deriving it. It is deliberately outside the UNIQUE key: the
`(src, name, kind, line)` tuple already identifies the reference.

Skew is a comparison between `suppressed_version` — the pinned vendored
snapshot — and the `owner_member`'s **live working tree**, which has no
version string to store: it is whatever is checked out at read time. So one
version column is sufficient and a second would be a lie; §3.4's
`workspace-status` reads the owner's checkout when it renders the skew.

### 4. Registry write (build time)

```go
// ReplaceRegistry mirrors ws into the overlay as-built, in manifest order,
// and prunes every row belonging to a member the manifest no longer names.
func (s *Store) ReplaceRegistry(ws *config.Workspace) error

// Registry returns the registry in manifest order, as config.Member values.
func (s *Store) Registry() ([]config.Member, error)
```

The read returns `config.Member`, not a structurally identical
`overlay.Member`. `ReplaceRegistry` already takes `*config.Workspace`, so the
package imports `config` regardless; minting a shadow type would force every
§3.3/§3.4 caller to convert between two identical structs for nothing.

**Duplicate namespaces and deps.** `config.validate()` never inspects those
two slices, so a hand-authored manifest with `"namespaces": ["acme/auth",
"acme/auth"]` loads cleanly and would then abort `ReplaceRegistry`'s single
transaction on `PRIMARY KEY (member_id, namespace)`. `ReplaceRegistry`
therefore **de-duplicates both slices in Go before inserting, preserving
first-occurrence order**, and does not error: the manifest is legal input by
the loader's own rules, the duplicate carries no information, and a storage
write is the wrong place to start rejecting manifests the loader accepts.
`INSERT OR IGNORE` was rejected as the mechanism because it would leave `ord`
values with a silent gap.

`ReplaceRegistry` runs in one transaction: delete-all + re-insert the three
registry tables (a mirror; diffing a mirror buys nothing), then, for every
member id present in the overlay's dependent tables but absent from the new
manifest, delete its `member_stamps`, `dep_suppressions`, and every
`cross_edges` / `cross_ambiguities` row naming it on **either** end (with the
orphaned `cross_ambiguity_candidates` rows). A dropped member's cross-edges
are dangling references to a graph nobody will re-map, and leaving them
would let a removed member keep contributing to `impact`.

It mirrors the **full declared** membership, including members absent from
disk. Presence is a runtime condition reported by `config.Resolve`, exactly
as 0009 settled; a persisted presence column would be stale the moment
someone checked a member out.

### 5. Cross-edge and side-record API (stable key)

```go
// SymKey is the stable identity of a symbol across member rebuilds:
// member id + member-relative file path + qualified name. Never a rowid.
type SymKey struct{ Member, File, QName string }

type CrossEdge struct {
    Src, Dst   SymKey
    Kind       string // a graph.EdgeKind value, stored verbatim, unvalidated
    Provenance string // "cross_repo_import" | "cross_repo_name"
    Confidence string // "exact" | "inferred"
    Line       int
}

type Ambiguity struct {
    Src        SymKey
    RefName    string
    RefNS      string
    Kind       string
    Line       int
    Candidates []SymKey // in recorded order; deps-named candidate first
    Count      int      // as recorded; len(Candidates) unless truncated upstream
}

type Suppression struct {
    ConsumerMember    string // whose tier-1 attachment was suppressed
    Namespace         string
    OwnerMember       string // the member that claims the namespace and won
    SuppressedVersion string // the vendored snapshot's version; "" if unknown
}

// Writes — all idempotent, all in one transaction per call. PutCrossEdges
// and PutSuppressions are upserts; PutAmbiguities is a delete-then-insert
// (decision 18).
func (s *Store) PutCrossEdges(edges []CrossEdge) error
func (s *Store) PutAmbiguities(a []Ambiguity) error
func (s *Store) PutSuppressions(sup []Suppression) error

// Whole-member replacement: the unit §3.4's stamp-gated re-resolution needs.
// Deletes cross-edges and ambiguities incident to memberID on EITHER end,
// and suppressions whose consumer_member is memberID — owner-side rows are
// deliberately left alone (decision 21) — then writes the given records, all
// in one transaction. A sup entry whose ConsumerMember != memberID is
// rejected: it would land outside this call's own delete scope.
func (s *Store) ReplaceMemberEdges(memberID string, edges []CrossEdge,
    a []Ambiguity, sup []Suppression) error

// Reads.
func (s *Store) OutEdges(src SymKey) ([]CrossEdge, error)
func (s *Store) InEdges(dst SymKey) ([]CrossEdge, error)
func (s *Store) MemberEdges(memberID string) ([]CrossEdge, error)
func (s *Store) AmbiguitiesFor(memberID string) ([]Ambiguity, error)
func (s *Store) Suppressions() ([]Suppression, error)
```

Every read returns rows in a deterministic order (`ORDER BY` the stable-key
columns then `line`), matching the project's rebuild-determinism rule.

`ReplaceMemberEdges` is the API §3.4 actually needs — "re-resolve only
overlay edges incident to that member" is a replace, and a caller composing
it out of a delete plus three puts would have to own the transaction
boundary itself.

**Conflict action on `cross_edges` — the stronger class wins.** The UNIQUE
key deliberately excludes `provenance` and `confidence`, because two D3 rungs
can reach the same `(src, dst, kind, line)` tuple: rung 1 with
`cross_repo_import`/`exact` and rung 2 with `cross_repo_name`/`inferred`.
`PutCrossEdges` uses `ON CONFLICT(<the 8 key columns>) DO UPDATE SET
provenance=excluded.provenance, confidence=excluded.confidence WHERE
excluded.confidence='exact' AND cross_edges.confidence<>'exact'` — an `exact`
overwrites an `inferred`, an `inferred` never demotes an `exact`, and a
re-put of identical input is a no-op. This is not the storage layer inventing
D3 semantics: D3's ladder is frozen with rung 1 above rung 2, and this is the
only conflict action consistent with that order. A bare `INSERT OR IGNORE`
would make the outcome depend on ladder emission order; `INSERT OR REPLACE`
would let a rung-2 pass silently demote a rung-1 answer.

**`PutAmbiguities` deletes before it inserts.** `cross_ambiguities` has a
surrogate `id` plus a natural UNIQUE key, and its candidates reference that
`id` with no foreign key (there is no `PRAGMA foreign_keys`, decision 13). An
`INSERT OR REPLACE` on the natural key would delete the conflicting row and
insert a new one under a **new rowid**, stranding the old candidate rows
forever while every read still looked correct. So `PutAmbiguities`, inside
its transaction and per record: `DELETE FROM cross_ambiguity_candidates
WHERE ambiguity_id IN (SELECT id FROM cross_ambiguities WHERE <natural
key>)`, then `DELETE FROM cross_ambiguities WHERE <natural key>`, then insert
the row and its candidates. It also rejects `Count < len(Candidates)` as a
programming error rather than storing a count its own candidate list
contradicts.

**What `ReplaceMemberEdges` deletes for suppressions.** `cross_edges` and
`cross_ambiguities` have an unambiguous "incident to `memberID`" — either
endpoint. Suppressions do not: they name a `consumer_member` and an
`owner_member`. `ReplaceMemberEdges(M, …)` deletes only rows where
**`consumer_member = M`**, and leaves rows where M is merely the
`owner_member` untouched. A suppression is a fact about a consumer's
attachment being overridden; it is re-derived when *that consumer* is
re-resolved, so deleting owner-side rows would drop records for members this
call is not re-resolving and cannot rewrite.

### 6. Stamp API

```go
type Stamp struct {
    MemberID   string
    MerkleRoot string // opaque token, compared for equality only
    ResolvedAt int64  // unix seconds
}

func (s *Store) PutStamp(memberID, merkleRoot string) error // ResolvedAt = now
func (s *Store) Stamp(memberID string) (Stamp, bool, error)
func (s *Store) Stamps() ([]Stamp, error)
```

`MerkleRoot` is an **opaque string** this slice never computes or parses; it
is compared for equality by §3.4 and nothing else. See decision 9.

## Acceptance checklist

- `internal/overlay` exists with the API above; `go build ./...` clean;
  `gofmt` clean; `go vet ./...` clean.
- `go test -tags nollama ./...` green, including new tests for:
  - open-creates-file; reopen is a no-op; `SchemaVersion()`/
    `FileSchemaVersion()` agree on a freshly created file.
  - a file stamped with a wrong `user_version` (written via `OpenRaw`) is
    deleted and recreated empty by `Open`; a fresh (v0, table-less) file is
    recreated **without** a stderr warning.
  - `ReplaceRegistry` round-trips a 3-member manifest in manifest order with
    namespaces and deps intact; a re-run with a member dropped prunes that
    member's stamp, suppressions, and cross-edges on both ends.
  - `ReplaceRegistry` accepts a manifest carrying a duplicate namespace and a
    duplicate dep (both legal per `config.validate`), storing each once in
    first-occurrence order with contiguous `ord` values.
  - `PutCrossEdges` is idempotent (same slice twice ⇒ same row count) and
    round-trips through `OutEdges`/`InEdges`/`MemberEdges` in deterministic
    order.
  - **rung precedence:** writing the same `(src, dst, kind, line)` tuple as
    `inferred` then `exact` leaves one row at `exact`/`cross_repo_import`;
    writing it `exact` then `inferred` also leaves it at `exact` — the write
    order does not change the answer.
  - **no-rowid invariant** (the §3.2-local half of "stable-key survival"): a
    schema assertion that no table in the overlay has a column referencing a
    member DB's row ids — every symbol reference is `(member, file, qname)`.
    The cross-member-rebuild test this invariant exists to enable belongs to
    §3.5, which has a ladder to run; this slice never opens a member's
    `graph.db`, so a "rebuild the member DB" test here would assert only that
    two unrelated files do not affect each other.
  - `PutAmbiguities` round-trips candidates in rank order with the recorded
    count, and is **idempotent in the candidates table**: the same input
    twice leaves `cross_ambiguity_candidates` at the same row count (the
    orphan-growth regression). `Count < len(Candidates)` is rejected.
  - `ReplaceMemberEdges(M, …)` clears prior cross-edges and ambiguities
    incident to M on either end, clears suppressions with
    `consumer_member = M`, and **leaves** suppressions where M is only the
    `owner_member`.
  - `PutStamp`/`Stamp` round-trip; absent member ⇒ `(zero, false, nil)`.
- No change to any file under `internal/graph`, `internal/depmap`,
  `internal/engine`, `cmd/`, or the MCP surface.
- No new module dependency; `go.mod`/`go.sum` unchanged.

## Out of scope

- The resolution ladder (§3.3). This slice writes nothing into these tables
  from real data; it provides the storage the ladder will fill.
- Freshen policy, the `workspace-status` verb, and any decision about *when*
  a stamp is compared or refreshed (§3.4) — including how a member's merkle
  root is computed (decision 9).
- Union-graph query paths, key→id re-mapping at query time, CLI/MCP surfaces
  (§4.x), and the evidence gate (§5.x).
- **Confidence-vocabulary reconciliation.** The overlay stores D3's words;
  `graph.Confidence` uses different ones. Mapping them at the surface is
  §4.1's problem, recorded here as a hand-off (decision 8).
- Member discovery and corpus growth (change 0010).

## Assumptions

Every decision below was defaulted by the autonomous groomer. The frozen
design's rejected alternatives (copy-merge, query-time `ATTACH`
re-resolution) are settled and were not re-litigated.

| # | Decision | Chosen | Rejected | Why |
|---|---|---|---|---|
| 1 | Where the overlay store lives | new flat package `internal/overlay` | inside `internal/workspace`; inside `internal/graph` | `internal/workspace` is a discovery package that opens no database (it imports neither `database/sql` nor `internal/graph`; it does pull `x/mod/modfile` and `yaml.v3` for build-file parsing), and `internal/graph` owns the per-repo index whose schema D2 requires to stay untouched. `internal/depmap` is the in-repo precedent for a storage subsystem as a flat sibling |
| 2 | `Open` signature | `Open(path string)` + exported `Path(wsRoot)` | `Open(wsRoot string)` | path-taking matches `graph.Open` and keeps tests on temp files; exporting `Path` avoids repeating the six-site `graph.db` path duplication for a second database |
| 3 | Version mismatch handling | delete-and-rebuild, cloning `graph.Open`'s sequence including the warn-only-if-populated rule | in-place `ALTER TABLE` migrations | D2 states an overlay bump rebuilds the overlay only; every overlay row **will be** re-derivable from member DBs by re-running the ladder, so the overlay is a derived artifact by exactly the argument `store.go:38` already makes for `graph.db`. Named honestly: until §3.3 ships, nothing in-tree can refill an emptied overlay — but nothing fills it in the first place either, so the window is empty by construction |
| 4 | Stable-key representation | three separate columns (`member`, `file`, `qname`) per endpoint | one joined composite string (`member:file#qname`) | member ids forbid `:` and `/` but file paths and qualified names contain both, so any delimiter is ambiguous and would need escaping. Separate columns also index and `ORDER BY` directly |
| 5 | String interning | none — plain TEXT columns | copy `graph.db`'s `strs` + `_t` tables + reconstructing VIEWs | interning exists in `graph.db` for millions of symbol rows; the overlay holds only cross-boundary edges (orders of magnitude fewer) and paying the same complexity there buys nothing while making the schema unreadable for a table a human will inspect while debugging the ladder |
| 6 | Registry write shape | full replace in one transaction, with orphan pruning of dropped members | incremental diff/upsert; replace without pruning | it is a mirror, so diffing is pure cost. Not pruning is a correctness bug: a dropped member's cross-edges are dangling references that would keep contributing to `impact` |
| 7 | Registry mirrors declared vs on-disk members | all declared members; no presence column | only `Resolve`-present members; a `present` boolean | 0009 deliberately split absence out of the load path — absence is a runtime condition, and a persisted presence bit goes stale on the next checkout |
| 8 | Confidence vocabulary | store D3/frozen-spec words verbatim (`exact`, `inferred`) as TEXT | reuse `graph.Confidence` (`unambiguous`/`ambiguous`/`unresolved`); introduce a new shared enum now | the frozen spec is normative for what the ladder writes, and the overlay is the ladder's sink. Reusing the graph words would silently discard the exact/inferred distinction rung 1 exists to make. A shared enum is a surfacing concern §4.1 owns, and inventing it here would edit a frozen schema from a storage slice. Recorded as an explicit hand-off in *Out of scope* |
| 9 | Merkle-root stamp semantics | opaque `TEXT`, never computed or parsed in this slice | define and implement a repo-level merkle root here | there is **no** repo-level merkle root in the tree today — the `merkle` table is per-file — so this would be new derived semantics, and "how freshness is decided" is explicitly §3.4's. An opaque token is fully testable (round-trip + equality) and constrains §3.4 not at all. Flagged as the one real gap this slice hands forward |
| 10 | Ambiguity storage | its own table keyed by the unresolved reference, plus a ranked candidates child table and a stored `candidate_count` | ambiguous entries as `cross_edges` rows with a flag | an ambiguity is not an edge — writing one as N edges is exactly the "presents one candidate as exact" failure the frozen spec's scenario forbids. Storing the count rather than deriving it preserves "as recorded at resolution time" |
| 11 | Per-member replacement API | ship `ReplaceMemberEdges` alongside the puts | leave §3.4 to compose delete + puts | §3.4's incident-edge re-resolution is a replace; composing it at the call site would push the transaction boundary onto a caller in another package. This designs an API *shape* for a later caller — which is not in tension with 12: 12 declines to invent a **call site**, 11 declines to leave a **transaction boundary** undefined. Storage owns its own atomicity; it does not own who calls it |
| 12 | No call site in this slice | ship the API unwired | wire a CLI verb or hook it into the build path to prove it runs | `tasks.md` gives §3.2 no verb and puts `workspace-status` in §3.4. The 0009 precedent is narrower than a blanket one: 0009 *did* wire `init-workspace` (`cmd/codeindex/initworkspace.go` calls `LoadWorkspace`/`SaveWorkspace`/`Scan`/`Merge`), and shipped only `Resolve`/`ResolvedMember` and `DetectRootKind` unwired — deliberately, for the slice that would own their first callers. That is this slice's situation exactly |
| 13 | SQLite pragmas | none beyond `user_version` | WAL / `synchronous` tuning | `graph.Open` sets none, usage is single-writer, and an untested journal-mode divergence between the two databases is a debugging trap for no measured gain |
| 14 | New ADR | none | an ADR for the second database file | D2 already froze the overlay-vs-copy-merge decision at campaign level; a storage slice restating it would create a competing record |
| 15 | Test gate | `go test -tags nollama ./...` | plain `go test ./...` | plain `go test` fails 10 packages on every ref for missing vendored llama.cpp headers — environmental. The tagged form is pinned in `.docket.local.yml` |
| 16 | Duplicate `namespaces`/`deps` entries (legal per `config.validate`, fatal against the registry primary keys) | de-duplicate in Go, first-occurrence order, no error | error out; `INSERT OR IGNORE` | the loader accepts them, so a storage write rejecting them would make `ReplaceRegistry` fail on legal input — a latent abort of the whole transaction. `INSERT OR IGNORE` would silently gap the `ord` sequence |
| 17 | `cross_edges` conflict action when two D3 rungs hit the same key tuple | `ON CONFLICT DO UPDATE` that upgrades `inferred` → `exact` and never demotes | `INSERT OR IGNORE`; `INSERT OR REPLACE` | D3's ladder is frozen with rung 1 above rung 2, and this is the only action consistent with it. `IGNORE` makes the stored answer depend on emission order; `REPLACE` lets a rung-2 pass demote a rung-1 `exact` |
| 18 | `PutAmbiguities` re-put | explicit delete of the natural-key row **and its candidates**, then insert | `INSERT OR REPLACE` on the natural key | `REPLACE` assigns a new rowid, so every candidate row of the replaced ambiguity is orphaned. With no `PRAGMA foreign_keys` (decision 13) nothing collects them and every read still looks right — the candidates table grows monotonically |
| 19 | `dep_suppressions` column naming and version cardinality | `consumer_member` / `owner_member` / one `suppressed_version` | a generic `member_id`; a second column for the owner's version | `member_id` cannot say which of D3's two members it names. The owner is a live checkout with no version string to store, so a second column would be fabricated data; §3.4 reads the checkout when it renders skew |
| 20 | Registry read type | return `[]config.Member` | a structurally identical `overlay.Member` | the package already imports `config` for `ReplaceRegistry`; a shadow type would make every §3.3/§3.4 caller convert between two identical structs for no gain |
| 21 | What `ReplaceMemberEdges(M)` deletes for suppressions | only rows with `consumer_member = M` | rows where M appears in either column | a suppression records a *consumer's* attachment being overridden and is re-derived when that consumer re-resolves. Deleting owner-side rows would drop records for members the call is not re-resolving and cannot rewrite |
| 22 | No key/value meta table in v1 | omit `wsmeta` | ship it empty, mirroring `depmeta`/`vecmeta`/`obs_meta` | those three each have a defined payload; an empty one has none, and unused schema is how a table acquires an undesigned meaning. Adding it later costs one version bump, which the rebuild path already handles |

**Dependency state:** `depends_on: [9]`, which is **satisfied** — change 0009
merged 2026-08-19 and is archived, and its `internal/config/workspace.go`
types are the direct input to §4's `ReplaceRegistry`. `related: [9, 10]` —
both informational at this point; no bench artifact is touched by this slice.

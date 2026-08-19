<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0012 — Workspace overlay store — member registry, cross-edges by stable key, freshness stamps](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0012-workspace-overlay-store.md)**
<!-- docket:backlink:end -->

# Plan: Workspace overlay store (`internal/overlay`) — change 0012

Spec: `docs/superpowers/specs/2026-08-19-workspace-overlay-store-design.md`
on `origin/docket` (the frozen design; its 22-assumption block and decision
rows are **binding** — do not re-litigate a decision, implement it).

openspec task §3.2 of `openspec/changes/workspace-graph/tasks.md`.

> **Plan-role degradation.** `skills.plan` resolved to
> `superpowers:writing-plans`, which is not invocable on this machine. Per the
> convention's missing-skill rule the role degraded to `auto` and this plan was
> authored by `docket-implement-next` directly.

## Ground rules for every task

- **Test gate:** `go test -tags nollama ./...`. Plain `go test ./...` fails 10
  packages on **every** ref for missing vendored llama.cpp headers — that is
  environmental, not a regression. Never "fix" it, never report it as one.
- **Zero new failures.** The tagged suite is green on `origin/main`; this
  branch must keep it green.
- **Do not touch** anything under `internal/graph`, `internal/depmap`,
  `internal/engine`, `internal/config`, `cmd/`, or the MCP surface. `go.mod`
  and `go.sum` must be byte-unchanged. The whole change is additive inside the
  new directory `internal/overlay/`.
- `gofmt` clean and `go vet ./...` clean at every commit.
- The package is shipped **unwired** — no CLI verb, no caller (decision 12).
  Do not "prove it runs" by wiring it into a command.
- Module is `codeindex`, so imports are `codeindex/internal/config` etc.
- Every read returns rows in a deterministic order (`ORDER BY` the stable-key
  columns, then `line`) — the project's rebuild-determinism rule.

---

## Task 1 — package skeleton: `Path`, `Open`, schema, version

**Files:** `internal/overlay/overlay.go`, `internal/overlay/overlay_test.go`

Write the tests first, then the code.

Create the package with:

```go
const FileName = ".codeindex/workspace.db"
func Path(wsRoot string) string          // filepath.Join(wsRoot, FileName)
const schemaVersion = 1                  // v1: member registry, cross-edges, stamps
func Open(path string) (*Store, error)
func (s *Store) Close() error
func OpenRaw(path string) (*sql.DB, error)
func SchemaVersion() int
func FileSchemaVersion(path string) (int, error)
```

`Store` wraps a `*sql.DB` (no `strCache` — decision 5 rejects interning).

`Open` **clones `internal/graph/store.go:139-174` exactly**, changing only the
constant and the warning text: open; read `PRAGMA user_version`; on mismatch
`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`, `Close`, `os.Remove`
(tolerating `os.IsNotExist`), warn on stderr **only if `tables > 0`** with

```
"codeindex: workspace overlay schema v%d -> v%d, rebuilding\n"
```

then reopen; `Exec(schema)`; `Exec(PRAGMA user_version = 1)`. Set **no other
pragma** — no WAL, no `synchronous`, no `foreign_keys` (decision 13). Note that
`Open` does **not** create parent directories (neither does `graph.Open`);
tests create the temp dir themselves.

The `schema` const is the spec's §3 SQL verbatim: `members`,
`member_namespaces` (+ `idx_member_ns`), `member_deps`, `cross_edges` (+ its
three indexes), `cross_ambiguities` (+ `idx_ambig_src`),
`cross_ambiguity_candidates`, `dep_suppressions` (+ `idx_supp_owner`),
`member_stamps`. `CREATE TABLE IF NOT EXISTS` throughout. **No meta key/value
table** (decision 22).

**Tests:**
- `Path` joins as documented.
- open creates the file; reopening is a no-op (no error, tables still there).
- `SchemaVersion()` == `FileSchemaVersion()` on a freshly created file.
- a file stamped with a **wrong** `user_version` via `OpenRaw` (create a table,
  set `user_version = 99`) is deleted and recreated empty by `Open` — assert
  the previously-created table is gone.
- a fresh, table-less v0 file (just `OpenRaw` + close, no tables) is recreated
  by `Open` **without** a stderr warning. Capture stderr by swapping
  `os.Stderr` for an `os.Pipe` around the call.

---

## Task 2 — registry write/read

**Files:** `internal/overlay/registry.go`, `internal/overlay/registry_test.go`

```go
func (s *Store) ReplaceRegistry(ws *config.Workspace) error
func (s *Store) Registry() ([]config.Member, error)
```

`ReplaceRegistry`, in **one transaction**:

1. `DELETE FROM` `members`, `member_namespaces`, `member_deps` (full replace —
   it is a mirror; decision 6).
2. Insert every **declared** member in manifest order with `ord` = index —
   including members absent from disk (decision 7; no presence column).
3. For `Namespaces` and `Deps`: **de-duplicate in Go, preserving
   first-occurrence order**, before inserting, and assign contiguous `ord`
   values over the de-duplicated slice (decision 16). Do **not** error, and do
   **not** use `INSERT OR IGNORE` — it would leave `ord` gaps.
4. **Prune orphans:** for every member id present in the overlay's dependent
   tables but absent from the new manifest, delete its `member_stamps` rows,
   its `dep_suppressions` rows (**either** column — a dropped member is gone as
   consumer and as owner alike; this is registry pruning, distinct from
   `ReplaceMemberEdges`' consumer-only rule in task 4), every `cross_edges` and
   `cross_ambiguities` row naming it on **either** end, and the
   `cross_ambiguity_candidates` rows orphaned by those ambiguity deletions
   (delete the candidates **before or by subquery on** the ambiguity rows, so
   the ids are still resolvable). Also drop candidate rows whose `member_id` is
   the dropped member.

   Implement the prune as set-based SQL against the new id set (e.g. build a
   temp id list, or `NOT IN (?,?,…)` — with an empty manifest meaning "delete
   everything", which the `NOT IN` form must handle explicitly rather than
   producing `NOT IN ()`).

`Registry()` returns `[]config.Member` (decision 20 — no shadow type) in
manifest order (`ORDER BY ord`), with `Namespaces` and `Deps` each ordered by
their own `ord`. Return a non-nil empty slice for an empty registry.

**Tests:**
- round-trips a 3-member manifest in manifest order with namespaces and deps
  intact.
- a re-run with a member dropped prunes that member's stamp, its suppressions,
  and its cross-edges **on both ends** (seed via task 3/4 APIs, or raw SQL if
  those tasks land later — prefer the real APIs, so order this test after
  task 4 if needed).
- a manifest carrying a duplicate namespace **and** a duplicate dep (both legal
  per `config.validate`) stores each once, first-occurrence order, contiguous
  `ord`.
- `ReplaceRegistry` twice with the same manifest is idempotent.

---

## Task 3 — cross-edges, ambiguities, suppressions

**Files:** `internal/overlay/edges.go`, `internal/overlay/edges_test.go`

Types exactly as the spec §5 gives them: `SymKey{Member, File, QName string}`,
`CrossEdge{Src, Dst SymKey; Kind, Provenance, Confidence string; Line int}`,
`Ambiguity{Src SymKey; RefName, RefNS, Kind string; Line int; Candidates
[]SymKey; Count int}`, `Suppression{ConsumerMember, Namespace, OwnerMember,
SuppressedVersion string}`.

`Kind` is a `graph.EdgeKind` **value stored verbatim as a string and never
validated** — do **not** import `internal/graph` for it.

Writes, one transaction per call, all idempotent:

- **`PutCrossEdges(edges []CrossEdge) error`** — upsert with the
  stronger-class-wins conflict action (decision 17):
  ```sql
  ON CONFLICT(src_member, src_file, src_qname, dst_member, dst_file, dst_qname, kind, line)
  DO UPDATE SET provenance = excluded.provenance, confidence = excluded.confidence
  WHERE excluded.confidence = 'exact' AND cross_edges.confidence <> 'exact'
  ```
  An `exact` upgrades an `inferred`; an `inferred` never demotes an `exact`; a
  re-put of identical input is a no-op.
- **`PutAmbiguities(a []Ambiguity) error`** — **delete-then-insert**, never
  `INSERT OR REPLACE` (decision 18: `REPLACE` mints a new rowid and orphans the
  candidate rows forever). Per record, inside the transaction: delete
  `cross_ambiguity_candidates` whose `ambiguity_id` is in the natural-key
  match, then delete the `cross_ambiguities` natural-key row, then insert the
  row and its candidates at `rank` = index. Reject `Count < len(Candidates)` as
  a programming error (return an error naming both numbers) **before** any
  write, so a bad record never half-applies.
- **`PutSuppressions(sup []Suppression) error`** — upsert on the
  `(consumer_member, namespace)` primary key.

- **`ReplaceMemberEdges(memberID string, edges []CrossEdge, a []Ambiguity, sup
  []Suppression) error`** — one transaction: delete `cross_edges` and
  `cross_ambiguities` incident to `memberID` on **either** end (plus the
  orphaned candidates), and delete suppressions **only where `consumer_member =
  memberID`** — rows where `memberID` is merely the `owner_member` are
  deliberately **left alone** (decision 21). Say exactly that in the doc
  comment; do not write "either end" for the suppression clause. Then write the
  given records via the same logic as the puts. Reject any `sup` entry whose
  `ConsumerMember != memberID` (it would land outside this call's delete scope)
  before writing anything.

Reads, each deterministically ordered:

```go
func (s *Store) OutEdges(src SymKey) ([]CrossEdge, error)
func (s *Store) InEdges(dst SymKey) ([]CrossEdge, error)
func (s *Store) MemberEdges(memberID string) ([]CrossEdge, error)   // either end
func (s *Store) AmbiguitiesFor(memberID string) ([]Ambiguity, error) // candidates by rank
func (s *Store) Suppressions() ([]Suppression, error)
```

**Tests:**
- `PutCrossEdges` idempotent (same slice twice ⇒ same row count) and
  round-trips through `OutEdges`/`InEdges`/`MemberEdges` in deterministic
  order.
- **rung precedence, both orders:** `inferred` then `exact` ⇒ one row at
  `exact`/`cross_repo_import`; `exact` then `inferred` ⇒ still `exact`. Write
  order does not change the answer.
- `PutAmbiguities` round-trips candidates in rank order with the recorded
  count, and is **idempotent in the candidates table** — same input twice
  leaves `cross_ambiguity_candidates` at the same row count (this is the
  orphan-growth regression decision 18 exists for; assert the raw count via
  `OpenRaw`/a direct query, not just the read API).
- `Count < len(Candidates)` is rejected.
- `ReplaceMemberEdges(M, …)` clears prior cross-edges and ambiguities incident
  to M on either end, clears suppressions with `consumer_member = M`, and
  **leaves** a suppression where M is only the `owner_member`.
- `ReplaceMemberEdges` rejects a `sup` entry whose `ConsumerMember != M`.

---

## Task 4 — stamps

**Files:** `internal/overlay/stamps.go`, `internal/overlay/stamps_test.go`

```go
type Stamp struct {
    MemberID   string
    MerkleRoot string // opaque token, compared for equality only
    ResolvedAt int64  // unix seconds
}
func (s *Store) PutStamp(memberID, merkleRoot string) error // ResolvedAt = now
func (s *Store) Stamp(memberID string) (Stamp, bool, error)
func (s *Store) Stamps() ([]Stamp, error)                   // ORDER BY member_id
```

`MerkleRoot` is **opaque** — this slice never computes, parses, or validates it
(decision 9; repo-level merkle aggregation is §3.4's). `PutStamp` upserts on
the `member_id` primary key with `ResolvedAt = time.Now().Unix()`.

**Tests:** `PutStamp`/`Stamp` round-trip; an absent member yields
`(Stamp{}, false, nil)`; re-`PutStamp` overwrites rather than duplicating;
`Stamps()` is ordered.

---

## Task 5 — no-rowid invariant test

**File:** `internal/overlay/schema_test.go`

The §3.2-local half of "stable-key survival": a **schema assertion** that no
table in the overlay has a column referencing a member DB's row ids — every
symbol reference is `(member, file, qname)`.

Implement by walking `sqlite_master` for the overlay's tables and, via `PRAGMA
table_info(<t>)`, asserting that no column name matches a rowid-reference
shape (`symbol_id`, `file_id`, `src_id`, `dst_id`, `name_id`, `edge_id`, … — a
regexp over `_id$` with an **explicit allowlist** for the overlay's own legal
`_id` columns: `member_id`, `dep_id`, `ambiguity_id`, and the surrogate `id`
primary keys of `cross_edges`/`cross_ambiguities`). Comment *why* the allowlist
entries are legal: `member_id`/`dep_id` are manifest ids (TEXT), and
`ambiguity_id` is overlay-internal — none of them is a member `graph.db` rowid.

Per the spec, the cross-member-rebuild test this invariant enables belongs to
**§3.5**, not here: this slice never opens a member's `graph.db`, so a "rebuild
the member DB" test would assert only that two unrelated files do not affect
each other. Do not write one.

---

## Task 6 — final gate

- `gofmt -l internal/overlay` empty.
- `go vet ./...` clean.
- `go build ./...` clean.
- `go test -tags nollama ./...` green — zero new failures vs `origin/main`.
- `git diff --stat origin/main...HEAD` touches **only** `internal/overlay/**`
  and `docs/superpowers/plans/**`. `go.mod`/`go.sum` unchanged.

## Acceptance

The spec's *Acceptance checklist* section is the authority; tasks 1–5 above map
onto it one-for-one. Re-read it before declaring the build done.

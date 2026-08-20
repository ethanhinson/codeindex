<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0013 — Cross-repo resolution ladder — import-mediated exact, bare-name inferred, ambiguity, suppression](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0013-workspace-resolution-ladder.md)**
<!-- docket:backlink:end -->

# Plan: Cross-repo resolution ladder (change 0013, openspec §3.3)

Spec: `docs/superpowers/specs/2026-08-19-workspace-resolution-ladder-design.md` on
`origin/docket` — **binding**, including its 17 assumptions and every decision row.
Base: `origin/main` `f6d4dee`. Branch: `feat/workspace-resolution-ladder`.

Test gate for every task and for the final full-suite run:
**`go test -tags nollama ./...`**. Plain `go test ./...` fails 10 packages on every ref
(missing vendored llama.cpp headers) — environmental, not a regression. The branch must
add **zero** failures to the tagged run.

## Standing constraints (apply to every task)

1. **`graph.Open` is banned from `internal/wsresolve` non-test code.** It creates an absent
   file and deletes-and-recreates a version-mismatched one. Use `graph.OpenExisting`
   (task 1) exclusively. Fixtures legitimately use `graph.Open` to *build* member indexes;
   the wrong-version fixture uses `graph.OpenRaw` plus a `PRAGMA user_version` write.
2. **No member `graph.db` is ever written by a resolution pass** — no schema change, no
   `graph.schemaVersion` bump, no row mutation, not even for the tier-1 rows a suppression
   overrides.
3. **Confidence vocabulary is `exact` / `inferred`.** `graph.Confidence` is untouched;
   reconciling the two vocabularies is §4.1's recorded hand-off, not this slice's.
4. **Ambiguity records are never thinned** (owner sign-off on 0012): stale records are
   deleted whole and re-derived. `Count` is always `len(Candidates)` — this slice never
   truncates, so any other value would be a lie the overlay's validation happens to permit.
5. **Determinism.** Every new reader returns rows in a stated, stable order; two passes
   over an unchanged workspace must produce byte-identical overlay content.
6. Only `internal/overlay` change permitted is the one-line `RefNS` **comment** in task 9.

### Learnings that bear on this change (read before tasks 7 and 8)

- **`one-invariant-many-sites-drifts`** (harvested from 0012, whose defect was exactly this
  shape). `nsPrefix` is one invariant with **two** call sites — rung 1 and suppression
  detection. Define it once; neither site restates the boundary rule; task 5's test asserts
  the two sites agree on a shared table. Also: when two functions explain opposite
  treatments of the same data in their doc comments, one is a bug.
- **`rollback-untested-when-errors-precede-the-transaction`.** This slice opens no
  transactions of its own, so the rollback half does not apply — but its corollary does:
  assert the **shape** a thing takes, not a name heuristic. Task 8's "member indexes are
  never written" bar is a byte-snapshot comparison, not a "we didn't call Open" grep.

---

## Task 1 — `graph.OpenExisting`: the non-creating index open

**Files:** `internal/graph/store.go` (or a small new file in `internal/graph`),
`internal/graph/*_test.go`.

Add:

```go
// OpenExisting opens an existing index read-only in the sense that matters: it never
// creates the file and never deletes-and-rebuilds on a version mismatch. Absence returns
// an error wrapping fs.ErrNotExist; a mismatch returns a version error naming both
// versions; an unreadable or corrupt file returns the underlying open/pragma error.
// Open remains the build path.
func OpenExisting(path string) (*Store, error)
```

Notes:

- `SchemaVersion() int` (`store.go:182`) and `FileSchemaVersion(path string) (int, error)`
  (`store.go:186`) are **already exported** — reuse them; do not invent a new accessor.
- The absence error must satisfy `errors.Is(err, fs.ErrNotExist)`.
- The version error must name **both** versions in its message (file's and
  `SchemaVersion()`), so an operator can act on it.
- Return a fully usable `*Store` — the new readers in tasks 2–4 are `*Store` methods, which
  is exactly why `OpenRaw` alone is insufficient.

**Tests (`-tags nollama`):**

- absent path → error, `errors.Is(err, fs.ErrNotExist)` true, **and the file is still
  absent afterward** (the whole point).
- a valid index built with `Open` → `OpenExisting` succeeds and can read from it.
- an index whose `user_version` is deliberately wrong (write it via `OpenRaw` +
  `PRAGMA user_version = <bogus>`) → error mentioning both versions, **and the file's bytes
  are unchanged afterward**. Snapshot the bytes before and compare.
- a corrupt/garbage file → error, bytes unchanged.

---

## Task 2 — `graph.UnresolvedEdges` and `graph.ProjectDefs`

**Files:** `internal/graph` (readers + their types), tests alongside.

```go
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

// UnresolvedEdges returns every edge with dst_symbol_id = 0 whose source is a real symbol
// (src_symbol_id != 0), in deterministic order.
func (s *Store) UnresolvedEdges() ([]UnresolvedEdge, error)

// ProjectDefs returns the TIER-0 definitions of name (optionally restricted to parent),
// in deterministic order.
func (s *Store) ProjectDefs(name, parent string) ([]Symbol, error)
```

- The `edges` VIEW exposes `src_file` but **not** `src_name`/`src_parent`, so
  `UnresolvedEdges` is a join from `edges` to `symbols` on `src_symbol_id` — not a
  single-table select.
- **File-level import edges carry `src_symbol_id = 0` and are excluded** (spec assumption 2):
  they have no source symbol, so no `SymKey` can name their source end. Their information is
  not lost — those imports are exactly the hints `H` rung 1 consumes.
- `ProjectDefs` filters `tier = 0` and returns `Symbol` (so `Symbol.QName()` supplies the
  stable key's `QName`). Admitting tier-1 would let a vendored copy become a cross-repo
  target — the exact failure member-over-dep precedence exists to prevent, one level out.
  `Definitions` cannot be reused: it neither filters tier nor returns `Tier`/`Namespace`.
- Order: `UnresolvedEdges` by `(src_file, src_name, src_parent, dst_name, kind, line)`;
  `ProjectDefs` by `(file, start_line, name)`.

**Tests:** a fixture index with resolved edges, unresolved symbol-sourced edges, and
file-level import edges — assert only the middle class is returned, in the stated order.
For `ProjectDefs`: a tier-0 and a tier-1 symbol of the same name → only tier-0 returned;
parent-qualified vs bare lookups; empty result is `nil`/empty, not an error.

---

## Task 3 — `graph.TierOneEdges`

```go
// TierOneEdge is UnresolvedEdge plus the resolved target's namespace.
type TierOneEdge struct { /* UnresolvedEdge fields + DstNamespace string */ }

// TierOneEdges returns every edge resolved to a tier-1 symbol, with the resolved target's
// namespace, in deterministic order. Same source-symbol requirement as UnresolvedEdges.
func (s *Store) TierOneEdges() ([]TierOneEdge, error)
```

Symmetric with task 2 — same source-symbol requirement, same ordering rule, plus the
namespace of the tier-1 symbol the edge currently points at (that is what the suppression
set is matched against in task 7).

**Tests:** an index with an attached depmap; assert tier-1-resolved edges come back with the
right `DstNamespace`, tier-0-resolved and unresolved edges do not.

---

## Task 4 — `graph.MemberMerkleRoot`

```go
// MemberMerkleRoot folds a member index's content state into one repo-level token:
// sha256 over "path\x00hash\n" for every merkle row in path order, then a
// "\x01depfiles\n" separator, then "path\x00namespace\x00version\x00curhash\n" for every
// depfiles row in path order. The value is opaque to the overlay, which compares it for
// equality only.
func (s *Store) MemberMerkleRoot() (string, error)
```

- **The `depfiles` half is not padding.** `merkle` covers project files only — `AttachMap`
  writes `depfiles`, never `merkle`. A merkle-only fold would leave the stamp unchanged
  across a re-attach or a dependency version bump, and §3.4's gate would then skip a member
  whose overlay contribution had in fact moved.
- `namespace` and `version` are folded alongside `curhash` for the same reason one level in:
  `Suppression.SuppressedVersion` comes from `depfiles.version`, not from file content.
- `DepFileState` is `{Path, Namespace, Version, MapHash, CurHash, Size, Mtime}` — there is
  **no `Modified` field** on the read-back struct even though the table has the column. Fold
  only fields that exist.
- This is explicitly **not** a policy commitment — §3.4 owns staleness policy and may replace
  the fold. It is not an overlay schema bump either; the overlay stores whatever string it is
  handed.

**Tests:** deterministic across two calls; changes when a project file's merkle hash changes;
**changes when a depmap is re-attached at a new version with no project-file change** (the
§3.5-adjacent bar the spec names); stable across an unrelated no-op.

---

## Task 5 — `internal/wsresolve/nsprefix.go`: the boundary matcher

New package `internal/wsresolve` (flat sibling of `graph`/`depmap`/`overlay`).

```go
// nsPrefix reports whether the import hint h falls inside the declared member namespace
// ns, matching only on a namespace boundary: exact equality, or h continuing after ns with
// one of the separators "/", "\", ".". Trailing separators on ns are trimmed first,
// because PSR-4 autoload keys are declared with one ("Symfony\Component\").
func nsPrefix(ns, h string) bool
```

Case-sensitive, purely lexical — no toolchain resolution. This is a **prefix** rule, the
opposite direction from `graph.nsMatch`'s bidirectional suffix alignment; `nsMatch` is
unexported and must **not** be exported or reused (its either-direction rule would match
`github.com/other/auth` against a member declaring `auth`, manufacturing `exact` cross-edges
out of a coincidence).

**Tests — the shared table.** One table over all four namespace shapes the scanner emits:

| ns | h | want |
|---|---|---|
| `github.com/acme/auth` | `github.com/acme/auth/token` | true |
| `github.com/acme/auth` | `github.com/acme/auth` | true |
| `github.com/acme/auth` | `github.com/acme/authz` | **false** |
| `Symfony\Component\` | `Symfony\Component\Console` | true (trailing sep trimmed) |
| `Acme\Auth` | `Acme\Auth\Token` | true |
| `flask` | `flask.helpers` | true |
| `flask` | `flasky` | **false** |
| `@nestjs/common` | `@nestjs/common/decorators` | true |

Per the `one-invariant-many-sites-drifts` learning, **this same table is asserted against
both call sites** — rung 1 (task 6) and suppression detection (task 7) — so the two can never
drift apart. Structure the test so the table is defined once and both site-level assertions
consume it.

---

## Task 6 — `internal/wsresolve/ladder.go`: the four rungs

Per unresolved edge in member S, with `N = DstName`, `Q = DstQualifier`, `H = DstNS`, and
`others = available members other than S, in manifest order`. **S is never a candidate** —
resolution inside S already ran and already lost, which is what made the edge unresolved.

`defs(X) = X.store.ProjectDefs(N, Q)`, **retried with `Q = ""` when the qualified form
returns nothing** — the qualifier is a lexical narrowing hint, and a cross-repo lookup must
not be stricter than the local one it extends (assumption 13; mirrors `graph.resolve`'s own
fall-through).

1. **Import-mediated (exact).** `H != ""` and exactly one `M ∈ others` satisfies
   `nsPrefix(ns, H)` for one of M's declared namespaces, and `len(defs(M)) == 1` →
   `CrossEdge{Src: key(S, edge.Src), Dst: key(M, defs(M)[0]), Kind, Provenance:
   "cross_repo_import", Confidence: "exact", Line}`. **The only rung that produces `exact`.**
2. **Unique bare name (inferred).** Otherwise `hits = {X ∈ others : len(defs(X)) == 1}`;
   if `len(hits) == 1` → same edge with `Provenance: "cross_repo_name"`,
   `Confidence: "inferred"`.
3. **Ambiguous.** Otherwise `cands` = every `(X, d)` for `X ∈ others`, `d ∈ defs(X)`; if
   `len(cands) >= 2` → `Ambiguity{Src, RefName: N, RefNS: H, Kind, Line,
   Candidates: ordered(cands), Count: len(cands)}`.
4. **Unresolved.** Otherwise, write **nothing** — no overlay row of any kind.

`ordered` puts the candidates of the single member named in S's manifest `deps` first —
**only when `deps` names exactly one candidate member** (D3's literal tiebreaker; naming two
or more means no reorder at all) — then remaining candidates in manifest order, then within a
member by `(file, start_line, name)`.

**Rung 2 fires on "no rung-1 hit", not on "H is literally empty"** (assumption 6, a
critic-ratified call). The frozen order is untouched — rung 1 still runs first and still
wins; this only settles the rung-1 *miss* case when H happens to be non-empty. The literal
reading is non-monotonic on its own terms: rung 3's guard carries no H condition, so a
rung-1 miss with non-empty H would still record an *ambiguous* answer when N resolves in ≥2
members, while the better-evidenced unique-hit case got dropped to unresolved. A consequence
is that **a rung-3 record may now carry a non-empty `RefNS`** — which is why task 9 corrects
the overlay comment.

`SymKey.QName` uses `graph.Symbol.QName()` (`Parent.Name`, else `Name`) at both ends —
assumption 3; do not mint a second convention.

**Tests — the §3.5 "ladder order" bar.** One fixture workspace exercising, in turn: rung 1
hit; rung 1 miss because the hint matches two members → falls to 2/3; rung 1 miss because N
is ambiguous inside the hinted member → falls to 2/3; rung 2 unique hit; rung 3 with `Count`
and candidate order, **with** the `deps` tiebreaker, **without** it, and with `deps` naming
two candidates (no reorder); rung 4 leaves the overlay empty for that edge. Plus: rung 1
beats rung 2 when both could fire; the degenerate **one-member workspace writes no
cross-edges and no ambiguities** (the assertable part of the deferred §3.5 single-member
bar); and a re-run in the other member order produces byte-identical overlay content.

Fixtures are small hand-built member indexes created through `graph.Open` + `PutFile` in a
temp dir — no repo checkouts, no network.

---

## Task 7 — `internal/wsresolve/suppress.go`: member-over-dep precedence

For each available member C, read `C.store.DepFiles()`; group by `DepFileState.Namespace`.
For each attached namespace `A`, find members `O ∈ present, O != C` such that
`nsPrefix(o_ns, A)` for one of O's declared namespaces — **the same `nsPrefix` from task 5,
called, not restated**.

- **Exactly one such O** → the member wins. Record
  `Suppression{ConsumerMember: C.ID, Namespace: A, OwnerMember: O.ID, SuppressedVersion:
  <version on C's depfiles rows for A, "" if unset>}`. When C's rows disagree on version
  (possible across re-attachments), take the **lexicographically smallest non-empty** value,
  deterministically.
- **More than one possible owner** → suppress nothing, record nothing. "The member wins"
  presupposes an unambiguous member; inventing a winner is a silent lie about which code the
  agent would edit (assumption 11).
- **Re-point C's affected edges.** Every `TierOneEdge` in C whose `DstNamespace` matches A
  becomes a ladder candidate, **entering at rung 1 with `H = A`** — which, since exactly one
  member owns A, selects O directly. Result is `cross_repo_import`/`exact` when N resolves
  uniquely in O, otherwise it falls through rungs 2–4 unchanged.

Re-pointing is the **named widening** of D3's stated candidate set (assumption 8) — these
edges are not "today's unresolved edges". It belongs here for a mechanical reason: producing
the cross-edge into O requires resolving N inside O's index, which *is* the ladder; deferring
it would duplicate the ladder in the query layer or force query-time re-resolution, the
option D2 rejected by name.

Suppression **does not mutate C's `graph.db`**: the tier-1 rows stay, the intra-repo edge
stays, and the overlay additionally carries the cross-edge into O.

**Recorded obligation to §4.1 (do not implement here):** because C's intra-repo tier-1 edge
survives alongside the new cross-edge, a union-graph query over C would count the same call
twice. §4.1 must read `dep_suppressions` and filter intra-repo edges whose resolved target is
a tier-1 symbol in a suppressed namespace. This slice writes the record that makes that
filter possible; nothing reads the overlay yet, so no double-count is observable before §4.1.
State this obligation in the package doc comment so it cannot be lost.

Known non-match, deliberately left alone: a member declaring only a composer name whose
vendored copy is recorded under a PSR-4 root will not match, so it is not suppressed. That is
a discovery gap (change 0010's territory); record nothing, resolve nothing — the safe
direction.

**Tests:** a member vendoring another member's namespace → record carries the right
consumer/owner/version; the affected tier-1 edge is re-pointed at the owning member; **two
possible owners suppress nothing**; version disagreement picks the smallest non-empty;
**the consumer's own `graph.db` is byte-unchanged**.

---

## Task 8 — `internal/wsresolve/wsresolve.go`: `Resolve` and the write order

```go
type Stats struct {
    MembersResolved    int // present members whose contribution was rewritten
    MembersUnavailable int // declared members with no usable index, untouched
    CrossEdges         int
    Ambiguities        int
    Suppressions       int
}

func Resolve(wsRoot string) (Stats, error)
```

The only exported entry point. **No CLI verb, no MCP surface, no plugin-note change** — D5
names `init-workspace` and `workspace-status` as the only new verbs and `workspace-status` is
§3.4's, so `Resolve` ships with no in-tree caller, exactly as §3.1 shipped `config.Resolve`
and §3.2 shipped the whole overlay package.

Order:

```
1. engine.DetectRootKind(wsRoot) must be RootWorkspace — else return an error NAMING the
   path (otherwise the manifest load reports it as a bare fs.ErrNotExist)
2. ws := config.LoadWorkspace(wsRoot)
3. present, missing := ws.Resolve(wsRoot)
4. ov := overlay.Open(overlay.Path(wsRoot)); defer ov.Close()
5. ov.ReplaceRegistry(ws)                      // mirror the manifest as-built
6. for each present member: graph.OpenExisting(<AbsRoot>/.codeindex/graph.db) — NEVER
   graph.Open; defer close. Absent, wrong-version, or unopenable => UNAVAILABLE, skipped
7. derive suppressions (task 7) over all available members
8. for each available member S (manifest order): run the ladder over S's UnresolvedEdges
   + its suppressed TierOneEdges, accumulating records in memory
9. for each available member S: ov.ReplaceMemberEdges(S.ID, nil, nil, nil)   // clear all
10. ov.PutCrossEdges(all); ov.PutAmbiguities(all); ov.PutSuppressions(all)   // non-deleting
11. for each available member S: ov.PutStamp(S.ID, memberMerkleRoot(S))      // stamps LAST
```

**Steps 9 and 10 are split deliberately — this is the load-bearing orchestration decision**
(assumption 7). `ReplaceMemberEdges(M, …)` deletes on **either** end, so the naive
derive-then-write-per-member loop has the call for M₂ deleting the `S₁ → M₂` edges the call
for M₁ just wrote. Clearing every available member first with empty inputs (which the API's
own validation accepts trivially — 0012's `TestReplaceMemberEdgesEmptyClears` already
exercises it) and then writing the whole derived set with the non-deleting `Put*` calls is
the only composition of the existing API that is correct for a full pass. **Add no new
overlay method.**

**Unavailable members** are treated exactly like missing ones: no candidates contributed, not
a candidate target, no stamp, overlay rows not cleared. Counted in `Stats.MembersUnavailable`;
the pass returns **no error** — an unbuilt member is a runtime condition, the same class as an
absent one, and D2/D4 require a workspace to keep answering while a member is unavailable.

**Missing members are left alone.** Step 9 does collaterally delete a missing member's rows
that are *incident to a present member* — unavoidable and correct, since a present member's
whole contribution is being rewritten and such an edge could not be re-derived at this pass.
**Rows joining two missing members survive untouched.**

**The pass is not one transaction and does not need to be.** Each overlay call is
individually atomic, the overlay holds no primary data, and every row is re-derivable by
re-running. Stamps last (step 11) means a pass that dies part-way leaves the affected members
stampless and §3.4's stamp gate re-resolves them — crash-safety carried by the ordering, not
by a transaction the API does not offer.

**Tests:**

- **Whole-pass write order** — the regression decision 7 exists for: a workspace where
  member 1 sources an edge into member 2; assert the edge is present after a full pass. It is
  *absent* if steps 9/10 are collapsed back into a per-member `ReplaceMemberEdges`, so this
  test must fail against that collapse. Verify that by actually trying the collapse locally
  before finishing the task.
- **Stable-key survival across member rebuild** (§3.5 bar): resolve; rebuild one member's
  `graph.db` from scratch so its symbol rowids are renumbered; assert every cross-edge's
  `SymKey` still names a live symbol in the rebuilt member, and a second `Resolve` produces
  the identical overlay row set.
- **Idempotence** — two consecutive passes over an unchanged workspace produce identical
  overlay content and identical stamps except `ResolvedAt`.
- **Missing member** — a declared member absent from disk contributes nothing, gets no stamp,
  and its rows joining another missing member survive a pass.
- **Member indexes are never written** — snapshot every member `graph.db`'s **bytes** before
  a pass, assert unchanged after; include a member whose index is **absent** (must stay
  absent, not be created) and one at a **wrong `user_version`** (must stay byte-identical,
  not be rebuilt). This is the regression test for the `graph.Open` hazard.
- **Non-workspace root** — `Resolve` on a `RootRepo` errors and names the path.
- **`Stats`** counts are correct across available / unavailable / missing members.

Also assert (cheap, and it is an acceptance item): `graph.Open` appears nowhere in
`internal/wsresolve`'s non-test code.

---

## Task 9 — comment correction and openspec bookkeeping

1. `internal/overlay/edges.go:36` currently reads
   `// RefNS is D3's namespace hint: empty for a rung-2 bare name.` — the literal-rung-2
   invariant, which task 6's rung-2 reading breaks (a rung-3 record may now carry a non-empty
   `RefNS`). Update it to say `RefNS` is the hint **as recorded, empty when the reference
   carried none**. **Comment only** — no schema change, no behavior change, nothing else in
   `internal/overlay` is touched.
2. `openspec/changes/workspace-graph/tasks.md`: tick **both** `3.2` (left unchecked though
   change 0012 is `done` — bookkeeping this slice corrects) and `3.3`.

---

## Acceptance checklist (from the spec — verify all before the suite gate)

- [ ] `internal/wsresolve.Resolve(wsRoot) (Stats, error)` exists, is the only exported entry
      point, and has no in-tree caller.
- [ ] No new CLI verb, no MCP change, no plugin-note change.
- [ ] `graph.UnresolvedEdges`, `graph.ProjectDefs`, `graph.TierOneEdges`,
      `graph.MemberMerkleRoot`, `graph.OpenExisting` added; no member `graph.db` schema
      change and no `graph.schemaVersion` bump.
- [ ] No pass creates, truncates, or otherwise writes any member `graph.db`; `graph.Open`
      appears nowhere in `internal/wsresolve`'s NON-TEST code.
- [ ] A present member with an absent, wrong-version, or unopenable index is handled as
      unavailable, counted in `Stats.MembersUnavailable`, and does not fail the pass.
- [ ] `MemberMerkleRoot` changes when `depfiles` changes with no project-file change.
- [ ] `internal/overlay/edges.go`'s `RefNS` comment updated; no other change to
      `internal/overlay`.
- [ ] `openspec/changes/workspace-graph/tasks.md` §3.2 and §3.3 both ticked.
- [ ] The four rungs fire in the frozen order; `exact` is produced by rung 1 only;
      `cross_repo_import` / `cross_repo_name` are the only provenance values written.
- [ ] Ambiguity records are never thinned; `Count == len(Candidates)`; the `deps` tiebreaker
      orders first only when it names exactly one candidate member.
- [ ] Suppressions are consumer-scoped, recorded only when exactly one member owns the
      namespace, and carry the vendored version from `depfiles`.
- [ ] A full pass writes every cross-edge regardless of member order.
- [ ] Missing members' overlay rows and stamps are not rewritten.
- [ ] Stamps are written last, one per resolved available member.
- [ ] `graph.Confidence` is untouched; the overlay stores `exact`/`inferred`.
- [ ] `go test -tags nollama ./...` green.

# Design: workspace-graph

## Context

The one-repo assumption today: identity is a filesystem path; every verb
threads `<repo-root>`; the index lives at `<root>/.codeindex/graph.db`
(`cmd/codeindex/main.go:308`); resolution runs in a single-DB
transaction with no repo concept in the schema
(`internal/graph/store.go:1052`); imports that leave the repo are stored
unresolved (`dst_symbol_id=0`, Go paths verbatim at
`internal/graph/store.go:374`). The proven seam is the depmap layer:
`internal/depmap` builds a separate index per dependency, caches it at
`~/.codeindex/depmaps/<ns>@<ver>.db`, and `AttachMap`
(`internal/graph/depmaps.go:81`) merges it in as tier-1 symbols with a
per-file hash overlay for local modifications. Cross-repo support is
that mechanism generalized — with the direction reversed (symmetric
queries need in-edges too) and freshness made first-class.

Primary use case (fixed by the owner): **query across a workspace of
projects/repos exactly as if it were a single graph.** Same verbs, same
answer shapes, same trust contract.

## Goals / Non-Goals

**Goals:** workspace manifest + scan; all graph verbs over the union
graph with per-reference repo provenance; overlay storage that preserves
per-repo sources of truth and the always-fresh contract; conservative
cross-repo resolution with M3-compatible provenance; a pre-registered
evidence gate.

**Non-Goals:** cross-workspace semantic search / shared vector store
(vectors stay per-repo; separate change); any UI (lore-graph spec's
"one repo root per serve process" stands); git-remote or network-based
repo identity; remote/artifact fetching of members; language expansion;
re-ranking or merged-list scoring of any kind (graph verbs return
complete sets, so no rank-merge policy exists to tune — keeping the
re-ranking freeze intact); runtime-evidence across repos.

## Decisions

### D1: Identity — manifest at the workspace root, filesystem only

`<workspace-root>/.codeindex/workspace.json`:

```json
{
  "version": 1,
  "members": [
    {"id": "api",    "root": "services/api",
     "namespaces": ["github.com/acme/api"]},
    {"id": "shared", "root": "libs/shared",
     "namespaces": ["github.com/acme/shared", "@acme/shared"],
     "deps": []}
  ]
}
```

Member id is the stable name used in answers and cross-edge records;
`root` is relative to the workspace root (members may live anywhere the
filesystem reaches, including outside the workspace directory via
relative paths); `namespaces` are the language-level namespaces the
member exports (auto-discovered by `--scan` from go.mod / package.json /
composer.json / Python top-level modules; manifest overrides win);
`deps` is optional and used only as an ambiguity tiebreaker (D3), never
as a build order. `--scan` SHALL also read monorepo member declarations
(go.work, pnpm-workspace.yaml, composer path repositories) in slice 1.
For repos scattered with no natural parent, a dedicated workspace
directory owning the manifest with relative paths out to the members
(`~/dev/acme-ws/.codeindex/workspace.json` → `../api`, `../shared`) is
the sanctioned shape — explicit, committable, zero global state. A repo
root with no manifest behaves exactly as today — single-repo mode is
the absence of a workspace, not a mode flag. Root-kind detection: a root containing `workspace.json` is a
workspace; containing neither manifest nor indexable source is an
error naming both possibilities.

Rejected: git-remote identity (network, auth, and detached-checkout
ambiguity for zero benefit at this scale); a machine-global registry
(hidden state; the manifest is reviewable and committable).

### D2: Storage — overlay DB, not copy-merge, not query-time re-resolution

`<workspace-root>/.codeindex/workspace.db` holds exactly three things:
the member registry (mirror of the manifest as-built), **cross-repo
edges**, and **per-member freshness stamps** (the member's merkle root
at last overlay resolution). Per-repo `graph.db` files are untouched
and remain individually buildable, patchable, and artifact-importable.

Cross-edges reference symbols by stable key (member id + file path +
qualified name), not by per-DB rowid — member rebuilds renumber symbol
ids, and the overlay must survive them. Keys are re-mapped to ids at
query time via the member's own DB.

Freshness: workspace `Fresh` = for each member, run the existing
per-repo freshen; then for each member whose merkle root differs from
its stamp, re-resolve only overlay edges incident to that member and
update the stamp. Unchanged members cost one stamp comparison. This is
the depfiles hash-overlay pattern promoted to repo granularity.

Rejected — copy-merge `workspace.db` (the subagent draft's Approach A):
duplicates every symbol, and the merged DB goes stale between merges,
breaking the single guarantee ("always fresh") the product's trust
story rests on. Rejected — pure query-time `ATTACH` + re-resolve:
per-query cost scales with workspace size, and SQLITE_MAX_ATTACHED
(default 10) caps workspace size from below; precomputed cross-edges
make query cost proportional to the answer instead.

### D3: Cross-repo resolution ladder (order frozen)

Candidate cross-edges are exactly today's unresolved edges — resolution
inside a member is unchanged and always wins first. For an unresolved
edge in member S with name N, namespace hint H:

1. **Import-mediated (exact-class):** H maps to exactly one member M's
   declared namespace (prefix match on namespace boundaries) and N
   resolves uniquely inside M → provenance `cross_repo_import`,
   confidence exact. This is the only rung that can produce exact.
2. **Unique bare name (inferred):** no H; N resolves in exactly one
   member other than S → provenance `cross_repo_name`, confidence
   inferred.
3. **Ambiguous:** N resolves in multiple members → candidates recorded
   with count; if S's manifest `deps` names exactly one candidate
   member, it is listed first (tiebreaker, still ambiguous).
4. **Unresolved:** stays unresolved, exactly as today.

**Member-over-dep precedence:** when a namespace is both claimed by a
workspace member and present as a tier-1 depmap attachment inside
another member (a shared lib that is a member *and* vendored into a
consumer), the member wins and the tier-1 attachment is suppressed for
that namespace. The member is the live, editable source; the vendored
copy is a snapshot — blast radius must point at code the agent would
actually edit. The suppression is recorded so `workspace-status` can
surface member/vendor version skew.

Consistent with the M1/M3 epistemics rule: confidence classes are
resolver-visibility claims. `exact` here means "the import binding
names exactly one member namespace and one symbol within it," not "the
true runtime target."

### D4: Query semantics — the single-graph illusion

`callers(X)` where X lives in member M = M's own callers (per-repo path,
unchanged) ∪ overlay in-edges into X, resolved to full references via
each source member's DB. `callees` and `nav` symmetric. `impact`'s
transitive closure crosses member boundaries **by default** — the
workspace-wide blast radius is the point of the feature, and the
coverage clause reports the boundary either way; no flag is required
to get the honest answer. `find` /
`grep` fan out across members and concatenate complete result sets
(these verbs return complete sets, not ranked lists — no merge scoring
exists; within-member order is preserved, members ordered by manifest).
Paths in answers are workspace-relative (`services/api/handlers/u.go`);
each reference carries `repo: "<member-id>"`. Anchor arguments accept
an optional `<member-id>:` prefix for disambiguation
(`api:HandleLogin`); bare anchors that match in multiple members return
the same multi-candidate disambiguation answer the single-repo path
returns today for duplicate names.

### D5: Surfaces

CLI: every verb's `<repo-root>` argument accepts a workspace root;
`init-workspace --scan` and `workspace-status` (per-member build/stamp
state) are the only new verbs. MCP: `codeindex mcp <workspace-root>`
serves the union graph; tool schemas gain the `repo` result field and
the optional anchor prefix — no new tools. Plugin: the ambient note is
untouched (house rule); workspace detection follows the CLI's.

### D6: M3 schema reservations (why this spec exists now)

M3's frozen edge schema SHALL reserve: provenance mechanism values
`cross_repo_import` and `cross_repo_name`; a coverage clause of shape
`workspace: {members_consulted, members_stale, boundary}` where
`boundary` states that symbols outside the workspace are unknown, and
the graph-vs-retrieval layer policy (Critic #5) names which layer a
workspace coverage clause describes per verb. Reserved enum values cost
nothing if this change never builds; their absence costs a schema
migration if it does.

### D7: Evidence gate (pre-registered, before any build)

Corpus: ≥30 cross-repo impact/caller tasks over a 3–5 member workspace
(2+ languages, one shared-library member with ≥2 consumers), mined
organically per M2's discipline; plus the existing single-repo goldens.
Arms: A = agent + shell with all member checkouts on disk (grep-across
control — honest, not blinded); B = A + workspace-graph MCP.

Bars, all required:
- Cross-repo caller/blast-radius recall: B ≥ A, and B ≥ 0.9 absolute
  on import-mediated (rung-1) edges.
- Efficiency: B uses ≥40% fewer exploration tokens or ≥40% fewer
  tool/shell calls on the cross-repo tasks (mirrors the M5 gate form).
- Non-regression: single-repo golden suite byte-identical for non-
  workspace roots; workspace-mode answers on a single-member workspace
  match single-repo answers modulo the `repo` field.
- Freshness property: mutate one member, query from another without an
  explicit rebuild → the answer reflects the mutation or its coverage
  clause names the member stale. Silent staleness is a hard fail.
- Discipline rule all four leak classes, including grader-blind
  formatting.

Kill condition: if B does not beat A on recall or efficiency, the
result is published as a FINDINGS entry and the change closes — the
grep-across control winning is a legitimate answer to the frontier
hypothesis.

## Risks / Trade-offs

- [Same-name collisions across members explode ambiguity] → rung 1
  requires import mediation; bare-name matches never exact; candidate
  counts surface in answers (M3 fields do the honest talking).
- [Overlay re-resolution cost on big members] → incident-edges-only
  re-resolve, stamp-gated; measure against the per-repo freshen budget;
  worst case is bounded by a full overlay rebuild which is itself
  bounded by the unresolved-edge count, not the symbol count.
- [Workspace freshen latency = sum of member freshens] → members
  freshen lazily (only those consulted by the query) where the verb
  permits; `workspace-status` makes cost visible. If real usage hits a
  wall, the artifact-import path (82.5s→1.5s) is the sanctioned answer,
  not a cache with weaker guarantees.
- [Schema reservations drift from what implementation needs] → the
  reservations are enum values + one coverage clause shape, chosen to
  be implementation-independent; D2/D3 can change without moving them.
- [Estimate optimism (debate Critic #4 pattern)] → no estimate is
  registered here; scope is gated by C2 fork outcome and priced then.

## Migration Plan

Spec-now: only D6's reservations ride into M3; zero runtime change.
Build (post-GO): additive — new files, root-kind branch in CLI/MCP;
single-repo behavior byte-identical (golden-gated); rollback = delete
`workspace.json`/`workspace.db`, per-repo indexes unaffected. Overlay
schema carries its own version; a version bump rebuilds the overlay
only (cheap), never member indexes.

## Open Questions

None — the four scoping questions (impact default, monorepo scan in
slice 1, member-over-dep precedence, dedicated workspace directory)
were settled by the owner 2026-08-17 and folded into D1, D3, and D4.

## Amendments

### 2026-08-18: D1 monorepo declaration sources — add lerna.json and package.json `workspaces`

D1's `--scan` monorepo source list (design.md:65-66) names only
`go.work`, `pnpm-workspace.yaml`, and composer path repositories. It
SHALL also read `lerna.json` (`packages`) and `package.json`
`workspaces` — **both** the array form and the object form
`{"packages": [...]}` — bringing the list to five sources.

Reason: the bench corpus's only monorepo (nest) declares its members
**solely** via `lerna.json`. With the original three sources the scan
discovers zero members there, so the amendment is what keeps the
monorepo path exercised at all. Everything else about D1 is unchanged:
manifest overrides still win, ids are still derived only for members
the scan introduces, and the over-collection guards apply identically
to hits from the two new sources.

### 2026-08-18: D7 merge-gate interpretation — query-behavior slices only

The frozen SHALL at
`openspec/changes/workspace-graph/specs/workspace-graph/spec.md:125`
("Implementation SHALL NOT merge before the pre-registered gate
passes") is read as gating **query-behavior** slices: work that can
change the answer any verb returns.

The §3.1 slice (manifest load/validate + `init-workspace --scan`)
changes no query answer. It adds a manifest loader with no query call
site, one new CLI verb that only reads and writes `workspace.json`, and
a root-kind detection helper that has no non-test caller. Single-repo
goldens are byte-identical by construction, not by measurement. On this
reading it may merge ahead of the gate.

**The limit, which is the load-bearing half of this interpretation:**
the gate still **hard-blocks §3.3+ and §4 from merging**. The
resolution ladder, overlay freshen, `workspace-status`, the union-graph
query paths, the CLI/MCP root-kind wiring, and the workspace goldens
are all query-behavior work and SHALL NOT merge until the
pre-registered D7 gate passes. This amendment narrows *when* the gate
binds; it does not weaken any bar, and it does not touch the kill
condition.

### 2026-08-19: D7 merge-gate interpretation — the block binds at wiring

The 2026-08-18 amendment's explicit block list named the resolution
ladder, but the list was written broader than the amendment's own
criterion ("work that can change the answer any verb returns"). Owner
ruling 2026-08-19: the criterion is the load-bearing rule, and the
hard-block binds where a verb gets **wired** to workspace/overlay data
— `workspace-status` (§3.4), the union-graph query paths, CLI/MCP
root-kind wiring, and the workspace goldens (§4.x). Those SHALL NOT
merge before the pre-registered D7 gate passes.

Unwired engine internals — the §3.3 ladder, whose only entry point
(`internal/wsresolve.Resolve`) has no non-test caller and whose output
no verb reads — change no answer any verb returns and may merge ahead,
on the same rationale that admitted §3.1 and §3.2. This narrows *where*
the gate binds; every D7 bar and the kill condition are untouched. If
the gate fails, unwired engine code is removed without any shipped
behavior to walk back.

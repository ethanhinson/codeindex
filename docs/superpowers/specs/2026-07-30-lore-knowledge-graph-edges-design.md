# Lore knowledge-graph edges + lore-in-impact + graph-health

Date: 2026-07-30
Status: design (approved for planning)

## Motivation

The primary aim is to **build dogfooding capability**: make codeindex more
usable *on itself* while we build it, and measure the impact of doing so. The
measurement is a **qualitative friction log** (a convention, not a feature)
that must span *indexing and enrichment* usage, not just call-graph queries —
because this project is becoming a knowledge graph of the codebase, closer to
Obsidian than to a plain call-graph index.

Two concrete gaps block that:

1. **No free-form record→record edge.** Lore has record→code (`anchors`),
   record→external (`refs`), and only two typed record→record edges
   (`supersedes`, `blocked_by`). There is no associative link — the Obsidian
   primitive that turns a pile of records into a graph.

2. **Code queries surface no knowledge in the terminal.** The one code→knowledge
   bridge, `relatedLoreBlock`, exists **only in the MCP tools**
   (`internal/mcpserver/mcpserver.go`) and matches **only by symbol anchor**.
   Running `codeindex impact <symbol>` from the CLI attaches zero lore — the
   single biggest dogfooding hole, since that is how we navigate our own code.

This spec closes both, and adds a cheap quantitative spine (graph-health in
`lore doctor`) under the qualitative friction log.

## Constraints (active decisions)

- **No graph.db coupling** (dec-01KYR17XECDN2T35W7ERZ932Y8). Record↔record
  edges and their derived index live in `.codeindex/lore.db`, never `graph.db`.
  The code→knowledge bridge stays a query-time join across the anchor, not a
  schema link. This design adds a `lore_links` table to lore.db only.
- **Lore must never break code navigation** (existing invariant in
  `relatedLoreBlock`: every error path collapses to empty). The CLI impact
  integration keeps that property — a lore failure degrades to the plain
  blast-radius output.
- Records are Markdown + YAML frontmatter, layered committed `.lore/` + private
  overlay (dec-01KYR17XEC92B4WWESXCHZ5XD6). The new edge is authored in
  frontmatter, like every other edge.

## Design

### 1. Data model — the `related:` edge

Add one field to `lore.Record` and the `wire` frontmatter struct:

```yaml
related:
    - dec-01KYR17XECDN2T35W7ERZ932Y8   # id
    - go-side-scoring-and-separate-lore-db   # or slug (resolved to id at index time)
```

- `Related []string` on `Record`; `related` added to `wire` (flow list) and to
  `knownKeys` (enforced by `TestKnownKeysCoversWireFields`).
- Round-trips through `Parse`/`Marshal` unchanged in shape from the other list
  fields (`blocked_by`, `tags`).
- **Source of truth is frontmatter.** `[[wikilink]]` body parsing is explicitly
  **deferred** (YAGNI) — revisit only if authoring friction proves real.
- Values may be a full id or a slug; resolution to a canonical id happens at
  index time (see §4). Unresolved values are retained and reported by doctor
  (§5), never silently dropped.

### 2. Index — `lore_links`

New table in lore.db:

```sql
CREATE TABLE lore_links (record_id TEXT, related_id TEXT);
```

- Written in `Store.Upsert` alongside the existing per-record edge tables
  (`lore_anchors`, `lore_refs`, `lore_blocked`, `lore_tags`): delete-by-record
  then re-insert, inside the same transaction.
- `related_id` is stored as the **resolved canonical id** where the slug/id
  resolves to a known record; otherwise the raw value is stored so doctor can
  flag it as dangling.
- Bump `schemaVersion`. The existing derived-data wipe-on-mismatch path
  (store.go Open) handles migration; add `lore_links` to the wipe list.

### 3. Backlinks

Inbound edges for a record = reverse of `lore_links` **unioned** with the
existing reverse edges already in the store:

- `lore_links.related_id = X` → X is linked *from* those records
- `supersedes = X`, `superseded_by = X`
- `lore_blocked.blocked_by = X`
- `discovered_from` — consumed **if present** (that field is a separate backlog
  item; this design does not add it, but the backlink query tolerates it).

Exposed two ways:

- `lore show <id>` gains a **"Referenced by:"** section listing direct (depth-1)
  inbound records.
- New `lore related <id>` view prints out-links and back-links together — the
  Obsidian backlink pane rendered as text — and supports **full-trace
  traversal** (see §4a): `lore related <id> [--depth N | --depth all]`, each
  reached record annotated with its distance and the edge type it arrived on.

### 4a. Traversal — full trace, cycle-safe, depth-bounded

Both `lore related` and the impact expansion (§5) walk the record graph rather
than doing a fixed one-hop lookup. One shared traversal primitive in
`internal/lore/index`:

```
Trace(recs, startID, opts) -> []Reached   // Reached = {id, distance, viaEdge, viaParent}
```

- **Cycle-safe**: a visited set keyed by record id; the graph has cycles
  (mutual `related`, supersede chains), so re-visits are skipped and the first
  (shortest) distance wins. Breadth-first so distances are minimal.
- **Edges walked**: `related` (both directions — the graph is treated as
  undirected for discovery), plus `supersedes`/`superseded_by` and
  `blocked_by`/its reverse. Anchors are *not* traversed as record→record edges
  (they bridge to code, handled at the query entrypoint in §5).
- **Depth is configurable and inferred, not fixed at 1.**
  - `--depth N` bounds the walk to N hops.
  - `--depth all` (and the `lore related` default) is **full trace**: walk until
    the connected component is exhausted, bounded only by the safety cap below.
  - The *inferred default* for a context where unbounded output would be noise
    (the `impact` block, §5) is a small N derived from output budget, overridable
    by the same flag — never a hard-coded 1.
- **Safety cap**: a total-reached ceiling (e.g. 200 records) stops pathological
  walks on a densely linked graph; when the cap truncates, the output says so
  (no silent truncation).

### 4. Resolution + shared formatter

- A small resolver maps a `related` value (id or slug) to a canonical record id
  using the in-memory record set already loaded for indexing. Slug match is
  exact against `lore.Slug(title)`; ambiguous or missing → left raw + flagged.
- **One formatter** renders the "Related lore" block for both CLI and MCP.
  Today `relatedLoreBlock` lives in `internal/mcpserver`; `cmd/codeindex`
  (package main) must not import mcpserver just for formatting. Move the
  matching + formatting into a **neutral internal package** (`internal/lore/index`,
  which already owns `RecordsForAnchor` and `StoredRecord`), exposing something
  like `index.RelatedLoreBlock(recs, symbol)`. Both `cmd/codeindex`'s `runImpact`
  and mcpserver's tools then call it. This retires part of the CLI/MCP
  duplication noted in itm-01KYSZT2F9K5CZYYEXZKFFT2Y7.

### 5. The dogfooding payoff — lore in `impact` (CLI + MCP), one hop

- Bring the Related-lore block to the **CLI `impact`** command (`runImpact` →
  composed after `query.ImpactText`). Same "never break navigation" contract.
- **Entrypoint**: records directly anchored to the queried symbol are the walk
  roots (distance 0). From those roots, `Trace` (§4a) expands along the record
  graph to the configured depth — so `impact Foo` surfaces the decision anchored
  to `Foo`, the note *it* links to, the item *that* note links to, and so on for
  a full trace when asked. `codeindex impact <symbol> [--related-depth N|all]`
  exposes the control; the default is the inferred small-N budget, not 1.
- Ordering within the block: distance ascending, then active/open first; each
  entry annotated with its distance from the anchored root so a deep trace stays
  legible. The prior cap becomes the §4a safety cap; truncation is stated.

### 6. Measurement — graph-health in `lore doctor`

`lore doctor` (`loreDoctor`, cmd/codeindex/lore.go) gains a graph-health
section reporting, over the record set:

- **Orphans** — records with no record→record edge (in or out) *and* no anchor.
  These are the "floating notes" an Obsidian graph view would show detached.
- **Dangling links** — `related` / `blocked_by` values that resolve to no
  record (typos, deleted targets).
- **Link density** — total record→record edges / record count.
- **Anchor health** — reuse existing stale/unresolved-anchor signals doctor
  already has; group them under this section.

Running `doctor` before and after a work session is the quantitative spine
under the qualitative friction log: it shows whether a session actually knit
the graph together (fewer orphans, higher density) or just added floating
records.

### 7. The friction log is a convention, not a feature

We do **not** build a `dogfooding` tag, record type, or capture path.
Friction observed while dogfooding a surface is recorded as an **ordinary lore
note**, `related:`-linked to the surface or record it concerns. That makes the
friction log a connected subgraph reachable by normal search and backlinks —
dogfooding the graph to record the experience of using the graph — with zero
new feature surface to maintain.

## Out of scope (separate backlog items that consume this)

- `lore board` rendered graph/board view — itm-01KYR5Z1KBBK2VW8AJ7E7CS9SC.
- `discovered_from` provenance + branch/pr fields — itm-01KYR5Z1KB4Z4DPZ31RZ914SS9
  (this design's backlink query already tolerates it).
- Notes-promotion pipeline — itm-01KYR5Z1KB8PYV49601870F1VY.
- `[[wikilink]]` body parsing.
- Full backlog filter/sort consolidation — itm-01KYSZT2F9K5CZYYEXZKFFT2Y7 (this
  design retires only the related-lore-block duplication as a side effect).

## Testing

- `record_test.go`: `related` round-trips through Parse/Marshal;
  `TestKnownKeysCoversWireFields` extended to cover the new key.
- `store_test.go`: `lore_links` upsert + reindex; schemaVersion-mismatch wipe
  includes `lore_links`.
- `index`: backlink union query; slug/id resolution (exact, ambiguous, missing);
  `Trace` traversal — depth bounding (N and `all`), cycle safety (mutual
  `related` and supersede loops terminate), shortest-distance-wins, edge-type
  and distance annotation, and safety-cap truncation reporting.
- `lore_tools_test.go`: shared formatter used by both CLI and MCP paths; CLI
  impact degrades to plain output when lore is unavailable.
- `loreDoctor`: orphan / dangling-link / density reporting on a fixture graph.

## Success criteria

1. `related:` authored in a record survives round-trip and appears in
   `lore show` out-links and the target's "Referenced by:".
2. `codeindex impact <symbol>` (CLI) surfaces the same Related-lore block the
   MCP tool does; `--related-depth N|all` traces the record graph from the
   anchored roots (distance-annotated, cycle-safe), and the block never fails
   the navigation output when lore is broken.
3. `lore doctor` reports orphans, dangling links, and link density; the numbers
   move measurably as we link the existing backlog.

# Lore + codeindex Graph UI — Design

Date: 2026-07-30
Status: approved (brainstorm) — pending implementation-plan breakdown
Branch: `worktree-lore-graph-ui` (off `main`)

## Summary

A single-pane, graph-native viewer for everything codeindex knows: the code
call graph (symbols + callers/callees), the lore layer (decisions, items,
notes with their `anchors`, `related`, `blocked_by`, refs, tags), and the
backlog. The primary way you move through the system is **traversing the
graph** — focus a node, expand its neighborhood, pivot — not clicking down a
nested tree. A classic list/board "Browse" mode exists as a secondary launcher
and scanning surface, not the default.

The unique value is the **join**: lore records are anchored to code symbols and
paths, so a single graph can show code blast-radius and decision blast-radius
together. No memory/backlog tool has this because none own a symbol graph
(see `dec-01KYR17XECA6GT2VX6QCGGRXKK`).

### Non-goals (v1)

- **Read-only.** No creating/editing/committing lore from the UI. All mutations
  stay in the CLI and MCP surface; the `.lore/` files remain the single source
  of truth.
- No multi-repo view. One repo root per `serve` process.
- No auth / remote hosting. Local dev tool bound to loopback.

## Decisions

### Platform: hybrid — web core + CLI, one read model

A Go `codeindex serve` command hosts an HTTP/JSON API and an embedded SPA. The
same Go **read-model layer** also backs CLI renderers (e.g. `lore board`). Web
is the rich graph surface; CLI is the terminal fallback. Both read through one
layer that reuses `internal/query` (call graph) and `internal/lore` (records).

### Frontend stack: embedded React + Vite + Cytoscape.js

- **Cytoscape.js** is the graph engine: it is built for interactive relationship
  graphs with mixed node types, multiple typed edges, expand/collapse, and real
  layout algorithms — the exact shape of this problem. (Rejected: react-flow —
  aimed at hand-built DAG/node editors; sigma.js — optimized for very large
  static graphs over interaction ergonomics.)
- **React + Vite** — turnkey, agent-friendly, strong Cytoscape integration. The
  compiled `dist/` is committed and served via Go `embed.FS`, so the binary
  stays single-file and Node is a dev-only dependency.
- Kept lean: React Query for server-cache, minimal CSS, no global state library.
  (Rejected: Svelte — smaller bundle but weaker turnkey graph ecosystem;
  server-rendered + htmx — leanest, but a robust interactive graph canvas fights
  the server-render model.)

### Interaction model: graph-native navigation (primary)

- Enter via a **command palette / search** → land on a focus node.
- The canvas shows **focus + 1-hop neighborhood** (progressive disclosure), never
  the whole universe at once.
- **Click a neighbor to re-focus** (canvas re-centers/animates); **expand** a node
  to pull in its neighbors. This expand/pivot loop *is* "dig deeper."
- Focused-node **details render in a contextual inspector** (peek card / drawer),
  not a separate full-page view — you stay on the canvas.
- **Browse mode** (lists + backlog board) is secondary: a scanning surface and a
  launcher into the graph.

## Architecture

```
┌── codeindex serve (Go) ───────────────────────────────────┐
│  read-model layer  ── reuses internal/query + internal/lore│
│     ├── HTTP/JSON API  ──►  React SPA (served via embed.FS)│
│     └── CLI renderers  ──►  lore board / graph (ASCII)     │
└────────────────────────────────────────────────────────────┘
   SPA surface (graph-primary):
     ┌──────────────────────────────────────────────┐
     │  ⌘K command palette / search                   │
     │  ┌───────────── graph canvas ──────────────┐  │
     │  │  focus + neighborhood, expand / pivot    │  │
     │  │  typed nodes · typed edges · semantic    │  │
     │  │  zoom · path trace · breadcrumbs         │  │
     │  └──────────────────────────────────────────┘  │
     │   inspector drawer (focused node detail)  ◀── contextual, collapsible
     │   [Browse mode] ── lists + backlog board (secondary)
     └──────────────────────────────────────────────┘
```

### The unified graph model

Nodes (typed, distinct visual language):

- **symbol** — a code symbol (function/method/type) from the call graph.
- **decision** / **item** / **note** — lore records.
- (paths appear as anchor targets; treated as lightweight nodes or inspector
  metadata — resolved during Epic B.)

Edges (typed, individually filterable, colored):

- **calls** — symbol → symbol (from the call graph; callers/callees).
- **anchors** — lore record → symbol/path (the cross-domain join).
- **related** — lore record ↔ lore record (free-form edge,
  `dec-01KYTG2C8BPFS0GV787Y8AA4QM`).
- **blocked_by** — item → item.

## Epics

Priority key: **P0** = v1-critical for the graph-native loop; **P1** = v1 but can
land progressively; **P2** = trailing / after the loop feels good.

### Epic A — Read model + `serve` host  (P0, foundation)

A Go read-model layer that reuses `internal/query` and `internal/lore`, plus a
`codeindex serve` command: an HTTP server that hosts JSON endpoints and the
embedded SPA (`embed.FS`), bound to loopback. Everything depends on this.

Acceptance: `codeindex serve <root>` starts, serves the SPA shell, and answers a
health/version endpoint; read-model layer is consumed by at least one endpoint.

### Epic B — Read API: graph + cross-domain join  (P0)

The domain endpoints, with the graph endpoints as the *primary* API:

- `graph(focus)` — nodes + typed edges for a focus node's neighborhood.
- neighborhood expand (`node → its neighbors`), and **path trace** between two
  nodes (reuses lore `related`-traversal + call graph).
- symbol lookup + callers/callees/impact.
- lore list/get for decisions·items·notes; backlog (status → priority → age).
- the **join**: lore anchored to a symbol/path, and the symbols a record touches.

Acceptance: given a focus id, the API returns a typed node/edge neighborhood
mixing symbols and lore; expand and path-trace return correct sub-graphs.

### Epic C — Graph-canvas shell  (P0)

Canvas-dominant React shell: command-palette/search entry, single focus/selection
state driving the canvas + inspector, inspector drawer, and traversal
breadcrumbs (navigable history of your path). Replaces any co-equal multi-pane
layout.

Acceptance: search → focus → canvas renders neighborhood; breadcrumbs record the
traversal; inspector reflects the focused node.

### Epic F — Graph navigation & interaction  (P0, headline)

The core experience:

- focus + context with progressive disclosure (1-hop, expand on demand).
- **semantic zoom** — labels → summary cards → full inline cards.
- **typed visual language** for node kinds; **typed, filterable, colored edges**.
- expand / collapse / pivot with animated force layout + node pinning.
- **path tracing** — highlight how two nodes connect.
- **saved views / pinned lenses**.

Acceptance: a user can start from one node and navigate the whole reachable
system by expanding and pivoting, filter by node/edge type, trace a path, and
save a view — without ever leaving the canvas.

### Epic E — Inspector (contextual detail)  (P1)

The peek/drawer on the focused node (not a full-page drill-down):

- lore record: structured frontmatter + Markdown body + anchors/refs/related/
  blocked_by as clickable chips that re-drive graph focus.
- symbol: signature, `file:line`, caller/callee counts, jump-to-impact.

Acceptance: focusing any node populates the inspector; every cross-link in it
re-focuses the graph.

### Epic D — Browse mode (lists + backlog board)  (P1, secondary)

Secondary scanning surface and graph launcher: the backlog board with readiness
cells (status → priority → age; the "waiting on <id> — needs your merge"
distinction), plus decision/note lists and symbol search, filterable by
status/priority/tag. Selecting a row launches into the graph.

This fulfills the existing item `itm-01KYR5Z1KBBK2VW8AJ7E7CS9SC` (lore board).

Acceptance: board renders grouped/readiness cells; any row opens that record as
graph focus.

### Epic G — CLI parity renderers  (P2, trailing)

The terminal half of the hybrid over the *same* read model: `lore board` and an
ASCII relationship/tree view. Keeps the hybrid honest.

Acceptance: `codeindex lore board` (and an ASCII graph view) render from the
shared read-model layer, deterministic, stdout by default.

## First vertical slice

Proves graph-native navigation end-to-end before deepening any single epic:

**A** (serve + one endpoint) → **B** (`graph(focus)` + neighborhood) → **C**
(canvas shell + command palette) → **F** (focus, expand, pivot, typed
nodes/edges) → **E** (inspector peek).

Then deepen **F** (semantic zoom, path trace, saved views), add **D** (Browse),
and **G** (CLI) last.

## Open questions (resolve during planning, not blocking)

- Path-node modeling: are file paths first-class graph nodes or inspector-only
  anchor metadata? (Epic B.)
- Graph endpoint shape: single `graph(focus, depth, edgeTypes)` vs. separate
  neighborhood/expand/path endpoints. (Epic B.)
- Semantic-zoom thresholds and default neighborhood size (perf vs. legibility).
- Where the Vite build slots into the Go build/release flow (committed `dist/`
  vs. build-time generation).

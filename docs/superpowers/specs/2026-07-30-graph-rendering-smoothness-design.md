# Graph Rendering Smoothness — Overview-First, Ranked Reveal

**Date:** 2026-07-30
**Status:** Approved
**Builds on:** [2026-07-30-lore-graph-ui-design.md](2026-07-30-lore-graph-ui-design.md) (first slice: serve + full-graph SPA)

## Problem

The whole-graph view renders everything at once and feels rough on all four axes:

1. **Initial load/layout** — `/api/graph/full` returns ~1.5k nodes / ~4.6k edges (this repo); fcose lays out the entire graph with `randomize: true` in one blocking pass. Multi-second freeze with no feedback.
2. **Layout instability** — `randomize: true` means a different map on every load; users lose their bearings.
3. **Visual overload** — all symbols, labels, and edges draw at every zoom level. The result is a hairball; important structure is buried.
4. **Interaction framerate** — the hover handler adds/removes a `dim` class on **all** ~6k elements on every `mouseover`/`mouseout`, causing a full style recalc per mouse transit. All edges/labels render during pan/zoom.

## Decision (approach)

**Client-side aggregation over the existing single `/api/graph/full` payload.** No backend changes. The client never renders the raw full graph on landing; it derives an overview graph and reveals detail on demand, entirely in memory — expansion costs zero network round-trips.

Rejected alternatives:

- **Server-side aggregation endpoints** (`/api/graph/overview`, `/api/graph/package`): scales to much larger repos, but every expand pays a round-trip, and at this scale layout/rendering — not payload — is the bottleneck. Clean later upgrade behind the same client abstraction if payloads outgrow a single fetch (~20k+ symbols).
- **Rendering-engine swap (sigma.js/WebGL, semantic zoom):** a rewrite of styles/compounds/interactions; Cytoscape is nowhere near its limits at this node count once we stop rendering everything at once.

## Design

### 1. Aggregation module (`web/src/graph/aggregate.ts`)

Pure functions, no cytoscape imports — full graph in, view model out:

- **Overview graph:** one node per package group, sized by symbol count; one *bundled* edge per package pair carrying `count` (number of underlying call edges, mapped to edge width). Lore nodes (decision/item/note) appear individually in the overview, attached to the package(s) their anchored symbols live in; lore↔lore edges (e.g. `blocked_by`) pass through unchanged.
- **Ranking:** per package, symbols sorted by total degree (in+out, computed once over the full edge list). Exposes `topSymbols(pkg, n)` and the remainder for the `+N more` tail.
- **Visible-edge resolution:** given the set of currently visible symbol ids, returns (a) concrete symbol↔symbol edges where both ends are visible, (b) symbol↔package bundled edges where only one end is visible, (c) package↔package bundled edges otherwise. Collapse re-bundles by construction.

Unit-tested against fixture graphs (empty, single package, cross-package edges, lore anchored in two packages).

### 2. Expand / collapse interaction

- **Landing view:** overview only (~60 nodes for this repo). Lays out in tens of milliseconds.
- **Expand (tap a package):** package node becomes a compound parent containing its top **12** symbols by degree plus one `+N more` chip node. Tapping the chip reveals the remaining symbols in degree order (all of them — one tap, no pagination ladder). Tapping an expanded package's header (or a dedicated collapse control) collapses it back to a single node.
- **Edges follow visibility:** only edges between visible symbols render concretely; everything else stays bundled at package level via the resolver above.
- Expanded state lives in React state (`Set<string>` of expanded package names + `Set<string>` of packages with tail revealed); the canvas component diffs desired elements vs. present elements and adds/removes incrementally — never `elements().remove()` + full rebuild.

### 3. Deterministic, incremental layout

- **Initial placement:** packages seeded on a circle ordered by a stable hash of package name; fcose runs with `randomize: false` to refine from that deterministic start. Same repo → same map every load.
- **Expansion layout:** no layout runs on expand at all — newly revealed children are placed on a deterministic phyllotaxis spiral around their package's current position, so every pre-existing node is pinned by construction. The map never jumps under the user.
- **Viewport:** layouts stay `animate: false`; viewport moves (fit on load, center on select/expand) animate ~300ms. Motion where it orients, none where it costs.

### 4. Interaction smoothness

- **Hover:** maintain a ref to the previously highlighted collection; on hover, compute the new closed neighborhood and toggle classes only on the symmetric difference. Dimming uses a class on the *container* driven by a stylesheet rule (`.hovering node:not(.hl)`) if feasible, otherwise classes scoped to visible elements only. ~10ms debounce so fast mouse transit doesn't thrash.
- **Level of detail (LOD):** on zoom-band change (crossing a threshold, checked in a `zoom` handler — not per-frame work): below the threshold, symbol labels hide and bundled edges with `count === 1` hide. Class toggles only fire when the band actually changes.
- Existing `textureOnViewport: true` / `hideEdgesOnViewport: true` stay as-is; `pixelRatio: 1` stays (revisit only if blurriness is reported after the element count drops).

### 5. Search and deep links

Behavior preserved, mechanics updated: search / `?focus=` resolves against the **full** node list (not just visible nodes). Selecting a symbol auto-expands its package, force-reveals the symbol even if it's in the long tail, highlights its neighborhood, and centers it. The Inspector continues to receive the full nodes/edges arrays, so its data is unaffected by what's rendered.

### 6. Error handling

Unchanged from the first slice: load errors banner in `App`. Aggregation is total (no throws on odd data — unknown groups fall into an `(ungrouped)` package; edges referencing missing nodes are dropped, counted, and logged to console once).

## Components

| Unit | Responsibility |
|---|---|
| `graph/aggregate.ts` (new) | Pure aggregation: overview graph, degree ranking, visible-edge resolution |
| `graph/GraphCanvas.tsx` (rework) | Incremental element diffing, pinned/seeded layouts, hover diffing, LOD bands, expand/collapse events |
| `App.tsx` (edit) | Owns expanded/revealed state; wires search → auto-expand; passes view model to canvas |
| `graph/style.ts` (edit) | Styles for package nodes (sized), bundled edges (width ∝ count), chip node, LOD classes |
| `useFullGraph.ts` (unchanged) | Single full-graph fetch |

## Testing

- **Unit (vitest):** `aggregate.ts` — overview construction, ranking order, tail counts, edge resolution across visibility states, degenerate inputs.
- **e2e (Playwright):** lands on overview (package count visible, no symbol labels); expand shows top-12 + `+N more`; chip reveals tail; collapse re-bundles; search for a known tail symbol auto-expands and selects it; reload produces the same overview positions (assert two loads' node positions match within tolerance).

## Out of scope

- Server-side aggregation endpoints (future, behind the same client abstraction).
- WebGL/sigma.js renderer swap.
- Persisting user layout edits (drag positions) across sessions.

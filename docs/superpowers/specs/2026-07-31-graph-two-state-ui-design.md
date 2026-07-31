# Graph UI Clarity — Two-State Model (Overview ⇄ Package Focus)

**Date:** 2026-07-31
**Status:** Approved (mockup reviewed in visual companion; user added the motion requirement)
**Builds on:** [2026-07-30-graph-rendering-smoothness-design.md](2026-07-30-graph-rendering-smoothness-design.md) (overview-first, ranked reveal — merged in b9e298b)

## Problem

The overview-first rework helped, but the live UI (screenshot review, 2026-07-31) still fails on clarity:

1. **Expanded-package label soup** — revealing a 100-symbol tail crams everything into the compound; every symbol renders its label; the result is an unreadable blob that also swallows neighboring packages.
2. **Lore pile-ups** — lore nodes (especially session notes with long dated titles) collide into unreadable rows; far-flung lore drags long dotted `blocked_by`/`anchors` edges across the whole canvas.
3. **Package overlap** — compound growth pushes packages onto each other.
4. **Always-on labels** are the root cause of most of the soup.

## Decisions (user-selected, one fork at a time)

1. **Tail reveal → Focus mode.** In-place compound expansion (top-12 + chip) is **removed entirely**. Tapping a package enters a focused sub-view where that package gets the whole canvas. Rejected: list-panel reveal (kept map static but hid structure); keeping in-place expand alongside focus (two expansion paradigms; the blob returns via the tail).
2. **Focus context → Neighbor satellites.** Packages the focused one calls / is called by appear as small package chips pinned around the rim, with bundled edges to the exact symbols they touch. Tapping a satellite refocuses on it (ego-graph navigation). Rejected: dimmed-map underlay (100-symbol packages still fight the map for space); pure isolation (loses all surrounding structure).
3. **Labels → Earned labels.** A node renders its label only when it earns it: top ~8 by degree in the current view, hovered node + neighbors, selected node, or zoom past a reading threshold (then all label). Rejected: zoom-only (hubs anonymous when zoomed out); always-on with harder truncation (soup shrinks but survives).
4. **Lore → HTML rail + kind filter.** Lore leaves the canvas/force-layout entirely: a right-side DOM panel groups lore by kind (decisions, items, notes), sorted by recency, one truncated line each; **session notes off by default**, legend-style chips toggle kinds. Cross-links are hover-sync (rail item ↔ anchored packages/symbols highlight both ways), not drawn edges; `blocked_by` renders as a badge inside the rail. Clicking a rail item opens it in the Inspector (which lives in the same right panel as a detail view). Rejected: anchor-adjacent canvas placement (crowded packages collect their own pile); hidden-until-asked (lore stops being ambient).
5. **Motion (user addition).** The graph should feel alive, Obsidian-style: subtle idle oscillation on nodes, plus animated transitions between states. Constraint: motion must not break the determinism guarantees — see §Motion.

## Design

### 1. Two view states

```
state = { mode: 'overview' } | { mode: 'focus', pkg: string }
```

- **Overview (landing):** package nodes only (~30–50), bundled edges (width ∝ call count — unchanged), no symbols, no lore, no compounds ever. Node spacing gets a modest bump now that compounds no longer distort the map. Deterministic layout machinery carries over unchanged.
- **Focus:** the focused package's symbols laid out by their real intra-package call structure — seeded fcose over the package's symbols + intra edges, deterministic per package (same seeded-`Math.random` wrapper, seed derived from package name). Satellites (packages with ≥1 call edge to/from the focused package's symbols) pinned on a rim circle, sorted by name; bundled satellite↔symbol edges aggregate per (satellite, symbol) pair with counts. Breadcrumb (`⟵ overview / <pkg>`) and Esc return to overview. Tapping a satellite refocuses.
- **URL:** `?pkg=<group>` for focus state, plus `&focus=<symbol>` when a symbol is selected. Search/deep-link for a symbol enters focus on its package and selects it. Plain `/` is the overview. Back/forward buttons work (history push on state change).

### 2. View-model layer (`web/src/graph/aggregate.ts` grows; stays pure)

- `overviewVM(index)`: package nodes + bundled package↔package edges. (Today's `viewModel` minus expansion/chip/lore paths — expansion state, `tails`, `extras`, and chip logic are deleted.)
- `focusVM(index, pkg)`: symbol nodes of `pkg` (all of them — density is handled by canvas + labels, not by hiding), intra-package concrete edges, satellite package nodes, bundled satellite edges.
- `earnedLabels(vm, hovered, selected, zoomBand)`: returns the set of node ids that currently render labels (top-8 by degree in view ∪ hovered neighborhood ∪ selected ∪ all-if-near-band). Pure; unit-testable.
- `loreRailModel(index, filters)`: lore records grouped by kind, sorted by recency (falls back to id order when no date), with per-record anchored package list for hover-sync; `blocked_by` resolved to badge references. Session-note detection: `note` records whose title matches `/^Session /` (the capture convention) — they land in a `sessions` group, off by default.

### 3. Canvas (`GraphCanvas.tsx` rework)

- Renders whichever VM the state demands; state transitions swap element sets via the existing incremental diff.
- **Labels:** node `label` style keys off a `labeled` class only; `earnedLabels` output is diffed onto elements (same symmetric-difference pattern as hover). No label collision handling needed — at most ~a dozen labels render at once outside the near band.
- **Layout:** overview identical to today (seeded circle + fcose `randomize:false`). Focus: symbols seeded on a phyllotaxis disc at canvas center, fcose `randomize:false` over intra edges under the seeded wrapper; satellites placed analytically on the rim (no layout). Both deterministic; anchors recorded per §Motion.
- Hover diffing, LOD band machinery, `pixelRatio`/`textureOnViewport` settings all carry over.

### 4. Lore rail + Inspector (right panel, `App.tsx` + new `LoreRail.tsx`)

Right panel hosts two stacked sections: the Inspector detail (when something is selected) and the lore rail. Rail state: kind filters (`decisions`/`items`/`notes` on, `sessions` off). Hover-sync: rail hover adds a highlight class to anchored package/symbol nodes present on canvas; canvas hover/selection of a package or symbol adds a highlight class to matching rail rows. In focus mode the rail filters to lore anchored in the focused package (with a chip to show all).

### 5. Motion

Two layers, both subordinate to determinism:

- **Anchors are truth.** Every node's deterministic position is its *anchor*, stored in node data (`ax`, `ay`). Layouts write anchors. All determinism/no-move e2e assertions read anchors (or run with motion disabled).
- **Idle oscillation:** rendered position = anchor + a small Lissajous offset — amplitude ≤ 2.5px, per-node period 5–9s and phase both derived from an FNV-1a hash of the node id (spatially deterministic, time-varying). One `requestAnimationFrame` loop updates positions in a single `cy.batch` per frame, throttled to 30fps; it pauses during pan/zoom/drag gestures, while a layout runs, when the tab is hidden (rAF does this natively), and entirely when `prefers-reduced-motion` is set or `?motion=0` is in the URL (the e2e hook). Oscillation applies only to symbol and package nodes currently visible.
- **State transitions (~350ms, ease-out):** entering focus, the tapped package's symbols fade+scale in from the package's anchor while the viewport animates to fit; satellites fade in at the rim. Leaving focus reverses (symbols fade out toward the package anchor, overview fades back, viewport fits). Hover labels fade in ~120ms via CSS-like style transition on the `labeled` class.
- **Perf guardrail:** the oscillation loop must skip frames while cytoscape reports an active user gesture and must be a no-op ≤ 0.5ms/frame at overview scale; if the focus view's node count makes 30fps infeasible, oscillate only labeled/hub nodes there.

Implementation note (2026-07-31): the focus-exit reverse morph and satellite fade-in were cut during implementation — exit is an instant element swap with an animated viewport fit; revisit only if exit feels jarring in use.

### 6. Error handling

Unchanged foundations: dropped-edge count + one-time warn; aggregation total (no throws). New: focusing a package name that doesn't exist (stale URL) falls back to overview with a console.warn; empty packages render an empty-focus hint instead of a blank canvas.

## Components

| Unit | Responsibility |
|---|---|
| `graph/aggregate.ts` (rework) | `overviewVM`, `focusVM`, `earnedLabels`, `loreRailModel`; expansion/chip logic deleted |
| `graph/GraphCanvas.tsx` (rework) | State-driven element diffing, anchors, labeled-class diffing, transitions |
| `graph/motion.ts` (new) | Oscillation loop: hash-derived offsets, rAF/30fps, pause conditions |
| `LoreRail.tsx` (new) | Kind-grouped lore list, filters, hover-sync events, blocked_by badges |
| `App.tsx` (rework) | `view` state + URL/history sync, selection, hover-sync wiring, right-panel composition |
| `graph/style.ts` (edit) | `labeled` class labels, satellite style, highlight classes; chip/compound styles deleted |
| `useFullGraph.ts` (unchanged) | Single full-payload fetch |

## Testing

- **Unit:** `focusVM` (satellite derivation, intra-edge selection, bundling counts), `earnedLabels` (top-8, hover/selection/zoom unions), `loreRailModel` (grouping, session filtering, recency sort, anchor lists), motion offset function (bounded amplitude, hash determinism).
- **e2e (all with `?motion=0` except the motion test):** overview has zero symbol/lore canvas nodes; tap package → focus with satellites + only hub labels rendered; tap satellite → refocus; Esc → overview; browser back returns to prior state; search from overview lands focused + selected; lore rail hides session notes by default and hover-sync highlights; determinism of overview and focus anchors across reloads; one motion smoke test (without `motion=0`): two anchor snapshots 500ms apart are identical while rendered positions differ.

## Out of scope

- Server-side aggregation (unchanged stance).
- Multi-package focus / comparison views.
- Persisting rail filter choices across sessions.
- Renderer swap (Cytoscape stays; oscillation perf guardrail above is the tripwire to revisit).

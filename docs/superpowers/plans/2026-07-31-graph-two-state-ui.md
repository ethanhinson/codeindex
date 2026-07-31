# Graph Two-State UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace in-place package expansion with a two-state graph (overview ⇄ package focus with rim satellites), earned labels, an HTML lore rail with hover-sync, and anchored motion (idle oscillation + state transitions).

**Architecture:** `aggregate.ts` stays the pure view-model layer and gains `overviewVM` / `focusVM` / `earnedLabels` / `loreRailModel` (expansion/chip logic deleted). `GraphCanvas` renders whichever VM the `view` state demands, diffing incrementally; every deterministic position is an *anchor* stored in node data (`ax`/`ay`), and a new `motion.ts` renders a small hash-seeded oscillation offset on top at 30fps. Lore leaves the canvas for a DOM rail (`LoreRail.tsx`); cross-links are class-based hover-sync, not drawn edges. `App.tsx` owns `view` + URL/history sync.

**Tech Stack:** React 18, cytoscape 3.30 + cytoscape-fcose, Vite, vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-07-31-graph-two-state-ui-design.md`

## Global Constraints

- Work on branch `feature/graph-two-state-ui` in an isolated worktree (created via superpowers:using-git-worktrees at execution start). All paths below are relative to that worktree root.
- `cd web && npm run build` must stay green at every commit that claims it (runs `tsc --noEmit`, then emits into `../internal/webserver/dist`, which the Go binary embeds).
- `web/src/graph/aggregate.ts` stays pure: no cytoscape or react imports; no throws on odd data. `web/src/graph/motion.ts` may import cytoscape **types** only (`import type`); its offset math is pure.
- Determinism: anchors (`ax`/`ay` node data) are a deterministic function of (payload, view). Oscillation and transitions never modify anchors. `Math.random` is only ever overridden inside the seeded-layout wrapper.
- Motion disables entirely when `prefers-reduced-motion: reduce` matches or the URL has `motion=0`. Oscillation amplitude ≤ 2.5px, throttled to 30fps, paused while a node is grabbed, while a transition/layout runs, and within 150ms of a user pan/zoom event.
- Earned labels: `LABEL_TOP = 8` symbols by degree; package/satellite nodes always labeled; hovered (`.hl`) and selected (`.sel`) nodes labeled; zoom ≥ `LABEL_NEAR_ZOOM = 1.1` labels everything.
- Session notes are `note` records whose label matches `/^Session /`; they are a separate rail group, hidden by default.
- Node id namespaces: package nodes `pkg:<group>` (both overview role `map` and focus role `satellite` — same id, so diffs carry them between views); symbol/lore ids pass through from the API. No `chip:` ids remain anywhere.
- Cytoscape stylesheets are strictly order-precedence (no specificity): generic rules first, feature rules (`edge[?bundled]`) after them, per-kind edge rules last. Do not reorder existing style blocks except as a task explicitly instructs (see lore note-01KYWFH3ZV61A9CA43XX7QSH54).
- Backend (`internal/webserver`, `internal/readmodel`) untouched.

---

### Task 1: aggregate.ts rework — overviewVM, focusVM, earnedLabels, loreRailModel

**Files:**
- Rewrite: `web/src/graph/aggregate.ts`
- Rewrite: `web/src/graph/aggregate.test.ts`

**Interfaces:**
- Consumes: `Graph`, `GraphNode`, `GraphEdge` from `web/src/types.ts` (unchanged).
- Produces (used by Tasks 4, 5, 6):

```ts
export const LABEL_TOP = 8
export const UNGROUPED = '(ungrouped)'
export function pkgId(name: string): string          // `pkg:${name}`
export interface GraphIndex { /* unchanged from current file */ }
export function buildIndex(g: Graph): GraphIndex     // unchanged from current file
export interface VisNode {
  id: string
  kind: 'package' | 'symbol'
  label: string
  degree: number
  symCount?: number                 // package nodes
  pkg?: string                      // symbols: owning package
  role?: 'map' | 'satellite'        // package nodes only
}
export interface VisEdge { id: string; source: string; target: string; kind: string; count: number; bundled: boolean; conf?: string }
export interface ViewModel { nodes: VisNode[]; edges: VisEdge[] }
export function overviewVM(index: GraphIndex): ViewModel
export function focusVM(index: GraphIndex, pkg: string): ViewModel
export function earnedLabels(vm: ViewModel, selected: string | null, zoomNear: boolean): Set<string>
export type LoreKind = 'decision' | 'item' | 'note'
export interface LoreRecord {
  id: string
  kind: LoreKind
  label: string
  status?: string
  pkgs: string[]        // package groups its anchored symbols live in (deduped, sorted)
  blockedBy: string[]   // lore ids this record is blocked by (blocked_by edges outgoing from it)
  session: boolean
}
export interface LoreRailGroups { decisions: LoreRecord[]; items: LoreRecord[]; notes: LoreRecord[]; sessions: LoreRecord[] }
export function loreRailModel(index: GraphIndex): LoreRailGroups
```

Deleted (Tasks 4/6 must not reference them): `TOP_N`, `chipId`, `ViewState`, `visibleSymbols`, `viewModel`, `VisNode.parent`, `VisNode.rank`.

- [ ] **Step 1: Rewrite the test file**

Replace `web/src/graph/aggregate.test.ts` entirely with:

```ts
import { describe, expect, test } from 'vitest'
import {
  buildIndex, pkgId, UNGROUPED, LABEL_TOP,
  overviewVM, focusVM, earnedLabels, loreRailModel,
} from './aggregate'
import type { Graph, GraphEdge, GraphNode } from '../types'

export function sym(id: string, group?: string, label?: string): GraphNode {
  return { id, kind: 'symbol', label: label ?? id, group }
}
export function lore(id: string, kind: 'decision' | 'item' | 'note', label?: string): GraphNode {
  return { id, kind, label: label ?? id }
}
export function call(source: string, target: string): GraphEdge {
  return { source, target, kind: 'calls' }
}
export function g(nodes: GraphNode[], edges: GraphEdge[]): Graph {
  return { focus: '', nodes, edges }
}

describe('buildIndex', () => {
  test('empty graph', () => {
    const ix = buildIndex(g([], []))
    expect(ix.packages.size).toBe(0)
    expect(ix.dropped).toBe(0)
  })

  test('drops edges with unknown endpoints, counts them', () => {
    const ix = buildIndex(g([sym('a', 'p')], [call('a', 'ghost'), call('ghost', 'a')]))
    expect(ix.edges).toHaveLength(0)
    expect(ix.dropped).toBe(2)
  })

  test('groups symbols by package; missing group falls into (ungrouped)', () => {
    const ix = buildIndex(g([sym('a', 'p'), sym('b', 'p'), sym('c')], []))
    expect(ix.packages.get('p')).toEqual(['a', 'b'])
    expect(ix.packages.get(UNGROUPED)).toEqual(['c'])
  })

  test('ranks symbols in a package by degree desc, id asc tiebreak', () => {
    const nodes = [sym('lo', 'p'), sym('hub', 'p'), sym('mid', 'p'), sym('x', 'q')]
    const edges = [call('hub', 'x'), call('hub', 'mid'), call('mid', 'x')]
    const ix = buildIndex(g(nodes, edges))
    expect(ix.packages.get('p')).toEqual(['hub', 'mid', 'lo'])
  })
})

describe('overviewVM', () => {
  test('packages only — no symbols, no lore nodes', () => {
    const ix = buildIndex(
      g([sym('a', 'p'), sym('b', 'q'), lore('dec-1', 'decision')], [call('a', 'b')]),
    )
    const vm = overviewVM(ix)
    expect(vm.nodes.map((n) => n.id).sort()).toEqual(['pkg:p', 'pkg:q'])
    const p = vm.nodes.find((n) => n.id === 'pkg:p')!
    expect(p).toMatchObject({ kind: 'package', role: 'map', symCount: 1 })
  })

  test('bundles cross-package calls with counts; drops intra and lore edges', () => {
    const ix = buildIndex(
      g(
        [sym('a', 'p'), sym('b', 'p'), sym('c', 'q'), lore('dec-1', 'decision')],
        [call('a', 'c'), call('b', 'c'), call('a', 'b'), { source: 'dec-1', target: 'a', kind: 'anchors' }],
      ),
    )
    const vm = overviewVM(ix)
    expect(vm.edges).toHaveLength(1)
    expect(vm.edges[0]).toMatchObject({ source: 'pkg:p', target: 'pkg:q', count: 2, bundled: true, kind: 'calls' })
  })
})

describe('focusVM', () => {
  const fixture = () =>
    buildIndex(
      g(
        [
          sym('a', 'p'), sym('b', 'p'), sym('c', 'p'),
          sym('x', 'q'), sym('y', 'r'), sym('z', 'zed'),
          lore('dec-1', 'decision'),
        ],
        [
          call('a', 'b'), call('a', 'b'), call('b', 'c'),      // intra p (a->b twice)
          call('a', 'x'), call('x', 'b'), call('c', 'y'),      // cross to q, r
          { source: 'dec-1', target: 'a', kind: 'anchors' },   // lore edge: excluded
        ],
      ),
    )

  test('all focus symbols + satellites for connected packages only', () => {
    const vm = focusVM(fixture(), 'p')
    const symbols = vm.nodes.filter((n) => n.kind === 'symbol')
    expect(symbols.map((n) => n.id).sort()).toEqual(['a', 'b', 'c'])
    const sats = vm.nodes.filter((n) => n.role === 'satellite')
    expect(sats.map((n) => n.id).sort()).toEqual(['pkg:q', 'pkg:r'])  // zed has no edge to p
    expect(vm.nodes.some((n) => n.id === 'pkg:zed')).toBe(false)
    expect(vm.nodes.some((n) => n.id === 'dec-1')).toBe(false)
  })

  test('intra edges concrete and merged by pair; satellite edges bundled per (satellite, symbol)', () => {
    const vm = focusVM(fixture(), 'p')
    const intra = vm.edges.filter((e) => !e.bundled)
    expect(intra).toHaveLength(2) // a->b (count 2), b->c
    expect(intra.find((e) => e.source === 'a' && e.target === 'b')!.count).toBe(2)
    const satEdges = vm.edges.filter((e) => e.bundled)
    const keys = satEdges.map((e) => `${e.source}->${e.target}`).sort()
    expect(keys).toEqual(['a->pkg:q', 'c->pkg:r', 'pkg:q->b'])  // direction preserved
  })

  test('unknown package yields empty view model', () => {
    const vm = focusVM(fixture(), 'nope')
    expect(vm.nodes).toHaveLength(0)
    expect(vm.edges).toHaveLength(0)
  })
})

describe('earnedLabels', () => {
  const bigVM = () => {
    const syms = Array.from({ length: 20 }, (_, i) => sym(`s${String(i).padStart(2, '0')}`, 'p'))
    const hub = sym('hub', 'q')
    const edges = syms.flatMap((s, i) => Array.from({ length: 20 - i }, () => call(s.id, 'hub')))
    return focusVM(buildIndex(g([...syms, hub], edges)), 'p')
  }

  test('top LABEL_TOP symbols by degree + all package nodes', () => {
    const set = earnedLabels(bigVM(), null, false)
    const vm = bigVM()
    const labeledSyms = vm.nodes.filter((n) => n.kind === 'symbol' && set.has(n.id))
    expect(labeledSyms).toHaveLength(LABEL_TOP)
    expect(set.has('s00')).toBe(true)   // highest degree
    expect(set.has('s19')).toBe(false)  // lowest degree
    expect(set.has('pkg:q')).toBe(true) // satellites always labeled
  })

  test('selected is always labeled; zoomNear labels everything', () => {
    const vm = bigVM()
    expect(earnedLabels(vm, 's19', false).has('s19')).toBe(true)
    const near = earnedLabels(vm, null, true)
    expect(near.size).toBe(vm.nodes.length)
  })
})

describe('loreRailModel', () => {
  test('groups by kind, session notes split out, recency = id desc', () => {
    const ix = buildIndex(
      g(
        [
          sym('a', 'p'), sym('b', 'q'),
          lore('dec-02', 'decision', 'Newer decision'), lore('dec-01', 'decision', 'Older decision'),
          lore('itm-01', 'item', 'An item'),
          lore('note-01', 'note', 'Session 2026-07-31 — stuff'), lore('note-02', 'note', 'A real note'),
        ],
        [
          { source: 'dec-01', target: 'a', kind: 'anchors' },
          { source: 'dec-01', target: 'b', kind: 'anchors' },
          { source: 'itm-01', target: 'itm-99', kind: 'blocked_by' }, // unknown target: dropped by buildIndex
          { source: 'itm-01', target: 'dec-01', kind: 'blocked_by' },
        ],
      ),
    )
    const rail = loreRailModel(ix)
    expect(rail.decisions.map((r) => r.id)).toEqual(['dec-02', 'dec-01'])  // id desc
    expect(rail.decisions[1].pkgs).toEqual(['p', 'q'])
    expect(rail.items[0].blockedBy).toEqual(['dec-01'])
    expect(rail.notes.map((r) => r.id)).toEqual(['note-02'])
    expect(rail.sessions.map((r) => r.id)).toEqual(['note-01'])
    expect(rail.sessions[0].session).toBe(true)
  })

  test('empty graph yields empty groups', () => {
    const rail = loreRailModel(buildIndex(g([], [])))
    expect(rail.decisions).toEqual([])
    expect(rail.sessions).toEqual([])
  })
})
```

- [ ] **Step 2: Run tests to verify the new ones fail**

Run: `cd web && npx vitest run src/graph/aggregate.test.ts`
Expected: FAIL — `overviewVM`/`focusVM`/`earnedLabels`/`loreRailModel` not exported.

- [ ] **Step 3: Rewrite `web/src/graph/aggregate.ts`**

Keep `buildIndex` and its helpers EXACTLY as they are in the current file (the `GraphIndex` interface, `UNGROUPED`, `pkgId`, degree/packages/pkgOf/dropped logic). Delete `TOP_N`, `chipId`, `ViewState`, `visibleSymbols`, `viewModel`, and the old `VisNode`/`VisEdge`. Then append:

```ts
export const LABEL_TOP = 8

export interface VisNode {
  id: string
  kind: 'package' | 'symbol'
  label: string
  degree: number
  symCount?: number
  pkg?: string
  role?: 'map' | 'satellite'
}

export interface VisEdge {
  id: string
  source: string
  target: string
  kind: string
  count: number
  bundled: boolean
  conf?: string
}

export interface ViewModel {
  nodes: VisNode[]
  edges: VisEdge[]
}

// Accumulate edges into a keyed map, merging duplicates into counts.
function accumulate(
  acc: Map<string, VisEdge>,
  source: string,
  target: string,
  kind: string,
  bundled: boolean,
  conf?: string,
) {
  const key = `${bundled ? 'b' : 'e'}:${source}|${target}|${kind}`
  const cur = acc.get(key)
  if (cur) cur.count++
  else acc.set(key, { id: key, source, target, kind, count: 1, bundled, conf: bundled ? undefined : conf })
}

// The landing map: one node per package, bundled package↔package call edges.
// Lore and symbols never appear here.
export function overviewVM(index: GraphIndex): ViewModel {
  const nodes: VisNode[] = []
  for (const [pkg, ids] of index.packages) {
    nodes.push({ id: pkgId(pkg), kind: 'package', label: pkg, degree: 0, symCount: ids.length, role: 'map' })
  }
  const acc = new Map<string, VisEdge>()
  for (const e of index.edges) {
    const sn = index.nodeById.get(e.source)
    const tn = index.nodeById.get(e.target)
    if (!sn || !tn || sn.kind !== 'symbol' || tn.kind !== 'symbol') continue
    const sp = index.pkgOf.get(e.source) as string
    const tp = index.pkgOf.get(e.target) as string
    if (sp === tp) continue
    accumulate(acc, pkgId(sp), pkgId(tp), e.kind, true)
  }
  return { nodes, edges: [...acc.values()] }
}

// One package gets the canvas: all its symbols with concrete intra edges,
// plus connected packages as rim satellites with bundled edges to the exact
// symbols they touch. Lore never appears on canvas.
export function focusVM(index: GraphIndex, pkg: string): ViewModel {
  const ids = index.packages.get(pkg)
  if (!ids) return { nodes: [], edges: [] }
  const inPkg = new Set(ids)
  const nodes: VisNode[] = []
  for (const id of ids) {
    const n = index.nodeById.get(id) as GraphNode
    nodes.push({ id, kind: 'symbol', label: n.label, pkg, degree: index.degree.get(id) ?? 0 })
  }
  const acc = new Map<string, VisEdge>()
  const satellites = new Set<string>()
  for (const e of index.edges) {
    const sn = index.nodeById.get(e.source)
    const tn = index.nodeById.get(e.target)
    if (!sn || !tn || sn.kind !== 'symbol' || tn.kind !== 'symbol') continue
    const sIn = inPkg.has(e.source)
    const tIn = inPkg.has(e.target)
    if (!sIn && !tIn) continue
    if (sIn && tIn) {
      accumulate(acc, e.source, e.target, e.kind, false, e.conf)
    } else if (sIn) {
      const other = index.pkgOf.get(e.target) as string
      satellites.add(other)
      accumulate(acc, e.source, pkgId(other), e.kind, true)
    } else {
      const other = index.pkgOf.get(e.source) as string
      satellites.add(other)
      accumulate(acc, pkgId(other), e.target, e.kind, true)
    }
  }
  for (const s of [...satellites].sort()) {
    nodes.push({
      id: pkgId(s),
      kind: 'package',
      label: s,
      degree: 0,
      symCount: (index.packages.get(s) ?? []).length,
      role: 'satellite',
    })
  }
  return { nodes, edges: [...acc.values()] }
}

// Which node ids currently render labels. Hover (.hl) and near-zoom
// all-labels are layered on top by the canvas; this is the base set.
export function earnedLabels(vm: ViewModel, selected: string | null, zoomNear: boolean): Set<string> {
  if (zoomNear) return new Set(vm.nodes.map((n) => n.id))
  const set = new Set<string>()
  for (const n of vm.nodes) if (n.kind === 'package') set.add(n.id)
  const top = vm.nodes
    .filter((n) => n.kind === 'symbol')
    .sort((a, b) => b.degree - a.degree || (a.id < b.id ? -1 : 1))
    .slice(0, LABEL_TOP)
  for (const n of top) set.add(n.id)
  if (selected) set.add(selected)
  return set
}

export type LoreKind = 'decision' | 'item' | 'note'

export interface LoreRecord {
  id: string
  kind: LoreKind
  label: string
  status?: string
  pkgs: string[]
  blockedBy: string[]
  session: boolean
}

export interface LoreRailGroups {
  decisions: LoreRecord[]
  items: LoreRecord[]
  notes: LoreRecord[]
  sessions: LoreRecord[]
}

const SESSION_RE = /^Session /

// Lore leaves the canvas: group records by kind for the rail, id-desc
// (ULIDs sort by creation time), with anchored packages for hover-sync.
export function loreRailModel(index: GraphIndex): LoreRailGroups {
  const byId = new Map<string, LoreRecord>()
  for (const n of index.nodes) {
    if (n.kind !== 'decision' && n.kind !== 'item' && n.kind !== 'note') continue
    byId.set(n.id, {
      id: n.id,
      kind: n.kind,
      label: n.label,
      status: n.status,
      pkgs: [],
      blockedBy: [],
      session: n.kind === 'note' && SESSION_RE.test(n.label),
    })
  }
  for (const e of index.edges) {
    const rec = byId.get(e.source)
    if (!rec) continue
    if (e.kind === 'blocked_by' && byId.has(e.target)) {
      rec.blockedBy.push(e.target)
    } else {
      const pkg = index.pkgOf.get(e.target)
      if (pkg && !rec.pkgs.includes(pkg)) rec.pkgs.push(pkg)
    }
  }
  for (const rec of byId.values()) rec.pkgs.sort()
  const all = [...byId.values()].sort((a, b) => (a.id > b.id ? -1 : 1))
  return {
    decisions: all.filter((r) => r.kind === 'decision'),
    items: all.filter((r) => r.kind === 'item'),
    notes: all.filter((r) => r.kind === 'note' && !r.session),
    sessions: all.filter((r) => r.session),
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/graph/aggregate.test.ts`
Expected: PASS (all describe blocks).

- [ ] **Step 5: Commit**

```bash
git add web/src/graph/aggregate.ts web/src/graph/aggregate.test.ts
git commit -m "feat(web): two-state view models — overviewVM, focusVM, earnedLabels, loreRailModel"
```

Note: `npm run build` is expected RED after this task (GraphCanvas/App still import deleted symbols) — it comes back green in Task 6. Do not attempt to fix those files here.

---

### Task 2: style.ts rework — earned labels, satellites, hover-sync highlight

**Files:**
- Rewrite: `web/src/graph/style.ts`

**Interfaces:**
- Consumes: data fields from Task 1 (`kind`, `role`, `symCount`, `degree`, `count`, `bundled`).
- Produces: classes used by Task 4 — `labeled`, `dim`, `hl`, `sel`, `selhl`, `lod-hide`, `lore-hot`, `entering`. Symbol labels render ONLY when `.labeled`/`.hl`/`.sel`/`.selhl` (via `text-opacity`); package labels always render.

- [ ] **Step 1: Rewrite `web/src/graph/style.ts`**

Keep the exported color/shape maps (`NODE_COLORS`, `NODE_SHAPES`, `EDGE_COLORS`, `EDGE_STYLES`) exactly as they are — `App.tsx`'s legend and the Inspector import them. Replace the `stylesheet()` function body with:

```ts
export function stylesheet(): StylesheetCSS[] {
  const edgeKinds = Object.keys(EDGE_COLORS) as EdgeKind[]

  const base: Array<{ selector: string; style: Record<string, unknown> }> = [
    // Symbols: quiet dots; labels exist but are invisible until earned.
    {
      selector: 'node',
      style: {
        label: 'data(label)',
        color: '#c5ccd8',
        'font-size': 10,
        'text-opacity': 0,
        'text-valign': 'bottom',
        'text-halign': 'center',
        'text-margin-y': 3,
        'text-wrap': 'ellipsis',
        'text-max-width': '150px',
        width: 'mapData(degree, 0, 40, 8, 40)',
        height: 'mapData(degree, 0, 40, 8, 40)',
        'border-width': 0,
        'background-color': NODE_COLORS.symbol,
        shape: 'ellipse',
        'transition-property': 'text-opacity',
        'transition-duration': '0.12s',
      },
    },
    // Earned/hovered/selected labels fade in.
    { selector: 'node.labeled', style: { 'text-opacity': 1 } },
    { selector: 'node.hl', style: { 'text-opacity': 1, opacity: 1, 'z-index': 30 } },
    // Overview package nodes: always-labeled map tiles sized by symbol count.
    {
      selector: 'node[kind = "package"]',
      style: {
        shape: 'round-rectangle',
        'background-color': '#22304a',
        'border-width': 1.5,
        'border-color': '#3a4a66',
        color: '#aab6c8',
        'font-size': 12,
        'text-opacity': 1,
        'text-valign': 'center',
        'text-halign': 'center',
        width: 'mapData(symCount, 1, 120, 34, 116)',
        height: 'mapData(symCount, 1, 120, 24, 48)',
      },
    },
    // Focus-view satellites: smaller, muted chips on the rim.
    {
      selector: 'node[role = "satellite"]',
      style: {
        'background-color': '#182238',
        'font-size': 11,
        width: 'mapData(symCount, 1, 120, 28, 84)',
        height: 22,
      },
    },
    // Generic edge first; feature rules AFTER it (order-precedence, see lore).
    {
      selector: 'edge',
      style: {
        width: 1,
        'curve-style': 'straight',
        'line-color': '#2f3745',
        'target-arrow-shape': 'none',
        opacity: 0.7,
      },
    },
    {
      selector: 'edge[?bundled]',
      style: {
        width: 'mapData(count, 1, 60, 1, 7)',
        'curve-style': 'straight',
        'line-color': '#3a4356',
        opacity: 0.55,
      },
    },
    // Interaction states.
    { selector: '.dim', style: { opacity: 0.15 } },
    { selector: 'edge.hl', style: { opacity: 1, width: 2, 'line-color': '#9fb2d6', 'z-index': 30 } },
    {
      selector: 'node.sel',
      style: { 'border-width': 3, 'border-color': '#ffffff', 'font-size': 13, 'text-opacity': 1, 'z-index': 40 },
    },
    { selector: 'node.selhl', style: { 'text-opacity': 1 } },
    { selector: 'edge.selhl', style: { width: 2, opacity: 1, 'line-color': '#9fb2d6', 'z-index': 25 } },
    // Lore-rail hover-sync target.
    { selector: 'node.lore-hot', style: { 'border-width': 2.5, 'border-color': '#f2b134' } },
    // Transition entry state (Task 4 morphs from this).
    { selector: 'node.entering', style: { opacity: 0.2 } },
    // LOD: elements hidden at far zoom.
    { selector: '.lod-hide', style: { display: 'none' } },
  ]

  for (const k of edgeKinds) {
    if (k === 'calls') continue // calls keep the neutral defaults for density
    base.push({
      selector: `edge[kind = "${k}"]`,
      style: { 'line-color': EDGE_COLORS[k], 'line-style': EDGE_STYLES[k], opacity: 0.9, width: 1.5 },
    })
  }
  return base as unknown as StylesheetCSS[]
}
```

Deleted relative to the current file: the per-node-kind loop (lore kinds never render on canvas now; symbol styling folded into the base rule), the compound `:parent` rule, the chip rule, the `node.group` legacy comment block if still present.

- [ ] **Step 2: Verify style.ts itself typechecks**

Run: `cd web && npx tsc --noEmit 2>&1 | grep -v "GraphCanvas.tsx\|App.tsx" | head`
Expected: no errors outside GraphCanvas.tsx/App.tsx (both still reference Task 1's deleted exports until Tasks 4/6).

- [ ] **Step 3: Commit**

```bash
git add web/src/graph/style.ts
git commit -m "feat(web): earned-label styles, satellite chips, lore-hot highlight"
```

---

### Task 3: motion.ts — anchored oscillation

**Files:**
- Create: `web/src/graph/motion.ts`
- Test: `web/src/graph/motion.test.ts`

**Interfaces:**
- Consumes: cytoscape `Core` type only (`import type`).
- Produces (used by Tasks 4, 6, 7):

```ts
export function motionEnabled(search: string): boolean   // false on motion=0 param or prefers-reduced-motion
export function oscOffset(id: string, tMs: number): { x: number; y: number }  // deterministic per (id, t); |x|,|y| ≤ 2.5
export function startMotion(cy: Core, isBusy: () => boolean): () => void       // returns stop()
```

- [ ] **Step 1: Write the failing tests**

Create `web/src/graph/motion.test.ts`:

```ts
import { describe, expect, test } from 'vitest'
import { motionEnabled, oscOffset } from './motion'

describe('oscOffset', () => {
  test('deterministic per (id, t)', () => {
    expect(oscOffset('sym#42', 1234)).toEqual(oscOffset('sym#42', 1234))
  })

  test('bounded by max amplitude', () => {
    for (const id of ['a', 'pkg:internal/graph', 'sym#1', 'sym#999']) {
      for (const t of [0, 500, 5000, 123456]) {
        const { x, y } = oscOffset(id, t)
        expect(Math.abs(x)).toBeLessThanOrEqual(2.5)
        expect(Math.abs(y)).toBeLessThanOrEqual(2.5)
      }
    }
  })

  test('varies over time and across ids', () => {
    const a0 = oscOffset('a', 0)
    const a1 = oscOffset('a', 1700)
    expect(a0.x !== a1.x || a0.y !== a1.y).toBe(true)
    const b0 = oscOffset('b', 0)
    expect(a0.x !== b0.x || a0.y !== b0.y).toBe(true)
  })
})

describe('motionEnabled', () => {
  test('motion=0 disables; default enables (jsdom has no reduced-motion)', () => {
    expect(motionEnabled('?motion=0')).toBe(false)
    expect(motionEnabled('?pkg=x&motion=0')).toBe(false)
    expect(motionEnabled('')).toBe(true)
    expect(motionEnabled('?pkg=x')).toBe(true)
  })
})
```

Note: vitest's default environment is `node`, where `window` is undefined — `motionEnabled` must guard its `matchMedia` access, which the test exercises implicitly.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/graph/motion.test.ts`
Expected: FAIL — cannot find module './motion'.

- [ ] **Step 3: Implement `web/src/graph/motion.ts`**

```ts
// Idle motion: a small, hash-seeded Lissajous drift rendered on top of each
// node's deterministic anchor (data ax/ay). Anchors are truth — this module
// never writes them, so layouts, diffs, and determinism tests are unaffected.
import type { Core } from 'cytoscape'

const MAX_AMP = 2.5
const MIN_AMP = 1.5
const FRAME_MS = 1000 / 30

function hash32(s: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return h >>> 0
}

export function motionEnabled(search: string): boolean {
  if (new URLSearchParams(search).get('motion') === '0') return false
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return false
  }
  return true
}

export function oscOffset(id: string, tMs: number): { x: number; y: number } {
  const h = hash32(id)
  const period = 5000 + (h % 4000)                 // 5–9s
  const phase = (((h >>> 8) % 1000) / 1000) * 2 * Math.PI
  const amp = MIN_AMP + (((h >>> 16) % 100) / 100) * (MAX_AMP - MIN_AMP)
  const w = (2 * Math.PI * tMs) / period
  return {
    x: amp * Math.sin(w + phase),
    y: amp * Math.cos(w / 1.13 + phase * 1.7),
  }
}

// Drive rendered positions at ≤30fps. Pauses while isBusy() (layouts,
// transitions), while a node is grabbed, and within 150ms of a user
// pan/zoom. Stops cleanly via the returned function.
export function startMotion(cy: Core, isBusy: () => boolean): () => void {
  let raf = 0
  let last = 0
  let gestureUntil = 0
  let stopped = false

  const onGesture = () => {
    gestureUntil = performance.now() + 150
  }
  cy.on('pan zoom', onGesture)

  // A user drag moves a node away from its anchor deliberately: adopt the
  // new position as the anchor so motion doesn't snap it back.
  const onDragFree = (evt: { target: { position: () => { x: number; y: number }; data: (k: string, v?: unknown) => unknown } }) => {
    const n = evt.target
    const p = n.position()
    n.data('ax', p.x)
    n.data('ay', p.y)
  }
  cy.on('dragfree', 'node', onDragFree)

  const tick = (now: number) => {
    if (stopped) return
    raf = requestAnimationFrame(tick)
    if (now - last < FRAME_MS) return
    last = now
    if (isBusy() || now < gestureUntil) return
    if (cy.nodes(':grabbed').nonempty()) return
    cy.batch(() => {
      cy.nodes('[ax]').forEach((n) => {
        const off = oscOffset(n.id(), now)
        n.position({ x: (n.data('ax') as number) + off.x, y: (n.data('ay') as number) + off.y })
      })
    })
  }
  raf = requestAnimationFrame(tick)

  return () => {
    stopped = true
    cancelAnimationFrame(raf)
    cy.off('pan zoom', onGesture)
    cy.off('dragfree', 'node', onDragFree as never)
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/graph/motion.test.ts`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/graph/motion.ts web/src/graph/motion.test.ts
git commit -m "feat(web): anchored idle motion — hash-seeded oscillation, 30fps, gesture-aware"
```

---

### Task 4: GraphCanvas rewrite — two-state rendering, anchors, transitions

**Files:**
- Rewrite: `web/src/graph/GraphCanvas.tsx`

**Interfaces:**
- Consumes: `ViewModel`, `VisNode`, `earnedLabels` inputs from Task 1; classes from Task 2; `motionEnabled`, `startMotion` from Task 3.
- Produces (used by Task 6):

```ts
export type View = { mode: 'overview' } | { mode: 'focus'; pkg: string }
interface Props {
  vm: ViewModel
  view: View
  selected: string | null
  labeled: Set<string>                 // earnedLabels output (base set, no hover)
  hot: Set<string>                     // canvas node ids to flag .lore-hot (rail hover-sync)
  onSelect: (id: string | null) => void
  onFocusPackage: (pkg: string) => void
  onHoverNode: (id: string | null) => void
}
export function GraphCanvas(props: Props): JSX.Element
```

Also exposes `window.__cy` and sets `window.__layoutDone = true` after each view's layout + fit completes (reset to `false` when a view transition starts). Every node carries `ax`/`ay` anchor data once placed.

- [ ] **Step 1: Rewrite `web/src/graph/GraphCanvas.tsx`**

```tsx
import { useEffect, useRef } from 'react'
import cytoscape from 'cytoscape'
import type { CollectionReturnValue, Core, NodeSingular } from 'cytoscape'
import fcose from 'cytoscape-fcose'
import type { ViewModel } from './aggregate'
import { motionEnabled, startMotion } from './motion'
import { stylesheet } from './style'

let registered = false
function ensureFcose() {
  if (!registered) {
    cytoscape.use(fcose)
    registered = true
  }
}

const LOD_ZOOM = 0.4
const LABEL_NEAR_ZOOM = 1.1
const GOLDEN_ANGLE = 2.399963229728653
const MORPH_MS = 350
const FOCUS_MORPH_MAX = 150

export type View = { mode: 'overview' } | { mode: 'focus'; pkg: string }

function viewKey(v: View): string {
  return v.mode === 'focus' ? `focus:${v.pkg}` : 'overview'
}

// NOTE: no `declare global` here — the e2e spec declares Window.__cy/__layoutDone
// with different types; a second declaration would conflict if tests ever join
// the tsc program. Use the local cast helper instead.
function win(): { __cy?: Core; __layoutDone?: boolean } {
  return window as unknown as { __cy?: Core; __layoutDone?: boolean }
}

// fcose has internal Math.random calls; pin them to a seeded LCG for the
// duration of the layout so the same input always yields the same map.
function seededLayout(cy: Core, opts: Record<string, unknown>, onDone: () => void): () => void {
  const orig = Math.random
  let s = 42
  Math.random = () => {
    s = (s * 1664525 + 1013904223) >>> 0
    return s / 4294967296
  }
  const restore = () => {
    Math.random = orig
  }
  try {
    const layout = cy.layout({ name: 'fcose', ...opts } as never)
    layout.one('layoutstop', () => {
      restore()
      onDone()
    })
    layout.run()
  } catch (err) {
    restore()
    throw err
  }
  return restore
}

// Anchors are truth: record the current position of every node as its
// deterministic anchor. Motion renders offsets on top; determinism tests
// read ax/ay.
function writeAnchors(cy: Core) {
  cy.batch(() => {
    cy.nodes().forEach((n) => {
      const p = n.position()
      n.data('ax', p.x)
      n.data('ay', p.y)
    })
  })
}

// Diff the desired view model into the live instance. Nodes are added
// without positions (the caller places them); nodes whose id persists
// across views (satellites ⇄ overview packages) keep position but get
// their data refreshed (role/degree change between views).
function applyViewModel(cy: Core, vm: ViewModel): CollectionReturnValue {
  const wantNodes = new Map(vm.nodes.map((n) => [n.id, n]))
  const wantEdges = new Map(vm.edges.map((e) => [e.id, e]))
  let added = cy.collection()
  cy.batch(() => {
    cy.edges().forEach((e) => {
      if (!wantEdges.has(e.id())) e.remove()
    })
    cy.nodes().forEach((n) => {
      const want = wantNodes.get(n.id())
      if (!want) {
        n.remove()
      } else {
        n.data({ ...want, ax: n.data('ax'), ay: n.data('ay') })
      }
    })
    for (const n of vm.nodes) {
      if (cy.$id(n.id).nonempty()) continue
      added = added.union(cy.add({ data: { ...n } }))
    }
    for (const e of vm.edges) {
      if (cy.$id(e.id).empty()) cy.add({ data: { ...e } })
    }
  })
  return added
}

// Deterministic seeds. Overview: packages on a circle in sorted-label order.
function seedOverview(vm: ViewModel): Map<string, { x: number; y: number }> {
  const pos = new Map<string, { x: number; y: number }>()
  const pkgs = vm.nodes.filter((n) => n.kind === 'package').sort((a, b) => (a.label < b.label ? -1 : 1))
  const r = Math.max(220, (pkgs.length * 110) / (2 * Math.PI))
  pkgs.forEach((n, i) => {
    const a = (i / Math.max(1, pkgs.length)) * 2 * Math.PI
    pos.set(n.id, { x: r * Math.cos(a), y: r * Math.sin(a) })
  })
  return pos
}

// Focus: symbols phyllotaxis around `center` in degree-desc/id-asc order,
// satellites on an outer rim in sorted-label order.
function seedFocus(vm: ViewModel, center: { x: number; y: number }) {
  const symbolPos = new Map<string, { x: number; y: number }>()
  const rimPos = new Map<string, { x: number; y: number }>()
  const symbols = vm.nodes
    .filter((n) => n.kind === 'symbol')
    .sort((a, b) => b.degree - a.degree || (a.id < b.id ? -1 : 1))
  symbols.forEach((n, i) => {
    const a = i * GOLDEN_ANGLE
    const r = 26 * Math.sqrt(i + 0.5)
    symbolPos.set(n.id, { x: center.x + r * Math.cos(a), y: center.y + r * Math.sin(a) })
  })
  const rim = Math.max(300, 26 * Math.sqrt(symbols.length) * 2.6)
  const sats = vm.nodes.filter((n) => n.role === 'satellite').sort((a, b) => (a.label < b.label ? -1 : 1))
  sats.forEach((n, i) => {
    const a = (i / Math.max(1, sats.length)) * 2 * Math.PI - Math.PI / 2
    rimPos.set(n.id, { x: center.x + rim * Math.cos(a), y: center.y + rim * Math.sin(a) })
  })
  return { symbolPos, rimPos, rim }
}

function applyLod(cy: Core) {
  const far = cy.zoom() < LOD_ZOOM
  cy.batch(() => {
    const weak = cy.edges('[count = 1]')
    if (far) weak.addClass('lod-hide')
    else weak.removeClass('lod-hide')
  })
}

// Toggle a class so exactly `want` has it, touching only the difference.
function diffClass(cy: Core, cls: string, want: Set<string>, prevRef: { current: Set<string> }) {
  const prev = prevRef.current
  cy.batch(() => {
    for (const id of want) if (!prev.has(id)) cy.$id(id).addClass(cls)
    for (const id of prev) if (!want.has(id)) cy.$id(id).removeClass(cls)
  })
  prevRef.current = new Set(want)
}

interface Props {
  vm: ViewModel
  view: View
  selected: string | null
  labeled: Set<string>
  hot: Set<string>
  onSelect: (id: string | null) => void
  onFocusPackage: (pkg: string) => void
  onHoverNode: (id: string | null) => void
}

export function GraphCanvas({ vm, view, selected, labeled, hot, onSelect, onFocusPackage, onHoverNode }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<Core | null>(null)
  const prevKey = useRef<string | null>(null)
  const lodFar = useRef(false)
  const nearBand = useRef(false)
  const hood = useRef<CollectionReturnValue | null>(null)
  const clearTimer = useRef<number | undefined>(undefined)
  const lastCentered = useRef<string | null>(null)
  const layoutRestore = useRef<(() => void) | null>(null)
  const busy = useRef(false)
  const overviewAnchors = useRef<Map<string, { x: number; y: number }> | null>(null)
  const labeledPrev = useRef(new Set<string>())
  const hotPrev = useRef(new Set<string>())
  const cb = useRef({ onSelect, onFocusPackage, onHoverNode })
  cb.current = { onSelect, onFocusPackage, onHoverNode }
  const instant = !motionEnabled(window.location.search)

  useEffect(() => {
    ensureFcose()
    if (!containerRef.current) return
    const cy = cytoscape({
      container: containerRef.current,
      style: stylesheet(),
      elements: [],
      minZoom: 0.05,
      maxZoom: 4,
      wheelSensitivity: 0.25,
      pixelRatio: 1,
      textureOnViewport: true,
      hideEdgesOnViewport: true,
      motionBlur: false,
    })

    cy.on('tap', 'node', (evt) => {
      const n = evt.target
      if ((n.data('kind') as string) === 'package') cb.current.onFocusPackage(n.data('label') as string)
      else cb.current.onSelect(n.id())
    })
    cy.on('tap', (evt) => {
      if (evt.target === cy) cb.current.onSelect(null)
    })

    // Hover: symmetric-difference class toggling with a debounced clear.
    const clearHover = () => {
      if (!hood.current) return
      hood.current = null
      cy.batch(() => cy.elements().removeClass('dim hl'))
      cb.current.onHoverNode(null)
    }
    cy.on('mouseover', 'node', (evt) => {
      const n = evt.target as NodeSingular
      window.clearTimeout(clearTimer.current)
      const next = n.closedNeighborhood()
      const prev = hood.current
      cy.batch(() => {
        if (!prev) {
          cy.elements().difference(next).addClass('dim')
          cy.nodes('[kind = "package"]').removeClass('dim')
          next.addClass('hl')
        } else {
          const on = next.difference(prev)
          const off = prev.difference(next)
          on.removeClass('dim').addClass('hl')
          off.removeClass('hl').addClass('dim')
          off.nodes('[kind = "package"]').removeClass('dim')
        }
      })
      hood.current = next
      cb.current.onHoverNode(n.id())
    })
    cy.on('mouseout', 'node', () => {
      window.clearTimeout(clearTimer.current)
      clearTimer.current = window.setTimeout(clearHover, 60)
    })

    cy.on('zoom', () => {
      const far = cy.zoom() < LOD_ZOOM
      if (far !== lodFar.current) {
        lodFar.current = far
        applyLod(cy)
      }
      const near = cy.zoom() >= LABEL_NEAR_ZOOM
      if (near !== nearBand.current) {
        nearBand.current = near
        cy.batch(() => {
          if (near) cy.nodes().addClass('labeled')
          else {
            cy.nodes().removeClass('labeled')
            for (const id of labeledPrev.current) cy.$id(id).addClass('labeled')
          }
        })
      }
    })

    win().__cy = cy
    cyRef.current = cy
    const stopMotion = motionEnabled(window.location.search)
      ? startMotion(cy, () => busy.current)
      : () => {}
    return () => {
      window.clearTimeout(clearTimer.current)
      stopMotion()
      layoutRestore.current?.()
      layoutRestore.current = null
      cy.destroy()
      cyRef.current = null
    }
  }, [])

  // Render the view: full layout on view change, incremental diff otherwise.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy || vm.nodes.length === 0) return
    const key = viewKey(view)
    if (key === prevKey.current) {
      applyViewModel(cy, vm)
      applyLod(cy)
      return
    }
    const fromKey = prevKey.current
    prevKey.current = key
    busy.current = true
    win().__layoutDone = false

    const finalize = () => {
      writeAnchors(cy)
      if (view.mode === 'overview') {
        overviewAnchors.current = new Map(
          cy.nodes().map((n) => [n.id(), { x: n.data('ax') as number, y: n.data('ay') as number }]),
        )
      }
      applyLod(cy)
      cy.elements().removeClass('entering')
      busy.current = false
      win().__layoutDone = true
    }
    const fit = () => {
      if (instant) {
        cy.fit(undefined, 50)
        finalize()
      } else {
        cy.animate({ fit: { eles: cy.elements(), padding: 50 } }, { duration: MORPH_MS, complete: finalize })
      }
    }

    if (view.mode === 'overview') {
      applyViewModel(cy, vm)
      const cached = overviewAnchors.current
      if (cached && vm.nodes.every((n) => cached.has(n.id))) {
        // Returning from focus: restore the exact cached map, no relayout.
        cy.batch(() => {
          for (const n of vm.nodes) cy.$id(n.id).position(cached.get(n.id) as { x: number; y: number })
        })
        fit()
      } else {
        const seeds = seedOverview(vm)
        cy.batch(() => {
          for (const [id, p] of seeds) cy.$id(id).position(p)
        })
        layoutRestore.current = seededLayout(
          cy,
          { quality: 'default', animate: false, randomize: false, fit: false, nodeSeparation: 140, idealEdgeLength: 150, nodeRepulsion: 7500, gravity: 0.15, packComponents: false },
          () => {
            layoutRestore.current = null
            fit()
          },
        )
      }
      return
    }

    // Entering focus. Morph origin: the tapped package's current position if
    // it is on canvas (coming from overview), else the canvas origin.
    const pkgNode = cy.$id(`pkg:${view.pkg}`)
    const origin = fromKey === 'overview' && pkgNode.nonempty() ? { ...pkgNode.position() } : { x: 0, y: 0 }
    applyViewModel(cy, vm)
    const { symbolPos, rimPos } = seedFocus(vm, origin)
    cy.batch(() => {
      for (const [id, p] of symbolPos) cy.$id(id).position(p)
      for (const [id, p] of rimPos) cy.$id(id).position(p)
    })
    const fixed = [...rimPos.entries()].map(([nodeId, position]) => ({ nodeId, position }))
    layoutRestore.current = seededLayout(
      cy,
      { quality: 'default', animate: false, randomize: false, fit: false, nodeSeparation: 60, idealEdgeLength: 70, nodeRepulsion: 4500, gravity: 0.25, packComponents: false, fixedNodeConstraint: fixed },
      () => {
        layoutRestore.current = null
        const symbols = cy.nodes('[kind = "symbol"]')
        if (instant || symbols.length > FOCUS_MORPH_MAX || fromKey === null) {
          fit()
          return
        }
        // Morph: snap symbols back to the origin, then animate to targets.
        const targets = new Map(symbols.map((n) => [n.id(), { ...n.position() }]))
        cy.batch(() => {
          symbols.forEach((n) => {
            n.position(origin)
            n.addClass('entering')
          })
        })
        symbols.forEach((n) => {
          n.animate(
            { position: targets.get(n.id()) as { x: number; y: number } },
            { duration: MORPH_MS, easing: 'ease-out' },
          )
          n.removeClass('entering')
        })
        fit()
      },
    )
  }, [vm, view, instant])

  // Earned labels: diff the base set; near-band overrides with all-labeled.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    if (nearBand.current) {
      labeledPrev.current = new Set(labeled)
      return
    }
    diffClass(cy, 'labeled', labeled, labeledPrev)
  }, [labeled, vm])

  // Lore-rail hover-sync highlight.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    diffClass(cy, 'lore-hot', hot, hotPrev)
  }, [hot, vm])

  // Selection: classes unconditionally; center only when the id changes.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    cy.elements().removeClass('sel selhl')
    if (!selected) {
      lastCentered.current = null
      return
    }
    const n = cy.$id(selected)
    if (n.empty()) return
    n.addClass('sel')
    n.closedNeighborhood().addClass('selhl')
    if (selected !== lastCentered.current) {
      lastCentered.current = selected
      cy.animate({ center: { eles: n }, zoom: Math.max(cy.zoom(), 0.8) }, { duration: instant ? 0 : 300 })
    }
  }, [selected, vm, instant])

  return <div ref={containerRef} className="graph-canvas" data-testid="graph-canvas" />
}
```

- [ ] **Step 2: Typecheck**

Run: `cd web && npx tsc --noEmit 2>&1 | grep -v "App.tsx" | head`
Expected: no errors outside `App.tsx` (it still passes the old props until Task 6). Errors in GraphCanvas.tsx are yours to fix.

- [ ] **Step 3: Commit**

```bash
git add web/src/graph/GraphCanvas.tsx
git commit -m "feat(web): two-state GraphCanvas — anchors, focus morph, earned-label diffing"
```

---

### Task 5: LoreRail component + styles

**Files:**
- Create: `web/src/LoreRail.tsx`
- Modify: `web/src/styles.css` (append rail styles)

**Interfaces:**
- Consumes: `LoreRailGroups`, `LoreRecord` from Task 1.
- Produces (used by Task 6):

```ts
export type RailGroupKey = 'decisions' | 'items' | 'notes' | 'sessions'
interface Props {
  rail: LoreRailGroups
  visible: Set<RailGroupKey>
  onToggleGroup: (k: RailGroupKey) => void
  hotIds: Set<string>                      // records to highlight (canvas → rail sync)
  onHover: (rec: LoreRecord | null) => void
  onOpen: (id: string) => void
  focusPkg: string | null                  // when set, filter to records anchored in this package
  showAll: boolean                         // override the focusPkg filter
  onToggleShowAll: () => void
}
export function LoreRail(props: Props): JSX.Element
```

Testids: `lore-rail`, `rail-item` (each row), `rail-chip-<key>` (filter chips), `rail-showall`.

- [ ] **Step 1: Create `web/src/LoreRail.tsx`**

```tsx
import type { LoreRailGroups, LoreRecord } from './graph/aggregate'
import { NODE_COLORS } from './graph/style'

export type RailGroupKey = 'decisions' | 'items' | 'notes' | 'sessions'

const GROUP_META: Array<{ key: RailGroupKey; title: string; color: string }> = [
  { key: 'decisions', title: 'Decisions', color: NODE_COLORS.decision },
  { key: 'items', title: 'Work items', color: NODE_COLORS.item },
  { key: 'notes', title: 'Notes', color: NODE_COLORS.note },
  { key: 'sessions', title: 'Sessions', color: NODE_COLORS.note },
]

interface Props {
  rail: LoreRailGroups
  visible: Set<RailGroupKey>
  onToggleGroup: (k: RailGroupKey) => void
  hotIds: Set<string>
  onHover: (rec: LoreRecord | null) => void
  onOpen: (id: string) => void
  focusPkg: string | null
  showAll: boolean
  onToggleShowAll: () => void
}

export function LoreRail({ rail, visible, onToggleGroup, hotIds, onHover, onOpen, focusPkg, showAll, onToggleShowAll }: Props) {
  const filterPkg = focusPkg && !showAll ? focusPkg : null
  const rows = (recs: LoreRecord[]) =>
    filterPkg ? recs.filter((r) => r.pkgs.includes(filterPkg)) : recs

  return (
    <div className="lore-rail" data-testid="lore-rail" onMouseLeave={() => onHover(null)}>
      <div className="rail-head">
        <span className="section-label">Lore</span>
        {focusPkg && (
          <button className="chip rail-showall" data-testid="rail-showall" onClick={onToggleShowAll}>
            {showAll ? 'this package' : 'show all'}
          </button>
        )}
      </div>
      {GROUP_META.map(({ key, title, color }) => {
        if (!visible.has(key)) return null
        const recs = rows(rail[key])
        if (recs.length === 0) return null
        return (
          <div className="rail-group" key={key}>
            <div className="rail-group-title">{title}</div>
            {recs.map((r) => (
              <button
                key={r.id}
                className={`rail-item${hotIds.has(r.id) ? ' hot' : ''}`}
                data-testid="rail-item"
                data-kind={key}
                title={r.label}
                onMouseEnter={() => onHover(r)}
                onClick={() => onOpen(r.id)}
              >
                <span className="rail-dot" style={{ background: color }} />
                <span className="rail-label">{r.label}</span>
                {r.blockedBy.length > 0 && <span className="rail-badge">⛓{r.blockedBy.length}</span>}
              </button>
            ))}
          </div>
        )
      })}
      <div className="rail-chips">
        {GROUP_META.map(({ key, title }) => (
          <button
            key={key}
            className={`chip rail-chip${visible.has(key) ? ' on' : ' off'}`}
            data-testid={`rail-chip-${key}`}
            onClick={() => onToggleGroup(key)}
          >
            {title.toLowerCase()} {rail[key].length}
          </button>
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Append rail styles to `web/src/styles.css`**

```css
/* Lore rail */
.right-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--panel);
  border-left: 1px solid var(--border);
}
.right-panel .inspector {
  border-left: none;
  flex: 0 1 auto;
  max-height: 45%;
}
.lore-rail {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 12px 12px 8px;
  border-top: 1px solid var(--border);
  display: flex;
  flex-direction: column;
}
.rail-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}
.rail-group-title {
  color: var(--muted);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin: 8px 0 4px;
}
.rail-item {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  text-align: left;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  padding: 4px 6px;
  color: var(--text);
  cursor: pointer;
  font: inherit;
}
.rail-item:hover {
  background: var(--panel-2);
}
.rail-item.hot {
  border-color: #f2b134;
  background: var(--panel-2);
}
.rail-dot {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  flex: none;
}
.rail-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rail-badge {
  color: #e5484d;
  font-size: 10px;
  white-space: nowrap;
}
.rail-chips {
  display: flex;
  gap: 5px;
  flex-wrap: wrap;
  margin-top: auto;
  padding-top: 10px;
}
.rail-chip.off {
  opacity: 0.4;
}

/* Focus breadcrumb overlay */
.focus-crumb {
  position: absolute;
  top: 10px;
  left: 12px;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(22, 27, 34, 0.85);
  border: 1px solid var(--border);
  border-radius: 7px;
  padding: 5px 10px;
}
.focus-crumb b {
  font-weight: 600;
}
.focus-crumb .crumb {
  color: var(--muted);
}
.focus-crumb .crumb:hover {
  color: var(--text);
}
```

- [ ] **Step 3: Typecheck (LoreRail compiles; only App.tsx errors remain)**

Run: `cd web && npx tsc --noEmit 2>&1 | grep -v "App.tsx" | head`
Expected: empty.

- [ ] **Step 4: Commit**

```bash
git add web/src/LoreRail.tsx web/src/styles.css
git commit -m "feat(web): lore rail component — kind groups, filters, hover-sync, badges"
```

---

### Task 6: App rewrite — view state, URL/history, hover-sync wiring

**Files:**
- Rewrite: `web/src/App.tsx`

**Interfaces:**
- Consumes: everything above — `buildIndex`, `overviewVM`, `focusVM`, `earnedLabels`, `loreRailModel`, `pkgId` (Task 1); `GraphCanvas`, `View` (Task 4); `LoreRail`, `RailGroupKey` (Task 5); existing `useFullGraph`, `resolveFocus`, `CommandPalette`, `Inspector`.
- Produces: the running app. URL contract: `/?pkg=<group>` = focus view; `&focus=<node id>` = selection; `motion=0` passthrough preserved on every URL we write.

- [ ] **Step 1: Rewrite `web/src/App.tsx`**

```tsx
import { useCallback, useEffect, useMemo, useState } from 'react'
import { getHealth } from './api'
import type { Health } from './types'
import type { Suggestion } from './CommandPalette'
import { useFullGraph, resolveFocus } from './useFullGraph'
import { buildIndex, overviewVM, focusVM, earnedLabels, loreRailModel, pkgId } from './graph/aggregate'
import type { LoreRecord } from './graph/aggregate'
import { GraphCanvas, type View } from './graph/GraphCanvas'
import { LoreRail, type RailGroupKey } from './LoreRail'
import { CommandPalette } from './CommandPalette'
import { Inspector } from './Inspector'
import { EDGE_COLORS, EDGE_STYLES, NODE_COLORS } from './graph/style'

function parseView(search: string, packages: Set<string>): View {
  const pkg = new URLSearchParams(search).get('pkg')
  if (pkg && packages.has(pkg)) return { mode: 'focus', pkg }
  if (pkg) console.warn(`graph: unknown package in URL: ${pkg}`)
  return { mode: 'overview' }
}

function writeUrl(view: View, selected: string | null) {
  const params = new URLSearchParams(window.location.search)
  params.delete('pkg')
  params.delete('focus')
  if (view.mode === 'focus') params.set('pkg', view.pkg)
  if (selected) params.set('focus', selected)
  const qs = params.toString()
  const url = qs ? `?${qs}` : window.location.pathname
  if (window.location.search !== (qs ? `?${qs}` : '')) {
    window.history.pushState(null, '', url)
  }
}

export default function App() {
  const { nodes, edges, loading, error } = useFullGraph()
  const [health, setHealth] = useState<Health | null>(null)
  const [view, setView] = useState<View>({ mode: 'overview' })
  const [selected, setSelected] = useState<string | null>(null)
  const [railVisible, setRailVisible] = useState<Set<RailGroupKey>>(
    new Set(['decisions', 'items', 'notes']),
  )
  const [railShowAll, setRailShowAll] = useState(false)
  const [hotCanvas, setHotCanvas] = useState<Set<string>>(new Set())
  const [hotRail, setHotRail] = useState<Set<string>>(new Set())

  const index = useMemo(() => buildIndex({ focus: '', nodes, edges }), [nodes, edges])
  const rail = useMemo(() => loreRailModel(index), [index])
  const vm = useMemo(
    () => (view.mode === 'focus' ? focusVM(index, view.pkg) : overviewVM(index)),
    [index, view],
  )
  // Near-zoom "label everything" is layered on by the canvas zoom handler;
  // the base earned set is computed with zoomNear=false here.
  const labeled = useMemo(() => earnedLabels(vm, selected, false), [vm, selected])

  useEffect(() => {
    getHealth().then(setHealth).catch(() => setHealth(null))
  }, [])

  useEffect(() => {
    if (index.dropped > 0) {
      console.warn(`graph: dropped ${index.dropped} edge(s) with unknown endpoints`)
    }
  }, [index])

  // Initial URL + back/forward.
  useEffect(() => {
    if (nodes.length === 0) return
    const packages = new Set(index.packages.keys())
    const apply = () => {
      const v = parseView(window.location.search, packages)
      setView(v)
      const focusParam = new URLSearchParams(window.location.search).get('focus')
      if (focusParam) {
        const id = resolveFocus(nodes, focusParam)
        if (id) {
          const pkg = index.pkgOf.get(id)
          if (pkg) setView({ mode: 'focus', pkg })
          setSelected(id)
          return
        }
      }
      setSelected(null)
    }
    apply()
    window.addEventListener('popstate', apply)
    return () => window.removeEventListener('popstate', apply)
  }, [nodes, index])

  const focusPackage = useCallback((pkg: string) => {
    setView((v) => (v.mode === 'focus' && v.pkg === pkg ? v : { mode: 'focus', pkg }))
    setSelected(null)
    setRailShowAll(false)
    writeUrl({ mode: 'focus', pkg }, null)
  }, [])

  const backToOverview = useCallback(() => {
    setView({ mode: 'overview' })
    setSelected(null)
    setRailShowAll(false)
    writeUrl({ mode: 'overview' }, null)
  }, [])
  // (declared before the Esc effect uses it — keep this ordering)

  // Esc leaves focus mode (unless typing in an input). Bound per-view so the
  // handler reads current state without side effects inside a setState updater.
  useEffect(() => {
    if (view.mode !== 'focus') return
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      if ((e.target as HTMLElement)?.tagName === 'INPUT') return
      backToOverview()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [view, backToOverview])


  // Selecting a symbol focuses its package; lore ids just select (Inspector
  // reads from the full node list — lore has no canvas node).
  const selectNode = useCallback(
    (id: string | null) => {
      let nextView: View | null = null
      if (id) {
        const pkg = index.pkgOf.get(id)
        if (pkg) {
          nextView = { mode: 'focus', pkg }
          setView((v) => (v.mode === 'focus' && v.pkg === pkg ? v : nextView as View))
        }
      }
      setSelected(id)
      writeUrl(nextView ?? view, id)
    },
    [index, view],
  )

  // Rail → canvas hover-sync: light up anchored packages (overview) or the
  // anchored symbols + their packages (focus).
  const railHover = useCallback(
    (rec: LoreRecord | null) => {
      if (!rec) {
        setHotCanvas(new Set())
        return
      }
      const ids = new Set<string>()
      for (const p of rec.pkgs) ids.add(pkgId(p))
      for (const e of index.edges) {
        if (e.source === rec.id && index.pkgOf.has(e.target)) ids.add(e.target)
      }
      setHotCanvas(ids)
    },
    [index],
  )

  // Canvas → rail hover-sync: records anchored in the hovered package/symbol.
  const canvasHover = useCallback(
    (id: string | null) => {
      if (!id) {
        setHotRail(new Set())
        return
      }
      const pkg = id.startsWith('pkg:') ? id.slice(4) : index.pkgOf.get(id)
      if (!pkg) {
        setHotRail(new Set())
        return
      }
      const ids = new Set<string>()
      for (const group of [rail.decisions, rail.items, rail.notes, rail.sessions]) {
        for (const r of group) if (r.pkgs.includes(pkg)) ids.add(r.id)
      }
      setHotRail(ids)
    },
    [index, rail],
  )

  const toggleGroup = useCallback((k: RailGroupKey) => {
    setRailVisible((prev) => {
      const next = new Set(prev)
      if (next.has(k)) next.delete(k)
      else next.add(k)
      return next
    })
  }, [])

  const selectedNode = useMemo(
    () => (selected ? nodes.find((n) => n.id === selected) ?? null : null),
    [selected, nodes],
  )

  const suggestions = useMemo<Suggestion[]>(
    () =>
      nodes
        .filter((n) => n.kind === 'symbol')
        .sort((a, b) => (index.degree.get(b.id) ?? 0) - (index.degree.get(a.id) ?? 0))
        .slice(0, 4)
        .map((n) => ({ id: n.id, label: n.label })),
    [nodes, index],
  )

  const symbolCount = useMemo(() => nodes.filter((n) => n.kind === 'symbol').length, [nodes])
  const loreCount = nodes.length - symbolCount

  function onSearch(query: string) {
    selectNode(resolveFocus(nodes, query))
  }

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          codeindex <span className="brand-sub">· lore graph</span>
        </div>
        <CommandPalette onSubmit={onSearch} suggestions={suggestions} />
        <div className="status" data-testid="health">
          {loading
            ? 'loading graph…'
            : `${index.packages.size} packages · ${symbolCount} symbols · ${loreCount} lore${health ? ` · ● ${health.version}` : ''}`}
        </div>
      </header>

      <main className="stage">
        <div className="canvas-wrap">
          {error && <div className="error-banner" data-testid="error">{error}</div>}
          {loading && <div className="empty-hint" data-testid="loading-hint">building the graph…</div>}
          {view.mode === 'focus' && (
            <div className="focus-crumb" data-testid="focus-crumb">
              <button className="crumb" data-testid="crumb-overview" onClick={backToOverview}>
                ⟵ overview
              </button>
              <span className="crumb-sep">/</span>
              <b data-testid="crumb-pkg">{view.pkg}</b>
              <span className="crumb-hint">esc</span>
            </div>
          )}
          {view.mode === 'focus' && vm.nodes.length === 0 && !loading && (
            <div className="empty-hint" data-testid="empty-focus">no symbols in this package</div>
          )}
          <GraphCanvas
            vm={vm}
            view={view}
            selected={selected}
            labeled={labeled}
            hot={hotCanvas}
            onSelect={selectNode}
            onFocusPackage={focusPackage}
            onHoverNode={canvasHover}
          />
          <Legend />
        </div>
        <div className="right-panel">
          <Inspector node={selectedNode} nodes={nodes} edges={edges} onOpen={selectNode} />
          <LoreRail
            rail={rail}
            visible={railVisible}
            onToggleGroup={toggleGroup}
            hotIds={hotRail}
            onHover={railHover}
            onOpen={selectNode}
            focusPkg={view.mode === 'focus' ? view.pkg : null}
            showAll={railShowAll}
            onToggleShowAll={() => setRailShowAll((s) => !s)}
          />
        </div>
      </main>
    </div>
  )
}

function Legend() {
  const edgeKinds = Object.keys(EDGE_COLORS) as (keyof typeof EDGE_COLORS)[]
  return (
    <div className="legend" data-testid="legend">
      <div className="legend-group">
        <span className="legend-item">
          <span className="kind-dot sm" style={{ background: '#22304a', border: '1px solid #3a4a66' }} />
          package
        </span>
        <span className="legend-item">
          <span className="kind-dot sm" style={{ background: NODE_COLORS.symbol, borderRadius: '50%' }} />
          symbol
        </span>
      </div>
      <div className="legend-group">
        {edgeKinds.map((k) => (
          <span key={k} className="legend-item">
            <span
              className="edge-swatch"
              style={{ borderBottom: `2px ${EDGE_STYLES[k]} ${EDGE_COLORS[k]}` }}
            />
            {k}
          </span>
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Build fully green + unit tests**

Run: `cd web && npm run build && npm test`
Expected: build clean (dist regenerated), all unit tests pass.

- [ ] **Step 3: Manual smoke check**

```bash
go build -o /tmp/two-state-serve ./cmd/codeindex && /tmp/two-state-serve serve "$(pwd)" --addr 127.0.0.1:7799
```

Open `http://127.0.0.1:7799`: package map with gentle node drift; lore rail on the right (no session rows); click a package → morph into focus view with satellites and ≤8 labeled symbols; hover dots → labels pop; Esc → overview restored where it was; `?motion=0` → everything static. Stop the server. (If you cannot drive a browser, verify `curl -s 127.0.0.1:7799/api/health` and note it — Task 7 e2e covers interaction.)

- [ ] **Step 4: Commit (includes rebuilt dist)**

```bash
git add web/src/App.tsx internal/webserver/dist
git commit -m "feat(web): two-state app — view/URL state, lore rail wiring, hover-sync"
```

---

### Task 7: e2e suite rewrite

**Files:**
- Rewrite: `web/tests/e2e.spec.ts`

**Interfaces:**
- Consumes: `window.__cy`, `window.__layoutDone` (Task 4); testids `graph-canvas`, `health`, `palette-input`, `inspector-title`, `neighbor`, `focus-crumb`, `crumb-pkg`, `crumb-overview`, `lore-rail`, `rail-item`, `rail-chip-sessions` (Tasks 5–6). All tests navigate with `motion=0` except the motion smoke test.

- [ ] **Step 1: Rewrite `web/tests/e2e.spec.ts`**

```ts
import { test, expect, type Page } from '@playwright/test'

declare global {
  interface Window {
    __cy?: any
    __layoutDone?: boolean
  }
}

const ready = (page: Page) =>
  expect.poll(() => page.evaluate(() => window.__layoutDone === true), { timeout: 20000 }).toBe(true)

const count = (page: Page, selector: string) =>
  page.evaluate((sel) => window.__cy?.$(sel).length ?? 0, selector)

const anchors = (page: Page, selector: string) =>
  page.evaluate((sel) => {
    const o: Record<string, { x: number; y: number }> = {}
    window.__cy.$(sel).forEach((n: any) => {
      o[n.id()] = { x: Math.round(n.data('ax')), y: Math.round(n.data('ay')) }
    })
    return o
  }, selector)

const biggestPackage = (page: Page) =>
  page.evaluate(() => {
    const pkgs = window.__cy.$('node[kind = "package"]')
    let best: any = null
    pkgs.forEach((n: any) => {
      if (!best || n.data('symCount') > best.data('symCount')) best = n
    })
    return { label: best.data('label') as string, symCount: best.data('symCount') as number }
  })

// Reset the ready flag BEFORE triggering an in-page view change: the flag is
// only cleared inside a React effect, so polling immediately after a tap could
// otherwise observe the previous view's stale `true`.
const tapNode = (page: Page, id: string) =>
  page.evaluate((nid) => {
    window.__layoutDone = false
    window.__cy.$id(nid).emit('tap')
  }, id)

const resetReady = (page: Page) => page.evaluate(() => void (window.__layoutDone = false))

test('overview: packages only, bundled widths render, no symbols or lore on canvas', async ({ page }) => {
  await page.goto('/?motion=0')
  await expect(page.getByTestId('graph-canvas')).toBeVisible()
  await ready(page)
  await expect(page.getByTestId('health')).toContainText('packages')
  expect(await count(page, 'node[kind = "package"]')).toBeGreaterThan(10)
  expect(await count(page, 'node[kind = "symbol"]')).toBe(0)
  expect(await count(page, 'node[kind = "decision"], node[kind = "item"], node[kind = "note"]')).toBe(0)
  expect(await count(page, 'edge[?bundled]')).toBeGreaterThan(5)
  const maxBundledWidth = await page.evaluate(() =>
    Math.max(...window.__cy.$('edge[?bundled]').map((e: any) => e.numericStyle('width'))),
  )
  expect(maxBundledWidth).toBeGreaterThan(1)
})

test('focus: all symbols, satellites, ≤8 earned labels, breadcrumb + URL', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const pkg = await biggestPackage(page)
  expect(pkg.symCount).toBeGreaterThan(8)
  await tapNode(page, `pkg:${pkg.label}`)
  await ready(page)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBe(pkg.symCount)
  expect(await count(page, 'node[role = "satellite"]')).toBeGreaterThan(0)
  expect(await count(page, 'node[kind = "symbol"].labeled')).toBe(8)
  await expect(page.getByTestId('crumb-pkg')).toHaveText(pkg.label)
  expect(page.url()).toContain(`pkg=${encodeURIComponent(pkg.label)}`)
})

test('satellite tap refocuses; Esc returns to the same overview map', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const before = await anchors(page, 'node[kind = "package"]')
  const pkg = await biggestPackage(page)
  await tapNode(page, `pkg:${pkg.label}`)
  await ready(page)
  const satLabel = await page.evaluate(
    () => window.__cy.$('node[role = "satellite"]').first().data('label') as string,
  )
  await tapNode(page, `pkg:${satLabel}`)
  await ready(page)
  await expect(page.getByTestId('crumb-pkg')).toHaveText(satLabel)
  await resetReady(page)
  await page.keyboard.press('Escape')
  await ready(page)
  await expect(page.getByTestId('focus-crumb')).toHaveCount(0)
  const after = await anchors(page, 'node[kind = "package"]')
  expect(after).toEqual(before)
})

test('browser back leaves focus mode', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const pkg = await biggestPackage(page)
  await tapNode(page, `pkg:${pkg.label}`)
  await ready(page)
  await resetReady(page)
  await page.goBack()
  await ready(page)
  await expect(page.getByTestId('focus-crumb')).toHaveCount(0)
  expect(await count(page, 'node[kind = "symbol"]')).toBe(0)
})

test('search enters focus and selects the symbol', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  await page.getByTestId('palette-input').fill('Neighborhood')
  await resetReady(page)
  await page.getByTestId('palette-input').press('Enter')
  await ready(page)
  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood')
  await expect(page.getByTestId('focus-crumb')).toBeVisible()
  await expect.poll(() => count(page, 'node.sel')).toBe(1)
  await expect(page.getByTestId('neighbor').first()).toBeVisible()
})

test('deep link ?pkg=&focus= restores focus + selection', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const pkg = await biggestPackage(page)
  await page.goto(`/?motion=0&pkg=${encodeURIComponent(pkg.label)}`)
  await ready(page)
  await expect(page.getByTestId('crumb-pkg')).toHaveText(pkg.label)
  expect(await count(page, 'node[kind = "symbol"]')).toBe(pkg.symCount)
})

test('lore rail: sessions hidden by default, chip reveals, hover lights the canvas', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  await expect(page.getByTestId('lore-rail')).toBeVisible()
  const sessionRows = page.locator('[data-testid="rail-item"][data-kind="sessions"]')
  await expect(sessionRows).toHaveCount(0)
  await page.getByTestId('rail-chip-sessions').click()
  expect(await sessionRows.count()).toBeGreaterThan(0)
  // Hover the first rail item that has anchored packages.
  const items = page.getByTestId('rail-item')
  const n = await items.count()
  let lit = 0
  for (let i = 0; i < n && lit === 0; i++) {
    await items.nth(i).hover()
    lit = await count(page, 'node.lore-hot')
  }
  expect(lit).toBeGreaterThan(0)
})

test('anchors are deterministic across reloads (overview and focus)', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const ov1 = await anchors(page, 'node[kind = "package"]')
  const pkg = await biggestPackage(page)
  await page.goto(`/?motion=0&pkg=${encodeURIComponent(pkg.label)}`)
  await ready(page)
  const fc1 = await anchors(page, 'node[kind = "symbol"]')
  await page.goto('/?motion=0')
  await ready(page)
  const ov2 = await anchors(page, 'node[kind = "package"]')
  await page.goto(`/?motion=0&pkg=${encodeURIComponent(pkg.label)}`)
  await ready(page)
  const fc2 = await anchors(page, 'node[kind = "symbol"]')
  expect(ov2).toEqual(ov1)
  expect(fc2).toEqual(fc1)
})

test('motion: anchors still, rendered positions drift', async ({ page }) => {
  await page.goto('/')
  await ready(page)
  const snap = () =>
    page.evaluate(() => {
      const a: Record<string, { ax: number; ay: number; x: number; y: number }> = {}
      window.__cy.$('node[kind = "package"]').forEach((n: any) => {
        const p = n.position()
        a[n.id()] = { ax: n.data('ax'), ay: n.data('ay'), x: p.x, y: p.y }
      })
      return a
    })
  const s1 = await snap()
  await page.waitForTimeout(600)
  const s2 = await snap()
  const ids = Object.keys(s1)
  expect(ids.length).toBeGreaterThan(0)
  for (const id of ids) {
    expect(s2[id].ax).toBe(s1[id].ax)
    expect(s2[id].ay).toBe(s1[id].ay)
  }
  const moved = ids.some((id) => s1[id].x !== s2[id].x || s1[id].y !== s2[id].y)
  expect(moved).toBe(true)
})
```

- [ ] **Step 2: Build + run e2e**

Run: `cd web && npm run build && npx playwright test`
Expected: 9/9 pass. Judgement calls on failure — debug, do not weaken assertions:
- Determinism failures → something outside the seeded wrapper is nondeterministic (check transition completion order writes anchors after `layoutstop` only).
- `labeled` count ≠ 8 → the near-zoom band may be active after `cy.fit` on a small package (zoom ≥ 1.1 labels everything). If the biggest package legitimately fits at zoom ≥ 1.1, assert `>= 8` is NOT the fix — instead zoom the viewport out in the test before counting (`window.__cy.zoom(0.5)`) to leave the near band.
- Motion test flaky on `moved` → increase the wait to 900ms; the oscillation period floor is 5s so 600–900ms is well inside a quarter-cycle.
- If a real app bug surfaces, report BLOCKED with evidence; app fixes belong to a review cycle.

- [ ] **Step 3: Commit (includes rebuilt dist if changed)**

```bash
git add web/tests/e2e.spec.ts internal/webserver/dist
git commit -m "test(web): e2e for two-state UI — focus, satellites, rail, determinism, motion"
```

---

### Task 8: Full verification + lore close-out

**Files:** none (verification); lore via CLI.

- [ ] **Step 1: Run everything**

```bash
cd web && npm test && npm run build && npx playwright test
cd .. && go build ./... && go test ./internal/webserver/ ./internal/readmodel/
```

Expected: all green (backend untouched — this confirms it).

- [ ] **Step 2: Record completion in lore**

```bash
go run ./cmd/codeindex lore /Users/ethanhinson/codeindex add note --title "Graph two-state UI landed on feature/graph-two-state-ui" --body "Implemented per dec-01KYWJEEHNAP5HB52AVA7WVB45: overview ⇄ focus with rim satellites (focusVM), earned labels (LABEL_TOP=8 + hover + selection + near-zoom), HTML lore rail with kind filters and hover-sync (sessions hidden by default), anchored motion (ax/ay anchors + hash-seeded oscillation, motion=0 / reduced-motion opt-outs, 350ms focus morph). In-place expansion and chips removed."
```

- [ ] **Step 3: Confirm clean tree**

```bash
git status --short   # expect only session-local .claude/settings.local.json if anything
```

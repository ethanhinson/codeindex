# Graph Rendering Smoothness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the render-everything graph view with an overview-first, ranked-reveal model: land on ~60 package/lore nodes, expand packages to their top-12 symbols + a `+N more` chip, deterministic layout, diffed hover highlighting, LOD edge hiding.

**Architecture:** All client-side over the existing single `/api/graph/full` fetch (no backend changes). A new pure module `web/src/graph/aggregate.ts` turns the full graph + expansion state into a view model; `GraphCanvas` diffs the view model into cytoscape incrementally (never full rebuild); layout is deterministic (sorted-circle seeding + fcose `randomize:false` under a seeded `Math.random`, expansion children placed by deterministic phyllotaxis around their package — nothing existing ever moves).

**Tech Stack:** React 18, cytoscape 3.30 + cytoscape-fcose, Vite, vitest (new devDep), Playwright.

**Spec:** `docs/superpowers/specs/2026-07-30-graph-rendering-smoothness-design.md`

## Global Constraints

- Work happens in the `lore-graph-ui` worktree (`.claude/worktrees/lore-graph-ui`), branch `worktree-lore-graph-ui`. All paths below are relative to that worktree root.
- `web/` build must stay green: `cd web && npm run build` (runs `tsc --noEmit` first, then emits into `../internal/webserver/dist` which the Go binary embeds).
- `aggregate.ts` is pure: it must not import cytoscape or react. No throws on odd data: symbols without a group fall into the `(ungrouped)` package; edges referencing unknown node ids are dropped and counted.
- Reveal count is `TOP_N = 12`. Chip label format is exactly `+N more`.
- Node id namespaces: package nodes `pkg:<group>`, chip nodes `chip:<group>`; symbol/lore ids pass through unchanged from the API.
- No `Date.now()`/`Math.random()`-dependent behavior outside the seeded-layout wrapper: everything renderable must be a deterministic function of (payload, expansion state).
- Backend (`internal/webserver`, `internal/readmodel`) is untouched by this plan.

---

### Task 1: Vitest setup + `buildIndex` (degrees, package ranking)

**Files:**
- Modify: `web/package.json` (add vitest, `test` script)
- Create: `web/src/graph/aggregate.ts`
- Test: `web/src/graph/aggregate.test.ts`

**Interfaces:**
- Consumes: `Graph`, `GraphNode`, `GraphEdge` from `web/src/types.ts` (existing).
- Produces (used by Tasks 2, 5):

```ts
export const TOP_N = 12
export const UNGROUPED = '(ungrouped)'
export function pkgId(name: string): string   // `pkg:${name}`
export function chipId(name: string): string  // `chip:${name}`

export interface GraphIndex {
  nodes: GraphNode[]
  edges: GraphEdge[]              // only edges whose both endpoints exist
  nodeById: Map<string, GraphNode>
  degree: Map<string, number>     // total degree over kept edges
  packages: Map<string, string[]> // group -> symbol ids, degree desc then id asc
  pkgOf: Map<string, string>      // symbol id -> group
  dropped: number                 // edges dropped for unknown endpoints
}
export function buildIndex(g: Graph): GraphIndex
```

- [ ] **Step 1: Install vitest and add the test script**

```bash
cd web && npm install -D vitest
```

In `web/package.json` scripts, add:

```json
"test": "vitest run"
```

- [ ] **Step 2: Write the failing tests**

Create `web/src/graph/aggregate.test.ts`:

```ts
import { describe, expect, test } from 'vitest'
import { buildIndex, pkgId, chipId, TOP_N, UNGROUPED } from './aggregate'
import type { Graph, GraphEdge, GraphNode } from '../types'

export function sym(id: string, group?: string, label?: string): GraphNode {
  return { id, kind: 'symbol', label: label ?? id, group }
}
export function lore(id: string, kind: 'decision' | 'item' | 'note'): GraphNode {
  return { id, kind, label: id }
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
    expect(ix.degree.get('a')).toBeUndefined()
  })

  test('groups symbols by package; missing group falls into (ungrouped)', () => {
    const ix = buildIndex(g([sym('a', 'p'), sym('b', 'p'), sym('c')], []))
    expect(ix.packages.get('p')).toEqual(['a', 'b'])
    expect(ix.packages.get(UNGROUPED)).toEqual(['c'])
    expect(ix.pkgOf.get('c')).toBe(UNGROUPED)
  })

  test('ranks symbols in a package by degree desc, id asc tiebreak', () => {
    const nodes = [sym('lo', 'p'), sym('hub', 'p'), sym('mid', 'p'), sym('x', 'q')]
    const edges = [call('hub', 'x'), call('hub', 'mid'), call('mid', 'x')]
    const ix = buildIndex(g(nodes, edges))
    expect(ix.packages.get('p')).toEqual(['hub', 'mid', 'lo'])
    expect(ix.degree.get('hub')).toBe(2)
  })

  test('lore nodes are not package members', () => {
    const ix = buildIndex(g([sym('a', 'p'), lore('d1', 'decision')], []))
    expect(ix.packages.get('p')).toEqual(['a'])
    expect(ix.pkgOf.has('d1')).toBe(false)
  })

  test('id helpers', () => {
    expect(pkgId('internal/graph')).toBe('pkg:internal/graph')
    expect(chipId('internal/graph')).toBe('chip:internal/graph')
    expect(TOP_N).toBe(12)
  })
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd web && npx vitest run src/graph/aggregate.test.ts`
Expected: FAIL — `Cannot find module './aggregate'` (or missing exports).

- [ ] **Step 4: Implement `buildIndex`**

Create `web/src/graph/aggregate.ts`:

```ts
// Pure aggregation over the full graph payload: package ranking and the
// visible view model. No cytoscape or react imports — unit-testable as-is.
import type { Graph, GraphEdge, GraphNode } from '../types'

export const TOP_N = 12
export const UNGROUPED = '(ungrouped)'

export function pkgId(name: string): string {
  return `pkg:${name}`
}
export function chipId(name: string): string {
  return `chip:${name}`
}

export interface GraphIndex {
  nodes: GraphNode[]
  edges: GraphEdge[]
  nodeById: Map<string, GraphNode>
  degree: Map<string, number>
  packages: Map<string, string[]>
  pkgOf: Map<string, string>
  dropped: number
}

export function buildIndex(g: Graph): GraphIndex {
  const nodeById = new Map(g.nodes.map((n) => [n.id, n]))
  const degree = new Map<string, number>()
  const edges: GraphEdge[] = []
  let dropped = 0
  for (const e of g.edges) {
    if (!nodeById.has(e.source) || !nodeById.has(e.target)) {
      dropped++
      continue
    }
    edges.push(e)
    degree.set(e.source, (degree.get(e.source) ?? 0) + 1)
    degree.set(e.target, (degree.get(e.target) ?? 0) + 1)
  }
  const packages = new Map<string, string[]>()
  const pkgOf = new Map<string, string>()
  for (const n of g.nodes) {
    if (n.kind !== 'symbol') continue
    const pkg = n.group || UNGROUPED
    pkgOf.set(n.id, pkg)
    const ids = packages.get(pkg)
    if (ids) ids.push(n.id)
    else packages.set(pkg, [n.id])
  }
  for (const ids of packages.values()) {
    ids.sort((a, b) => (degree.get(b) ?? 0) - (degree.get(a) ?? 0) || (a < b ? -1 : 1))
  }
  return { nodes: g.nodes, edges, nodeById, degree, packages, pkgOf, dropped }
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd web && npx vitest run src/graph/aggregate.test.ts`
Expected: PASS (6 tests).

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/package-lock.json web/src/graph/aggregate.ts web/src/graph/aggregate.test.ts
git commit -m "feat(web): aggregate index — degree ranking per package, vitest setup"
```

---

### Task 2: `viewModel` — visibility, chip, bundled-edge resolution

**Files:**
- Modify: `web/src/graph/aggregate.ts`
- Test: `web/src/graph/aggregate.test.ts` (append)

**Interfaces:**
- Consumes: `GraphIndex`, `TOP_N`, `pkgId`, `chipId` from Task 1.
- Produces (used by Tasks 4, 5):

```ts
export interface ViewState {
  expanded: Set<string> // package names currently expanded
  tails: Set<string>    // packages whose long tail is revealed
  extras: Set<string>   // symbol ids force-revealed (search/selection)
}
export interface VisNode {
  id: string
  kind: string // NodeKind | 'package' | 'chip'
  label: string
  parent?: string
  degree: number
  symCount?: number // package nodes only
  pkg?: string      // symbols and chips: owning package
  rank?: number     // symbols: rank within package (0 = top hub)
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
export interface ViewModel { nodes: VisNode[]; edges: VisEdge[] }
export function visibleSymbols(index: GraphIndex, state: ViewState): Set<string>
export function viewModel(index: GraphIndex, state: ViewState): ViewModel
```

- [ ] **Step 1: Write the failing tests**

Append to `web/src/graph/aggregate.test.ts`:

```ts
import { viewModel, visibleSymbols, type ViewState } from './aggregate'

function state(p: Partial<ViewState> = {}): ViewState {
  return { expanded: new Set(), tails: new Set(), extras: new Set(), ...p }
}

describe('viewModel', () => {
  test('overview: one package node per group, sized; lore individual; no symbols', () => {
    const ix = buildIndex(
      g([sym('a', 'p'), sym('b', 'p'), sym('c', 'q'), lore('d1', 'decision')], [call('a', 'c')]),
    )
    const vm = viewModel(ix, state())
    const ids = vm.nodes.map((n) => n.id).sort()
    expect(ids).toEqual(['d1', 'pkg:p', 'pkg:q'])
    const p = vm.nodes.find((n) => n.id === 'pkg:p')!
    expect(p.kind).toBe('package')
    expect(p.symCount).toBe(2)
  })

  test('cross-package calls bundle with counts; intra-package hidden edges drop', () => {
    const ix = buildIndex(
      g(
        [sym('a', 'p'), sym('b', 'p'), sym('c', 'q'), sym('d', 'q')],
        [call('a', 'c'), call('b', 'c'), call('a', 'b'), call('c', 'd')],
      ),
    )
    const vm = viewModel(ix, state())
    expect(vm.edges).toHaveLength(1)
    expect(vm.edges[0]).toMatchObject({ source: 'pkg:p', target: 'pkg:q', count: 2, bundled: true })
  })

  test('expanded package under TOP_N: all symbols visible, no chip', () => {
    const ix = buildIndex(g([sym('a', 'p'), sym('b', 'p')], []))
    const vm = viewModel(ix, state({ expanded: new Set(['p']) }))
    const kinds = new Map(vm.nodes.map((n) => [n.id, n.kind]))
    expect(kinds.get('a')).toBe('symbol')
    expect(kinds.get('b')).toBe('symbol')
    expect(vm.nodes.some((n) => n.kind === 'chip')).toBe(false)
    expect(vm.nodes.find((n) => n.id === 'a')!.parent).toBe('pkg:p')
  })

  test('expanded big package: top-12 by rank + chip "+N more"; tail reveals all', () => {
    const syms = Array.from({ length: 20 }, (_, i) => sym(`s${String(i).padStart(2, '0')}`, 'p'))
    // s00 gets highest degree, s01 next, etc. via a chain of hub edges
    const hub = sym('hub', 'q')
    const edges = syms.flatMap((s, i) => Array.from({ length: 20 - i }, () => call(s.id, 'hub')))
    const ix = buildIndex(g([...syms, hub], edges))
    const vm = viewModel(ix, state({ expanded: new Set(['p']) }))
    const visSyms = vm.nodes.filter((n) => n.kind === 'symbol' && n.pkg === 'p')
    expect(visSyms).toHaveLength(TOP_N)
    expect(visSyms.map((n) => n.id)).toContain('s00')
    expect(visSyms.map((n) => n.id)).not.toContain('s19')
    const chip = vm.nodes.find((n) => n.kind === 'chip')!
    expect(chip).toMatchObject({ id: 'chip:p', label: '+8 more', parent: 'pkg:p' })

    const all = viewModel(ix, state({ expanded: new Set(['p']), tails: new Set(['p']) }))
    expect(all.nodes.filter((n) => n.kind === 'symbol' && n.pkg === 'p')).toHaveLength(20)
    expect(all.nodes.some((n) => n.kind === 'chip')).toBe(false)
  })

  test('extras force-reveal a tail symbol; chip count shrinks by one', () => {
    const syms = Array.from({ length: 15 }, (_, i) => sym(`s${String(i).padStart(2, '0')}`, 'p'))
    const hub = sym('hub', 'q')
    const edges = syms.flatMap((s, i) => Array.from({ length: 15 - i }, () => call(s.id, 'hub')))
    const ix = buildIndex(g([...syms, hub], edges))
    const vm = viewModel(ix, state({ expanded: new Set(['p']), extras: new Set(['s14']) }))
    const visIds = vm.nodes.filter((n) => n.kind === 'symbol' && n.pkg === 'p').map((n) => n.id)
    expect(visIds).toContain('s14')
    expect(visIds).toHaveLength(TOP_N + 1)
    expect(vm.nodes.find((n) => n.kind === 'chip')!.label).toBe('+2 more')
  })

  test('edges between visible symbols are concrete; visible-to-hidden bundles to package', () => {
    const ix = buildIndex(
      g([sym('a', 'p'), sym('c', 'q'), sym('d', 'q')], [call('a', 'c'), call('a', 'd')]),
    )
    // q expanded but only c revealed via extras trick: use tails to show all of q instead
    const vm = viewModel(ix, state({ expanded: new Set(['p', 'q']), tails: new Set(['p', 'q']) }))
    const concrete = vm.edges.filter((e) => !e.bundled)
    expect(concrete).toHaveLength(2)
    // now collapse q: a's two calls bundle into one a->pkg:q edge
    const vm2 = viewModel(ix, state({ expanded: new Set(['p']), tails: new Set(['p']) }))
    expect(vm2.edges).toHaveLength(1)
    expect(vm2.edges[0]).toMatchObject({ source: 'a', target: 'pkg:q', count: 2, bundled: true })
  })

  test('lore edge to hidden symbol bundles to its package, keeps kind', () => {
    const ix = buildIndex(
      g([sym('a', 'p'), lore('d1', 'decision')], [{ source: 'd1', target: 'a', kind: 'anchors' }]),
    )
    const vm = viewModel(ix, state())
    expect(vm.edges[0]).toMatchObject({ source: 'd1', target: 'pkg:p', kind: 'anchors', bundled: true })
  })

  test('visibleSymbols honors rank cutoff', () => {
    const ix = buildIndex(g([sym('a', 'p'), sym('b', 'p')], [call('a', 'b')]))
    expect(visibleSymbols(ix, state())).toEqual(new Set())
    expect(visibleSymbols(ix, state({ expanded: new Set(['p']) }))).toEqual(new Set(['a', 'b']))
  })
})
```

- [ ] **Step 2: Run tests to verify the new ones fail**

Run: `cd web && npx vitest run src/graph/aggregate.test.ts`
Expected: FAIL — `viewModel` is not exported.

- [ ] **Step 3: Implement `viewModel`**

Append to `web/src/graph/aggregate.ts`:

```ts
export interface ViewState {
  expanded: Set<string>
  tails: Set<string>
  extras: Set<string>
}

export interface VisNode {
  id: string
  kind: string
  label: string
  parent?: string
  degree: number
  symCount?: number
  pkg?: string
  rank?: number
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

export function visibleSymbols(index: GraphIndex, state: ViewState): Set<string> {
  const vis = new Set<string>()
  for (const pkg of state.expanded) {
    const ids = index.packages.get(pkg) ?? []
    const all = state.tails.has(pkg)
    ids.forEach((id, rank) => {
      if (all || rank < TOP_N || state.extras.has(id)) vis.add(id)
    })
  }
  return vis
}

export function viewModel(index: GraphIndex, state: ViewState): ViewModel {
  const vis = visibleSymbols(index, state)
  const nodes: VisNode[] = []
  for (const [pkg, ids] of index.packages) {
    nodes.push({ id: pkgId(pkg), kind: 'package', label: pkg, degree: 0, symCount: ids.length })
  }
  for (const n of index.nodes) {
    if (n.kind === 'symbol') {
      if (!vis.has(n.id)) continue
      const pkg = index.pkgOf.get(n.id) as string
      const rank = (index.packages.get(pkg) ?? []).indexOf(n.id)
      nodes.push({
        id: n.id,
        kind: n.kind,
        label: n.label,
        parent: pkgId(pkg),
        pkg,
        rank,
        degree: index.degree.get(n.id) ?? 0,
      })
    } else {
      nodes.push({ id: n.id, kind: n.kind, label: n.label, degree: index.degree.get(n.id) ?? 0 })
    }
  }
  for (const pkg of state.expanded) {
    const ids = index.packages.get(pkg) ?? []
    const hidden = ids.filter((id) => !vis.has(id)).length
    if (hidden > 0) {
      nodes.push({
        id: chipId(pkg),
        kind: 'chip',
        label: `+${hidden} more`,
        parent: pkgId(pkg),
        pkg,
        degree: 0,
      })
    }
  }

  // Each endpoint maps to its representative: itself if visible (lore always
  // is), else its package node. Same-representative edges vanish (collapsed
  // intra-package calls); everything else groups by (src, tgt, kind, form).
  const rep = (id: string): string => {
    const n = index.nodeById.get(id) as GraphNode
    if (n.kind !== 'symbol' || vis.has(id)) return id
    return pkgId(index.pkgOf.get(id) as string)
  }
  const acc = new Map<string, VisEdge>()
  for (const e of index.edges) {
    const s = rep(e.source)
    const t = rep(e.target)
    if (s === t) continue
    const bundled = s !== e.source || t !== e.target
    const key = `${s}|${t}|${e.kind}|${bundled ? 'b' : 'c'}`
    const cur = acc.get(key)
    if (cur) cur.count++
    else
      acc.set(key, {
        id: (bundled ? 'b:' : 'e:') + key,
        source: s,
        target: t,
        kind: e.kind,
        count: 1,
        bundled,
        conf: bundled ? undefined : e.conf,
      })
  }
  return { nodes, edges: [...acc.values()] }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/graph/aggregate.test.ts`
Expected: PASS (all tests from Tasks 1–2).

- [ ] **Step 5: Commit**

```bash
git add web/src/graph/aggregate.ts web/src/graph/aggregate.test.ts
git commit -m "feat(web): viewModel — ranked reveal, chip, visibility-driven edge bundling"
```

---

### Task 3: Styles for package / chip / bundled edges / LOD

**Files:**
- Modify: `web/src/graph/style.ts`

**Interfaces:**
- Consumes: nothing new.
- Produces: stylesheet rules keyed on data set by Task 2's `VisNode`/`VisEdge` fields (`kind: 'package' | 'chip'`, `symCount`, `count`, `bundled`) and classes used by Task 4 (`dim`, `hl`, `sel`, `selhl` existing; new `lod-hide`).

- [ ] **Step 1: Add the new style rules**

In `web/src/graph/style.ts`, inside `stylesheet()`, append to `base` (after the existing `node.group` block — leave that block in place until Task 4 deletes the old canvas code that uses it):

```ts
    // Collapsed package: a plain node sized by how many symbols it holds.
    {
      selector: 'node[kind = "package"]',
      style: {
        shape: 'round-rectangle',
        'background-color': '#22304a',
        'border-width': 1.5,
        'border-color': '#3a4a66',
        color: '#aab6c8',
        'font-size': 12,
        'min-zoomed-font-size': 0,
        'text-valign': 'center',
        'text-halign': 'center',
        width: 'mapData(symCount, 1, 120, 30, 110)',
        height: 'mapData(symCount, 1, 120, 22, 46)',
      },
    },
    // Expanded package: compound parent — translucent container, label on top.
    {
      selector: 'node[kind = "package"]:parent',
      style: {
        'background-color': '#4f8ff7',
        'background-opacity': 0.05,
        'border-style': 'dashed',
        'border-color': '#2a3140',
        'text-valign': 'top',
        'text-margin-y': -4,
        padding: 16,
        'z-compound-depth': 'bottom',
      },
    },
    // The "+N more" chip.
    {
      selector: 'node[kind = "chip"]',
      style: {
        shape: 'round-rectangle',
        'background-color': '#2a3140',
        'border-width': 1,
        'border-color': '#4f8ff7',
        color: '#c5ccd8',
        'font-size': 10,
        'min-zoomed-font-size': 0,
        'text-valign': 'center',
        'text-halign': 'center',
        width: 60,
        height: 18,
      },
    },
    // Bundled edges: width carries the call count between the two ends.
    {
      selector: 'edge[?bundled]',
      style: {
        width: 'mapData(count, 1, 60, 1, 7)',
        'curve-style': 'straight',
        'line-color': '#3a4356',
        opacity: 0.55,
      },
    },
    // LOD: elements hidden at far zoom.
    { selector: '.lod-hide', style: { display: 'none' } },
```

Note: the bundled-edge rule must come *before* the per-kind edge loop at the bottom of `stylesheet()` so that `anchors`/`blocked_by` bundles keep their kind color/dash; move it above that loop accordingly (the per-kind selectors win by order for shared properties).

- [ ] **Step 2: Verify typecheck/build**

Run: `cd web && npm run build`
Expected: clean build (styles are data-driven; nothing consumes them yet).

- [ ] **Step 3: Commit**

```bash
git add web/src/graph/style.ts
git commit -m "feat(web): styles for package, chip, bundled edges, LOD class"
```

---

### Task 4: GraphCanvas rework — incremental diff, deterministic layout, hover diff, LOD

**Files:**
- Rewrite: `web/src/graph/GraphCanvas.tsx`
- Modify: `web/src/graph/style.ts` (delete the now-dead `node.group` block)
- Modify: `docs/superpowers/specs/2026-07-30-graph-rendering-smoothness-design.md` (one line, see Step 3)

**Interfaces:**
- Consumes: `ViewModel`, `VisNode` from Task 2; `stylesheet()` from Task 3.
- Produces (used by Task 5):

```ts
interface Props {
  vm: ViewModel
  selected: string | null
  onSelect: (id: string | null) => void
  onTogglePackage: (pkg: string) => void
  onRevealTail: (pkg: string) => void
}
export function GraphCanvas(props: Props): JSX.Element
```

Also exposes `window.__cy` (existing) and sets `window.__layoutDone = true` after the initial overview layout (consumed by e2e in Task 6).

- [ ] **Step 1: Rewrite `web/src/graph/GraphCanvas.tsx`**

```tsx
import { useEffect, useRef } from 'react'
import cytoscape from 'cytoscape'
import type { CollectionReturnValue, Core, ElementDefinition } from 'cytoscape'
import fcose from 'cytoscape-fcose'
import type { ViewModel, VisNode } from './aggregate'
import { stylesheet } from './style'

let registered = false
function ensureFcose() {
  if (!registered) {
    cytoscape.use(fcose)
    registered = true
  }
}

const LOD_ZOOM = 0.4
const GOLDEN_ANGLE = 2.399963229728653

// Deterministic seed positions: packages on a circle in sorted-label order
// (path sort keeps sibling dirs adjacent), lore nodes on an outer ring.
function seedPositions(vm: ViewModel): Map<string, { x: number; y: number }> {
  const pos = new Map<string, { x: number; y: number }>()
  const pkgs = vm.nodes.filter((n) => n.kind === 'package').sort((a, b) => (a.label < b.label ? -1 : 1))
  const lore = vm.nodes.filter((n) => n.kind !== 'package' && !n.parent).sort((a, b) => (a.id < b.id ? -1 : 1))
  const r = Math.max(220, (pkgs.length * 95) / (2 * Math.PI))
  pkgs.forEach((n, i) => {
    const a = (i / Math.max(1, pkgs.length)) * 2 * Math.PI
    pos.set(n.id, { x: r * Math.cos(a), y: r * Math.sin(a) })
  })
  lore.forEach((n, i) => {
    const a = (i / Math.max(1, lore.length)) * 2 * Math.PI + 0.13
    pos.set(n.id, { x: 1.35 * r * Math.cos(a), y: 1.35 * r * Math.sin(a) })
  })
  return pos
}

// Children reveal in a phyllotaxis spiral around their package's current
// position — deterministic, compact, and nothing else on the map moves.
function childOffset(rank: number): { x: number; y: number } {
  const a = rank * GOLDEN_ANGLE
  const r = 24 + 13 * Math.sqrt(rank)
  return { x: r * Math.cos(a), y: r * Math.sin(a) }
}

function toElement(n: VisNode): ElementDefinition {
  return { data: { ...n } }
}

// fcose has internal Math.random calls; pin them to a seeded LCG for the
// duration of the layout so the same input always yields the same map.
function seededLayout(cy: Core, opts: Record<string, unknown>, onDone: () => void) {
  const orig = Math.random
  let s = 42
  Math.random = () => {
    s = (s * 1664525 + 1013904223) >>> 0
    return s / 4294967296
  }
  const layout = cy.layout({ name: 'fcose', ...opts } as never)
  layout.one('layoutstop', () => {
    Math.random = orig
    onDone()
  })
  layout.run()
}

// Diff the desired view model into the live instance: remove what left,
// add what arrived (children positioned around their package), never touch
// what stayed. Returns the newly added nodes.
function applyViewModel(cy: Core, vm: ViewModel): CollectionReturnValue {
  const wantNodes = new Map(vm.nodes.map((n) => [n.id, n]))
  const wantEdges = new Map(vm.edges.map((e) => [e.id, e]))
  let added = cy.collection()
  cy.batch(() => {
    cy.edges().forEach((e) => {
      if (!wantEdges.has(e.id())) e.remove()
    })
    cy.nodes().forEach((n) => {
      if (!wantNodes.has(n.id())) n.remove()
    })
    // Parents first so compound membership resolves on add.
    const fresh = vm.nodes.filter((n) => !cy.$id(n.id).nonempty())
    fresh.sort((a, b) => Number(!!a.parent) - Number(!!b.parent))
    for (const n of fresh) {
      const el = cy.add(toElement(n))
      if (n.parent) {
        const p = cy.$id(n.parent).position()
        const off = childOffset(n.kind === 'chip' ? 0 : (n.rank ?? 0) + 1)
        el.position({ x: p.x + off.x, y: p.y + off.y })
      }
      added = added.union(el)
    }
    for (const e of vm.edges) {
      if (cy.$id(e.id).empty()) cy.add({ data: { ...e } })
    }
  })
  return added
}

function applyLod(cy: Core) {
  const far = cy.zoom() < LOD_ZOOM
  cy.batch(() => {
    const weak = cy.edges('[count = 1]')
    if (far) weak.addClass('lod-hide')
    else weak.removeClass('lod-hide')
  })
}

interface Props {
  vm: ViewModel
  selected: string | null
  onSelect: (id: string | null) => void
  onTogglePackage: (pkg: string) => void
  onRevealTail: (pkg: string) => void
}

export function GraphCanvas({ vm, selected, onSelect, onTogglePackage, onRevealTail }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<Core | null>(null)
  const laidOut = useRef(false)
  const lodFar = useRef(false)
  const hood = useRef<CollectionReturnValue | null>(null)
  const clearTimer = useRef<number | undefined>(undefined)
  const cb = useRef({ onSelect, onTogglePackage, onRevealTail })
  cb.current = { onSelect, onTogglePackage, onRevealTail }

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
      const kind = n.data('kind') as string
      if (kind === 'package') cb.current.onTogglePackage(n.data('label') as string)
      else if (kind === 'chip') cb.current.onRevealTail(n.data('pkg') as string)
      else cb.current.onSelect(n.id())
    })
    cy.on('tap', (evt) => {
      if (evt.target === cy) cb.current.onSelect(null)
    })

    // Hover: toggle classes only on the symmetric difference between the old
    // and new neighborhoods — never a full-graph class sweep. The clear is
    // debounced so transiting between adjacent nodes doesn't flash.
    const clearHover = () => {
      const prev = hood.current
      if (!prev) return
      hood.current = null
      cy.batch(() => cy.elements().removeClass('dim hl'))
    }
    cy.on('mouseover', 'node', (evt) => {
      const n = evt.target
      if (n.data('kind') === 'package' && n.isParent()) return
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
    })
    ;(window as unknown as { __cy?: Core }).__cy = cy
    cyRef.current = cy
    return () => {
      window.clearTimeout(clearTimer.current)
      cy.destroy()
      cyRef.current = null
    }
  }, [])

  // Reflect the view model. First non-empty model: seed + fcose refine + fit.
  // Later models: incremental add/remove only — the map never jumps.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy || vm.nodes.length === 0) return
    if (!laidOut.current) {
      laidOut.current = true
      const seeds = seedPositions(vm)
      cy.batch(() => {
        applyViewModel(cy, vm)
        for (const [id, p] of seeds) cy.$id(id).position(p)
      })
      seededLayout(cy, { quality: 'default', animate: false, randomize: false, fit: false, nodeSeparation: 120, idealEdgeLength: 130, nodeRepulsion: 6500, gravity: 0.15, packComponents: false }, () => {
        cy.fit(undefined, 50)
        applyLod(cy)
        ;(window as unknown as { __layoutDone?: boolean }).__layoutDone = true
      })
    } else {
      const added = applyViewModel(cy, vm)
      if (added.nonempty()) applyLod(cy)
    }
  }, [vm])

  // Reflect selection: mark, highlight neighborhood, and center.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    cy.elements().removeClass('sel selhl')
    if (!selected) return
    const n = cy.$id(selected)
    if (n.empty()) return
    n.addClass('sel')
    n.closedNeighborhood().addClass('selhl')
    cy.animate({ center: { eles: n }, zoom: Math.max(cy.zoom(), 0.8) }, { duration: 300 })
  }, [selected, vm])

  return <div ref={containerRef} className="graph-canvas" data-testid="graph-canvas" />
}
```

- [ ] **Step 2: Delete the dead `node.group` style block**

In `web/src/graph/style.ts`, remove the whole `selector: 'node.group'` entry (the old compound-cluster style — package nodes now use `node[kind = "package"]`).

- [ ] **Step 3: Sync the spec's layout wording**

In `docs/superpowers/specs/2026-07-30-graph-rendering-smoothness-design.md`, section "3. Deterministic, incremental layout", replace the sentence

> **Expansion layout:** all pre-existing nodes are pinned via fcose `fixedNodeConstraint`; only the newly revealed children lay out, seeded around their package's current centroid. The map never jumps under the user.

with

> **Expansion layout:** no layout runs on expand at all — newly revealed children are placed on a deterministic phyllotaxis spiral around their package's current position, so every pre-existing node is pinned by construction. The map never jumps under the user.

(Same guarantee, simpler mechanism; discovered during implementation planning.)

- [ ] **Step 4: Typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: errors only in `App.tsx` (it still passes `nodes`/`edges` props — fixed in Task 5). No errors in `GraphCanvas.tsx` or `style.ts`. If `App.tsx` is the sole error source, proceed.

- [ ] **Step 5: Commit**

```bash
git add web/src/graph/GraphCanvas.tsx web/src/graph/style.ts docs/superpowers/specs/2026-07-30-graph-rendering-smoothness-design.md
git commit -m "feat(web): incremental GraphCanvas — deterministic seeded layout, hover diff, LOD"
```

---

### Task 5: App wiring — expansion state, search auto-expand

**Files:**
- Modify: `web/src/App.tsx`

**Interfaces:**
- Consumes: `buildIndex`, `viewModel`, `TOP_N` (Task 1–2); `GraphCanvas` props (Task 4); existing `useFullGraph`, `resolveFocus`, `CommandPalette`, `Inspector`, `Legend`.
- Produces: none downstream; `Inspector` keeps receiving the **full** `nodes`/`edges` arrays (its neighbor list is data-level, independent of what's rendered).

- [ ] **Step 1: Rework `App.tsx` state and wiring**

Replace the body of `App()` in `web/src/App.tsx` (imports shown; `Legend` and the JSX skeleton stay as-is except the `GraphCanvas` props and status line):

```tsx
import { useCallback, useEffect, useMemo, useState } from 'react'
import { getHealth } from './api'
import type { Health } from './types'
import type { Suggestion } from './CommandPalette'
import { useFullGraph, resolveFocus } from './useFullGraph'
import { buildIndex, viewModel } from './graph/aggregate'
import { GraphCanvas } from './graph/GraphCanvas'
import { CommandPalette } from './CommandPalette'
import { Inspector } from './Inspector'
import { EDGE_COLORS, EDGE_STYLES, NODE_COLORS } from './graph/style'

export default function App() {
  const { nodes, edges, loading, error } = useFullGraph()
  const [health, setHealth] = useState<Health | null>(null)
  const [selected, setSelected] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [tails, setTails] = useState<Set<string>>(new Set())
  const [extras, setExtras] = useState<Set<string>>(new Set())

  const index = useMemo(() => buildIndex({ focus: '', nodes, edges }), [nodes, edges])
  const vm = useMemo(
    () => viewModel(index, { expanded, tails, extras }),
    [index, expanded, tails, extras],
  )

  useEffect(() => {
    getHealth().then(setHealth).catch(() => setHealth(null))
  }, [])

  // Surface data oddities once: edges referencing unknown node ids are
  // dropped by buildIndex, not rendered.
  useEffect(() => {
    if (index.dropped > 0) {
      console.warn(`graph: dropped ${index.dropped} edge(s) with unknown endpoints`)
    }
  }, [index])

  // Selecting a symbol always reveals it: expand its package and force it
  // past the top-N cutoff if it lives in the long tail.
  const selectNode = useCallback(
    (id: string | null) => {
      if (id) {
        const pkg = index.pkgOf.get(id)
        if (pkg) {
          setExpanded((prev) => (prev.has(pkg) ? prev : new Set(prev).add(pkg)))
          setExtras((prev) => (prev.has(id) ? prev : new Set(prev).add(id)))
        }
      }
      setSelected(id)
    },
    [index],
  )

  const togglePackage = useCallback(
    (pkg: string) => {
      setExpanded((prev) => {
        const next = new Set(prev)
        if (next.has(pkg)) {
          next.delete(pkg)
          setTails((t) => {
            const nt = new Set(t)
            nt.delete(pkg)
            return nt
          })
          setExtras((x) => {
            const nx = new Set([...x].filter((id) => index.pkgOf.get(id) !== pkg))
            return nx.size === x.size ? x : nx
          })
          setSelected((s) => (s && index.pkgOf.get(s) === pkg ? null : s))
        } else {
          next.add(pkg)
        }
        return next
      })
    },
    [index],
  )

  const revealTail = useCallback((pkg: string) => {
    setTails((prev) => (prev.has(pkg) ? prev : new Set(prev).add(pkg)))
  }, [])

  // Once the graph is in, honor ?focus= (id or label) for deep links.
  useEffect(() => {
    if (nodes.length === 0) return
    const param = new URLSearchParams(window.location.search).get('focus')
    if (param) selectNode(resolveFocus(nodes, param))
  }, [nodes, selectNode])

  const selectedNode = useMemo(
    () => (selected ? nodes.find((n) => n.id === selected) ?? null : null),
    [selected, nodes],
  )

  // Suggestions: the highest-degree symbols — the hubs worth starting from.
  const suggestions = useMemo<Suggestion[]>(
    () =>
      nodes
        .filter((n) => n.kind === 'symbol')
        .sort((a, b) => (index.degree.get(b.id) ?? 0) - (index.degree.get(a.id) ?? 0))
        .slice(0, 4)
        .map((n) => ({ id: n.id, label: n.label })),
    [nodes, index],
  )

  const symbolCount = nodes.filter((n) => n.kind === 'symbol').length
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
          <GraphCanvas
            vm={vm}
            selected={selected}
            onSelect={selectNode}
            onTogglePackage={togglePackage}
            onRevealTail={revealTail}
          />
          <Legend />
        </div>
        <Inspector node={selectedNode} nodes={nodes} edges={edges} onOpen={selectNode} />
      </main>
    </div>
  )
}
```

(Keep the existing `Legend` function unchanged below `App`.)

Note the pre-existing quirk this preserves: `togglePackage` calls other setters inside the `setExpanded` updater — React batches these fine, but keep the updater pure of side effects on `next` only. If the reviewer prefers, hoist the `has` check out and branch before calling `setExpanded`; either is acceptable, behavior is identical.

- [ ] **Step 2: Build**

Run: `cd web && npm run build`
Expected: clean `tsc --noEmit` + vite build into `../internal/webserver/dist`.

- [ ] **Step 3: Unit tests still green**

Run: `cd web && npm test`
Expected: PASS.

- [ ] **Step 4: Manual smoke check**

```bash
cd /Users/ethanhinson/codeindex/.claude/worktrees/lore-graph-ui && go build -o /tmp/lore-graph-serve ./cmd/codeindex && /tmp/lore-graph-serve serve "$(pwd)" --addr 127.0.0.1:7799
```

Open `http://127.0.0.1:7799`: expect a small package map (not a hairball), click a package → symbols + chip appear inside it, click the chip → the rest appear, click the package label again → collapses. Stop the server.

- [ ] **Step 5: Commit**

```bash
git add web/src/App.tsx internal/webserver/dist
git commit -m "feat(web): overview-first app state — expand/collapse, tail reveal, search auto-expand"
```

---

### Task 6: e2e suite for the new interaction model

**Files:**
- Rewrite: `web/tests/e2e.spec.ts`

**Interfaces:**
- Consumes: `window.__cy` and `window.__layoutDone` (Task 4); testids `graph-canvas`, `legend`, `health`, `palette-input`, `inspector-title`, `neighbor` (existing).

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

// Largest package by symCount — stable target for expand/chip tests.
const biggestPackage = (page: Page) =>
  page.evaluate(() => {
    const pkgs = window.__cy.$('node[kind = "package"]')
    let best: any = null
    pkgs.forEach((n: any) => {
      if (!best || n.data('symCount') > best.data('symCount')) best = n
    })
    return { label: best.data('label') as string, symCount: best.data('symCount') as number }
  })

const tapNode = (page: Page, id: string) =>
  page.evaluate((nid) => window.__cy.$id(nid).emit('tap'), id)

test('landing is an overview: packages + lore, zero symbols', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('graph-canvas')).toBeVisible()
  await ready(page)
  await expect(page.getByTestId('health')).toContainText('packages')
  expect(await count(page, 'node[kind = "package"]')).toBeGreaterThan(10)
  expect(await count(page, 'node[kind = "symbol"]')).toBe(0)
  // Bundled edges connect the packages.
  expect(await count(page, 'edge[?bundled]')).toBeGreaterThan(5)
})

test('expand shows top-12 + chip; chip reveals the tail; collapse re-bundles', async ({ page }) => {
  await page.goto('/')
  await ready(page)
  const pkg = await biggestPackage(page)
  expect(pkg.symCount).toBeGreaterThan(12)

  await tapNode(page, `pkg:${pkg.label}`)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBe(12)
  expect(await count(page, 'node[kind = "chip"]')).toBe(1)
  const chipLabel = await page.evaluate(() => window.__cy.$('node[kind = "chip"]').data('label'))
  expect(chipLabel).toBe(`+${pkg.symCount - 12} more`)

  await tapNode(page, `chip:${pkg.label}`)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBe(pkg.symCount)
  expect(await count(page, 'node[kind = "chip"]')).toBe(0)

  await tapNode(page, `pkg:${pkg.label}`)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBe(0)
})

test('expansion never moves existing nodes', async ({ page }) => {
  await page.goto('/')
  await ready(page)
  const before = await page.evaluate(() => {
    const o: Record<string, { x: number; y: number }> = {}
    window.__cy.$('node[kind = "package"]').forEach((n: any) => {
      o[n.id()] = { x: Math.round(n.position('x')), y: Math.round(n.position('y')) }
    })
    return o
  })
  const pkg = await biggestPackage(page)
  await tapNode(page, `pkg:${pkg.label}`)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBeGreaterThan(0)
  const after = await page.evaluate(() => {
    const o: Record<string, { x: number; y: number }> = {}
    window.__cy.$('node[kind = "package"]').forEach((n: any) => {
      o[n.id()] = { x: Math.round(n.position('x')), y: Math.round(n.position('y')) }
    })
    return o
  })
  // Every collapsed package sits exactly where it was (the expanded one may
  // grow as a compound, so skip it).
  for (const id of Object.keys(before)) {
    if (id === `pkg:${pkg.label}`) continue
    expect(after[id], id).toEqual(before[id])
  }
})

test('layout is deterministic across reloads', async ({ page }) => {
  const positions = async () => {
    await page.goto('/')
    await ready(page)
    return page.evaluate(() => {
      const o: Record<string, { x: number; y: number }> = {}
      window.__cy.$('node[kind = "package"]').forEach((n: any) => {
        o[n.id()] = { x: Math.round(n.position('x')), y: Math.round(n.position('y')) }
      })
      return o
    })
  }
  const first = await positions()
  const second = await positions()
  expect(second).toEqual(first)
})

test('search auto-expands the package and selects the symbol', async ({ page }) => {
  await page.goto('/')
  await ready(page)
  await page.getByTestId('palette-input').fill('Neighborhood')
  await page.getByTestId('palette-input').press('Enter')

  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood')
  await expect.poll(() => count(page, 'node.sel')).toBe(1)
  // Its package is now an expanded compound holding the symbol.
  const inPkg = await page.evaluate(() => {
    const n = window.__cy.$('node.sel')
    return n.parent().data('kind') === 'package'
  })
  expect(inPkg).toBe(true)
  await expect(page.getByTestId('neighbor').first()).toBeVisible()
})

test('deep link ?focus= reveals and selects on load', async ({ page }) => {
  await page.goto('/?focus=SymbolNeighborhood')
  await ready(page)
  await expect(page.getByTestId('inspector-title')).toHaveText('SymbolNeighborhood', { timeout: 15000 })
  await expect.poll(() => count(page, 'node.sel')).toBe(1)
})

test('clicking an inspector neighbor changes selection', async ({ page }) => {
  await page.goto('/?focus=Neighborhood')
  await ready(page)
  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood', { timeout: 15000 })
  const neighbor = page.getByTestId('neighbor').first()
  const label = (await neighbor.locator('.neighbor-label').textContent())?.trim()
  await neighbor.click()
  if (label) {
    await expect(page.getByTestId('inspector-title')).toHaveText(label)
  }
  await expect.poll(() => count(page, 'node.sel')).toBe(1)
})
```

- [ ] **Step 2: Build the SPA so the embedded dist is current, then run e2e**

Run:

```bash
cd web && npm run build && npx playwright test
```

Expected: all 7 tests PASS. Known judgement calls if one fails:
- Determinism test failing by >0px on a handful of nodes → fcose is calling `Math.random` outside the seeded window (e.g. async phases). Fix by keeping the seeded `Math.random` installed from `layout.run()` until `layoutstop` fires (that is what Task 4's code does — verify the restore isn't happening early), not by loosening the assertion.
- Top-12 assertion failing with 13 → `?focus=` leaked an extra into `extras` from a prior test's URL; tests use plain `/` so this indicates state pollution in the app, not the test.

- [ ] **Step 3: Commit (includes rebuilt dist)**

```bash
git add web/tests/e2e.spec.ts internal/webserver/dist
git commit -m "test(web): e2e for overview-first model — expand, chip, determinism, search reveal"
```

---

### Task 7: Full verification + lore close-out

**Files:**
- Modify: none (verification); lore db via CLI.

- [ ] **Step 1: Run everything**

```bash
cd web && npm test && npm run build && npx playwright test
cd .. && go build ./... && go test ./internal/webserver/ ./internal/readmodel/
```

Expected: all green (Go side untouched — this confirms it).

- [ ] **Step 2: Record completion in lore**

```bash
./codeindex lore /Users/ethanhinson/codeindex add note --title "Graph UI smoothness slice landed on worktree-lore-graph-ui" --body "Overview-first rendering implemented per dec-01KYV5FRVTW6ACVNW4MQ8QGZ5V: aggregate.ts view model (ranked reveal, visibility-driven edge bundling), deterministic seeded fcose overview layout with seeded Math.random, phyllotaxis child placement (no relayout on expand), hover diffing, LOD edge hiding. Full graph is no longer rendered raw anywhere."
```

(Run from the main checkout root where the `codeindex` binary lives, or use `go run ./cmd/codeindex` from the worktree.)

- [ ] **Step 3: Final commit if anything is dirty**

```bash
git status --short   # expect clean; commit any stragglers with an accurate message
```

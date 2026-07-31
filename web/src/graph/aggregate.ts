// Pure aggregation over the full graph payload: package ranking and the
// visible view model. No cytoscape or react imports — unit-testable as-is.
import type { Graph, GraphEdge, GraphNode } from '../types'

export const UNGROUPED = '(ungrouped)'

export function pkgId(name: string): string {
  return `pkg:${name}`
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
    } else if (e.kind === 'anchors') {
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

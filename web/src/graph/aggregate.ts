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
  const rankOf = new Map<string, number>()
  for (const ids of index.packages.values()) {
    ids.forEach((id, rank) => rankOf.set(id, rank))
  }
  const nodes: VisNode[] = []
  for (const [pkg, ids] of index.packages) {
    nodes.push({ id: pkgId(pkg), kind: 'package', label: pkg, degree: 0, symCount: ids.length })
  }
  for (const n of index.nodes) {
    if (n.kind === 'symbol') {
      if (!vis.has(n.id)) continue
      const pkg = index.pkgOf.get(n.id) as string
      const rank = rankOf.get(n.id) ?? -1
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
    const n = index.nodeById.get(id)
    if (!n) return id
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

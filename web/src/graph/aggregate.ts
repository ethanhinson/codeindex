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

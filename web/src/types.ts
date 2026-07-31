// Mirrors the Go readmodel wire types served by /api/graph.

export type NodeKind = 'symbol' | 'decision' | 'item' | 'note' | 'path'
export type EdgeKind = 'calls' | 'anchors' | 'blocked_by'

export interface GraphNode {
  id: string
  kind: NodeKind
  label: string
  file?: string
  line?: number
  signature?: string
  status?: string
  priority?: string
}

export interface GraphEdge {
  source: string
  target: string
  kind: EdgeKind
  conf?: string
}

export interface Graph {
  focus: string
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface Health {
  status: string
  version: string
  root: string
}

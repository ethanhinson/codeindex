import { useEffect, useState } from 'react'
import { getFullGraph } from './api'
import type { GraphEdge, GraphNode } from './types'

export interface FullGraphState {
  nodes: GraphNode[]
  edges: GraphEdge[]
  loading: boolean
  error: string | null
}

// Loads the entire project graph (symbols + call edges + lore overlay) once.
export function useFullGraph(): FullGraphState {
  const [nodes, setNodes] = useState<GraphNode[]>([])
  const [edges, setEdges] = useState<GraphEdge[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let live = true
    getFullGraph()
      .then((g) => {
        if (!live) return
        setNodes(g.nodes)
        setEdges(g.edges)
      })
      .catch((e) => live && setError(e instanceof Error ? e.message : String(e)))
      .finally(() => live && setLoading(false))
    return () => {
      live = false
    }
  }, [])

  return { nodes, edges, loading, error }
}

// resolveFocus finds the best node id for a search query: exact id, then exact
// label, then case-insensitive label substring.
export function resolveFocus(nodes: GraphNode[], query: string): string | null {
  const q = query.trim()
  if (!q) return null
  if (nodes.some((n) => n.id === q)) return q
  const exact = nodes.find((n) => n.label === q)
  if (exact) return exact.id
  const lc = q.toLowerCase()
  const sub = nodes.find((n) => n.label.toLowerCase().includes(lc))
  return sub ? sub.id : null
}

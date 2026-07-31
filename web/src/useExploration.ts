import { useCallback, useState } from 'react'
import { getGraph } from './api'
import type { GraphEdge, GraphNode } from './types'

export interface Crumb {
  id: string
  label: string
}

export interface Exploration {
  nodes: GraphNode[]
  edges: GraphEdge[]
  focus: string | null
  crumbs: Crumb[]
  loading: boolean
  error: string | null
  /** Replace the canvas with a fresh neighborhood around id (jump). */
  focusOn: (id: string) => Promise<void>
  /** Merge id's neighborhood into the current canvas (dig deeper). */
  expand: (id: string) => Promise<void>
}

function edgeKey(e: GraphEdge): string {
  return `${e.source}|${e.target}|${e.kind}`
}

function labelFor(nodes: GraphNode[], id: string): string {
  return nodes.find((n) => n.id === id)?.label ?? id
}

export function useExploration(): Exploration {
  const [nodes, setNodes] = useState<GraphNode[]>([])
  const [edges, setEdges] = useState<GraphEdge[]>([])
  const [focus, setFocus] = useState<string | null>(null)
  const [crumbs, setCrumbs] = useState<Crumb[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const navigate = useCallback(async (id: string, mode: 'replace' | 'merge') => {
    setLoading(true)
    setError(null)
    try {
      const g = await getGraph(id)
      let mergedNodes: GraphNode[] = []
      if (mode === 'replace') {
        mergedNodes = g.nodes
        setNodes(g.nodes)
        setEdges(g.edges)
      } else {
        setNodes((prev) => {
          const byId = new Map(prev.map((n) => [n.id, n]))
          for (const n of g.nodes) byId.set(n.id, n)
          mergedNodes = [...byId.values()]
          return mergedNodes
        })
        setEdges((prev) => {
          const byKey = new Map(prev.map((e) => [edgeKey(e), e]))
          for (const e of g.edges) byKey.set(edgeKey(e), e)
          return [...byKey.values()]
        })
      }
      setFocus(g.focus)
      const label = labelFor(g.nodes, g.focus)
      setCrumbs((prev) => {
        if (prev.length && prev[prev.length - 1].id === g.focus) return prev
        return [...prev, { id: g.focus, label }]
      })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  const focusOn = useCallback((id: string) => navigate(id, 'replace'), [navigate])
  const expand = useCallback((id: string) => navigate(id, 'merge'), [navigate])

  return { nodes, edges, focus, crumbs, loading, error, focusOn, expand }
}

import type { Graph, Health } from './types'

export async function getGraph(focus: string): Promise<Graph> {
  const r = await fetch(`/api/graph?focus=${encodeURIComponent(focus)}`)
  if (!r.ok) {
    const body = await r.text()
    throw new Error(`graph ${r.status}: ${body.trim()}`)
  }
  return (await r.json()) as Graph
}

export async function getHealth(): Promise<Health> {
  const r = await fetch('/api/health')
  if (!r.ok) throw new Error(`health ${r.status}`)
  return (await r.json()) as Health
}

export async function getFullGraph(): Promise<Graph> {
  const r = await fetch('/api/graph/full')
  if (!r.ok) throw new Error(`full graph ${r.status}`)
  return (await r.json()) as Graph
}

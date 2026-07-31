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

export interface SeedFocus {
  id: string
  label: string
  kind: string
}

export async function getSeed(): Promise<SeedFocus[]> {
  const r = await fetch('/api/seed')
  if (!r.ok) throw new Error(`seed ${r.status}`)
  const j = (await r.json()) as { focuses?: SeedFocus[] }
  return j.focuses ?? []
}

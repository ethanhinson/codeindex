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

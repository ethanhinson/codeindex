import { describe, expect, test } from 'vitest'
import {
  buildIndex, UNGROUPED, LABEL_TOP,
  overviewVM, focusVM, earnedLabels, loreRailModel,
} from './aggregate'
import type { Graph, GraphEdge, GraphNode } from '../types'

export function sym(id: string, group?: string, label?: string): GraphNode {
  return { id, kind: 'symbol', label: label ?? id, group }
}
export function lore(id: string, kind: 'decision' | 'item' | 'note', label?: string): GraphNode {
  return { id, kind, label: label ?? id }
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
  })

  test('groups symbols by package; missing group falls into (ungrouped)', () => {
    const ix = buildIndex(g([sym('a', 'p'), sym('b', 'p'), sym('c')], []))
    expect(ix.packages.get('p')).toEqual(['a', 'b'])
    expect(ix.packages.get(UNGROUPED)).toEqual(['c'])
  })

  test('ranks symbols in a package by degree desc, id asc tiebreak', () => {
    const nodes = [sym('lo', 'p'), sym('hub', 'p'), sym('mid', 'p'), sym('x', 'q')]
    const edges = [call('hub', 'x'), call('hub', 'mid'), call('mid', 'x')]
    const ix = buildIndex(g(nodes, edges))
    expect(ix.packages.get('p')).toEqual(['hub', 'mid', 'lo'])
  })
})

describe('overviewVM', () => {
  test('packages only — no symbols, no lore nodes', () => {
    const ix = buildIndex(
      g([sym('a', 'p'), sym('b', 'q'), lore('dec-1', 'decision')], [call('a', 'b')]),
    )
    const vm = overviewVM(ix)
    expect(vm.nodes.map((n) => n.id).sort()).toEqual(['pkg:p', 'pkg:q'])
    const p = vm.nodes.find((n) => n.id === 'pkg:p')!
    expect(p).toMatchObject({ kind: 'package', role: 'map', symCount: 1 })
  })

  test('bundles cross-package calls with counts; drops intra and lore edges', () => {
    const ix = buildIndex(
      g(
        [sym('a', 'p'), sym('b', 'p'), sym('c', 'q'), lore('dec-1', 'decision')],
        [call('a', 'c'), call('b', 'c'), call('a', 'b'), { source: 'dec-1', target: 'a', kind: 'anchors' }],
      ),
    )
    const vm = overviewVM(ix)
    expect(vm.edges).toHaveLength(1)
    expect(vm.edges[0]).toMatchObject({ source: 'pkg:p', target: 'pkg:q', count: 2, bundled: true, kind: 'calls' })
  })
})

describe('focusVM', () => {
  const fixture = () =>
    buildIndex(
      g(
        [
          sym('a', 'p'), sym('b', 'p'), sym('c', 'p'),
          sym('x', 'q'), sym('y', 'r'), sym('z', 'zed'),
          lore('dec-1', 'decision'),
        ],
        [
          call('a', 'b'), call('a', 'b'), call('b', 'c'),      // intra p (a->b twice)
          call('a', 'x'), call('x', 'b'), call('c', 'y'),      // cross to q, r
          { source: 'dec-1', target: 'a', kind: 'anchors' },   // lore edge: excluded
        ],
      ),
    )

  test('all focus symbols + satellites for connected packages only', () => {
    const vm = focusVM(fixture(), 'p')
    const symbols = vm.nodes.filter((n) => n.kind === 'symbol')
    expect(symbols.map((n) => n.id).sort()).toEqual(['a', 'b', 'c'])
    const sats = vm.nodes.filter((n) => n.role === 'satellite')
    expect(sats.map((n) => n.id).sort()).toEqual(['pkg:q', 'pkg:r'])  // zed has no edge to p
    expect(vm.nodes.some((n) => n.id === 'pkg:zed')).toBe(false)
    expect(vm.nodes.some((n) => n.id === 'dec-1')).toBe(false)
  })

  test('intra edges concrete and merged by pair; satellite edges bundled per (satellite, symbol)', () => {
    const vm = focusVM(fixture(), 'p')
    const intra = vm.edges.filter((e) => !e.bundled)
    expect(intra).toHaveLength(2) // a->b (count 2), b->c
    expect(intra.find((e) => e.source === 'a' && e.target === 'b')!.count).toBe(2)
    const satEdges = vm.edges.filter((e) => e.bundled)
    const keys = satEdges.map((e) => `${e.source}->${e.target}`).sort()
    expect(keys).toEqual(['a->pkg:q', 'c->pkg:r', 'pkg:q->b'])  // direction preserved
  })

  test('unknown package yields empty view model', () => {
    const vm = focusVM(fixture(), 'nope')
    expect(vm.nodes).toHaveLength(0)
    expect(vm.edges).toHaveLength(0)
  })
})

describe('earnedLabels', () => {
  const bigVM = () => {
    const syms = Array.from({ length: 20 }, (_, i) => sym(`s${String(i).padStart(2, '0')}`, 'p'))
    const hub = sym('hub', 'q')
    const edges = syms.flatMap((s, i) => Array.from({ length: 20 - i }, () => call(s.id, 'hub')))
    return focusVM(buildIndex(g([...syms, hub], edges)), 'p')
  }

  test('top LABEL_TOP symbols by degree + all package nodes', () => {
    const set = earnedLabels(bigVM(), null, false)
    const vm = bigVM()
    const labeledSyms = vm.nodes.filter((n) => n.kind === 'symbol' && set.has(n.id))
    expect(labeledSyms).toHaveLength(LABEL_TOP)
    expect(set.has('s00')).toBe(true)   // highest degree
    expect(set.has('s19')).toBe(false)  // lowest degree
    expect(set.has('pkg:q')).toBe(true) // satellites always labeled
  })

  test('selected is always labeled; zoomNear labels everything', () => {
    const vm = bigVM()
    expect(earnedLabels(vm, 's19', false).has('s19')).toBe(true)
    const near = earnedLabels(vm, null, true)
    expect(near.size).toBe(vm.nodes.length)
  })
})

describe('loreRailModel', () => {
  test('groups by kind, session notes split out, recency = id desc', () => {
    const ix = buildIndex(
      g(
        [
          sym('a', 'p'), sym('b', 'q'),
          lore('dec-02', 'decision', 'Newer decision'), lore('dec-01', 'decision', 'Older decision'),
          lore('itm-01', 'item', 'An item'),
          lore('note-01', 'note', 'Session 2026-07-31 — stuff'), lore('note-02', 'note', 'A real note'),
        ],
        [
          { source: 'dec-01', target: 'a', kind: 'anchors' },
          { source: 'dec-01', target: 'b', kind: 'anchors' },
          { source: 'itm-01', target: 'itm-99', kind: 'blocked_by' }, // unknown target: dropped by buildIndex
          { source: 'itm-01', target: 'dec-01', kind: 'blocked_by' },
        ],
      ),
    )
    const rail = loreRailModel(ix)
    expect(rail.decisions.map((r) => r.id)).toEqual(['dec-02', 'dec-01'])  // id desc
    expect(rail.decisions[1].pkgs).toEqual(['p', 'q'])
    expect(rail.items[0].blockedBy).toEqual(['dec-01'])
    expect(rail.notes.map((r) => r.id)).toEqual(['note-02'])
    expect(rail.sessions.map((r) => r.id)).toEqual(['note-01'])
    expect(rail.sessions[0].session).toBe(true)
  })

  test('empty graph yields empty groups', () => {
    const rail = loreRailModel(buildIndex(g([], [])))
    expect(rail.decisions).toEqual([])
    expect(rail.sessions).toEqual([])
  })
})

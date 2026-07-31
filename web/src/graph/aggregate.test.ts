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

import { viewModel, visibleSymbols, type ViewState } from './aggregate'

function state(p: Partial<ViewState> = {}): ViewState {
  return { expanded: new Set(), tails: new Set(), extras: new Set(), ...p }
}

describe('viewModel', () => {
  test('overview: one package node per group, sized; lore individual; no symbols', () => {
    const ix = buildIndex(
      g([sym('a', 'p'), sym('b', 'p'), sym('c', 'q'), lore('d1', 'decision')], [call('a', 'c')]),
    )
    const vm = viewModel(ix, state())
    const ids = vm.nodes.map((n) => n.id).sort()
    expect(ids).toEqual(['d1', 'pkg:p', 'pkg:q'])
    const p = vm.nodes.find((n) => n.id === 'pkg:p')!
    expect(p.kind).toBe('package')
    expect(p.symCount).toBe(2)
  })

  test('cross-package calls bundle with counts; intra-package hidden edges drop', () => {
    const ix = buildIndex(
      g(
        [sym('a', 'p'), sym('b', 'p'), sym('c', 'q'), sym('d', 'q')],
        [call('a', 'c'), call('b', 'c'), call('a', 'b'), call('c', 'd')],
      ),
    )
    const vm = viewModel(ix, state())
    expect(vm.edges).toHaveLength(1)
    expect(vm.edges[0]).toMatchObject({ source: 'pkg:p', target: 'pkg:q', count: 2, bundled: true })
  })

  test('expanded package under TOP_N: all symbols visible, no chip', () => {
    const ix = buildIndex(g([sym('a', 'p'), sym('b', 'p')], []))
    const vm = viewModel(ix, state({ expanded: new Set(['p']) }))
    const kinds = new Map(vm.nodes.map((n) => [n.id, n.kind]))
    expect(kinds.get('a')).toBe('symbol')
    expect(kinds.get('b')).toBe('symbol')
    expect(vm.nodes.some((n) => n.kind === 'chip')).toBe(false)
    expect(vm.nodes.find((n) => n.id === 'a')!.parent).toBe('pkg:p')
  })

  test('expanded big package: top-12 by rank + chip "+N more"; tail reveals all', () => {
    const syms = Array.from({ length: 20 }, (_, i) => sym(`s${String(i).padStart(2, '0')}`, 'p'))
    // s00 gets highest degree, s01 next, etc. via a chain of hub edges
    const hub = sym('hub', 'q')
    const edges = syms.flatMap((s, i) => Array.from({ length: 20 - i }, () => call(s.id, 'hub')))
    const ix = buildIndex(g([...syms, hub], edges))
    const vm = viewModel(ix, state({ expanded: new Set(['p']) }))
    const visSyms = vm.nodes.filter((n) => n.kind === 'symbol' && n.pkg === 'p')
    expect(visSyms).toHaveLength(TOP_N)
    expect(visSyms.map((n) => n.id)).toContain('s00')
    expect(visSyms.map((n) => n.id)).not.toContain('s19')
    const chip = vm.nodes.find((n) => n.kind === 'chip')!
    expect(chip).toMatchObject({ id: 'chip:p', label: '+8 more', parent: 'pkg:p' })

    const all = viewModel(ix, state({ expanded: new Set(['p']), tails: new Set(['p']) }))
    expect(all.nodes.filter((n) => n.kind === 'symbol' && n.pkg === 'p')).toHaveLength(20)
    expect(all.nodes.some((n) => n.kind === 'chip')).toBe(false)
  })

  test('extras force-reveal a tail symbol; chip count shrinks by one', () => {
    const syms = Array.from({ length: 15 }, (_, i) => sym(`s${String(i).padStart(2, '0')}`, 'p'))
    const hub = sym('hub', 'q')
    const edges = syms.flatMap((s, i) => Array.from({ length: 15 - i }, () => call(s.id, 'hub')))
    const ix = buildIndex(g([...syms, hub], edges))
    const vm = viewModel(ix, state({ expanded: new Set(['p']), extras: new Set(['s14']) }))
    const visIds = vm.nodes.filter((n) => n.kind === 'symbol' && n.pkg === 'p').map((n) => n.id)
    expect(visIds).toContain('s14')
    expect(visIds).toHaveLength(TOP_N + 1)
    expect(vm.nodes.find((n) => n.kind === 'chip')!.label).toBe('+2 more')
  })

  test('edges between visible symbols are concrete; visible-to-hidden bundles to package', () => {
    const ix = buildIndex(
      g([sym('a', 'p'), sym('c', 'q'), sym('d', 'q')], [call('a', 'c'), call('a', 'd')]),
    )
    // q expanded but only c revealed via extras trick: use tails to show all of q instead
    const vm = viewModel(ix, state({ expanded: new Set(['p', 'q']), tails: new Set(['p', 'q']) }))
    const concrete = vm.edges.filter((e) => !e.bundled)
    expect(concrete).toHaveLength(2)
    // now collapse q: a's two calls bundle into one a->pkg:q edge
    const vm2 = viewModel(ix, state({ expanded: new Set(['p']), tails: new Set(['p']) }))
    expect(vm2.edges).toHaveLength(1)
    expect(vm2.edges[0]).toMatchObject({ source: 'a', target: 'pkg:q', count: 2, bundled: true })
  })

  test('lore edge to hidden symbol bundles to its package, keeps kind', () => {
    const ix = buildIndex(
      g([sym('a', 'p'), lore('d1', 'decision')], [{ source: 'd1', target: 'a', kind: 'anchors' }]),
    )
    const vm = viewModel(ix, state())
    expect(vm.edges[0]).toMatchObject({ source: 'd1', target: 'pkg:p', kind: 'anchors', bundled: true })
  })

  test('visibleSymbols honors rank cutoff', () => {
    const ix = buildIndex(g([sym('a', 'p'), sym('b', 'p')], [call('a', 'b')]))
    expect(visibleSymbols(ix, state())).toEqual(new Set())
    expect(visibleSymbols(ix, state({ expanded: new Set(['p']) }))).toEqual(new Set(['a', 'b']))
  })
})

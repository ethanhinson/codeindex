import { useCallback, useEffect, useMemo, useState } from 'react'
import { getHealth } from './api'
import type { Health } from './types'
import type { Suggestion } from './CommandPalette'
import { useFullGraph, resolveFocus } from './useFullGraph'
import { buildIndex, viewModel } from './graph/aggregate'
import { GraphCanvas } from './graph/GraphCanvas'
import { CommandPalette } from './CommandPalette'
import { Inspector } from './Inspector'
import { EDGE_COLORS, EDGE_STYLES, NODE_COLORS } from './graph/style'

export default function App() {
  const { nodes, edges, loading, error } = useFullGraph()
  const [health, setHealth] = useState<Health | null>(null)
  const [selected, setSelected] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [tails, setTails] = useState<Set<string>>(new Set())
  const [extras, setExtras] = useState<Set<string>>(new Set())

  const index = useMemo(() => buildIndex({ focus: '', nodes, edges }), [nodes, edges])
  const vm = useMemo(
    () => viewModel(index, { expanded, tails, extras }),
    [index, expanded, tails, extras],
  )

  useEffect(() => {
    getHealth().then(setHealth).catch(() => setHealth(null))
  }, [])

  // Surface data oddities once: edges referencing unknown node ids are
  // dropped by buildIndex, not rendered.
  useEffect(() => {
    if (index.dropped > 0) {
      console.warn(`graph: dropped ${index.dropped} edge(s) with unknown endpoints`)
    }
  }, [index])

  // Selecting a symbol always reveals it: expand its package and force it
  // past the top-N cutoff if it lives in the long tail.
  const selectNode = useCallback(
    (id: string | null) => {
      if (id) {
        const pkg = index.pkgOf.get(id)
        if (pkg) {
          setExpanded((prev) => (prev.has(pkg) ? prev : new Set(prev).add(pkg)))
          setExtras((prev) => (prev.has(id) ? prev : new Set(prev).add(id)))
        }
      }
      setSelected(id)
    },
    [index],
  )

  const togglePackage = useCallback(
    (pkg: string) => {
      setExpanded((prev) => {
        const next = new Set(prev)
        if (next.has(pkg)) {
          next.delete(pkg)
          setTails((t) => {
            const nt = new Set(t)
            nt.delete(pkg)
            return nt
          })
          setExtras((x) => {
            const nx = new Set([...x].filter((id) => index.pkgOf.get(id) !== pkg))
            return nx.size === x.size ? x : nx
          })
          setSelected((s) => (s && index.pkgOf.get(s) === pkg ? null : s))
        } else {
          next.add(pkg)
        }
        return next
      })
    },
    [index],
  )

  const revealTail = useCallback((pkg: string) => {
    setTails((prev) => (prev.has(pkg) ? prev : new Set(prev).add(pkg)))
  }, [])

  // Once the graph is in, honor ?focus= (id or label) for deep links.
  useEffect(() => {
    if (nodes.length === 0) return
    const param = new URLSearchParams(window.location.search).get('focus')
    if (param) selectNode(resolveFocus(nodes, param))
  }, [nodes, selectNode])

  const selectedNode = useMemo(
    () => (selected ? nodes.find((n) => n.id === selected) ?? null : null),
    [selected, nodes],
  )

  // Suggestions: the highest-degree symbols — the hubs worth starting from.
  const suggestions = useMemo<Suggestion[]>(
    () =>
      nodes
        .filter((n) => n.kind === 'symbol')
        .sort((a, b) => (index.degree.get(b.id) ?? 0) - (index.degree.get(a.id) ?? 0))
        .slice(0, 4)
        .map((n) => ({ id: n.id, label: n.label })),
    [nodes, index],
  )

  const symbolCount = nodes.filter((n) => n.kind === 'symbol').length
  const loreCount = nodes.length - symbolCount

  function onSearch(query: string) {
    selectNode(resolveFocus(nodes, query))
  }

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          codeindex <span className="brand-sub">· lore graph</span>
        </div>
        <CommandPalette onSubmit={onSearch} suggestions={suggestions} />
        <div className="status" data-testid="health">
          {loading
            ? 'loading graph…'
            : `${index.packages.size} packages · ${symbolCount} symbols · ${loreCount} lore${health ? ` · ● ${health.version}` : ''}`}
        </div>
      </header>

      <main className="stage">
        <div className="canvas-wrap">
          {error && <div className="error-banner" data-testid="error">{error}</div>}
          {loading && <div className="empty-hint" data-testid="loading-hint">building the graph…</div>}
          <GraphCanvas
            vm={vm}
            selected={selected}
            onSelect={selectNode}
            onTogglePackage={togglePackage}
            onRevealTail={revealTail}
          />
          <Legend />
        </div>
        <Inspector node={selectedNode} nodes={nodes} edges={edges} onOpen={selectNode} />
      </main>
    </div>
  )
}

function Legend() {
  const nodeKinds = Object.keys(NODE_COLORS) as (keyof typeof NODE_COLORS)[]
  const edgeKinds = Object.keys(EDGE_COLORS) as (keyof typeof EDGE_COLORS)[]
  return (
    <div className="legend" data-testid="legend">
      <div className="legend-group">
        {nodeKinds.map((k) => (
          <span key={k} className="legend-item">
            <span className="kind-dot sm" style={{ background: NODE_COLORS[k] }} />
            {k}
          </span>
        ))}
      </div>
      <div className="legend-group">
        {edgeKinds.map((k) => (
          <span key={k} className="legend-item">
            <span
              className="edge-swatch"
              style={{ borderBottom: `2px ${EDGE_STYLES[k]} ${EDGE_COLORS[k]}` }}
            />
            {k}
          </span>
        ))}
      </div>
    </div>
  )
}

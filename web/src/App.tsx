import { useEffect, useMemo, useState } from 'react'
import { getHealth } from './api'
import type { Health } from './types'
import type { Suggestion } from './CommandPalette'
import { useFullGraph, resolveFocus } from './useFullGraph'
import { GraphCanvas } from './graph/GraphCanvas'
import { CommandPalette } from './CommandPalette'
import { Inspector } from './Inspector'
import { EDGE_COLORS, EDGE_STYLES, NODE_COLORS } from './graph/style'

export default function App() {
  const { nodes, edges, loading, error } = useFullGraph()
  const [health, setHealth] = useState<Health | null>(null)
  const [selected, setSelected] = useState<string | null>(null)

  useEffect(() => {
    getHealth().then(setHealth).catch(() => setHealth(null))
  }, [])

  // Once the graph is in, honor ?focus= (id or label) for deep links.
  useEffect(() => {
    if (nodes.length === 0) return
    const param = new URLSearchParams(window.location.search).get('focus')
    if (param) setSelected(resolveFocus(nodes, param))
  }, [nodes])

  const selectedNode = useMemo(
    () => (selected ? nodes.find((n) => n.id === selected) ?? null : null),
    [selected, nodes],
  )

  // Suggestions: the highest-degree symbols — the hubs worth starting from.
  const suggestions = useMemo<Suggestion[]>(() => {
    const deg = new Map<string, number>()
    for (const e of edges) {
      deg.set(e.source, (deg.get(e.source) ?? 0) + 1)
      deg.set(e.target, (deg.get(e.target) ?? 0) + 1)
    }
    return nodes
      .filter((n) => n.kind === 'symbol')
      .sort((a, b) => (deg.get(b.id) ?? 0) - (deg.get(a.id) ?? 0))
      .slice(0, 4)
      .map((n) => ({ id: n.id, label: n.label }))
  }, [nodes, edges])

  const symbolCount = nodes.filter((n) => n.kind === 'symbol').length
  const loreCount = nodes.length - symbolCount

  function onSearch(query: string) {
    setSelected(resolveFocus(nodes, query))
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
            : `${symbolCount} symbols · ${loreCount} lore${health ? ` · ● ${health.version}` : ''}`}
        </div>
      </header>

      <main className="stage">
        <div className="canvas-wrap">
          {error && <div className="error-banner" data-testid="error">{error}</div>}
          {loading && <div className="empty-hint" data-testid="loading-hint">building the graph…</div>}
          <GraphCanvas nodes={nodes} edges={edges} selected={selected} onSelect={setSelected} />
          <Legend />
        </div>
        <Inspector node={selectedNode} nodes={nodes} edges={edges} onOpen={setSelected} />
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

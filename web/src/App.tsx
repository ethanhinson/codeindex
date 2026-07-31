import { useEffect, useState } from 'react'
import { getHealth } from './api'
import type { Health } from './types'
import { useExploration } from './useExploration'
import { GraphCanvas } from './graph/GraphCanvas'
import { CommandPalette } from './CommandPalette'
import { Inspector } from './Inspector'
import { EDGE_COLORS, EDGE_STYLES, NODE_COLORS } from './graph/style'

const SUGGESTIONS = ['sym:Neighborhood', 'sym:SymbolNeighborhood', 'sym:New']

export default function App() {
  const { nodes, edges, focus, crumbs, loading, error, focusOn, expand } = useExploration()
  const [health, setHealth] = useState<Health | null>(null)

  useEffect(() => {
    getHealth().then(setHealth).catch(() => setHealth(null))
    const param = new URLSearchParams(window.location.search).get('focus')
    if (param) focusOn(param)
  }, [focusOn])

  const selected = focus ? nodes.find((n) => n.id === focus) ?? null : null

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          codeindex <span className="brand-sub">· lore graph</span>
        </div>
        <CommandPalette onSubmit={focusOn} suggestions={SUGGESTIONS} />
        <div className="status" data-testid="health">
          {health ? `● ${health.version}` : '○ offline'}
        </div>
      </header>

      <nav className="breadcrumbs" data-testid="breadcrumbs">
        {crumbs.length === 0 && <span className="crumb-hint">no path yet</span>}
        {crumbs.map((c, i) => (
          <span key={`${c.id}-${i}`} className="crumb-wrap">
            {i > 0 && <span className="crumb-sep">›</span>}
            <button
              className={`crumb ${c.id === focus ? 'active' : ''}`}
              data-testid="crumb"
              onClick={() => focusOn(c.id)}
            >
              {c.label}
            </button>
          </span>
        ))}
        {loading && <span className="loading">…</span>}
      </nav>

      <main className="stage">
        <div className="canvas-wrap">
          {error && <div className="error-banner" data-testid="error">{error}</div>}
          {crumbs.length === 0 && !loading && (
            <div className="empty-hint" data-testid="empty-hint">
              Press <kbd>/</kbd> and enter a focus — try a suggestion above.
            </div>
          )}
          <GraphCanvas nodes={nodes} edges={edges} focus={focus} onNodeTap={expand} />
          <Legend />
        </div>
        <Inspector node={selected} nodes={nodes} edges={edges} onOpen={focusOn} />
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

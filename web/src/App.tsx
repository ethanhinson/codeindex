import { useCallback, useEffect, useMemo, useState } from 'react'
import { getHealth } from './api'
import type { Health } from './types'
import type { Suggestion } from './CommandPalette'
import { useFullGraph, resolveFocus } from './useFullGraph'
import { buildIndex, overviewVM, focusVM, earnedLabels, loreRailModel, pkgId } from './graph/aggregate'
import type { LoreRecord } from './graph/aggregate'
import { GraphCanvas, type View } from './graph/GraphCanvas'
import { LoreRail, type RailGroupKey } from './LoreRail'
import { CommandPalette } from './CommandPalette'
import { Inspector } from './Inspector'
import { EDGE_COLORS, EDGE_STYLES, NODE_COLORS } from './graph/style'

function parseView(search: string, packages: Set<string>): View {
  const pkg = new URLSearchParams(search).get('pkg')
  if (pkg && packages.has(pkg)) return { mode: 'focus', pkg }
  if (pkg) console.warn(`graph: unknown package in URL: ${pkg}`)
  return { mode: 'overview' }
}

function writeUrl(view: View, selected: string | null) {
  const params = new URLSearchParams(window.location.search)
  params.delete('pkg')
  params.delete('focus')
  if (view.mode === 'focus') params.set('pkg', view.pkg)
  if (selected) params.set('focus', selected)
  const qs = params.toString()
  const url = qs ? `?${qs}` : window.location.pathname
  if (window.location.search !== (qs ? `?${qs}` : '')) {
    window.history.pushState(null, '', url)
  }
}

export default function App() {
  const { nodes, edges, loading, error } = useFullGraph()
  const [health, setHealth] = useState<Health | null>(null)
  const [view, setView] = useState<View>({ mode: 'overview' })
  const [selected, setSelected] = useState<string | null>(null)
  const [railVisible, setRailVisible] = useState<Set<RailGroupKey>>(
    new Set(['decisions', 'items', 'notes']),
  )
  const [railShowAll, setRailShowAll] = useState(false)
  const [hotCanvas, setHotCanvas] = useState<Set<string>>(new Set())
  const [hotRail, setHotRail] = useState<Set<string>>(new Set())

  const index = useMemo(() => buildIndex({ focus: '', nodes, edges }), [nodes, edges])
  const rail = useMemo(() => loreRailModel(index), [index])
  const vm = useMemo(
    () => (view.mode === 'focus' ? focusVM(index, view.pkg) : overviewVM(index)),
    [index, view],
  )
  // Near-zoom "label everything" is layered on by the canvas zoom handler;
  // the base earned set is computed with zoomNear=false here.
  const labeled = useMemo(() => earnedLabels(vm, selected, false), [vm, selected])

  useEffect(() => {
    getHealth().then(setHealth).catch(() => setHealth(null))
  }, [])

  useEffect(() => {
    if (index.dropped > 0) {
      console.warn(`graph: dropped ${index.dropped} edge(s) with unknown endpoints`)
    }
  }, [index])

  // Initial URL + back/forward.
  useEffect(() => {
    if (nodes.length === 0) return
    const packages = new Set(index.packages.keys())
    const apply = () => {
      const v = parseView(window.location.search, packages)
      setView(v)
      const focusParam = new URLSearchParams(window.location.search).get('focus')
      if (focusParam) {
        const id = resolveFocus(nodes, focusParam)
        if (id) {
          const pkg = index.pkgOf.get(id)
          if (pkg) setView({ mode: 'focus', pkg })
          setSelected(id)
          return
        }
      }
      setSelected(null)
    }
    apply()
    window.addEventListener('popstate', apply)
    return () => window.removeEventListener('popstate', apply)
  }, [nodes, index])

  const focusPackage = useCallback((pkg: string) => {
    setView((v) => (v.mode === 'focus' && v.pkg === pkg ? v : { mode: 'focus', pkg }))
    setSelected(null)
    setRailShowAll(false)
    writeUrl({ mode: 'focus', pkg }, null)
  }, [])

  const backToOverview = useCallback(() => {
    setView({ mode: 'overview' })
    setSelected(null)
    setRailShowAll(false)
    writeUrl({ mode: 'overview' }, null)
  }, [])
  // (declared before the Esc effect uses it — keep this ordering)

  // Esc leaves focus mode (unless typing in an input). Bound per-view so the
  // handler reads current state without side effects inside a setState updater.
  useEffect(() => {
    if (view.mode !== 'focus') return
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      if ((e.target as HTMLElement)?.tagName === 'INPUT') return
      backToOverview()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [view, backToOverview])


  // Selecting a symbol focuses its package; lore ids just select (Inspector
  // reads from the full node list — lore has no canvas node).
  const selectNode = useCallback(
    (id: string | null) => {
      let nextView: View | null = null
      if (id) {
        const pkg = index.pkgOf.get(id)
        if (pkg) {
          nextView = { mode: 'focus', pkg }
          setView((v) => (v.mode === 'focus' && v.pkg === pkg ? v : nextView as View))
        }
      }
      setSelected(id)
      writeUrl(nextView ?? view, id)
    },
    [index, view],
  )

  // Rail → canvas hover-sync: light up anchored packages (overview) or the
  // anchored symbols + their packages (focus).
  const railHover = useCallback(
    (rec: LoreRecord | null) => {
      if (!rec) {
        setHotCanvas(new Set())
        return
      }
      const ids = new Set<string>()
      for (const p of rec.pkgs) ids.add(pkgId(p))
      for (const e of index.edges) {
        if (e.source === rec.id && index.pkgOf.has(e.target)) ids.add(e.target)
      }
      setHotCanvas(ids)
    },
    [index],
  )

  // Canvas → rail hover-sync: records anchored in the hovered package/symbol.
  const canvasHover = useCallback(
    (id: string | null) => {
      if (!id) {
        setHotRail(new Set())
        return
      }
      const pkg = id.startsWith('pkg:') ? id.slice(4) : index.pkgOf.get(id)
      if (!pkg) {
        setHotRail(new Set())
        return
      }
      const ids = new Set<string>()
      for (const group of [rail.decisions, rail.items, rail.notes, rail.sessions]) {
        for (const r of group) if (r.pkgs.includes(pkg)) ids.add(r.id)
      }
      setHotRail(ids)
    },
    [index, rail],
  )

  const toggleGroup = useCallback((k: RailGroupKey) => {
    setRailVisible((prev) => {
      const next = new Set(prev)
      if (next.has(k)) next.delete(k)
      else next.add(k)
      return next
    })
  }, [])

  const selectedNode = useMemo(
    () => (selected ? nodes.find((n) => n.id === selected) ?? null : null),
    [selected, nodes],
  )

  const suggestions = useMemo<Suggestion[]>(
    () =>
      nodes
        .filter((n) => n.kind === 'symbol')
        .sort((a, b) => (index.degree.get(b.id) ?? 0) - (index.degree.get(a.id) ?? 0))
        .slice(0, 4)
        .map((n) => ({ id: n.id, label: n.label })),
    [nodes, index],
  )

  const symbolCount = useMemo(() => nodes.filter((n) => n.kind === 'symbol').length, [nodes])
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
          {view.mode === 'focus' && (
            <div className="focus-crumb" data-testid="focus-crumb">
              <button className="crumb" data-testid="crumb-overview" onClick={backToOverview}>
                ⟵ overview
              </button>
              <span className="crumb-sep">/</span>
              <b data-testid="crumb-pkg">{view.pkg}</b>
              <span className="crumb-hint">esc</span>
            </div>
          )}
          {view.mode === 'focus' && vm.nodes.length === 0 && !loading && (
            <div className="empty-hint" data-testid="empty-focus">no symbols in this package</div>
          )}
          <GraphCanvas
            vm={vm}
            view={view}
            selected={selected}
            labeled={labeled}
            hot={hotCanvas}
            onSelect={selectNode}
            onFocusPackage={focusPackage}
            onHoverNode={canvasHover}
          />
          <Legend />
        </div>
        <div className="right-panel">
          <Inspector node={selectedNode} nodes={nodes} edges={edges} onOpen={selectNode} />
          <LoreRail
            rail={rail}
            visible={railVisible}
            onToggleGroup={toggleGroup}
            hotIds={hotRail}
            onHover={railHover}
            onOpen={selectNode}
            focusPkg={view.mode === 'focus' ? view.pkg : null}
            showAll={railShowAll}
            onToggleShowAll={() => setRailShowAll((s) => !s)}
          />
        </div>
      </main>
    </div>
  )
}

function Legend() {
  return (
    <div className="legend" data-testid="legend">
      <div className="legend-group">
        <span className="legend-item">
          <span className="kind-dot sm" style={{ background: '#22304a', border: '1px solid #3a4a66' }} />
          package
        </span>
        <span className="legend-item">
          <span className="kind-dot sm" style={{ background: NODE_COLORS.symbol, borderRadius: '50%' }} />
          symbol
        </span>
      </div>
      <div className="legend-group">
        <span className="legend-item">
          <span
            className="edge-swatch"
            style={{ borderBottom: `2px ${EDGE_STYLES.calls} ${EDGE_COLORS.calls}` }}
          />
          calls
        </span>
      </div>
    </div>
  )
}

import { useEffect, useRef } from 'react'
import cytoscape from 'cytoscape'
import type { CollectionReturnValue, Core, NodeSingular } from 'cytoscape'
import fcose from 'cytoscape-fcose'
import type { ViewModel } from './aggregate'
import { motionEnabled, startMotion } from './motion'
import { stylesheet } from './style'

let registered = false
function ensureFcose() {
  if (!registered) {
    cytoscape.use(fcose)
    registered = true
  }
}

const LOD_ZOOM = 0.4
const LABEL_NEAR_ZOOM = 1.1
const GOLDEN_ANGLE = 2.399963229728653
const MORPH_MS = 350
const FOCUS_MORPH_MAX = 150

export type View = { mode: 'overview' } | { mode: 'focus'; pkg: string }

function viewKey(v: View): string {
  return v.mode === 'focus' ? `focus:${v.pkg}` : 'overview'
}

// NOTE: no `declare global` here — the e2e spec declares Window.__cy/__layoutDone
// with different types; a second declaration would conflict if tests ever join
// the tsc program. Use the local cast helper instead.
function win(): { __cy?: Core; __layoutDone?: boolean } {
  return window as unknown as { __cy?: Core; __layoutDone?: boolean }
}

// fcose has internal Math.random calls; pin them to a seeded LCG for the
// duration of the layout so the same input always yields the same map.
function seededLayout(cy: Core, opts: Record<string, unknown>, onDone: () => void): () => void {
  const orig = Math.random
  let s = 42
  Math.random = () => {
    s = (s * 1664525 + 1013904223) >>> 0
    return s / 4294967296
  }
  const restore = () => {
    Math.random = orig
  }
  try {
    const layout = cy.layout({ name: 'fcose', ...opts } as never)
    layout.one('layoutstop', () => {
      restore()
      onDone()
    })
    layout.run()
  } catch (err) {
    restore()
    throw err
  }
  return restore
}

// Anchors are truth: record the current position of every node as its
// deterministic anchor. Motion renders offsets on top; determinism tests
// read ax/ay.
function writeAnchors(cy: Core) {
  cy.batch(() => {
    cy.nodes().forEach((n) => {
      const p = n.position()
      n.data('ax', p.x)
      n.data('ay', p.y)
    })
  })
}

// Diff the desired view model into the live instance. Nodes are added
// without positions (the caller places them); nodes whose id persists
// across views (satellites ⇄ overview packages) keep position but get
// their data refreshed (role/degree change between views).
function applyViewModel(cy: Core, vm: ViewModel): CollectionReturnValue {
  const wantNodes = new Map(vm.nodes.map((n) => [n.id, n]))
  const wantEdges = new Map(vm.edges.map((e) => [e.id, e]))
  let added = cy.collection()
  cy.batch(() => {
    cy.edges().forEach((e) => {
      if (!wantEdges.has(e.id())) e.remove()
    })
    cy.nodes().forEach((n) => {
      const want = wantNodes.get(n.id())
      if (!want) {
        n.remove()
      } else {
        n.data({ ...want, ax: n.data('ax'), ay: n.data('ay') })
      }
    })
    for (const n of vm.nodes) {
      if (cy.$id(n.id).nonempty()) continue
      added = added.union(cy.add({ data: { ...n } }))
    }
    for (const e of vm.edges) {
      if (cy.$id(e.id).empty()) cy.add({ data: { ...e } })
    }
  })
  return added
}

// Deterministic seeds. Overview: packages on a circle in sorted-label order.
function seedOverview(vm: ViewModel): Map<string, { x: number; y: number }> {
  const pos = new Map<string, { x: number; y: number }>()
  const pkgs = vm.nodes.filter((n) => n.kind === 'package').sort((a, b) => (a.label < b.label ? -1 : 1))
  const r = Math.max(220, (pkgs.length * 110) / (2 * Math.PI))
  pkgs.forEach((n, i) => {
    const a = (i / Math.max(1, pkgs.length)) * 2 * Math.PI
    pos.set(n.id, { x: r * Math.cos(a), y: r * Math.sin(a) })
  })
  return pos
}

// Focus: symbols phyllotaxis around `center` in degree-desc/id-asc order,
// satellites on an outer rim in sorted-label order.
function seedFocus(vm: ViewModel, center: { x: number; y: number }) {
  const symbolPos = new Map<string, { x: number; y: number }>()
  const rimPos = new Map<string, { x: number; y: number }>()
  const symbols = vm.nodes
    .filter((n) => n.kind === 'symbol')
    .sort((a, b) => b.degree - a.degree || (a.id < b.id ? -1 : 1))
  symbols.forEach((n, i) => {
    const a = i * GOLDEN_ANGLE
    const r = 26 * Math.sqrt(i + 0.5)
    symbolPos.set(n.id, { x: center.x + r * Math.cos(a), y: center.y + r * Math.sin(a) })
  })
  const rim = Math.max(300, 26 * Math.sqrt(symbols.length) * 2.6)
  const sats = vm.nodes.filter((n) => n.role === 'satellite').sort((a, b) => (a.label < b.label ? -1 : 1))
  sats.forEach((n, i) => {
    const a = (i / Math.max(1, sats.length)) * 2 * Math.PI - Math.PI / 2
    rimPos.set(n.id, { x: center.x + rim * Math.cos(a), y: center.y + rim * Math.sin(a) })
  })
  return { symbolPos, rimPos, rim }
}

function applyLod(cy: Core) {
  const far = cy.zoom() < LOD_ZOOM
  cy.batch(() => {
    const weak = cy.edges('[count = 1]')
    if (far) weak.addClass('lod-hide')
    else weak.removeClass('lod-hide')
  })
}

// Toggle a class so exactly `want` has it, touching only the difference.
function diffClass(cy: Core, cls: string, want: Set<string>, prevRef: { current: Set<string> }) {
  const prev = prevRef.current
  cy.batch(() => {
    for (const id of want) if (!prev.has(id)) cy.$id(id).addClass(cls)
    for (const id of prev) if (!want.has(id)) cy.$id(id).removeClass(cls)
  })
  prevRef.current = new Set(want)
}

interface Props {
  vm: ViewModel
  view: View
  selected: string | null
  labeled: Set<string>
  hot: Set<string>
  onSelect: (id: string | null) => void
  onFocusPackage: (pkg: string) => void
  onHoverNode: (id: string | null) => void
}

export function GraphCanvas({ vm, view, selected, labeled, hot, onSelect, onFocusPackage, onHoverNode }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<Core | null>(null)
  const prevKey = useRef<string | null>(null)
  const lodFar = useRef(false)
  const nearBand = useRef(false)
  const hood = useRef<CollectionReturnValue | null>(null)
  const clearTimer = useRef<number | undefined>(undefined)
  const lastCentered = useRef<string | null>(null)
  const layoutRestore = useRef<(() => void) | null>(null)
  const busy = useRef(false)
  const overviewAnchors = useRef<Map<string, { x: number; y: number }> | null>(null)
  const labeledPrev = useRef(new Set<string>())
  const hotPrev = useRef(new Set<string>())
  const cb = useRef({ onSelect, onFocusPackage, onHoverNode })
  cb.current = { onSelect, onFocusPackage, onHoverNode }
  const instant = !motionEnabled(window.location.search)

  useEffect(() => {
    ensureFcose()
    if (!containerRef.current) return
    const cy = cytoscape({
      container: containerRef.current,
      style: stylesheet(),
      elements: [],
      minZoom: 0.05,
      maxZoom: 4,
      wheelSensitivity: 0.25,
      pixelRatio: 1,
      textureOnViewport: true,
      hideEdgesOnViewport: true,
      motionBlur: false,
    })

    cy.on('tap', 'node', (evt) => {
      const n = evt.target
      if ((n.data('kind') as string) === 'package') cb.current.onFocusPackage(n.data('label') as string)
      else cb.current.onSelect(n.id())
    })
    cy.on('tap', (evt) => {
      if (evt.target === cy) cb.current.onSelect(null)
    })

    // Hover: symmetric-difference class toggling with a debounced clear.
    const clearHover = () => {
      if (!hood.current) return
      hood.current = null
      cy.batch(() => cy.elements().removeClass('dim hl'))
      cb.current.onHoverNode(null)
    }
    cy.on('mouseover', 'node', (evt) => {
      const n = evt.target as NodeSingular
      window.clearTimeout(clearTimer.current)
      const next = n.closedNeighborhood()
      const prev = hood.current
      cy.batch(() => {
        if (!prev) {
          cy.elements().difference(next).addClass('dim')
          cy.nodes('[kind = "package"]').removeClass('dim')
          next.addClass('hl')
        } else {
          const on = next.difference(prev)
          const off = prev.difference(next)
          on.removeClass('dim').addClass('hl')
          off.removeClass('hl').addClass('dim')
          off.nodes('[kind = "package"]').removeClass('dim')
        }
      })
      hood.current = next
      cb.current.onHoverNode(n.id())
    })
    cy.on('mouseout', 'node', () => {
      window.clearTimeout(clearTimer.current)
      clearTimer.current = window.setTimeout(clearHover, 60)
    })

    cy.on('zoom', () => {
      const far = cy.zoom() < LOD_ZOOM
      if (far !== lodFar.current) {
        lodFar.current = far
        applyLod(cy)
      }
      const near = cy.zoom() >= LABEL_NEAR_ZOOM
      if (near !== nearBand.current) {
        nearBand.current = near
        cy.batch(() => {
          if (near) cy.nodes().addClass('labeled')
          else {
            cy.nodes().removeClass('labeled')
            for (const id of labeledPrev.current) cy.$id(id).addClass('labeled')
          }
        })
      }
    })

    win().__cy = cy
    cyRef.current = cy
    const stopMotion = motionEnabled(window.location.search)
      ? startMotion(cy, () => busy.current)
      : () => {}
    return () => {
      window.clearTimeout(clearTimer.current)
      stopMotion()
      layoutRestore.current?.()
      layoutRestore.current = null
      cy.destroy()
      cyRef.current = null
    }
  }, [])

  // Render the view: full layout on view change, incremental diff otherwise.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy || vm.nodes.length === 0) return
    const key = viewKey(view)
    if (key === prevKey.current) {
      applyViewModel(cy, vm)
      applyLod(cy)
      return
    }
    const fromKey = prevKey.current
    prevKey.current = key
    busy.current = true
    win().__layoutDone = false

    const finalize = () => {
      writeAnchors(cy)
      if (view.mode === 'overview') {
        overviewAnchors.current = new Map(
          cy.nodes().map((n) => [n.id(), { x: n.data('ax') as number, y: n.data('ay') as number }]),
        )
      }
      applyLod(cy)
      cy.elements().removeClass('entering')
      busy.current = false
      win().__layoutDone = true
    }
    const fit = () => {
      if (instant) {
        cy.fit(undefined, 50)
        finalize()
      } else {
        cy.animate({ fit: { eles: cy.elements(), padding: 50 } }, { duration: MORPH_MS, complete: finalize })
      }
    }

    if (view.mode === 'overview') {
      applyViewModel(cy, vm)
      const cached = overviewAnchors.current
      if (cached && vm.nodes.every((n) => cached.has(n.id))) {
        // Returning from focus: restore the exact cached map, no relayout.
        cy.batch(() => {
          for (const n of vm.nodes) cy.$id(n.id).position(cached.get(n.id) as { x: number; y: number })
        })
        fit()
      } else {
        const seeds = seedOverview(vm)
        cy.batch(() => {
          for (const [id, p] of seeds) cy.$id(id).position(p)
        })
        layoutRestore.current = seededLayout(
          cy,
          { quality: 'default', animate: false, randomize: false, fit: false, nodeSeparation: 140, idealEdgeLength: 150, nodeRepulsion: 7500, gravity: 0.15, packComponents: false },
          () => {
            layoutRestore.current = null
            fit()
          },
        )
      }
      return
    }

    // Entering focus. Morph origin: the tapped package's current position if
    // it is on canvas (coming from overview), else the canvas origin.
    const pkgNode = cy.$id(`pkg:${view.pkg}`)
    const origin = fromKey === 'overview' && pkgNode.nonempty() ? { ...pkgNode.position() } : { x: 0, y: 0 }
    applyViewModel(cy, vm)
    const { symbolPos, rimPos } = seedFocus(vm, origin)
    cy.batch(() => {
      for (const [id, p] of symbolPos) cy.$id(id).position(p)
      for (const [id, p] of rimPos) cy.$id(id).position(p)
    })
    const fixed = [...rimPos.entries()].map(([nodeId, position]) => ({ nodeId, position }))
    layoutRestore.current = seededLayout(
      cy,
      { quality: 'default', animate: false, randomize: false, fit: false, nodeSeparation: 60, idealEdgeLength: 70, nodeRepulsion: 4500, gravity: 0.25, packComponents: false, fixedNodeConstraint: fixed },
      () => {
        layoutRestore.current = null
        const symbols = cy.nodes('[kind = "symbol"]')
        if (instant || symbols.length > FOCUS_MORPH_MAX || fromKey === null) {
          fit()
          return
        }
        // Morph: snap symbols back to the origin, then animate to targets.
        const targets = new Map(symbols.map((n) => [n.id(), { ...n.position() }]))
        cy.batch(() => {
          symbols.forEach((n) => {
            n.position(origin)
            n.addClass('entering')
          })
        })
        symbols.forEach((n) => {
          n.animate(
            { position: targets.get(n.id()) as { x: number; y: number } },
            { duration: MORPH_MS, easing: 'ease-out' },
          )
          n.removeClass('entering')
        })
        fit()
      },
    )
  }, [vm, view, instant])

  // Earned labels: diff the base set; near-band overrides with all-labeled.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    if (nearBand.current) {
      labeledPrev.current = new Set(labeled)
      return
    }
    diffClass(cy, 'labeled', labeled, labeledPrev)
  }, [labeled, vm])

  // Lore-rail hover-sync highlight.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    diffClass(cy, 'lore-hot', hot, hotPrev)
  }, [hot, vm])

  // Selection: classes unconditionally; center only when the id changes.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    cy.elements().removeClass('sel selhl')
    if (!selected) {
      lastCentered.current = null
      return
    }
    const n = cy.$id(selected)
    if (n.empty()) return
    n.addClass('sel')
    n.closedNeighborhood().addClass('selhl')
    if (selected !== lastCentered.current) {
      lastCentered.current = selected
      cy.animate({ center: { eles: n }, zoom: Math.max(cy.zoom(), 0.8) }, { duration: instant ? 0 : 300 })
    }
  }, [selected, vm, instant])

  return <div ref={containerRef} className="graph-canvas" data-testid="graph-canvas" />
}

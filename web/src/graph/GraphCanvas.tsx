import { useEffect, useRef } from 'react'
import cytoscape from 'cytoscape'
import type { CollectionReturnValue, Core, ElementDefinition } from 'cytoscape'
import fcose from 'cytoscape-fcose'
import type { ViewModel, VisNode } from './aggregate'
import { stylesheet } from './style'

let registered = false
function ensureFcose() {
  if (!registered) {
    cytoscape.use(fcose)
    registered = true
  }
}

const LOD_ZOOM = 0.4
const GOLDEN_ANGLE = 2.399963229728653

// Deterministic seed positions: packages on a circle in sorted-label order
// (path sort keeps sibling dirs adjacent), lore nodes on an outer ring.
function seedPositions(vm: ViewModel): Map<string, { x: number; y: number }> {
  const pos = new Map<string, { x: number; y: number }>()
  const pkgs = vm.nodes.filter((n) => n.kind === 'package').sort((a, b) => (a.label < b.label ? -1 : 1))
  const lore = vm.nodes.filter((n) => n.kind !== 'package' && !n.parent).sort((a, b) => (a.id < b.id ? -1 : 1))
  const r = Math.max(220, (pkgs.length * 95) / (2 * Math.PI))
  pkgs.forEach((n, i) => {
    const a = (i / Math.max(1, pkgs.length)) * 2 * Math.PI
    pos.set(n.id, { x: r * Math.cos(a), y: r * Math.sin(a) })
  })
  lore.forEach((n, i) => {
    const a = (i / Math.max(1, lore.length)) * 2 * Math.PI + 0.13
    pos.set(n.id, { x: 1.35 * r * Math.cos(a), y: 1.35 * r * Math.sin(a) })
  })
  return pos
}

// Children reveal in a phyllotaxis spiral around their package's current
// position — deterministic, compact, and nothing else on the map moves.
function childOffset(rank: number): { x: number; y: number } {
  const a = rank * GOLDEN_ANGLE
  const r = 24 + 13 * Math.sqrt(rank)
  return { x: r * Math.cos(a), y: r * Math.sin(a) }
}

function toElement(n: VisNode): ElementDefinition {
  return { data: { ...n } }
}

// fcose has internal Math.random calls; pin them to a seeded LCG for the
// duration of the layout so the same input always yields the same map.
// Returns the restore function so callers can cancel mid-layout (e.g. on unmount).
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

// Diff the desired view model into the live instance: remove what left,
// add what arrived (children positioned around their package), never touch
// what stayed. Returns the newly added nodes.
function applyViewModel(cy: Core, vm: ViewModel): CollectionReturnValue {
  const wantNodes = new Map(vm.nodes.map((n) => [n.id, n]))
  const wantEdges = new Map(vm.edges.map((e) => [e.id, e]))
  let added = cy.collection()

  // Snapshot parent positions BEFORE any cy.add of children. Once a compound
  // node gains children its position() becomes child-derived, so reading it
  // after adding children gives the wrong center for sibling placement.
  const parentPos = new Map<string, { x: number; y: number }>()
  for (const n of vm.nodes) {
    if (n.parent && !cy.$id(n.id).nonempty()) {
      const pkg = cy.$id(n.parent)
      if (pkg.nonempty() && !parentPos.has(n.parent)) {
        parentPos.set(n.parent, { ...pkg.position() })
      }
    }
  }

  cy.batch(() => {
    cy.edges().forEach((e) => {
      if (!wantEdges.has(e.id())) e.remove()
    })
    cy.nodes().forEach((n) => {
      if (!wantNodes.has(n.id())) n.remove()
    })
    // Parents first so compound membership resolves on add.
    const fresh = vm.nodes.filter((n) => !cy.$id(n.id).nonempty())
    fresh.sort((a, b) => Number(!!a.parent) - Number(!!b.parent))
    for (const n of fresh) {
      const el = cy.add(toElement(n))
      if (n.parent) {
        const p = parentPos.get(n.parent) ?? cy.$id(n.parent).position()
        const off = childOffset(n.kind === 'chip' ? 0 : (n.rank ?? 0) + 1)
        el.position({ x: p.x + off.x, y: p.y + off.y })
      }
      added = added.union(el)
    }
    for (const e of vm.edges) {
      if (cy.$id(e.id).empty()) cy.add({ data: { ...e } })
    }
  })
  return added
}

function applyLod(cy: Core) {
  const far = cy.zoom() < LOD_ZOOM
  cy.batch(() => {
    const weak = cy.edges('[count = 1]')
    if (far) weak.addClass('lod-hide')
    else weak.removeClass('lod-hide')
  })
}

interface Props {
  vm: ViewModel
  selected: string | null
  onSelect: (id: string | null) => void
  onTogglePackage: (pkg: string) => void
  onRevealTail: (pkg: string) => void
}

export function GraphCanvas({ vm, selected, onSelect, onTogglePackage, onRevealTail }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<Core | null>(null)
  const laidOut = useRef(false)
  const lodFar = useRef(false)
  const hood = useRef<CollectionReturnValue | null>(null)
  const clearTimer = useRef<number | undefined>(undefined)
  const cb = useRef({ onSelect, onTogglePackage, onRevealTail })
  cb.current = { onSelect, onTogglePackage, onRevealTail }
  // Track the last selected id that was centered so we don't re-animate on
  // every vm change while a node stays selected (Fix 3).
  const lastCentered = useRef<string | null>(null)
  // Hold the seededLayout restore function so we can cancel it on unmount (Fix 5).
  const layoutRestore = useRef<(() => void) | null>(null)

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
      const kind = n.data('kind') as string
      if (kind === 'package') cb.current.onTogglePackage(n.data('label') as string)
      else if (kind === 'chip') cb.current.onRevealTail(n.data('pkg') as string)
      else cb.current.onSelect(n.id())
    })
    cy.on('tap', (evt) => {
      if (evt.target === cy) cb.current.onSelect(null)
    })

    // Hover: toggle classes only on the symmetric difference between the old
    // and new neighborhoods — never a full-graph class sweep. The clear is
    // debounced so transiting between adjacent nodes doesn't flash.
    const clearHover = () => {
      const prev = hood.current
      if (!prev) return
      hood.current = null
      cy.batch(() => cy.elements().removeClass('dim hl'))
    }
    cy.on('mouseover', 'node', (evt) => {
      const n = evt.target
      if (n.data('kind') === 'package' && n.isParent()) return
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
    })
    ;(window as unknown as { __cy?: Core }).__cy = cy
    cyRef.current = cy
    return () => {
      window.clearTimeout(clearTimer.current)
      // Restore Math.random before destroy in case layout is still running (Fix 5).
      layoutRestore.current?.()
      layoutRestore.current = null
      cy.destroy()
      cyRef.current = null
    }
  }, [])

  // Reflect the view model. First non-empty model: seed + fcose refine + fit.
  // Later models: incremental add/remove only — the map never jumps.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy || vm.nodes.length === 0) return
    if (!laidOut.current) {
      laidOut.current = true
      const seeds = seedPositions(vm)
      cy.batch(() => {
        applyViewModel(cy, vm)
        for (const [id, p] of seeds) cy.$id(id).position(p)
      })
      layoutRestore.current = seededLayout(cy, { quality: 'default', animate: false, randomize: false, fit: false, nodeSeparation: 120, idealEdgeLength: 130, nodeRepulsion: 6500, gravity: 0.15, packComponents: false }, () => {
        layoutRestore.current = null
        cy.fit(undefined, 50)
        applyLod(cy)
        ;(window as unknown as { __layoutDone?: boolean }).__layoutDone = true
      })
    } else {
      applyViewModel(cy, vm)
      // Always re-apply LOD after incremental update: collapse produces new
      // bundled edges that may miss LOD classes at current zoom (Fix 4).
      applyLod(cy)
    }
  }, [vm])

  // Reflect selection: mark and highlight neighborhood unconditionally (vm may
  // reveal the node), but only animate to center when the selected id changes —
  // not on every vm change while the same node is already selected (Fix 3).
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
      cy.animate({ center: { eles: n }, zoom: Math.max(cy.zoom(), 0.8) }, { duration: 300 })
    }
  }, [selected, vm])

  return <div ref={containerRef} className="graph-canvas" data-testid="graph-canvas" />
}

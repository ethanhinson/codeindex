import { useEffect, useRef } from 'react'
import cytoscape from 'cytoscape'
import type { Core, ElementDefinition } from 'cytoscape'
import fcose from 'cytoscape-fcose'
import type { GraphEdge, GraphNode } from '../types'
import { stylesheet } from './style'

let registered = false
function ensureFcose() {
  if (!registered) {
    cytoscape.use(fcose)
    registered = true
  }
}

function edgeId(e: GraphEdge): string {
  return `${e.source}|${e.target}|${e.kind}`
}

// Build cytoscape elements from the whole graph: one compound parent per
// package group, symbol/lore nodes as children, and all edges. Node degree is
// precomputed for size mapping.
function build(nodes: GraphNode[], edges: GraphEdge[]): ElementDefinition[] {
  const degree = new Map<string, number>()
  for (const e of edges) {
    degree.set(e.source, (degree.get(e.source) ?? 0) + 1)
    degree.set(e.target, (degree.get(e.target) ?? 0) + 1)
  }
  const groups = new Set<string>()
  for (const n of nodes) if (n.group) groups.add(n.group)

  const els: ElementDefinition[] = []
  for (const g of groups) {
    els.push({ data: { id: `grp:${g}`, label: g, isGroup: 1 }, classes: 'group' })
  }
  for (const n of nodes) {
    els.push({
      data: {
        ...n,
        parent: n.group ? `grp:${n.group}` : undefined,
        degree: degree.get(n.id) ?? 0,
      },
    })
  }
  for (const e of edges) {
    els.push({ data: { id: edgeId(e), source: e.source, target: e.target, kind: e.kind, conf: e.conf } })
  }
  return els
}

interface Props {
  nodes: GraphNode[]
  edges: GraphEdge[]
  selected: string | null
  onSelect: (id: string | null) => void
}

export function GraphCanvas({ nodes, edges, selected, onSelect }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<Core | null>(null)
  const selectRef = useRef(onSelect)
  selectRef.current = onSelect

  // Create the instance once, tuned for a few-thousand-node graph.
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
      if (n.data('isGroup')) return
      selectRef.current(n.id())
    })
    cy.on('tap', (evt) => {
      if (evt.target === cy) selectRef.current(null)
    })
    // Hover: highlight the node and its closed neighborhood, dim the rest.
    cy.on('mouseover', 'node', (evt) => {
      const n = evt.target
      if (n.data('isGroup')) return
      const hood = n.closedNeighborhood()
      cy.elements().addClass('dim')
      hood.removeClass('dim').addClass('hl')
    })
    cy.on('mouseout', 'node', () => {
      cy.elements().removeClass('dim hl')
    })
    ;(window as unknown as { __cy?: Core }).__cy = cy
    cyRef.current = cy
    return () => {
      cy.destroy()
      cyRef.current = null
    }
  }, [])

  // Load elements (once the data arrives) and lay out.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy || nodes.length === 0) return
    cy.batch(() => {
      cy.elements().remove()
      cy.add(build(nodes, edges))
    })
    const layout = cy.layout({
      name: 'fcose',
      quality: 'default',
      animate: false,
      randomize: true,
      packComponents: true,
      tile: true,
      nodeSeparation: 110,
      idealEdgeLength: 85,
      nodeRepulsion: 5500,
      nestingFactor: 0.15,
      gravity: 0.2,
      gravityCompound: 1.0,
      padding: 40,
      fit: true,
    } as never)
    // Fit only once the (async) layout has actually placed the nodes.
    layout.one('layoutstop', () => cy.fit(undefined, 40))
    layout.run()
  }, [nodes, edges])

  // Reflect selection: mark, highlight neighborhood, and center.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    cy.elements().removeClass('sel selhl')
    if (!selected) return
    const n = cy.$id(selected)
    if (n.empty()) return
    n.addClass('sel')
    n.closedNeighborhood().addClass('selhl')
    cy.animate({ center: { eles: n }, zoom: Math.max(cy.zoom(), 0.8) }, { duration: 350 })
  }, [selected, nodes])

  return <div ref={containerRef} className="graph-canvas" data-testid="graph-canvas" />
}

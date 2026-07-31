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

function toElements(nodes: GraphNode[], edges: GraphEdge[]): ElementDefinition[] {
  const els: ElementDefinition[] = nodes.map((n) => ({ data: { ...n } }))
  for (const e of edges) {
    els.push({ data: { id: edgeId(e), source: e.source, target: e.target, kind: e.kind, conf: e.conf } })
  }
  return els
}

interface Props {
  nodes: GraphNode[]
  edges: GraphEdge[]
  focus: string | null
  onNodeTap: (id: string) => void
}

export function GraphCanvas({ nodes, edges, focus, onNodeTap }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<Core | null>(null)
  const tapRef = useRef(onNodeTap)
  tapRef.current = onNodeTap

  // Create the Cytoscape instance once.
  useEffect(() => {
    ensureFcose()
    if (!containerRef.current) return
    const cy = cytoscape({
      container: containerRef.current,
      style: stylesheet(),
      elements: [],
      minZoom: 0.2,
      maxZoom: 3,
      wheelSensitivity: 0.2,
    })
    cy.on('tap', 'node', (evt) => tapRef.current(evt.target.id()))
    cyRef.current = cy
    // Exposed for e2e assertions / debugging; harmless in production.
    ;(window as unknown as { __cy?: Core }).__cy = cy
    return () => {
      cy.destroy()
      cyRef.current = null
    }
  }, [])

  // Sync elements: add new, remove absent, relayout only when nodes appear.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    const defs = toElements(nodes, edges)
    const incoming = new Set(defs.map((d) => String(d.data.id)))
    const existing = new Set(cy.elements().map((e) => e.id()))

    cy.batch(() => {
      cy.elements()
        .filter((e) => !incoming.has(e.id()))
        .remove()
      const toAdd = defs.filter((d) => !existing.has(String(d.data.id)))
      if (toAdd.length) cy.add(toAdd)
    })

    const grew = defs.some((d) => !existing.has(String(d.data.id)))
    if (grew) {
      cy.layout({
        name: 'fcose',
        animate: true,
        animationDuration: 400,
        randomize: existing.size === 0,
        fit: true,
        padding: 48,
        nodeSeparation: 120,
        idealEdgeLength: 95,
      } as never).run()
    }
  }, [nodes, edges])

  // Reflect focus styling.
  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    cy.nodes().removeClass('focus')
    if (focus) cy.$id(focus).addClass('focus')
  }, [focus, nodes])

  return <div ref={containerRef} className="graph-canvas" data-testid="graph-canvas" />
}

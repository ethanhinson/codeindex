import type { StylesheetCSS } from 'cytoscape'
import type { EdgeKind, NodeKind } from '../types'

// Typed visual language: one distinct shape+color per node kind, one
// distinct line treatment+color per edge kind. Kept in one place so the
// canvas, the legend, and the inspector all agree.

export const NODE_COLORS: Record<NodeKind, string> = {
  symbol: '#4f8ff7',
  decision: '#f2b134',
  item: '#3ecf8e',
  note: '#8a94a6',
  path: '#7c8db5',
}

export const NODE_SHAPES: Record<NodeKind, string> = {
  symbol: 'ellipse',
  decision: 'diamond',
  item: 'round-rectangle',
  note: 'rectangle',
  path: 'hexagon',
}

export const EDGE_COLORS: Record<EdgeKind, string> = {
  calls: '#6b7385',
  anchors: '#f2b134',
  blocked_by: '#e5484d',
}

export const EDGE_STYLES: Record<EdgeKind, 'solid' | 'dashed' | 'dotted'> = {
  calls: 'solid',
  anchors: 'dashed',
  blocked_by: 'dotted',
}

// Return type is loosely typed: cytoscape's per-property Css types are very
// strict and reject valid selector-string style maps. We build plain objects
// and assert the array shape at the boundary.
export function stylesheet(): StylesheetCSS[] {
  const nodeKinds = Object.keys(NODE_COLORS) as NodeKind[]
  const edgeKinds = Object.keys(EDGE_COLORS) as EdgeKind[]

  const base: Array<{ selector: string; style: Record<string, unknown> }> = [
    {
      selector: 'node',
      style: {
        label: 'data(label)',
        color: '#e6e9ef',
        'font-size': 11,
        'text-valign': 'bottom',
        'text-halign': 'center',
        'text-margin-y': 4,
        'text-wrap': 'ellipsis',
        'text-max-width': '120px',
        width: 26,
        height: 26,
        'border-width': 0,
      },
    },
    {
      selector: 'node.focus',
      style: {
        'border-width': 3,
        'border-color': '#ffffff',
        width: 34,
        height: 34,
        'font-size': 13,
        'z-index': 10,
      },
    },
    {
      selector: 'edge',
      style: {
        width: 1.5,
        'curve-style': 'bezier',
        'target-arrow-shape': 'triangle',
        'arrow-scale': 0.9,
      },
    },
    {
      selector: 'edge[conf = "ambiguous"]',
      style: { opacity: 0.55 },
    },
  ]

  for (const k of nodeKinds) {
    base.push({
      selector: `node[kind = "${k}"]`,
      style: { 'background-color': NODE_COLORS[k], shape: NODE_SHAPES[k] },
    })
  }
  for (const k of edgeKinds) {
    base.push({
      selector: `edge[kind = "${k}"]`,
      style: {
        'line-color': EDGE_COLORS[k],
        'target-arrow-color': EDGE_COLORS[k],
        'line-style': EDGE_STYLES[k],
      },
    })
  }
  return base as unknown as StylesheetCSS[]
}

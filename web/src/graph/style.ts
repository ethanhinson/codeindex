import type { StylesheetCSS } from 'cytoscape'
import type { EdgeKind, NodeKind } from '../types'

// Typed visual language: one distinct shape+color per node kind, one distinct
// line treatment+color per edge kind. Shared by canvas, legend, and inspector.

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
  calls: '#3a4356',
  anchors: '#f2b134',
  blocked_by: '#e5484d',
}

export const EDGE_STYLES: Record<EdgeKind, 'solid' | 'dashed' | 'dotted'> = {
  calls: 'solid',
  anchors: 'dashed',
  blocked_by: 'dotted',
}

export function stylesheet(): StylesheetCSS[] {
  const edgeKinds = Object.keys(EDGE_COLORS) as EdgeKind[]

  const base: Array<{ selector: string; style: Record<string, unknown> }> = [
    // Symbols: quiet dots; labels exist but are invisible until earned.
    {
      selector: 'node',
      style: {
        label: 'data(label)',
        color: '#c5ccd8',
        'font-size': 10,
        'text-opacity': 0,
        'text-valign': 'bottom',
        'text-halign': 'center',
        'text-margin-y': 3,
        'text-wrap': 'ellipsis',
        'text-max-width': '150px',
        width: 'mapData(degree, 0, 40, 8, 40)',
        height: 'mapData(degree, 0, 40, 8, 40)',
        'border-width': 0,
        'background-color': NODE_COLORS.symbol,
        shape: 'ellipse',
        'transition-property': 'text-opacity',
        'transition-duration': '0.12s',
      },
    },
    // Earned/hovered/selected labels fade in.
    { selector: 'node.labeled', style: { 'text-opacity': 1 } },
    { selector: 'node.hl', style: { 'text-opacity': 1, opacity: 1, 'z-index': 30 } },
    // Overview package nodes: always-labeled map tiles sized by symbol count.
    {
      selector: 'node[kind = "package"]',
      style: {
        shape: 'round-rectangle',
        'background-color': '#22304a',
        'border-width': 1.5,
        'border-color': '#3a4a66',
        color: '#aab6c8',
        'font-size': 12,
        'text-opacity': 1,
        'text-valign': 'center',
        'text-halign': 'center',
        width: 'mapData(symCount, 1, 120, 34, 116)',
        height: 'mapData(symCount, 1, 120, 24, 48)',
      },
    },
    // Focus-view satellites: smaller, muted chips on the rim.
    {
      selector: 'node[role = "satellite"]',
      style: {
        'background-color': '#182238',
        'font-size': 11,
        width: 'mapData(symCount, 1, 120, 28, 84)',
        height: 22,
      },
    },
    // Generic edge first; feature rules AFTER it (order-precedence, see lore).
    {
      selector: 'edge',
      style: {
        width: 1,
        'curve-style': 'straight',
        'line-color': '#2f3745',
        'target-arrow-shape': 'none',
        opacity: 0.7,
      },
    },
    {
      selector: 'edge[?bundled]',
      style: {
        width: 'mapData(count, 1, 60, 1, 7)',
        'curve-style': 'straight',
        'line-color': '#3a4356',
        opacity: 0.55,
      },
    },
    // Interaction states.
    { selector: '.dim', style: { opacity: 0.15 } },
    { selector: 'edge.hl', style: { opacity: 1, width: 2, 'line-color': '#9fb2d6', 'z-index': 30 } },
    // Lore-rail hover-sync target (before sel so sel's 3px border wins when both present).
    { selector: 'node.lore-hot', style: { 'border-width': 2.5, 'border-color': '#f2b134' } },
    {
      selector: 'node.sel',
      style: { 'border-width': 3, 'border-color': '#ffffff', 'font-size': 13, 'text-opacity': 1, 'z-index': 40 },
    },
    { selector: 'node.selhl', style: { 'text-opacity': 1 } },
    { selector: 'edge.selhl', style: { width: 2, opacity: 1, 'line-color': '#9fb2d6', 'z-index': 25 } },
    // LOD: elements hidden at far zoom.
    { selector: '.lod-hide', style: { display: 'none' } },
  ]

  for (const k of edgeKinds) {
    if (k === 'calls') continue // calls keep the neutral defaults for density
    base.push({
      selector: `edge[kind = "${k}"]`,
      style: { 'line-color': EDGE_COLORS[k], 'line-style': EDGE_STYLES[k], opacity: 0.9, width: 1.5 },
    })
  }
  return base as unknown as StylesheetCSS[]
}

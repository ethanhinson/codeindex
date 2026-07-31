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
  const nodeKinds = Object.keys(NODE_COLORS) as NodeKind[]
  const edgeKinds = Object.keys(EDGE_COLORS) as EdgeKind[]

  const base: Array<{ selector: string; style: Record<string, unknown> }> = [
    {
      selector: 'node',
      style: {
        label: 'data(label)',
        color: '#c5ccd8',
        'font-size': 10,
        // Labels fade out when zoomed out — legible up close, clean at a distance.
        'min-zoomed-font-size': 9,
        'text-valign': 'bottom',
        'text-halign': 'center',
        'text-margin-y': 3,
        'text-wrap': 'ellipsis',
        'text-max-width': '140px',
        // Size by degree: hubs are bigger, like Obsidian.
        width: 'mapData(degree, 0, 40, 10, 46)',
        height: 'mapData(degree, 0, 40, 10, 46)',
        'border-width': 0,
      },
    },
    // Compound package clusters.
    {
      selector: 'node.group',
      style: {
        label: 'data(label)',
        shape: 'round-rectangle',
        'background-color': '#4f8ff7',
        'background-opacity': 0.04,
        'border-width': 1,
        'border-color': '#2a3140',
        'border-style': 'dashed',
        color: '#8a94a6',
        'font-size': 13,
        'min-zoomed-font-size': 0,
        'text-valign': 'top',
        'text-halign': 'center',
        'text-margin-y': -2,
        padding: 14,
        'z-compound-depth': 'bottom',
      },
    },
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
    // Interaction states.
    { selector: '.dim', style: { opacity: 0.15 } },
    { selector: 'node.hl', style: { opacity: 1, 'z-index': 30 } },
    { selector: 'edge.hl', style: { opacity: 1, width: 2, 'line-color': '#9fb2d6', 'z-index': 30 } },
    {
      selector: 'node.sel',
      style: {
        'border-width': 3,
        'border-color': '#ffffff',
        'font-size': 13,
        'min-zoomed-font-size': 0,
        'z-index': 40,
      },
    },
    { selector: 'node.selhl', style: { 'min-zoomed-font-size': 0 } },
    { selector: 'edge.selhl', style: { width: 2, opacity: 1, 'line-color': '#9fb2d6', 'z-index': 25 } },
  ]

  for (const k of nodeKinds) {
    base.push({
      selector: `node[kind = "${k}"]`,
      style: { 'background-color': NODE_COLORS[k], shape: NODE_SHAPES[k] },
    })
  }
  for (const k of edgeKinds) {
    if (k === 'calls') continue // calls keep the neutral default for density
    base.push({
      selector: `edge[kind = "${k}"]`,
      style: {
        'line-color': EDGE_COLORS[k],
        'line-style': EDGE_STYLES[k],
        opacity: 0.9,
        width: 1.5,
      },
    })
  }
  return base as unknown as StylesheetCSS[]
}

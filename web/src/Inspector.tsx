import type { GraphEdge, GraphNode } from './types'
import { EDGE_COLORS, NODE_COLORS } from './graph/style'

interface Props {
  node: GraphNode | null
  nodes: GraphNode[]
  edges: GraphEdge[]
  onOpen: (id: string) => void
}

interface Neighbor {
  id: string
  label: string
  kind: string
  edgeKind: string
  dir: 'out' | 'in'
}

function neighborsOf(focus: string, nodes: GraphNode[], edges: GraphEdge[]): Neighbor[] {
  const byId = new Map(nodes.map((n) => [n.id, n]))
  const out: Neighbor[] = []
  for (const e of edges) {
    if (e.source === focus) {
      const n = byId.get(e.target)
      out.push({ id: e.target, label: n?.label ?? e.target, kind: n?.kind ?? '?', edgeKind: e.kind, dir: 'out' })
    } else if (e.target === focus) {
      const n = byId.get(e.source)
      out.push({ id: e.source, label: n?.label ?? e.source, kind: n?.kind ?? '?', edgeKind: e.kind, dir: 'in' })
    }
  }
  return out
}

export function Inspector({ node, nodes, edges, onOpen }: Props) {
  if (!node) {
    return (
      <aside className="inspector" data-testid="inspector">
        <div className="inspector-empty">Select or search a node to inspect it.</div>
      </aside>
    )
  }
  const neighbors = neighborsOf(node.id, nodes, edges)
  return (
    <aside className="inspector" data-testid="inspector">
      <div className="inspector-head">
        <span className="kind-dot" style={{ background: NODE_COLORS[node.kind] }} />
        <span className="inspector-kind">{node.kind}</span>
      </div>
      <h2 className="inspector-title" data-testid="inspector-title">
        {node.label}
      </h2>
      <dl className="inspector-meta">
        {node.file && (
          <div>
            <dt>file</dt>
            <dd>
              {node.file}
              {node.line ? `:${node.line}` : ''}
            </dd>
          </div>
        )}
        {node.signature && (
          <div>
            <dt>signature</dt>
            <dd className="mono">{node.signature}</dd>
          </div>
        )}
        {node.status && (
          <div>
            <dt>status</dt>
            <dd>{node.status}</dd>
          </div>
        )}
        {node.priority && (
          <div>
            <dt>priority</dt>
            <dd>{node.priority}</dd>
          </div>
        )}
        <div>
          <dt>id</dt>
          <dd className="mono">{node.id}</dd>
        </div>
      </dl>

      <div className="inspector-neighbors">
        <div className="section-label">neighbors ({neighbors.length})</div>
        {neighbors.map((nb) => (
          <button
            key={`${nb.dir}-${nb.edgeKind}-${nb.id}`}
            className="neighbor"
            data-testid="neighbor"
            onClick={() => onOpen(nb.id)}
            title={`${nb.dir === 'out' ? '→' : '←'} ${nb.edgeKind}`}
          >
            <span className="kind-dot sm" style={{ background: NODE_COLORS[nb.kind as never] ?? '#888' }} />
            <span className="neighbor-label">{nb.label}</span>
            <span className="edge-tag" style={{ color: EDGE_COLORS[nb.edgeKind as never] ?? '#888' }}>
              {nb.dir === 'out' ? '→' : '←'} {nb.edgeKind}
            </span>
          </button>
        ))}
      </div>
    </aside>
  )
}

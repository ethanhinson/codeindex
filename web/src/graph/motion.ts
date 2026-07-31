// Idle motion: a small, hash-seeded Lissajous drift rendered on top of each
// node's deterministic anchor (data ax/ay). Anchors are truth — this module
// never writes them, so layouts, diffs, and determinism tests are unaffected.
import type { Core, EventObject } from 'cytoscape'

const MAX_AMP = 2.5
const MIN_AMP = 1.5
const FRAME_MS = 1000 / 30

function hash32(s: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return h >>> 0
}

export function motionEnabled(search: string): boolean {
  if (new URLSearchParams(search).get('motion') === '0') return false
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return false
  }
  return true
}

export function oscOffset(id: string, tMs: number): { x: number; y: number } {
  const h = hash32(id)
  const period = 5000 + (h % 4000)                 // 5–9s
  const phase = (((h >>> 8) % 1000) / 1000) * 2 * Math.PI
  const amp = MIN_AMP + (((h >>> 16) % 100) / 100) * (MAX_AMP - MIN_AMP)
  const w = (2 * Math.PI * tMs) / period
  return {
    x: amp * Math.sin(w + phase),
    y: amp * Math.cos(w / 1.13 + phase * 1.7),
  }
}

// Drive rendered positions at ≤30fps. Pauses while isBusy() (layouts,
// transitions), while a node is grabbed, and within 150ms of a user
// pan/zoom. Stops cleanly via the returned function.
export function startMotion(cy: Core, isBusy: () => boolean): () => void {
  let raf = 0
  let last = 0
  let gestureUntil = 0
  let stopped = false

  const onGesture = () => {
    gestureUntil = performance.now() + 150
  }
  cy.on('pan zoom', onGesture)

  // A user drag moves a node away from its anchor deliberately: adopt the
  // new position as the anchor so motion doesn't snap it back. The rendered
  // position still includes the last frame's oscillation offset, so subtract
  // it — periods are ≥5s, so ≤33ms of staleness is negligible (<0.01px).
  const onDragFree = (evt: EventObject) => {
    const n = evt.target
    const p = n.position()
    const off = oscOffset(n.id(), performance.now())
    n.data('ax', p.x - off.x)
    n.data('ay', p.y - off.y)
  }
  cy.on('dragfree', 'node', onDragFree)

  const tick = (now: number) => {
    if (stopped) return
    raf = requestAnimationFrame(tick)
    if (now - last < FRAME_MS) return
    last = now
    if (isBusy() || now < gestureUntil) return
    if (cy.nodes(':grabbed').nonempty()) return
    cy.batch(() => {
      cy.nodes('[ax]').forEach((n) => {
        const off = oscOffset(n.id(), now)
        n.position({ x: (n.data('ax') as number) + off.x, y: (n.data('ay') as number) + off.y })
      })
    })
  }
  raf = requestAnimationFrame(tick)

  return () => {
    stopped = true
    cancelAnimationFrame(raf)
    cy.off('pan zoom', onGesture)
    cy.off('dragfree', 'node', onDragFree)
  }
}

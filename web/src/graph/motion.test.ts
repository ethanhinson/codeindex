import { describe, expect, test } from 'vitest'
import { motionEnabled, oscOffset } from './motion'

describe('oscOffset', () => {
  test('deterministic per (id, t)', () => {
    expect(oscOffset('sym#42', 1234)).toEqual(oscOffset('sym#42', 1234))
  })

  test('bounded by max amplitude', () => {
    for (const id of ['a', 'pkg:internal/graph', 'sym#1', 'sym#999']) {
      for (const t of [0, 500, 5000, 123456]) {
        const { x, y } = oscOffset(id, t)
        expect(Math.abs(x)).toBeLessThanOrEqual(2.5)
        expect(Math.abs(y)).toBeLessThanOrEqual(2.5)
      }
    }
  })

  test('varies over time and across ids', () => {
    const a0 = oscOffset('a', 0)
    const a1 = oscOffset('a', 1700)
    expect(a0.x !== a1.x || a0.y !== a1.y).toBe(true)
    const b0 = oscOffset('b', 0)
    expect(a0.x !== b0.x || a0.y !== b0.y).toBe(true)
  })
})

describe('motionEnabled', () => {
  test('motion=0 disables; default enables (jsdom has no reduced-motion)', () => {
    expect(motionEnabled('?motion=0')).toBe(false)
    expect(motionEnabled('?pkg=x&motion=0')).toBe(false)
    expect(motionEnabled('')).toBe(true)
    expect(motionEnabled('?pkg=x')).toBe(true)
  })
})

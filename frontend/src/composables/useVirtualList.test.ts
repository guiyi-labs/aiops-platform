import { describe, expect, it } from 'vitest'
import { computeWindow, windowRows } from './useVirtualList'

describe('computeWindow', () => {
  it('returns a full window at the top for a large list', () => {
    const w = computeWindow({ total: 50_000, scrollTop: 0, viewportHeight: 640, rowHeight: 56 })
    expect(w.totalHeight).toBe(2_800_000)
    expect(w.start).toBe(0)
    expect(w.end).toBeGreaterThanOrEqual(Math.ceil(640 / 56))
    expect(w.offsetY).toBe(0)
    expect(w.visibleCount).toBeGreaterThan(0)
  })

  it('windows into the middle with overscan and offsetY', () => {
    const w = computeWindow({ total: 5000, scrollTop: 56 * 100, viewportHeight: 640, rowHeight: 56, overscan: 4 })
    expect(w.start).toBe(96) // 100 - overscan 4
    expect(w.offsetY).toBe(96 * 56)
    expect(w.end).toBeGreaterThan(100 + Math.floor(640 / 56) + 4)
  })

  it('renders nothing once scrolled past the last row', () => {
    const w = computeWindow({ total: 10, scrollTop: 1000, viewportHeight: 640, rowHeight: 56 })
    expect(w.start).toBe(10)
    expect(w.end).toBe(10)
    expect(w.visibleCount).toBe(0)
    expect(w.totalHeight).toBe(560)
  })

  it('handles an empty list', () => {
    const w = computeWindow({ total: 0, scrollTop: 0, viewportHeight: 640, rowHeight: 56 })
    expect(w.totalHeight).toBe(0)
    expect(w.start).toBe(0)
    expect(w.end).toBe(0)
  })

  it('treats non-positive row height/viewport as clamped to 1/0', () => {
    const w = computeWindow({ total: 100, scrollTop: 0, viewportHeight: 0, rowHeight: 0 })
    expect(w.rowHeight).toBe(1)
    expect(w.totalHeight).toBe(100)
    expect(w.visibleCount).toBe(0)
  })
})

describe('windowRows', () => {
  const items = Array.from({ length: 100 }, (_, i) => i)

  it('slices rows and computes spacer heights', () => {
    const w = computeWindow({ total: items.length, scrollTop: 56 * 50, viewportHeight: 640, rowHeight: 56 })
    const { visible, topPad, bottomPad } = windowRows(items, w)
    expect(visible.length).toBe(w.visibleCount)
    expect(topPad).toBe(w.start * 56)
    expect(bottomPad).toBeGreaterThanOrEqual(0)
    // total height is preserved: top + visible + bottom = totalHeight
    expect(topPad + visible.length * 56 + bottomPad).toBe(w.totalHeight)
  })
})

import { describe, expect, it } from 'vitest'
import { filterResourcesByName } from './resource-filter'

describe('filterResourcesByName', () => {
  const items = Array.from({ length: 50_000 }, (_, index) => ({
    metadata: { name: `pod-${String(index).padStart(6, '0')}` },
    index,
  }))

  it('reuses the response array when no filter is active', () => {
    expect(filterResourcesByName(items, '')).toBe(items)
  })

  it('keeps matching response objects by identity', () => {
    const result = filterResourcesByName(items, 'POD-049999')
    expect(result).toHaveLength(1)
    expect(result[0]).toBe(items[49_999])
  })
})

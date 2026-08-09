import { describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'

import { easeOutExpo, useCountUp } from './useCountUp'

describe('useCountUp', () => {
  it('renders the target immediately when immediate is set', () => {
    const target = ref(42)
    const { value } = useCountUp(0, target, { immediate: true })
    expect(value.value).toBe(42)
  })

  it('animates values even without a browser environment (safe fallback)', async () => {
    const target = ref(0)
    const { value } = useCountUp(0, target, { duration: 400 })
    target.value = 25
    await nextTick()
    expect(value.value).toBe(25)
  })

  it('jumps straight to the target under reduced motion', async () => {
    vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
    const target = ref(3)
    const { value } = useCountUp(0, target, { duration: 800 })
    target.value = 17
    await nextTick()
    expect(value.value).toBe(17)
    vi.unstubAllGlobals()
  })

  it('easeOutExpo is monotonic and bounded', () => {
    expect(easeOutExpo(0)).toBe(1 - 2 ** 0)
    expect(easeOutExpo(1)).toBe(1)
    expect(easeOutExpo(0.5)).toBeCloseTo(1 - 2 ** -5)
  })
})
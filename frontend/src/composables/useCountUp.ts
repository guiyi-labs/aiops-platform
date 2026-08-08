import { getCurrentInstance, onBeforeUnmount, readonly, ref, watch, type Ref } from 'vue'

/**
 * useCountUp — decelerating numeric counter driven by requestAnimationFrame,
 * honoring `prefers-reduced-motion` by jumping straight to the target.
 *
 * Re-runs whenever the target changes, so headline metrics animate on load and
 * on refresh. Degrades gracefully when animation frame APIs are unavailable
 * (e.g. the Node test environment) by rendering the target value directly.
 */
export interface CountUpOptions {
  /** Duration in ms; default 750 and clamped to 150–2000. */
  duration?: number
  /** Render the target immediately and skip the animation. */
  immediate?: boolean
}

export const easeOutExpo = (t: number): number =>
  t >= 1 ? 1 : 1 - 2 ** (-10 * t)

const prefersReducedMotion = (): boolean => {
  if (typeof globalThis === 'undefined' || typeof globalThis.matchMedia !== 'function') return false
  try {
    return globalThis.matchMedia('(prefers-reduced-motion: reduce)').matches
  } catch {
    return false
  }
}

const canAnimate = (): boolean =>
  typeof requestAnimationFrame === 'function' &&
  typeof cancelAnimationFrame === 'function' &&
  typeof performance !== 'undefined' &&
  typeof performance.now === 'function'

export function useCountUp(
  initial: number,
  target: Ref<number>,
  options: CountUpOptions = {},
) {
  const duration = Math.min(2000, Math.max(150, options.duration ?? 750))
  const value = ref(initial)
  let frame = 0
  let from = initial
  let start = 0

  function animate(next: number) {
    if (frame !== 0) {
      cancelAnimationFrame(frame)
      frame = 0
    }
    if (options.immediate || prefersReducedMotion() || !canAnimate()) {
      value.value = next
      return
    }
    from = value.value
    const delta = next - from
    if (delta === 0) {
      value.value = next
      return
    }
    start = performance.now()
    const tick = (now: number) => {
      const progress = Math.min(1, (now - start) / duration)
      value.value = Math.round(from + delta * easeOutExpo(progress))
      if (progress < 1) {
        frame = requestAnimationFrame(tick)
      } else {
        value.value = next
      }
    }
    frame = requestAnimationFrame(tick)
  }

  watch(target, (next) => animate(next), { immediate: true })

  if (getCurrentInstance()) {
    onBeforeUnmount(() => {
      if (frame !== 0) cancelAnimationFrame(frame)
    })
  }

  return { value: readonly(value) }
}
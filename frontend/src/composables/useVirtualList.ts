import { computed, onBeforeUnmount, ref, watch } from 'vue'

/**
 * M91 windowed virtual list helper.
 *
 * Pure, dependency-free slice calculation for fixed-row-height lists. The
 * virtualizer keeps a small overscan window so scrolling stays responsive
 * even for O(10^4) rows, and exposes a single `onScroll` handler the consumer
 * binds to the scrolling container.
 *
 * Core math is isolated in `computeWindow` so it can be unit-tested without a
 * DOM (the project Vitest environment is `node`).
 */

export interface VirtualWindow {
  /** Index of the first visible row (inclusive). */
  start: number
  /** Index just past the last visible row (exclusive). */
  end: number
  /** Pixel offset used to translate-tricks / pad the top spacer. */
  offsetY: number
  /** Total pixel height of all rows. */
  totalHeight: number
  /** Number of rows actually visible in the viewport. */
  visibleCount: number
  /** Measured row height used by the window (px). */
  rowHeight: number
}

export interface ComputeWindowOptions {
  /** Total number of rows in the dataset. */
  total: number
  /** Pixels scrolled past the top of the list. */
  scrollTop: number
  /** Pixel height of the visible viewport. */
  viewportHeight: number
  /** Pixel height of a single row. */
  rowHeight: number
  /** Extra rows rendered above/below the viewport. */
  overscan?: number
}

/** Deterministic window calculation (pure, unit-testable). */
export function computeWindow(opts: ComputeWindowOptions): VirtualWindow {
  const overscan = opts.overscan ?? 4
  const total = Math.max(0, opts.total)
  const rowHeight = Math.max(1, opts.rowHeight)
  const viewportHeight = Math.max(0, opts.viewportHeight)
  const scrollTop = Math.max(0, opts.scrollTop)

  const totalHeight = total * rowHeight
  // No viewport, or scrolled past the end: render nothing.
  if (total === 0 || viewportHeight <= 0 || scrollTop >= totalHeight) {
    return { start: total, end: total, offsetY: totalHeight, totalHeight, visibleCount: 0, rowHeight }
  }
  const start = Math.min(total, Math.max(0, Math.floor(scrollTop / rowHeight) - overscan))
  const end = Math.min(
    total,
    Math.max(start, Math.ceil((scrollTop + viewportHeight) / rowHeight) + overscan),
  )
  const offsetY = start * rowHeight
  return {
    start,
    end,
    offsetY,
    totalHeight,
    visibleCount: Math.max(0, end - start),
    rowHeight,
  }
}

/**
 * Reactive virtual window over a fixed-row-height list.
 *
 * Bind `container.el` (scrolling element) and call `onScroll(e)` from its
 * scroll handler; `window.value` then drives the spacer + slice rendering.
 * `slice(items)` returns just the rows within the window.
 */
export function useVirtualList(options: {
  total: () => number
  rowHeight?: number
  overscan?: number
  estimateViewport?: number
}) {
  const container = ref<HTMLElement | null>(null)
  const rowHeight = options.rowHeight ?? 56
  const overscan = options.overscan ?? 4
  const estimateViewport = options.estimateViewport ?? 640
  const scrollTop = ref(0)
  const viewportHeight = ref(estimateViewport)

  const totalCount = computed(() => Math.max(0, options.total()))

  const window = computed<VirtualWindow>(() =>
    computeWindow({
      total: totalCount.value,
      scrollTop: scrollTop.value,
      viewportHeight: viewportHeight.value,
      rowHeight,
      overscan,
    }),
  )

  let rafHandle = 0
  let lastSync = 0

  /** Recompute container height lazily (rAF-throttled). */
  const syncHeight = () => {
    if (!container.value) {
      viewportHeight.value = estimateViewport
      return
    }
    viewportHeight.value = container.value.clientHeight || estimateViewport
  }
  syncHeight()

  /** Scroll handler bound to the scrolling container. */
  const onScroll = (e: Event) => {
    const el = e.currentTarget as HTMLElement
    const next = Math.max(0, el.scrollTop)
    const now = performance.now()
    if (near(next, scrollTop.value) && lastSync > 0) return
    // rAF-throttle the reactive update.
    if (rafHandle) cancelAnimationFrame(rafHandle)
    rafHandle = requestAnimationFrame(() => {
      scrollTop.value = next
      if (container.value) {
        viewportHeight.value = container.value.clientHeight || viewportHeight.value
      }
      lastSync = now
    })
  }

  /** Rows (or generic values) within the current window. */
  const slice = <T,>(items: T[]): T[] => {
    const w = window.value
    if (items.length === 0) return items
    return items.slice(w.start, w.end)
  }

  // Reset scroll position whenever the dataset shrinks below the window, so
  // reveal doesn't leave a trailing spacer.
  watch(
    () => [totalCount.value, window.value.start] as const,
    () => {
      if (container.value && window.value.start >= totalCount.value && totalCount.value > 0) {
        container.value.scrollTop = Math.max(0, (totalCount.value - 1) * rowHeight)
      }
    },
  )

  onBeforeUnmount(() => {
    if (rafHandle) cancelAnimationFrame(rafHandle)
  })

  return {
    container,
    window,
    rowHeight,
    onScroll,
    slice,
    syncHeight,
  }
}

function near(a: number, b: number): boolean {
  return Math.abs(a - b) < 0.5
}

/**
 * Render-shell helper for a windowed `<tbody>`: returns top/bottom spacer row
 * properties and the visible rows. Kept separate so templates stay small.
 */
export function windowRows<T>(items: T[], w: VirtualWindow): {
  visible: T[]
  topPad: number
  bottomPad: number
} {
  const visible = items.slice(w.start, w.end)
  const topPad = w.offsetY
  const bottomPad = Math.max(0, w.totalHeight - w.offsetY - visible.length * w.rowHeight)
  return { visible, topPad, bottomPad }
}
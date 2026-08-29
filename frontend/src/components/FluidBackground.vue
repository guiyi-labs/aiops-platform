<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import { initFluidCursor, type FluidCursorConfig } from '../lib/fluidCursor'

const props = withDefaults(
  defineProps<Partial<FluidCursorConfig>>(),
  {},
)

const canvasRef = ref<HTMLCanvasElement | null>(null)
let controls: { destroy(): void } | null = null

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return

  const start = () => {
    try {
      controls = initFluidCursor(canvas, props)
    } catch (err) {
      console.warn('[FluidBackground] WebGL init failed, falling back to particle network', err)
      // gracefully degrade: canvas stays hidden
    }
  }

  // Ensure canvas has layout dimensions before WebGL init
  if (canvas.clientWidth > 0 && canvas.clientHeight > 0) {
    start()
  } else {
    const ro = new ResizeObserver(() => {
      if (canvas.clientWidth > 0 && canvas.clientHeight > 0) {
        ro.disconnect()
        start()
      }
    })
    ro.observe(canvas.parentElement || canvas)
  }
})

onBeforeUnmount(() => {
  controls?.destroy()
  controls = null
})
</script>

<template>
  <div class="fluid-bg">
    <canvas ref="canvasRef" />
  </div>
</template>

<style scoped>
.fluid-bg {
  position: absolute;
  inset: 0;
  z-index: -3;
  pointer-events: none;
}
.fluid-bg canvas {
  display: block;
  width: 100%;
  height: 100%;
}
</style>

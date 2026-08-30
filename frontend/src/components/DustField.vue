<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

/**
 * DustField — 静谧漂浮的灰黑色尘埃粒子场
 * 灵感来源：internal.limxdynamics.com 登录页背景
 * - 仅 Canvas 2D 渲染，无依赖
 * - 粒子缓慢漂浮 + 轻微缩放闪烁（twinkle）
 * - 无连线、无鼠标交互、不阻塞底层
 * - 适配 prefers-reduced-motion
 */

interface Particle {
  x: number
  y: number
  vx: number
  vy: number
  radius: number
  /** 基础不透明度（0~1） */
  baseAlpha: number
  /** twinkle 频率（弧度/秒） */
  twinkleSpeed: number
  /** twinkle 相位 */
  twinklePhase: number
}

interface DustFieldOptions {
  /** 每多少像素面积生成一个粒子，值越小越密 */
  densityPerPixel?: number
  /** 粒子数量上下限 */
  minCount?: number
  maxCount?: number
  /** 单粒子基础速度（像素/帧@60fps） */
  maxSpeed?: number
  /** 半径范围（CSS 像素） */
  minRadius?: number
  maxRadius?: number
  /** 不透明度范围 */
  minAlpha?: number
  maxAlpha?: number
  /** twinkle 振幅（叠加在 baseAlpha 上） */
  twinkleAmplitude?: number
  /** 颜色：HSL 灰阶范围 [light, dark]，偏暗以适配浅色背景 */
  colorLightness?: number[]
}

const props = withDefaults(defineProps<DustFieldOptions>(), {
  densityPerPixel: 18000,
  minCount: 36,
  maxCount: 96,
  maxSpeed: 0.22,
  minRadius: 0.7,
  maxRadius: 2.2,
  minAlpha: 0.18,
  maxAlpha: 0.62,
  twinkleAmplitude: 0.28,
  colorLightness: () => [22, 42],
})

const canvasRef = ref<HTMLCanvasElement | null>(null)

let context: CanvasRenderingContext2D | null = null
let resizeObserver: ResizeObserver | null = null
let motionQuery: MediaQueryList | null = null
let reducedMotion = false
let pageVisible = true
let animationFrame = 0
let running = false
let devicePixelRatio = 1
let width = 0
let height = 0
let lastTimestamp = 0
const particles: Particle[] = []

function rand(min: number, max: number): number {
  return min + Math.random() * (max - min)
}

function spawnParticle(x?: number, y?: number): Particle {
  const angle = Math.random() * Math.PI * 2
  const speed = rand(0.05, props.maxSpeed)
  return {
    x: x ?? Math.random() * width,
    y: y ?? Math.random() * height,
    vx: Math.cos(angle) * speed,
    vy: Math.sin(angle) * speed,
    radius: rand(props.minRadius, props.maxRadius),
    baseAlpha: rand(props.minAlpha, props.maxAlpha),
    twinkleSpeed: rand(0.4, 1.2),
    twinklePhase: Math.random() * Math.PI * 2,
  }
}

function initializeParticles() {
  particles.length = 0
  const target = Math.max(
    props.minCount,
    Math.min(props.maxCount, Math.floor((width * height) / props.densityPerPixel)),
  )
  for (let i = 0; i < target; i += 1) particles.push(spawnParticle())
}

function resizeCanvas() {
  const canvas = canvasRef.value
  if (!canvas || !context) return
  const parent = canvas.parentElement
  if (!parent) return
  const rect = parent.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return

  width = rect.width
  height = rect.height
  devicePixelRatio = Math.min(window.devicePixelRatio || 1, 2)
  canvas.width = Math.floor(width * devicePixelRatio)
  canvas.height = Math.floor(height * devicePixelRatio)
  canvas.style.width = `${width}px`
  canvas.style.height = `${height}px`
  context.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0)
  initializeParticles()
  draw(0)
}

  function draw(deltaSeconds: number) {
  if (!context) return
  context.clearRect(0, 0, width, height)

  const [light, dark] = props.colorLightness.length >= 2 ? props.colorLightness : [22, 42]

  for (const p of particles) {
    if (!reducedMotion) {
      p.x += p.vx * deltaSeconds * 60
      p.y += p.vy * deltaSeconds * 60
      // 边缘环回，留出一点 padding 避免突变
      if (p.x < -8) p.x = width + 8
      else if (p.x > width + 8) p.x = -8
      if (p.y < -8) p.y = height + 8
      else if (p.y > height + 8) p.y = -8
    }

    p.twinklePhase += p.twinkleSpeed * deltaSeconds
    const twinkle = Math.sin(p.twinklePhase) * props.twinkleAmplitude
    const alpha = Math.max(0.05, Math.min(0.95, p.baseAlpha + twinkle))
    // 颜色在深浅之间随机，模仿不同深浅的尘埃
    const lightness = light + Math.random() * (dark - light)
    context.fillStyle = `hsla(220, 12%, ${lightness}%, ${alpha})`
    context.beginPath()
    context.arc(p.x, p.y, p.radius, 0, Math.PI * 2)
    context.fill()
  }
}

function animationLoop(timestamp: number) {
  if (!running) return
  const delta = lastTimestamp ? Math.min(0.05, (timestamp - lastTimestamp) / 1000) : 0
  lastTimestamp = timestamp
  draw(delta || 0.016)
  animationFrame = requestAnimationFrame(animationLoop)
}

function startAnimation() {
  if (running || reducedMotion || !pageVisible) return
  running = true
  lastTimestamp = 0
  animationFrame = requestAnimationFrame(animationLoop)
}

function stopAnimation() {
  running = false
  if (animationFrame) cancelAnimationFrame(animationFrame)
  animationFrame = 0
}

function synchronize() {
  if (reducedMotion || !pageVisible) {
    stopAnimation()
    draw(0)
    return
  }
  startAnimation()
}

function handleVisibilityChange() {
  pageVisible = !document.hidden
  synchronize()
}

function handleMotionPreference(event: MediaQueryListEvent) {
  reducedMotion = event.matches
  synchronize()
}

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return
  context = canvas.getContext('2d', { alpha: true })

  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  reducedMotion = motionQuery.matches
  motionQuery.addEventListener('change', handleMotionPreference)
  document.addEventListener('visibilitychange', handleVisibilityChange)

  resizeObserver = new ResizeObserver(resizeCanvas)
  if (canvas.parentElement) resizeObserver.observe(canvas.parentElement)
  resizeCanvas()
  synchronize()
})

onBeforeUnmount(() => {
  stopAnimation()
  resizeObserver?.disconnect()
  motionQuery?.removeEventListener('change', handleMotionPreference)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<template>
  <canvas ref="canvasRef" class="dust-field" aria-hidden="true" />
</template>

<style scoped>
.dust-field {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  pointer-events: none;
}
</style>

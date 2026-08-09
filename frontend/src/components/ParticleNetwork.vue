<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

/**
 * ParticleNetwork — interactive Canvas2D particle field for the login backdrop.
 *
 * Particles drift freely; nearby pairs are connected with alpha-faded lines
 * to form a living mesh. The cursor acts as a soft gravity well that
 * gently attracts particles within a radius, then releases them.
 *
 * Respects prefers-reduced-motion by rendering a single static frame.
 */

const canvasRef = ref<HTMLCanvasElement | null>(null)
let ctx: CanvasRenderingContext2D | null = null
let raf = 0
let running = true
let dpr = 1
let width = 0
let height = 0

interface Particle {
  x: number
  y: number
  vx: number
  vy: number
  r: number
  hue: number
}

const particles: Particle[] = []
let mouse = { x: -9999, y: -9999, active: false }

const CONFIG = {
  density: 11000,        // 1 particle per N px² of canvas area
  maxParticles: 90,
  minParticles: 28,
  speed: 0.28,           // base velocity
  linkRadius: 132,       // distance for line connections
  mouseRadius: 180,      // cursor attraction radius
  mouseForce: 0.045,     // attraction strength
  colors: [
    { r: 94, g: 234, b: 212 },   // teal
    { r: 129, g: 140, b: 248 },  // indigo
    { r: 52, g: 211, b: 153 },   // emerald
    { r: 165, g: 180, b: 252 },  // periwinkle
  ],
}

function prefersReducedMotion(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches
}

function pickColor() {
  return CONFIG.colors[Math.floor(Math.random() * CONFIG.colors.length)]
}

function spawn(x?: number, y?: number): Particle {
  const color = pickColor()
  const angle = Math.random() * Math.PI * 2
  const speed = CONFIG.speed * (0.5 + Math.random())
  return {
    x: x ?? Math.random() * width,
    y: y ?? Math.random() * height,
    vx: Math.cos(angle) * speed,
    vy: Math.sin(angle) * speed,
    r: 1.2 + Math.random() * 1.8,
    hue: (color.r << 16) | (color.g << 8) | color.b,
  }
}

function colorStr(p: Particle, alpha: number): string {
  const r = (p.hue >> 16) & 0xff
  const g = (p.hue >> 8) & 0xff
  const b = p.hue & 0xff
  return `rgba(${r},${g},${b},${alpha})`
}

function initParticles() {
  const count = Math.min(CONFIG.maxParticles, Math.max(CONFIG.minParticles, Math.floor((width * height) / CONFIG.density)))
  particles.length = 0
  for (let i = 0; i < count; i++) particles.push(spawn())
}

function resize() {
  const canvas = canvasRef.value
  if (!canvas || !ctx) return
  const rect = canvas.parentElement!.getBoundingClientRect()
  dpr = Math.min(window.devicePixelRatio || 1, 2)
  width = rect.width
  height = rect.height
  canvas.width = Math.floor(width * dpr)
  canvas.height = Math.floor(height * dpr)
  canvas.style.width = `${width}px`
  canvas.style.height = `${height}px`
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  initParticles()
}

function update() {
  for (const p of particles) {
    p.x += p.vx
    p.y += p.vy

    // Mouse attraction
    if (mouse.active) {
      const dx = mouse.x - p.x
      const dy = mouse.y - p.y
      const dist = Math.hypot(dx, dy)
      if (dist < CONFIG.mouseRadius && dist > 0.1) {
        const force = (1 - dist / CONFIG.mouseRadius) * CONFIG.mouseForce
        p.vx += (dx / dist) * force
        p.vy += (dy / dist) * force
      }
    }

    // Friction
    p.vx *= 0.985
    p.vy *= 0.985

    // Minimum speed to prevent freezing
    const sp = Math.hypot(p.vx, p.vy)
    if (sp < CONFIG.speed * 0.35) {
      const angle = Math.atan2(p.vy, p.vx) || Math.random() * Math.PI * 2
      p.vx = Math.cos(angle) * CONFIG.speed * 0.5
      p.vy = Math.sin(angle) * CONFIG.speed * 0.5
    }

    // Wrap edges
    if (p.x < -12) p.x = width + 12
    else if (p.x > width + 12) p.x = -12
    if (p.y < -12) p.y = height + 12
    else if (p.y > height + 12) p.y = -12
  }
}

function draw() {
  if (!ctx) return
  ctx.clearRect(0, 0, width, height)

  // Draw connection lines
  const lr = CONFIG.linkRadius
  const lr2 = lr * lr
  for (let i = 0; i < particles.length; i++) {
    for (let j = i + 1; j < particles.length; j++) {
      const dx = particles[i].x - particles[j].x
      const dy = particles[i].y - particles[j].y
      const dist2 = dx * dx + dy * dy
      if (dist2 < lr2) {
        const dist = Math.sqrt(dist2)
        const alpha = (1 - dist / lr) * 0.22
        ctx.strokeStyle = `rgba(129, 140, 248, ${alpha})`
        ctx.lineWidth = 0.7
        ctx.beginPath()
        ctx.moveTo(particles[i].x, particles[i].y)
        ctx.lineTo(particles[j].x, particles[j].y)
        ctx.stroke()
      }
    }
  }

  // Mouse cursor links
  if (mouse.active) {
    const mr = CONFIG.mouseRadius
    for (const p of particles) {
      const dx = p.x - mouse.x
      const dy = p.y - mouse.y
      const dist = Math.hypot(dx, dy)
      if (dist < mr) {
        const alpha = (1 - dist / mr) * 0.4
        ctx.strokeStyle = `rgba(94, 234, 212, ${alpha})`
        ctx.lineWidth = 0.8
        ctx.beginPath()
        ctx.moveTo(p.x, p.y)
        ctx.lineTo(mouse.x, mouse.y)
        ctx.stroke()
      }
    }
  }

  // Draw particles
  for (const p of particles) {
    // Glow
    const glow = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, p.r * 4)
    glow.addColorStop(0, colorStr(p, 0.55))
    glow.addColorStop(1, colorStr(p, 0))
    ctx.fillStyle = glow
    ctx.beginPath()
    ctx.arc(p.x, p.y, p.r * 4, 0, Math.PI * 2)
    ctx.fill()

    // Core dot
    ctx.fillStyle = colorStr(p, 0.85)
    ctx.beginPath()
    ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2)
    ctx.fill()
  }
}

function loop() {
  if (!running) return
  update()
  draw()
  raf = requestAnimationFrame(loop)
}

function onResize() {
  resize()
  if (prefersReducedMotion()) draw()
}

function onMouseMove(e: MouseEvent) {
  const canvas = canvasRef.value
  if (!canvas) return
  const rect = canvas.getBoundingClientRect()
  mouse.x = e.clientX - rect.left
  mouse.y = e.clientY - rect.top
  mouse.active = true
}

function onMouseLeave() {
  mouse.active = false
  mouse.x = -9999
  mouse.y = -9999
}

function onTouchMove(e: TouchEvent) {
  const canvas = canvasRef.value
  if (!canvas || e.touches.length === 0) return
  const rect = canvas.getBoundingClientRect()
  mouse.x = e.touches[0].clientX - rect.left
  mouse.y = e.touches[0].clientY - rect.top
  mouse.active = true
}

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return
  ctx = canvas.getContext('2d', { alpha: true })
  if (!ctx) return
  resize()

  const parent = canvas.parentElement!
  parent.addEventListener('mousemove', onMouseMove)
  parent.addEventListener('mouseleave', onMouseLeave)
  parent.addEventListener('touchmove', onTouchMove, { passive: true })
  parent.addEventListener('touchend', onMouseLeave)
  window.addEventListener('resize', onResize)

  if (prefersReducedMotion()) {
    draw()
  } else {
    running = true
    loop()
  }
})

onBeforeUnmount(() => {
  running = false
  if (raf) cancelAnimationFrame(raf)
  const canvas = canvasRef.value
  if (canvas) {
    const parent = canvas.parentElement
    parent?.removeEventListener('mousemove', onMouseMove)
    parent?.removeEventListener('mouseleave', onMouseLeave)
    parent?.removeEventListener('touchmove', onTouchMove)
    parent?.removeEventListener('touchend', onMouseLeave)
  }
  window.removeEventListener('resize', onResize)
})
</script>

<template>
  <canvas ref="canvasRef" class="particle-network" aria-hidden="true"></canvas>
</template>

<style scoped>
.particle-network {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
}
</style>

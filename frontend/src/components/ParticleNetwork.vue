<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

type ParticlePhase = 'idle' | 'username' | 'password' | 'submitting' | 'success' | 'error'

const props = withDefaults(defineProps<{ phase?: ParticlePhase }>(), {
  phase: 'idle',
})

const canvasRef = ref<HTMLCanvasElement | null>(null)
const isRunning = ref(false)
const particleCount = ref(0)
const reducedMotion = ref(false)

let context: CanvasRenderingContext2D | null = null
let animationFrame = 0
let resizeObserver: ResizeObserver | null = null
let motionQuery: MediaQueryList | null = null
let parentElement: HTMLElement | null = null
let pageVisible = true
let devicePixelRatio = 1
let width = 0
let height = 0

interface Particle {
  x: number
  y: number
  velocityX: number
  velocityY: number
  radius: number
  color: number
}

interface ParticleProfile {
  density: number
  maximum: number
  minimum: number
  pixelRatioCap: number
}

const particles: Particle[] = []
const pointer = { x: -9999, y: -9999, active: false }

const CONFIG = {
  speed: 0.28,
  linkRadius: 132,
  pointerRadius: 180,
  pointerForce: 0.045,
  colors: [
    { red: 45, green: 212, blue: 191 },
    { red: 34, green: 211, blue: 238 },
    { red: 163, green: 230, blue: 53 },
    { red: 245, green: 158, blue: 11 },
  ],
}

function particleProfile(): ParticleProfile {
  if (width <= 720) {
    return { density: 15000, maximum: 48, minimum: 18, pixelRatioCap: 1.5 }
  }
  return { density: 14000, maximum: 70, minimum: 24, pixelRatioCap: 2 }
}

function pickColor() {
  return CONFIG.colors[Math.floor(Math.random() * CONFIG.colors.length)]
}

function spawnParticle(x?: number, y?: number): Particle {
  const color = pickColor()
  const angle = Math.random() * Math.PI * 2
  const speed = CONFIG.speed * (0.5 + Math.random())
  return {
    x: x ?? Math.random() * width,
    y: y ?? Math.random() * height,
    velocityX: Math.cos(angle) * speed,
    velocityY: Math.sin(angle) * speed,
    radius: 1.2 + Math.random() * 1.8,
    color: (color.red << 16) | (color.green << 8) | color.blue,
  }
}

function colorString(particle: Particle, alpha: number) {
  const red = (particle.color >> 16) & 0xff
  const green = (particle.color >> 8) & 0xff
  const blue = particle.color & 0xff
  return `rgba(${red},${green},${blue},${alpha})`
}

function initializeParticles() {
  const profile = particleProfile()
  const count = Math.min(profile.maximum, Math.max(profile.minimum, Math.floor((width * height) / profile.density)))
  particles.length = 0
  for (let index = 0; index < count; index += 1) particles.push(spawnParticle())
  particleCount.value = particles.length
}

function resizeCanvas() {
  const canvas = canvasRef.value
  if (!canvas || !context || !parentElement) return
  const bounds = parentElement.getBoundingClientRect()
  if (bounds.width <= 0 || bounds.height <= 0) return

  width = bounds.width
  height = bounds.height
  devicePixelRatio = Math.min(window.devicePixelRatio || 1, particleProfile().pixelRatioCap)
  canvas.width = Math.floor(width * devicePixelRatio)
  canvas.height = Math.floor(height * devicePixelRatio)
  canvas.style.width = `${width}px`
  canvas.style.height = `${height}px`
  context.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0)
  initializeParticles()
  draw()
}

function phaseMotionScale() {
  if (props.phase === 'submitting') return 1.9
  if (props.phase === 'success') return 0.72
  if (props.phase === 'username' || props.phase === 'password') return 1.12
  if (props.phase === 'error') return 0.55
  return 1
}

function updateParticles() {
  const motionScale = phaseMotionScale()
  const centerX = width / 2
  const centerY = height / 2

  for (const particle of particles) {
    particle.x += particle.velocityX * motionScale
    particle.y += particle.velocityY * motionScale

    if (props.phase === 'success') {
      const centerDistance = Math.hypot(centerX - particle.x, centerY - particle.y) || 1
      particle.velocityX += ((centerX - particle.x) / centerDistance) * 0.0018
      particle.velocityY += ((centerY - particle.y) / centerDistance) * 0.0018
    } else if (pointer.active) {
      const pointerDeltaX = pointer.x - particle.x
      const pointerDeltaY = pointer.y - particle.y
      const pointerDistance = Math.hypot(pointerDeltaX, pointerDeltaY)
      if (pointerDistance < CONFIG.pointerRadius && pointerDistance > 0.1) {
        const force = (1 - pointerDistance / CONFIG.pointerRadius) * CONFIG.pointerForce
        particle.velocityX += (pointerDeltaX / pointerDistance) * force
        particle.velocityY += (pointerDeltaY / pointerDistance) * force
      }
    }

    const friction = props.phase === 'error' ? 0.96 : 0.985
    particle.velocityX *= friction
    particle.velocityY *= friction

    const speed = Math.hypot(particle.velocityX, particle.velocityY)
    if (speed < CONFIG.speed * 0.35) {
      const angle = Math.atan2(particle.velocityY, particle.velocityX) || Math.random() * Math.PI * 2
      particle.velocityX = Math.cos(angle) * CONFIG.speed * 0.5
      particle.velocityY = Math.sin(angle) * CONFIG.speed * 0.5
    }

    if (particle.x < -12) particle.x = width + 12
    else if (particle.x > width + 12) particle.x = -12
    if (particle.y < -12) particle.y = height + 12
    else if (particle.y > height + 12) particle.y = -12
  }
}

function linkColor(alpha: number) {
  if (props.phase === 'error') return `rgba(245, 158, 11, ${alpha})`
  if (props.phase === 'password') return `rgba(34, 211, 238, ${alpha})`
  return `rgba(45, 212, 191, ${alpha})`
}

function draw() {
  if (!context) return
  context.clearRect(0, 0, width, height)

  const linkRadiusSquared = CONFIG.linkRadius * CONFIG.linkRadius
  const phaseAlpha = props.phase === 'submitting' || props.phase === 'success' ? 0.32 : 0.16
  for (let firstIndex = 0; firstIndex < particles.length; firstIndex += 1) {
    for (let secondIndex = firstIndex + 1; secondIndex < particles.length; secondIndex += 1) {
      const firstParticle = particles[firstIndex]
      const secondParticle = particles[secondIndex]
      const deltaX = firstParticle.x - secondParticle.x
      const deltaY = firstParticle.y - secondParticle.y
      const distanceSquared = deltaX * deltaX + deltaY * deltaY
      if (distanceSquared >= linkRadiusSquared) continue

      const distance = Math.sqrt(distanceSquared)
      const alpha = (1 - distance / CONFIG.linkRadius) * phaseAlpha
      context.strokeStyle = linkColor(alpha)
      context.lineWidth = props.phase === 'submitting' ? 0.9 : 0.7
      context.beginPath()
      context.moveTo(firstParticle.x, firstParticle.y)
      context.lineTo(secondParticle.x, secondParticle.y)
      context.stroke()
    }
  }

  if (pointer.active && props.phase !== 'success') {
    for (const particle of particles) {
      const distance = Math.hypot(particle.x - pointer.x, particle.y - pointer.y)
      if (distance >= CONFIG.pointerRadius) continue
      const alpha = (1 - distance / CONFIG.pointerRadius) * 0.4
      context.strokeStyle = `rgba(45, 212, 191, ${alpha})`
      context.lineWidth = 0.8
      context.beginPath()
      context.moveTo(particle.x, particle.y)
      context.lineTo(pointer.x, pointer.y)
      context.stroke()
    }
  }

  for (const particle of particles) {
    const glow = context.createRadialGradient(
      particle.x,
      particle.y,
      0,
      particle.x,
      particle.y,
      particle.radius * 4,
    )
    glow.addColorStop(0, colorString(particle, 0.55))
    glow.addColorStop(1, colorString(particle, 0))
    context.fillStyle = glow
    context.beginPath()
    context.arc(particle.x, particle.y, particle.radius * 4, 0, Math.PI * 2)
    context.fill()

    context.fillStyle = colorString(particle, 0.85)
    context.beginPath()
    context.arc(particle.x, particle.y, particle.radius, 0, Math.PI * 2)
    context.fill()
  }
}

function animationLoop() {
  if (!isRunning.value) return
  updateParticles()
  draw()
  animationFrame = requestAnimationFrame(animationLoop)
}

function stopAnimation() {
  isRunning.value = false
  if (animationFrame) cancelAnimationFrame(animationFrame)
  animationFrame = 0
}

function startAnimation() {
  if (isRunning.value || reducedMotion.value || !pageVisible || !context) return
  isRunning.value = true
  animationFrame = requestAnimationFrame(animationLoop)
}

function synchronizeAnimation() {
  if (reducedMotion.value || !pageVisible) {
    stopAnimation()
    draw()
    return
  }
  startAnimation()
}

function handlePointerMove(event: MouseEvent) {
  const canvas = canvasRef.value
  if (!canvas) return
  const bounds = canvas.getBoundingClientRect()
  pointer.x = event.clientX - bounds.left
  pointer.y = event.clientY - bounds.top
  pointer.active = true
}

function handlePointerLeave() {
  pointer.active = false
  pointer.x = -9999
  pointer.y = -9999
}

function handleTouchMove(event: TouchEvent) {
  const canvas = canvasRef.value
  const touch = event.touches[0]
  if (!canvas || !touch) return
  const bounds = canvas.getBoundingClientRect()
  pointer.x = touch.clientX - bounds.left
  pointer.y = touch.clientY - bounds.top
  pointer.active = true
}

function handleVisibilityChange() {
  pageVisible = !document.hidden
  synchronizeAnimation()
}

function handleMotionPreference(event: MediaQueryListEvent) {
  reducedMotion.value = event.matches
  handlePointerLeave()
  synchronizeAnimation()
}

watch(
  () => props.phase,
  () => {
    if (reducedMotion.value) draw()
  },
)

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return
  context = canvas.getContext('2d', { alpha: true })
  parentElement = canvas.parentElement
  if (!context || !parentElement) return

  pageVisible = !document.hidden
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  reducedMotion.value = motionQuery.matches
  motionQuery.addEventListener('change', handleMotionPreference)

  parentElement.addEventListener('mousemove', handlePointerMove)
  parentElement.addEventListener('mouseleave', handlePointerLeave)
  parentElement.addEventListener('touchmove', handleTouchMove, { passive: true })
  parentElement.addEventListener('touchend', handlePointerLeave)
  document.addEventListener('visibilitychange', handleVisibilityChange)

  resizeObserver = new ResizeObserver(resizeCanvas)
  resizeObserver.observe(parentElement)
  resizeCanvas()
  synchronizeAnimation()
})

onBeforeUnmount(() => {
  stopAnimation()
  resizeObserver?.disconnect()
  motionQuery?.removeEventListener('change', handleMotionPreference)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  parentElement?.removeEventListener('mousemove', handlePointerMove)
  parentElement?.removeEventListener('mouseleave', handlePointerLeave)
  parentElement?.removeEventListener('touchmove', handleTouchMove)
  parentElement?.removeEventListener('touchend', handlePointerLeave)
})
</script>

<template>
  <canvas
    ref="canvasRef"
    class="particle-network"
    aria-hidden="true"
    :data-particles="particleCount"
    :data-phase="phase"
    :data-reduced-motion="reducedMotion ? 'true' : 'false'"
    :data-running="isRunning ? 'true' : 'false'"
  ></canvas>
</template>

<style scoped>
.particle-network {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
}
</style>

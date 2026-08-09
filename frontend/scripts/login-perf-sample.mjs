#!/usr/bin/env node
/* global process */
/**
 * Login-page performance sampler (M93-B2).
 *
 * Launches the production preview server against `dist/`, opens the login route
 * in headless Chromium, and samples real runtime metrics across three paths.
 *
 * Output: `.artifacts/login-perf/login-samples.json`
 */

import { mkdirSync, writeFileSync, existsSync } from 'node:fs'
import { join } from 'node:path'
import { spawn } from 'node:child_process'
import { createRequire } from 'node:module'
import { chromium } from '@playwright/test'

const require = createRequire(import.meta.url)
const port = Number(process.env.LOGIN_PERF_PORT || 4173)
const baseUrl = `http://127.0.0.1:${port}`
const repeats = Number(process.env.LOGIN_PERF_REPEATS || 3)
const outDir = join(process.cwd(), '.artifacts', 'login-perf')

const profiles = [
  { name: 'desktop-normal', viewport: { width: 1440, height: 900 }, reducedMotion: 'no-preference' },
  { name: 'mobile-degraded', viewport: { width: 390, height: 844 }, reducedMotion: 'no-preference', deviceScaleFactor: 3 },
  { name: 'reduced-motion', viewport: { width: 1440, height: 900 }, reducedMotion: 'reduce' },
]

function maskSensitive(text) {
  return String(text)
    .replace(/(password=)[^&\s]+/gi, '$1***')
    .replace(/"(password|token|access_token|authorization)":"[^"]+"/gi, '"$1":"***"')
}

const anonymousSession = JSON.stringify({
  access_token: '',
  token_type: 'Bearer',
  expires_in: 0,
  user: null,
})

function resolveViteBin() {
  const vitePkg = require.resolve('vite/package.json')
  return join(vitePkg, '..', 'bin', 'vite.js')
}

function startPreview() {
  const viteBin = resolveViteBin()
  return spawn(process.execPath, [viteBin, 'preview', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], {
    cwd: process.cwd(),
    stdio: ['ignore', 'pipe', 'pipe'],
  })
}

async function waitForServer(timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${baseUrl}/login`)
      if (response.ok) return
    } catch {
      // server not up yet
    }
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  throw new Error(`Preview server not ready at ${baseUrl}`)
}

async function measureVisit(page) {
  const base = await page.evaluate(() => {
    const nav = performance.getEntriesByType('navigation')[0]
    const paintEntries = performance.getEntriesByType('paint')
    const lcpEntries = performance.getEntriesByType('largest-contentful-paint')
    const longTaskEntries = performance.getEntriesByType('longtask') || []
    const canvasElement = document.querySelector('canvas.particle-network')
    const canvas = canvasElement
      ? {
          width: canvasElement.width,
          height: canvasElement.height,
          cssWidthPx: Number.parseFloat(canvasElement.style.width) || 0,
          dpr: Number((canvasElement.width / (canvasElement.getBoundingClientRect().width || 1)).toFixed(3)),
          particles: Number(canvasElement.dataset.particles || 0),
          running: canvasElement.dataset.running === 'true',
          reducedMotion: canvasElement.dataset.reducedMotion === 'true',
          topologyCount: document.querySelectorAll('.login-topology, .topo-node, .topology-links line').length,
        }
      : null
    return {
      navigation: {
        domContentLoadedMs: nav ? nav.domContentLoadedEventEnd : null,
        loadEndMs: nav ? nav.loadEventEnd : null,
        totalMs: nav ? nav.duration : null,
      },
      paint: {
        firstPaintMs: paintEntries.find((entry) => entry.name === 'first-paint')?.startTime ?? null,
        firstContentfulPaintMs: paintEntries.find((entry) => entry.name === 'first-contentful-paint')?.startTime ?? null,
        largestContentfulPaintMs: (() => {
          const buffered = window.__lcpBuffer || []
          if (buffered.length > 0) return buffered[buffered.length - 1].startTime
          return lcpEntries.length > 0 ? lcpEntries[lcpEntries.length - 1].startTime : null
        })(),
      },
      longTasks: {
        count: longTaskEntries.length,
        totalDurationMs: longTaskEntries.reduce((sum, task) => sum + task.duration, 0),
        maxDurationMs: longTaskEntries.reduce((max, task) => Math.max(max, task.duration), 0),
      },
      canvas,
      memoryUsedJsHeapBytes: performance.memory ? performance.memory.usedJSHeapSize : null,
      memoryLimitJsHeapBytes: performance.memory ? performance.memory.jsHeapSizeLimit : null,
    }
  })

  const interaction = await page.evaluate(
    () =>
      new Promise((resolve) => {
        const start = performance.now()
        const input = document.querySelector('#username')
        if (input) input.focus()
        requestAnimationFrame(() =>
          requestAnimationFrame(() => resolve({ focusToSecondFrameMs: performance.now() - start })),
        )
      }),
  )

  const frames = await page.evaluate(
    () =>
      new Promise((resolve) => {
        const samples = []
        const start = performance.now()
        let last = start
        let raf = 0
        const tick = () => {
          const now = performance.now()
          samples.push(now - last)
          last = now
          raf += 1
          if (now - start < 1000) requestAnimationFrame(tick)
          else resolve({
            frames: raf,
            durationMs: now - start,
            avgFrameMs: Number(((now - start) / raf).toFixed(3)),
            maxFrameGapMs: Number(Math.max(...samples).toFixed(3)),
          })
        }
        requestAnimationFrame(tick)
      }),
  )

  const visibility = await page.evaluate(
    () =>
      new Promise((resolve) => {
        const canvasElement = document.querySelector('canvas.particle-network')
        const readRunning = () => canvasElement?.dataset.running === 'true'
        const beforeHidden = readRunning()

        Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
        document.dispatchEvent(new Event('visibilitychange'))
        setTimeout(() => {
          const wasHiddenAtPause = !readRunning()
          Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
          document.dispatchEvent(new Event('visibilitychange'))
          const resumeStart = performance.now()
          let resolved = false
          const finish = (resumed) => {
            if (resolved) return
            resolved = true
            resolve({
              runningBeforeHidden: beforeHidden,
              pausedWhileHidden: wasHiddenAtPause,
              resumeLatencyMs: performance.now() - resumeStart,
              resumedAfterRestore: resumed,
            })
          }
          const poll = (elapsed) => {
            if (readRunning()) finish(true)
            else if (elapsed > 500) finish(false)
            else setTimeout(() => poll(elapsed + 20), 20)
          }
          poll(0)
        }, 120)
      }),
  )

  return { ...base, interaction, frames, visibility }
}

async function main() {
  mkdirSync(outDir, { recursive: true })
  if (!existsSync(join(process.cwd(), 'dist', 'index.html'))) {
    console.error('dist build not found — run `pnpm build` first.')
    process.exit(1)
  }

  const server = startPreview()
  server.stdout.on('data', (data) => {
    const text = String(data).trim()
    if (text && process.env.LOGIN_PERF_VERBOSE === '1') process.stdout.write(`[preview] ${text}\n`)
  })
  server.stderr.on('data', (data) => {
    const text = String(data).trim()
    if (text && process.env.LOGIN_PERF_VERBOSE === '1') process.stderr.write(`[preview] ${text}\n`)
  })

  let browser
  try {
    await waitForServer()
    let browserLaunchError = null
    try {
      browser = await chromium.launch({ headless: true })
    } catch (error) {
      browserLaunchError = maskSensitive(error.message)
      console.error(`Browser launch failed: ${browserLaunchError}`)
    }

    const samples = []
    const failures = []

    for (const profile of profiles) {
      for (let iteration = 1; iteration <= repeats; iteration += 1) {
        if (!browser) {
          failures.push({ path: profile.name, iteration, error: `browser unavailable: ${browserLaunchError}` })
          continue
        }
        const context = await browser.newContext({
          viewport: profile.viewport,
          reducedMotion: profile.reducedMotion,
          deviceScaleFactor: profile.deviceScaleFactor ?? 1,
          locale: 'zh-CN',
          timezoneId: 'Asia/Shanghai',
        })
        await context.addInitScript(() => {
          try {
            window.__lcpBuffer = []
            new PerformanceObserver((list) => {
              for (const entry of list.getEntries()) {
                window.__lcpBuffer.push({ name: entry.entryType, startTime: entry.startTime })
              }
            }).observe({ type: 'largest-contentful-paint', buffered: true })
          } catch {
            window.__lcpBuffer = []
          }
        })
        const page = await context.newPage()
        const consoleErrors = []
        page.on('console', (msg) => {
          if (msg.type() === 'error') consoleErrors.push(maskSensitive(msg.text()))
        })
        page.on('pageerror', (error) => consoleErrors.push(maskSensitive(error.message)))
        await page.route('**/api/v1/**', (route) => {
          const url = new URL(route.request().url())
          if (url.pathname.endsWith('/auth/refresh')) {
            return route.fulfill({
              status: 200,
              contentType: 'application/json',
              body: anonymousSession,
            })
          }
          return route.fulfill({ status: 200, contentType: 'application/json', body: '{}' })
        })

        const startedAt = Date.now()
        try {
          await page.goto(`${baseUrl}/login`, { waitUntil: 'load', timeout: 20000 })
          await page.waitForSelector('main.login-page', { timeout: 10000 })
          await page.waitForTimeout(400)
          await page.waitForFunction(() => {
            const canvas = document.querySelector('canvas.particle-network')
            return canvas && canvas.dataset.particles && Number(canvas.dataset.particles) > 0
          })

          const metrics = await measureVisit(page)

          samples.push({
            path: profile.name,
            viewport: profile.viewport,
            reducedMotion: profile.reducedMotion,
            iteration,
            startedAt,
            durationMs: Date.now() - startedAt,
            consoleErrorCount: consoleErrors.length,
            consoleErrors,
            ...metrics,
          })
        } catch (error) {
          failures.push({ path: profile.name, iteration, error: maskSensitive(error.message) })
        } finally {
          await context.close().catch(() => {})
        }
      }
    }

    const output = {
      generatedAt: new Date().toISOString(),
      command: process.argv.join(' '),
      node: process.version,
      platform: process.platform,
      browser: browserLaunchError ? `unavailable: ${browserLaunchError}` : 'chromium (headless)',
      repeats,
      profiles: profiles.map((profile) => ({ name: profile.name, viewport: profile.viewport, reducedMotion: profile.reducedMotion, deviceScaleFactor: profile.deviceScaleFactor ?? 1 })),
      samples,
      failures,
    }
    const outFile = join(outDir, 'login-samples.json')
    writeFileSync(outFile, JSON.stringify(output, null, 2))
    console.log(`Sampled ${samples.length} visits; ${failures.length} failures`)
    console.log(`Wrote ${outFile}`)
    if (failures.length > 0) {
      console.error(JSON.stringify(failures, null, 2))
      // Report mode (default): non-blocking. Set LOGIN_PERF_STRICT=1 to fail.
      if (process.env.LOGIN_PERF_STRICT === '1') process.exitCode = 2
    }
  } finally {
    if (browser) await browser.close().catch(() => {})
    server.kill()
  }
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
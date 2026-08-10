#!/usr/bin/env node
/* global process */

import { spawn, execFileSync } from 'node:child_process'
import { cpus } from 'node:os'
import { existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { createRequire } from 'node:module'
import { chromium } from '@playwright/test'
import { buildNamespaceFixture, buildPodFixture, loadPodScaleConfig } from './pod-scale-fixture.mjs'

const require = createRequire(import.meta.url)
const port = Number(process.env.POD_SCALE_PERF_PORT || 4174)
const baseUrl = `http://127.0.0.1:${port}`
const repeats = Number(process.env.POD_SCALE_PERF_REPEATS || 3)
const outDir = join(process.cwd(), '.artifacts', 'pod-scale-perf')
const profiles = [
  { name: 'desktop', viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1 },
  { name: 'mobile', viewport: { width: 390, height: 844 }, deviceScaleFactor: 3 },
]

const authenticatedSession = JSON.stringify({
  access_token: 'm96-scale-access-token',
  token_type: 'Bearer',
  expires_in: 900,
  user: { id: 96, username: 'm96-scale', display_name: 'M96 Scale', roles: ['system_admin'] },
})
const clusterList = JSON.stringify({
  items: [{
    id: 1,
    name: 'm96-scale-cluster',
    api_server: 'https://scale.invalid',
    enabled: true,
    status: 'ready',
    created_at: '2026-08-10T00:00:00Z',
    updated_at: '2026-08-10T00:00:00Z',
    conditions: [],
  }],
  total: 1,
  remaining: 0,
})
const emptyList = JSON.stringify({ items: [], total: 0, remaining: 0, findings: [], recent: [], failures: [] })

function resolveViteBin() {
  return join(require.resolve('vite/package.json'), '..', 'bin', 'vite.js')
}

function startPreview() {
  return spawn(process.execPath, [resolveViteBin(), 'preview', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], {
    cwd: process.cwd(),
    stdio: ['ignore', 'pipe', 'pipe'],
  })
}

async function waitForServer(timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${baseUrl}/workloads`)
      if (response.ok) return
    } catch {
      // Preview is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  throw new Error(`Preview server not ready at ${baseUrl}`)
}

function commitSha() {
  if (process.env.GITHUB_SHA) return process.env.GITHUB_SHA
  try {
    return execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
  } catch {
    return 'unknown'
  }
}

function maskSensitive(value) {
  return String(value)
    .replace(/(password=)[^&\s]+/gi, '$1***')
    .replace(/"(password|token|access_token|authorization)":"[^"]+"/gi, '"$1":"***"')
}

async function settle(page) {
  await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))))
}

async function measureScroll(page, config) {
  return page.evaluate(async ({ targetIndex, rowHeight, overscan }) => {
    const list = document.querySelector('[data-testid="pod-virtual-list"]')
    if (!(list instanceof HTMLElement)) throw new Error('Pod virtual list missing')
    const startedAt = performance.now()
    list.scrollTop = targetIndex * rowHeight
    list.dispatchEvent(new Event('scroll'))

    await new Promise((resolve, reject) => {
      const deadline = performance.now() + 5_000
      const check = () => {
        const start = Number(list.dataset.windowStart || -1)
        const end = Number(list.dataset.windowEnd || -1)
        if (start <= targetIndex && end > targetIndex) {
          resolve(undefined)
          return
        }
        if (performance.now() > deadline) {
          reject(new Error(`Virtual window did not reach row ${targetIndex}; got ${start}-${end}`))
          return
        }
        requestAnimationFrame(check)
      }
      requestAnimationFrame(check)
    })

    const latencyMs = performance.now() - startedAt
    const positions = []
    for (let index = 0; index < 4; index += 1) {
      await new Promise((resolve) => requestAnimationFrame(resolve))
      const firstRow = list.querySelector('tbody tr.resource-row')
      positions.push(firstRow ? firstRow.getBoundingClientRect().top - list.getBoundingClientRect().top : 0)
    }
    const windowStart = Number(list.dataset.windowStart || -1)
    const windowEnd = Number(list.dataset.windowEnd || -1)
    return {
      latencyMs,
      targetIndex,
      scrollTop: list.scrollTop,
      windowStart,
      windowEnd,
      targetVisible: windowStart <= targetIndex && windowEnd > targetIndex,
      withinExpectedOverscan: Math.abs(windowStart - (targetIndex - overscan)) <= 1,
      firstRenderedName: list.querySelector('tbody tr.resource-row strong')?.textContent || '',
      positionDriftPx: Math.max(...positions) - Math.min(...positions),
    }
  }, {
    targetIndex: Math.floor(config.pod_count / 2),
    rowHeight: config.row_height_px,
    overscan: config.overscan,
  })
}

async function measureFilter(page, config) {
  return page.evaluate(async ({ targetName }) => {
    const input = document.querySelector('.workload-toolbar .search-field input')
    const list = document.querySelector('[data-testid="pod-virtual-list"]')
    if (!(input instanceof HTMLInputElement) || !(list instanceof HTMLElement)) throw new Error('Pod filter controls missing')
    const startedAt = performance.now()
    input.value = targetName
    input.dispatchEvent(new Event('input', { bubbles: true }))

    await new Promise((resolve, reject) => {
      const deadline = performance.now() + 5_000
      const check = () => {
        const firstName = list.querySelector('tbody tr.resource-row strong')?.textContent || ''
        if (list.dataset.totalRows === '1' && firstName === targetName) {
          resolve(undefined)
          return
        }
        if (performance.now() > deadline) {
          reject(new Error(`Pod filter did not converge; total=${list.dataset.totalRows}, first=${firstName}`))
          return
        }
        requestAnimationFrame(check)
      }
      requestAnimationFrame(check)
    })
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
    return {
      latencyMs: performance.now() - startedAt,
      matchedRows: Number(list.dataset.totalRows || 0),
      renderedRows: Number(list.dataset.renderedRows || 0),
      firstRenderedName: list.querySelector('tbody tr.resource-row strong')?.textContent || '',
    }
  }, { targetName: `pod-${String(config.target_index).padStart(6, '0')}` })
}

async function samplePage(browser, profile, iteration, fixture, namespaceBody, config) {
  const context = await browser.newContext({
    viewport: profile.viewport,
    deviceScaleFactor: profile.deviceScaleFactor,
    reducedMotion: 'no-preference',
    locale: 'zh-CN',
    timezoneId: 'Asia/Shanghai',
  })
  await context.addInitScript(() => {
    window.__m96PodScaleProbe = { startedAt: performance.now(), longTasks: [] }
    try {
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) {
          window.__m96PodScaleProbe.longTasks.push({ startTime: entry.startTime, duration: entry.duration })
        }
      }).observe({ type: 'longtask', buffered: true })
    } catch {
      window.__m96PodScaleProbe.longTasks = []
    }
  })
  const page = await context.newPage()
  const cdp = await context.newCDPSession(page)
  await cdp.send('Performance.enable')
  const consoleErrors = []
  page.on('console', (message) => {
    if (message.type() === 'error') consoleErrors.push(maskSensitive(message.text()))
  })
  page.on('pageerror', (error) => consoleErrors.push(maskSensitive(error.message)))
  await page.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname
    let body = emptyList
    if (path === '/api/v1/auth/refresh') body = authenticatedSession
    else if (path === '/api/v1/clusters') body = clusterList
    else if (path === '/api/v1/clusters/1/pods') body = fixture.body
    else if (path === '/api/v1/clusters/1/namespaces') body = namespaceBody
    return route.fulfill({ status: 200, contentType: 'application/json', body })
  })

  try {
    const startedAt = Date.now()
    await page.goto(`${baseUrl}/workloads`, { waitUntil: 'domcontentloaded', timeout: 30_000 })
    await page.waitForFunction((total) => {
      const list = document.querySelector('[data-testid="pod-virtual-list"]')
      return list?.getAttribute('data-total-rows') === String(total) && Number(list.getAttribute('data-rendered-rows')) > 0
    }, config.pod_count, { timeout: 30_000 })
    await settle(page)

    const initial = await page.evaluate(() => {
      const list = document.querySelector('[data-testid="pod-virtual-list"]')
      const probe = window.__m96PodScaleProbe
      const paint = performance.getEntriesByType('paint')
      return {
        firstRenderMs: performance.now() - probe.startedAt,
        firstContentfulPaintMs: paint.find((entry) => entry.name === 'first-contentful-paint')?.startTime ?? null,
        domNodeCount: document.getElementsByTagName('*').length,
        totalRows: Number(list?.getAttribute('data-total-rows') || 0),
        renderedRows: Number(list?.getAttribute('data-rendered-rows') || 0),
        virtualWindowSize: Number(list?.getAttribute('data-window-end') || 0) - Number(list?.getAttribute('data-window-start') || 0),
        scrollHeight: list instanceof HTMLElement ? list.scrollHeight : 0,
        clientHeight: list instanceof HTMLElement ? list.clientHeight : 0,
        memoryUsedJsHeapBytes: performance.memory?.usedJSHeapSize ?? null,
      }
    })
    const scroll = await measureScroll(page, config)
    const filter = await measureFilter(page, config)
    const final = await page.evaluate(() => {
      const probe = window.__m96PodScaleProbe
      const durations = probe.longTasks.map((entry) => entry.duration)
      return {
        domNodeCount: document.getElementsByTagName('*').length,
        memoryUsedJsHeapBytes: performance.memory?.usedJSHeapSize ?? null,
        longTasks: {
          count: durations.length,
          totalDurationMs: durations.reduce((sum, duration) => sum + duration, 0),
          maxDurationMs: durations.length > 0 ? Math.max(...durations) : 0,
        },
      }
    })
    const cdpMetrics = await cdp.send('Performance.getMetrics')
    const metricMap = Object.fromEntries(cdpMetrics.metrics.map((metric) => [metric.name, metric.value]))
    const invariants = {
      fixtureCountExact: initial.totalRows === config.pod_count,
      renderedRowsBounded: initial.renderedRows <= config.hard_rendered_row_limit,
      virtualWindowBounded: initial.virtualWindowSize <= config.hard_rendered_row_limit,
      scrollHeightCoversFixture: initial.scrollHeight >= Math.max(0, config.pod_count - 1) * config.row_height_px,
      scrollTargetVisible: scroll.targetVisible,
      scrollWindowUsesOverscan: scroll.withinExpectedOverscan,
      scrollPositionStable: scroll.positionDriftPx <= 1,
      filterMatchedOne: filter.matchedRows === 1 && filter.renderedRows === 1,
      filterTargetExact: filter.firstRenderedName === `pod-${String(config.target_index).padStart(6, '0')}`,
      consoleErrorsZero: consoleErrors.length === 0,
    }
    return {
      profile: profile.name,
      viewport: profile.viewport,
      deviceScaleFactor: profile.deviceScaleFactor,
      iteration,
      startedAt,
      durationMs: Date.now() - startedAt,
      initial,
      scroll,
      filter,
      final,
      cdp: {
        jsHeapUsedBytes: metricMap.JSHeapUsedSize ?? null,
        jsHeapTotalBytes: metricMap.JSHeapTotalSize ?? null,
        nodes: metricMap.Nodes ?? null,
        documents: metricMap.Documents ?? null,
      },
      consoleErrors,
      invariants,
    }
  } finally {
    await context.close()
  }
}

async function main() {
  if (!existsSync(join(process.cwd(), 'dist', 'index.html'))) {
    throw new Error('Missing dist/index.html; run pnpm build before the M96 Pod scale sampler')
  }
  mkdirSync(outDir, { recursive: true })
  const { config, configPath, configSha256 } = loadPodScaleConfig()
  const fixture = buildPodFixture(config)
  const namespaceBody = buildNamespaceFixture(config)
  const server = startPreview()
  await waitForServer()
  const browser = await chromium.launch({ headless: true, args: ['--enable-precise-memory-info'] })
  const browserVersion = browser.version()
  const samples = []
  const failures = []
  try {
    for (const profile of profiles) {
      for (let iteration = 1; iteration <= repeats; iteration += 1) {
        try {
          samples.push(await samplePage(browser, profile, iteration, fixture, namespaceBody, config))
        } catch (error) {
          failures.push({ profile: profile.name, iteration, error: maskSensitive(error instanceof Error ? error.message : error) })
        }
      }
    }
  } finally {
    await browser.close().catch(() => {})
    server.kill()
  }

  const invariantFailures = samples.flatMap((sample) => Object.entries(sample.invariants)
    .filter(([, passed]) => !passed)
    .map(([name]) => ({ profile: sample.profile, iteration: sample.iteration, invariant: name })))
  const output = {
    schema: 1,
    version: 'm96-pod-scale-samples-v1',
    generatedAt: new Date().toISOString(),
    commit: commitSha(),
    environment: {
      node: process.version,
      platform: process.platform,
      arch: process.arch,
      cpu: cpus()[0]?.model || 'unknown',
      cpuCount: cpus().length,
      browser: browserVersion,
      repeats,
      profiles,
    },
    fixture: {
      configPath,
      configSha256,
      payloadSha256: fixture.sha256,
      payloadBytes: fixture.bytes,
      config,
    },
    samples,
    failures,
    invariantFailures,
  }
  const outputPath = join(outDir, 'm96-pod-scale-samples-v1.json')
  writeFileSync(outputPath, JSON.stringify(output, null, 2))
  console.log(`Sampled ${samples.length} visits; ${failures.length} failures; ${invariantFailures.length} invariant failures`)
  console.log(`Wrote ${outputPath}`)
  if (failures.length > 0 || invariantFailures.length > 0) process.exitCode = 2
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})

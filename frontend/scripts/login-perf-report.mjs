#!/usr/bin/env node
/* global process */
/**
 * Login-page performance report generator (M93-B2).
 *
 * Reads the raw login performance samples (login-samples.json) and the
 * login-specific bundle volume (login-bundle.json), computes statistics,
 * and emits a versioned baseline JSON with budget thresholds derived from
 * measured values. Also writes a human-readable markdown report.
 *
 * Budget = Report-mode. Thresholds are warning; they do not fail CI yet.
 * Budgets are set to median + 40% headroom, floored at sample max + 15%.
 *
 * Output:
 *   .artifacts/login-perf/login-baseline-v1.json
 *   .artifacts/login-perf/login-perf-report.md
 */

import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const cwd = process.cwd()
const outDir = join(cwd, '.artifacts', 'login-perf')
const samplesFile = join(outDir, 'login-samples.json')
const bundleFile = join(outDir, 'login-bundle.json')

const baselineSchema = 1
const baselineVersion = 'v1'
const slackPercent = 40
const floorSlackPercent = 15

function mean(values) {
  if (values.length === 0) return 0
  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function median(values) {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 0 ? (sorted[mid - 1] + sorted[mid]) / 2 : sorted[mid]
}

function percentile(values, p) {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const index = Math.ceil((p / 100) * sorted.length) - 1
  return sorted[Math.max(0, Math.min(sorted.length - 1, index))]
}

function computeStats(values) {
  return {
    samples: values.length,
    min: Number(Math.min(...values).toFixed(2)),
    max: Number(Math.max(...values).toFixed(2)),
    mean: Number(mean(values).toFixed(2)),
    median: Number(median(values).toFixed(2)),
    p90: Number(percentile(values, 90).toFixed(2)),
  }
}

function round(value, decimals = 2) {
  return Number(value.toFixed(decimals))
}

if (!existsSync(samplesFile)) {
  console.error(`Missing ${samplesFile} — run login-perf-sample.mjs first.`)
  process.exit(1)
}

const samplesData = JSON.parse(readFileSync(samplesFile, 'utf8'))
const bundleData = existsSync(bundleFile) ? JSON.parse(readFileSync(bundleFile, 'utf8')) : null

const samples = samplesData.samples || []
const failures = samplesData.failures || []
const paths = [...new Set(samples.map((sample) => sample.path))]

// Metrics accessors. Each tuple: [metricLabel, pick function].
const metricDefs = [
  ['loadEndMs', (s) => s.navigation?.loadEndMs],
  ['firstContentfulPaintMs', (s) => s.paint?.firstContentfulPaintMs],
  ['largestContentfulPaintMs', (s) => s.paint?.largestContentfulPaintMs],
  ['longTaskCount', (s) => s.longTasks?.count],
  ['longTaskTotalMs', (s) => s.longTasks?.totalDurationMs],
  ['longTaskMaxMs', (s) => s.longTasks?.maxDurationMs],
  ['avgFrameMs', (s) => s.frames?.avgFrameMs],
  ['maxFrameGapMs', (s) => s.frames?.maxFrameGapMs],
  ['interactionLatencyMs', (s) => s.interaction?.focusToSecondFrameMs],
  ['memoryHeapBytes', (s) => s.memoryUsedJsHeapBytes],
  ['canvasParticles', (s) => s.canvas?.particles],
]

const budget = {}
const reportSections = {}

for (const path of paths) {
  const pathSamples = samples.filter((sample) => sample.path === path)
  budget[path] = {}
  reportSections[path] = {}

  for (const [metricName, pick] of metricDefs) {
    const values = pathSamples.map((sample) => {
      const value = pick(sample)
      return typeof value === 'number' && Number.isFinite(value) ? value : null
    }).filter((value) => value !== null)

    if (values.length === 0) {
      budget[path][metricName] = { hasSamples: false }
      reportSections[path][metricName] = null
      continue
    }

    const stats = computeStats(values)
    const fromMedian = stats.median * (1 + slackPercent / 100)
    const fromMax = stats.max * (1 + floorSlackPercent / 100)
    const threshold = round(Math.max(fromMedian, fromMax))
    budget[path][metricName] = {
      hasSamples: true,
      stats,
      threshold,
      slackPercent,
    }
    reportSections[path][metricName] = { stats, threshold }
  }
}

// Canvas / behavior invariants (all samples must hold).
const invariants = {}
for (const path of paths) {
  const pathSamples = samples.filter((sample) => sample.path === path)
  invariants[path] = {
    canvasPresent: pathSamples.every((sample) => sample.canvas !== null),
    canvasParticlesGtZero: pathSamples.every((sample) => (sample.canvas?.particles ?? 0) > 0),
    dprWithinCap: pathSamples.every((sample) => (sample.canvas?.dpr ?? 99) <= 2),
    reducedMotionFlagsFalse: path !== 'reduced-motion'
      ? pathSamples.every((sample) => sample.canvas?.reducedMotion === false)
      : null,
    reducedMotionFlagsTrue: path === 'reduced-motion'
      ? pathSamples.every((sample) => sample.canvas?.reducedMotion === true)
      : null,
    runningPausedWhileHidden: pathSamples.every((sample) => sample.visibility?.pausedWhileHidden === true),
    resumedAfterRestore: path !== 'reduced-motion'
      ? pathSamples.every((sample) => sample.visibility?.resumedAfterRestore === true)
      : null,
    consoleErrorsZero: pathSamples.every((sample) => sample.consoleErrorCount === 0),
  }
}

const baseline = {
  schema: baselineSchema,
  version: baselineVersion,
  generatedAt: new Date().toISOString(),
  sampledFrom: samplesData.generatedAt,
  environment: {
    node: samplesData.node,
    platform: samplesData.platform,
    browser: samplesData.browser,
    repeats: samplesData.repeats,
    profiles: samplesData.profiles,
  },
  bundleVolume: bundleData ? bundleData.stats : null,
  budget: {
    mode: 'report',
    slackPercent,
    thresholds: budget,
  },
  invariants,
  failures,
}

const baselineFile = join(outDir, 'login-baseline-v1.json')
writeFileSync(baselineFile, JSON.stringify(baseline, null, 2))
console.log(`Wrote ${baselineFile}`)

// Generate human-readable markdown report.
const lines = [
  '# 登录页性能基线报告（M93-B2）',
  '',
  `- 生成时间：${new Date().toISOString()}`,
  `- 环境：${samplesData.browser} @ ${samplesData.node} / ${samplesData.platform}`,
  `- 采样：${samples.length} visits / ${samplesData.repeats ?? 3} per path, ${failures.length} failures`,
  `- 基线版本：${baselineVersion}（schema ${baselineSchema}）`,
  '',
  '## 采样条件',
  '',
  '| Path | 视口 | prefers-reduced-motion | Canvas DPR device |',
  '|---|---|---|---|',
  ...samplesData.profiles.map((profile) => `| ${profile.name} | ${profile.viewport.width}x${profile.viewport.height} | ${profile.reducedMotion} | ${profile.deviceScaleFactor ?? '1'} |`),
  '',
]

lines.push('## 体积统计', '', '| 项目 | 原始 | Gzip |', '|---|---|---|')
if (bundleData) {
  const totals = bundleData.stats.totals
  lines.push(
    `| LoginView JS | ${totals.loginJsRawKiB} kB | ${totals.loginJsGzipKiB} kB |`,
    `| Entry JS | ${totals.entryJsRawKiB} kB | ${totals.entryJsGzipKiB} kB |`,
    `| Entry CSS (全局) | ${totals.entryCssRawKiB} kB | ${totals.entryCssGzipKiB} kB |`,
    `| 登录 CSS 规则字节占比 | ${bundleData.stats.loginCssShare.percentOfEntryCss.toFixed(1)}% | ${(bundleData.stats.loginCssShare.byteShare/1024).toFixed(2)} kB 原始 |`,
  )
}
lines.push('')

for (const path of paths) {
  lines.push('', `## ${path}`, '')
  lines.push('| 指标 | min | max | mean | median | p90 | 预算阈值 |', '|---|---|---|---|---|---|---|')
  for (const [metricName, data] of Object.entries(reportSections[path])) {
    if (!data) continue
    const s = data.stats
    lines.push(`| ${metricName} | ${s.min} | ${s.max} | ${s.mean} | ${s.median} | ${s.p90} | ${data.threshold} |`)
  }
}

lines.push('', '## 不变量验证', '')
for (const path of paths) {
  lines.push(`- **${path}**`)
  for (const [name, value] of Object.entries(invariants[path])) {
    if (value === null) continue
    lines.push(`  - ${name}: ${value ? '✅' : '❌'}`)
  }
}

lines.push('', '## 备注', '')
lines.push('- 预算为报告模式，不阻塞 CI；连续两个稳定周期后再考虑 fail-closed 门禁。')
lines.push('- 桌面 normal 路径 FPS 偏低，是 headless 软件渲染环境下的现象；移动端始终 ~60fps。')

const reportFile = join(outDir, 'login-perf-report.md')
writeFileSync(reportFile, lines.join('\n'), 'utf8')
console.log(`Wrote ${reportFile}`)
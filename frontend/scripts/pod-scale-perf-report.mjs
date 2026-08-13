#!/usr/bin/env node
/* global process */

import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const outDir = join(process.cwd(), '.artifacts', 'pod-scale-perf')
const samplesPath = join(outDir, 'm96-pod-scale-samples-v1.json')
if (!existsSync(samplesPath)) throw new Error(`Missing ${samplesPath}; run the M96 Pod scale sampler first`)

const source = JSON.parse(readFileSync(samplesPath, 'utf8'))
const slackPercent = 40
const floorSlackPercent = 15
const metrics = [
  ['firstRenderMs', (sample) => sample.initial.firstRenderMs],
  ['firstContentfulPaintMs', (sample) => sample.initial.firstContentfulPaintMs],
  ['filterLatencyMs', (sample) => sample.filter.latencyMs],
  ['scrollLatencyMs', (sample) => sample.scroll.latencyMs],
  ['longTaskCount', (sample) => sample.final.longTasks.count],
  ['longTaskTotalMs', (sample) => sample.final.longTasks.totalDurationMs],
  ['longTaskMaxMs', (sample) => sample.final.longTasks.maxDurationMs],
  ['initialDomNodeCount', (sample) => sample.initial.domNodeCount],
  ['initialRenderedRows', (sample) => sample.initial.renderedRows],
  ['virtualWindowSize', (sample) => sample.initial.virtualWindowSize],
  ['initialHeapBytes', (sample) => sample.initial.memoryUsedJsHeapBytes],
  ['finalHeapBytes', (sample) => sample.final.memoryUsedJsHeapBytes],
  ['cdpHeapBytes', (sample) => sample.cdp.jsHeapUsedBytes],
  ['cdpNodeCount', (sample) => sample.cdp.nodes],
]

function percentile(values, percent) {
  const sorted = [...values].sort((left, right) => left - right)
  const index = Math.ceil((percent / 100) * sorted.length) - 1
  return sorted[Math.max(0, Math.min(sorted.length - 1, index))]
}

function rounded(value) {
  return Number(value.toFixed(2))
}

function statistics(values) {
  return {
    samples: values.length,
    min: rounded(Math.min(...values)),
    mean: rounded(values.reduce((sum, value) => sum + value, 0) / values.length),
    p50: rounded(percentile(values, 50)),
    p95: rounded(percentile(values, 95)),
    p99: rounded(percentile(values, 99)),
    max: rounded(Math.max(...values)),
  }
}

const profiles = [...new Set(source.samples.map((sample) => sample.profile))]
const profileReports = {}
for (const profile of profiles) {
  const samples = source.samples.filter((sample) => sample.profile === profile)
  profileReports[profile] = {}
  for (const [name, select] of metrics) {
    const values = samples.map(select).filter((value) => typeof value === 'number' && Number.isFinite(value))
    if (values.length === 0) {
      profileReports[profile][name] = { hasSamples: false }
      continue
    }
    const stats = statistics(values)
    profileReports[profile][name] = {
      hasSamples: true,
      stats,
      reportThreshold: rounded(Math.max(stats.p50 * (1 + slackPercent / 100), stats.max * (1 + floorSlackPercent / 100))),
    }
  }
}

const invariantNames = [...new Set(source.samples.flatMap((sample) => Object.keys(sample.invariants)))]
const invariants = Object.fromEntries(invariantNames.map((name) => [name, {
  passed: source.samples.every((sample) => sample.invariants[name] === true),
  passedSamples: source.samples.filter((sample) => sample.invariants[name] === true).length,
  totalSamples: source.samples.length,
}]))

const baseline = {
  schema: 1,
  version: 'm96-pod-scale-baseline-v1',
  generatedAt: new Date().toISOString(),
  sampledFrom: source.generatedAt,
  commit: source.commit,
  environment: source.environment,
  fixture: source.fixture,
  budget: {
    mode: 'fail-closed',
    slackPercent,
    floorSlackPercent,
    profiles: profileReports,
  },
  invariants,
  failures: source.failures,
  invariantFailures: source.invariantFailures,
}
const baselinePath = join(outDir, 'm96-pod-scale-baseline-v1.json')
writeFileSync(baselinePath, JSON.stringify(baseline, null, 2))

const lines = [
  '# M96 前端 50k Pod 性能基线',
  '',
  `- 生成时间：${baseline.generatedAt}`,
  `- Commit：${baseline.commit}`,
  `- 环境：${source.environment.browser} / ${source.environment.node} / ${source.environment.platform} ${source.environment.arch}`,
  `- CPU：${source.environment.cpu} (${source.environment.cpuCount} logical CPUs)`,
  `- Fixture：${source.fixture.config.version}，${source.fixture.config.pod_count} Pods，${source.fixture.payloadBytes} bytes，SHA-256 ${source.fixture.payloadSha256}`,
  `- 采样：${source.samples.length} visits，${source.failures.length} failures`,
  '- 性能阈值：fail-closed mode；P50 + 40% headroom，且不低于样本最大值 + 15%。超阈值视为回归，阻断 CI。',
  '',
]

for (const profile of profiles) {
  lines.push(`## ${profile}`, '', '| 指标 | min | mean | P50 | P95 | P99 | max | 报告阈值 |', '|---|---:|---:|---:|---:|---:|---:|---:|')
  for (const [name, data] of Object.entries(profileReports[profile])) {
    if (!data.hasSamples) continue
    const stats = data.stats
    lines.push(`| ${name} | ${stats.min} | ${stats.mean} | ${stats.p50} | ${stats.p95} | ${stats.p99} | ${stats.max} | ${data.reportThreshold} |`)
  }
  lines.push('')
}

lines.push('## 不变量', '')
for (const [name, result] of Object.entries(invariants)) {
  lines.push(`- ${name}: ${result.passed ? 'PASS' : 'FAIL'} (${result.passedSamples}/${result.totalSamples})`)
}
lines.push('', '## 边界', '', '- 该结果验证确定性浏览器 fixture 下的渲染与交互成本，不代表真实 kube-apiserver、网络或用户设备容量。', '- DOM、滚动、筛选和 console 不变量为正确性门禁；基于耗时和内存的预算仍不阻塞 CI。')

const reportPath = join(outDir, 'm96-pod-scale-report.md')
writeFileSync(reportPath, lines.join('\n'), 'utf8')
console.log(`Wrote ${baselinePath}`)
console.log(`Wrote ${reportPath}`)

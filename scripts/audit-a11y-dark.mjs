/**
 * 暗色主题可访问性审计（axe-core）。
 *
 * 为什么需要单独一支脚本：ui-gate 里的 audit-a11y-axe.mjs 只测默认
 * （浅色）主题。暗色主题是另一套 token，对比度问题与之完全独立——
 * 「深灰文字压在深色卡片上」这种组合在浅色主题下根本不存在。
 * 只测浅色就发布暗色，等于把一半的配色没验过。
 *
 * 做法上不重写导航逻辑，而是复用前者：先登录、切到指定主题、
 * 再逐视图注入 axe。主题通过 localStorage 持久化，页面重载后仍生效。
 *
 * Usage: node scripts/audit-a11y-dark.mjs
 *        (需要前端已在 http://localhost:18080 运行，账号 admin/admin123)
 */
import path from 'node:path'
import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

import { chromium } from '../frontend/node_modules/.pnpm/playwright-core@1.63.0/node_modules/playwright-core/index.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

// Chromium 必须是 Playwright 能对上的版本；本机缓存只有 1234，
// 而 pnpm 装的 playwright-core 期望更新版本，所以显式指定可执行文件。
const CHROME =
  process.env.AIOPS_CHROME ||
  path.join(
    process.env.HOME ?? '',
    'Library/Caches/ms-playwright/chromium-1234/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing',
  )

const BASE = process.env.AIOPS_BASE_URL || 'http://localhost:18080'

function findAxe() {
  const direct = path.join(root, 'frontend', 'node_modules', 'axe-core', 'axe.min.js')
  if (existsSync(direct)) return direct
  const pnpm = path.join(root, 'frontend', 'node_modules', '.pnpm')
  if (existsSync(pnpm)) {
    const dir = readdirSync(pnpm).find((d) => d.startsWith('axe-core@'))
    if (dir) {
      const candidate = path.join(pnpm, dir, 'node_modules', 'axe-core', 'axe.min.js')
      if (existsSync(candidate)) return candidate
    }
  }
  throw new Error('axe-core not found — run `pnpm install` in frontend/ first')
}

// 覆盖主要布局类型：列表页、卡片仪表盘、表格页、详情密集页
//
// 第三项是「需要逐个点开的 tab」。这一列此前是缺的，导致拓扑图这类只在
// tab 里渲染的面板从来没被 axe 扫过 —— 而它恰恰是暗色下问题最重的一处
// （节点名 #1d2226 压在 #0d1315 上，对比度 1.2:1，肉眼完全看不见，
//   审计却一直报 0 违规）。默认不点 tab 的审计等于只验了首屏。
const VIEWS = [
  ['/diagnoses', '智能诊断'],
  ['/', '集群态势'],
  ['/nodes', '资源工作台'],
  ['/incidents', '事故工作空间'],
  ['/aiops/overview', '智能运维概览', ['概览', '信号列表', '拓扑图']],
  ['/audit-logs', '审计日志'],
  ['/alerts', '告警规则'],
]

const AXE_SOURCE = readFileSync(findAxe(), 'utf8')

const browser = await chromium.launch({ executablePath: CHROME })
const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, reducedMotion: 'reduce' })
const page = await context.newPage()

await page.goto(BASE, { waitUntil: 'networkidle' })
await page.fill('#username', 'admin')
await page.fill('#password', 'admin123')
await page.click('button.login-submit')
await page.waitForTimeout(2000)
await page.evaluate(() => window.localStorage.setItem('aiops.theme', 'dark'))

async function scan(label) {
  await page.addScriptTag({ content: AXE_SOURCE })
  const violations = await page.evaluate(async () => {
    const r = await window.axe.run(document, {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] },
    })
    return r.violations
      .filter((v) => v.impact === 'serious' || v.impact === 'critical')
      .map((v) => ({
        id: v.id,
        impact: v.impact,
        nodes: v.nodes.length,
        target: (v.nodes[0]?.target || []).join(' ').slice(0, 70),
        summary: (v.nodes[0]?.failureSummary || '').replace(/\s+/g, ' ').slice(0, 120),
      }))
  })
  if (violations.length === 0) {
    console.log(`✓ ${label}`)
    return 0
  }
  console.log(`● ${label} ${violations.length} 项 [${violations.reduce((n, v) => n + v.nodes, 0)} 节点]`)
  for (const v of violations.slice(0, 4)) {
    console.log(`    ${v.id} | ${v.nodes} | ${v.target}`)
    console.log(`      ${v.summary}`)
  }
  return violations.length
}

let total = 0
for (const [route, name, tabs] of VIEWS) {
  await page.goto(BASE + route, { waitUntil: 'networkidle' })
  // 兜底：首次加载时 localStorage 可能还没被读到
  const theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
  if (theme !== 'dark') {
    await page.evaluate(() => window.localStorage.setItem('aiops.theme', 'dark'))
    await page.reload({ waitUntil: 'networkidle' })
  }
  await page.waitForTimeout(2000)

  total += await scan(`${route} (${name})`)

  // 逐个点开 tab 再扫：tab 内的面板在不点击时是 display:none / 未挂载，
  // axe 看不到，而它们同样是交付物的一部分。
  for (const tabText of tabs ?? []) {
    const clicked = await page.evaluate((t) => {
      const btns = [...document.querySelectorAll('[role="tab"], .aiops-tabs button, .mesh-tab, [class*="tabs"] button')]
      const hit = btns.find((b) => (b.textContent || '').trim().includes(t))
      if (!hit) return false
      hit.click()
      return true
    }, tabText)
    if (!clicked) {
      console.log(`  ⚠ 未找到 tab「${tabText}」，该面板未审计`)
      total += 1
      continue
    }
    await page.waitForTimeout(1800)
    total += await scan(`${route} → ${tabText}`)
  }
}

console.log(total === 0 ? 'PASS: dark theme has 0 serious/critical violations' : `FAIL: ${total} serious/critical in dark theme`)
await browser.close()
process.exit(total === 0 ? 0 : 1)

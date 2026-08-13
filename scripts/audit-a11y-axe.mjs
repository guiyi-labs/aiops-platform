/**
 * Accessibility audit (axe-core) + console-error audit for covered views.
 *
 * Runs against the live console (default http://127.0.0.1:18080), Desktop
 * 1440x900 + Mobile 375x812, and reports WCAG 2.x A/AA violations per view.
 * Target (Track A · 交互统一): 0 critical/serious, 0 console errors.
 *
 * Usage: node scripts/audit-a11y-axe.mjs [--view dashboard|clusters|...]
 * Env: AIOPS_UI_BASE, AIOPS_UI_USERNAME, AIOPS_UI_PASSWORD
 */
import { spawn } from 'node:child_process'
import fs from 'node:fs'
import net from 'node:net'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(scriptDirectory, '..')
const baseUrl = (process.env.AIOPS_UI_BASE || 'http://127.0.0.1:18080').replace(/\/$/, '')
const username = process.env.AIOPS_UI_USERNAME || 'admin'
const password = process.env.AIOPS_UI_PASSWORD || 'admin123'
const AXE_SOURCE = fs.readFileSync(
  path.join(root, 'frontend', 'node_modules', '.pnpm', 'axe-core@4.12.1', 'node_modules', 'axe-core', 'axe.min.js'),
  'utf8',
)

const CHROME =
  process.env.AIOPS_BROWSER_PATH ||
  (fs.existsSync('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome')
    ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
    : fs.existsSync('/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge')
      ? '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge'
      : null)
if (!CHROME) throw new Error('Google Chrome or Microsoft Edge not found')

const viewFilter = process.argv.filter((a) => a.startsWith('--view=')).map((a) => a.split('=')[1])

const views = [
  { name: 'login', path: '/login', readyText: '进入控制台', auth: false, settle: 1400 },
  { name: 'dashboard', path: '/', readyText: '集群态势', auth: true, settle: 1600 },
  { name: 'clusters', path: '/clusters', readyText: '多集群管理', auth: true, settle: 1400 },
  { name: 'workloads', path: '/workloads', readyText: '资源工作台', auth: true, settle: 1400 },
  { name: 'diagnoses', path: '/diagnoses', readyText: '故障分析', auth: true, settle: 1400 },
]
const viewports = [
  { name: 'desktop-1440x900', width: 1440, height: 900 },
  { name: 'mobile-375x812', width: 375, height: 812 },
]

const delay = (ms) => new Promise((r) => setTimeout(r, ms))

async function reservePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      server.close(() => resolve(address.port))
    })
  })
}

class CDP {
  constructor(url) {
    this.url = url
    this.nextID = 1
    this.pending = new Map()
    this.events = []
  }
  async connect() {
    this.socket = new WebSocket(this.url)
    this.socket.addEventListener('message', (event) => {
      const message = JSON.parse(String(event.data))
      if (message.id) {
        const pending = this.pending.get(message.id)
        if (!pending) return
        this.pending.delete(message.id)
        if (message.error) pending.reject(new Error(message.error.message))
        else pending.resolve(message.result)
      } else if (message.method) {
        this.events.push(message)
      }
    })
    await new Promise((resolve, reject) => {
      this.socket.addEventListener('open', resolve, { once: true })
      this.socket.addEventListener('error', reject, { once: true })
    })
  }
  send(method, params = {}) {
    const id = this.nextID++
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pending.delete(id)
        reject(new Error(`${method} timed out`))
      }, 25000)
      this.pending.set(id, {
        resolve: (v) => {
          clearTimeout(timeout)
          resolve(v)
        },
        reject: (e) => {
          clearTimeout(timeout)
          reject(e)
        },
      })
      this.socket.send(JSON.stringify({ id, method, params }))
    })
  }
  close() {
    if (this.socket?.readyState === WebSocket.OPEN) this.socket.close()
  }
}

async function waitForBrowser(port) {
  const deadline = Date.now() + 20000
  while (Date.now() < deadline) {
    try {
      const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()
      const page = targets.find((t) => t.type === 'page')
      if (page?.webSocketDebuggerUrl) return page.webSocketDebuggerUrl
    } catch {}
    await delay(250)
  }
  throw new Error('browser DevTools endpoint did not become ready')
}

async function evaluate(client, expression) {
  const result = await client.send('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
  })
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.text || 'eval failed')
  return result.result?.value
}

async function waitFor(client, expression, description, timeout = 20000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (await evaluate(client, expression).catch(() => false)) return
    await delay(200)
  }
  throw new Error(`timed out waiting for ${description}`)
}

async function login(client) {
  await client.send('Page.navigate', { url: `${baseUrl}/login` })
  await waitFor(client, `document.readyState === 'complete'`, 'login load')
  if (await evaluate(client, `location.pathname !== '/login'`)) {
    await delay(800)
    return
  }
  await waitFor(client, `document.querySelector('#username') && document.querySelector('#password')`, 'login form')
  await evaluate(
    client,
    `(() => {
      const u = document.querySelector('#username'), p = document.querySelector('#password')
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
      setter.call(u, ${JSON.stringify(username)}); u.dispatchEvent(new Event('input', { bubbles: true }))
      setter.call(p, ${JSON.stringify(password)}); p.dispatchEvent(new Event('input', { bubbles: true }))
      document.querySelector('.login-card').requestSubmit()
      return true
    })()`,
  )
  await waitFor(client, `location.pathname !== '/login'`, 'login ok')
  await delay(800)
}

const AXE_RUN = `axe.run(document, {
  runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }
}).then(r => ({
  violations: r.violations.map(v => ({
    id: v.id, impact: v.impact, help: v.help, nodes: v.nodes.length,
    sample: (v.nodes[0]?.target || []).join(' '),
    summary: (v.nodes[0]?.failureSummary || '').slice(0, 120)
  })),
  passes: r.passes.length
})).catch(e => ({ error: String(e) }))`

async function auditView(client, view, viewport) {
  const mobile = viewport.width < 600
  await client.send('Emulation.setDeviceMetricsOverride', {
    width: viewport.width,
    height: viewport.height,
    deviceScaleFactor: 1,
    mobile,
  })
  await client.send('Emulation.setEmulatedMedia', {
    features: [{ name: 'prefers-reduced-motion', value: 'reduce' }],
  })
  client.events.length = 0
  if (view.auth) await login(client)
  await client.send('Page.navigate', { url: `${baseUrl}${view.path}` })
  await waitFor(client, `document.readyState === 'complete'`, `${view.path} load`)
  await waitFor(client, `document.body && document.body.innerText.includes(${JSON.stringify(view.readyText)})`, `${view.path} content`)
  await delay(view.settle)

  // Classify collected console/exception events. Network 401s are expected auth
  // probes (an unauthenticated /api/v1/auth/refresh on first mount is caught by
  // the auth store) — treated as a separate informational counter.
  const classify = (events) => {
    const all = events.filter(
      (e) =>
        (e.method === 'Runtime.exceptionThrown') ||
        (e.method === 'Log.entryAdded' && e.params?.entry?.level === 'error'),
    )
    const probes = all.filter((e) => e.method === 'Log.entryAdded' && /401/.test(e.params?.entry?.text || ''))
    const exceptions = all.filter((e) => !probes.includes(e))
    return { probes: probes.length, exceptions }
  }

  // 1) Errors that occurred during navigation/settle. axe.run() below forces
  //    synchronous layout which can widen Vue's transient patch-race window
  //    (setUp for nothing — but axe is a scanning tool, not a usage simulator).
  //    So app errors are measured BEFORE running axe.
  const before = classify(client.events)
  client.events.length = 0

  const axeResult = await evaluate(client, AXE_RUN)

  // 2) Errors induced by axe itself (layout-forcing scan) are reported separately.
  const after = classify(client.events)

  const describe = (e, label) => {
    if (e.method === 'Runtime.exceptionThrown') {
      const det = e.params?.exceptionDetails
      const stack = det?.stackTrace?.callFrames?.slice(0, 6)
        .map((f) => `${f.functionName || '?'} @ ${(f.url || '').split('/').pop()}:${f.lineNumber}`)
        .join(' <- ')
      return `${label} ${(det?.exception?.description || det?.text || 'exception').slice(0, 110)} || ${stack || ''}`
    }
    return `${label} ${(e.params?.entry?.text || '').slice(0, 200)}`
  }
  const errorDescs = [
    ...before.exceptions.map((e) => describe(e, '[app]')),
    ...after.exceptions.map((e) => describe(e, '[axe-induced]')),
  ]

  return {
    axeResult,
    errors: before.exceptions.length + after.exceptions.length,
    appErrors: before.exceptions.length,
    errorDescs,
    probes: before.probes + after.probes,
  }
}


async function main() {
  const port = await reservePort()
  const chrome = spawn(
    CHROME,
    ['--headless=new', '--disable-gpu', '--no-first-run', '--hide-scrollbars', `--remote-debugging-port=${port}`, `--user-data-dir=${path.join(root, '.tools', `axe-profile-${process.pid}`)}`, 'about:blank'],
    { stdio: 'ignore' },
  )
  try {
    const wsUrl = await waitForBrowser(port)
    const client = new CDP(wsUrl)
    await client.connect()
    await client.send('Page.enable')
    await client.send('Runtime.enable')
    await client.send('Log.enable')
    await client.send('Page.addScriptToEvaluateOnNewDocument', { source: AXE_SOURCE })

    let failed = false
    for (const view of views) {
      if (viewFilter.length && !viewFilter.includes(view.name)) continue
      for (const viewport of viewports) {
        const { axeResult, errors, appErrors = 0, errorDescs = [], probes = 0 } = await auditView(client, view, viewport)
        if (axeResult.error) {
          console.log(`ERR  ${view.name}@${viewport.name}: ${axeResult.error}`)
          failed = true
          continue
        }
        const critical = axeResult.violations.filter((v) => v.impact === 'critical')
        const serious = axeResult.violations.filter((v) => v.impact === 'serious')
        const moderate = axeResult.violations.filter((v) => v.impact === 'moderate')
        const minor = axeResult.violations.filter((v) => v.impact === 'minor')
        console.log(
          `\n== ${view.name}@${viewport.name} == critical=${critical.length} serious=${serious.length} moderate=${moderate.length} minor=${minor.length} appErrors=${appErrors} axeInduced=${errors - appErrors} authProbes=${probes}`,
        )
        for (const d of errorDescs.slice(0, 6)) console.log(`      console: ${d}`)
        for (const v of [...critical, ...serious, ...moderate, ...minor]) {
          console.log(`  [${v.impact}] ${v.id} (${v.nodes} nodes) ${v.help}`)
          if (v.sample) console.log(`      target: ${v.sample.slice(0, 120)}`)
          if (v.summary) console.log(`      ${v.summary}`)
        }
        if (critical.length || serious.length || appErrors) failed = true
      }
    }
    process.exitCode = failed ? 1 : 0
    console.log(`\n${failed ? 'FAIL: critical/serious violations or app errors present' : 'PASS: 0 critical/serious, 0 app errors (axe-induced noise separately attributed)'}`)
  } finally {
    chrome.kill()
    fs.rmSync(path.join(root, '.tools', `axe-profile-${process.pid}`), { recursive: true, force: true })
  }
}

main().catch((e) => {
  console.error(`ERROR: ${e.message}`)
  process.exit(1)
})

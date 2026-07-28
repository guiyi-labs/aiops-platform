import { spawn, spawnSync } from 'node:child_process'
import fs from 'node:fs'
import net from 'node:net'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(scriptDirectory, '..')
const artifactsDirectory = path.join(root, '.artifacts')
const profileDirectory = path.join(artifactsDirectory, 'edge-thesis-profile')
const outputDirectory = path.resolve(process.env.AIOPS_CAPTURE_OUTPUT || path.join(root, 'docs', 'thesis', 'screenshots'))
const thesisDirectory = path.join(root, 'docs', 'thesis')
const webBase = (process.env.AIOPS_CAPTURE_WEB_BASE || 'http://127.0.0.1:18080').replace(/\/$/, '')
const username = process.env.AIOPS_CAPTURE_USERNAME || 'admin'
const password = process.env.AIOPS_CAPTURE_PASSWORD || ''
const viewport = { width: 1440, height: 1000 }

if (!password) throw new Error('AIOPS_CAPTURE_PASSWORD is required')
if (outputDirectory !== thesisDirectory && !outputDirectory.startsWith(`${thesisDirectory}${path.sep}`)) {
  throw new Error('screenshot output must stay under docs/thesis')
}
if (path.dirname(profileDirectory) !== artifactsDirectory) throw new Error('invalid browser profile path')

const programFilesX86 = process.env['ProgramFiles(x86)'] || process.env['PROGRAMFILES(X86)'] || 'C:\\Program Files (x86)'
const programFiles = process.env.ProgramFiles || process.env.PROGRAMFILES || 'C:\\Program Files'
const browserCandidates = [
  process.env.AIOPS_BROWSER_PATH,
  path.join(programFilesX86, 'Microsoft', 'Edge', 'Application', 'msedge.exe'),
  path.join(programFiles, 'Microsoft', 'Edge', 'Application', 'msedge.exe'),
  path.join(programFiles, 'Google', 'Chrome', 'Application', 'chrome.exe'),
  path.join(programFilesX86, 'Google', 'Chrome', 'Application', 'chrome.exe'),
].filter(Boolean)
const browserPath = browserCandidates.find((candidate) => fs.existsSync(candidate))
if (!browserPath) throw new Error('Microsoft Edge or Google Chrome was not found')

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds))
}

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

async function waitForBrowser(port) {
  const deadline = Date.now() + 20000
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/list`)
      const targets = await response.json()
      const page = targets.find((target) => target.type === 'page')
      if (page?.webSocketDebuggerUrl) return page.webSocketDebuggerUrl
    } catch {}
    await delay(200)
  }
  throw new Error('browser DevTools endpoint did not become ready')
}

class CDPClient {
  constructor(url) {
    this.url = url
    this.nextID = 1
    this.pending = new Map()
  }

  async connect() {
    this.socket = new WebSocket(this.url)
    this.socket.addEventListener('message', (event) => {
      const message = JSON.parse(String(event.data))
      if (!message.id) return
      const pending = this.pending.get(message.id)
      if (!pending) return
      this.pending.delete(message.id)
      if (message.error) pending.reject(new Error(message.error.message))
      else pending.resolve(message.result)
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
      }, 15000)
      this.pending.set(id, {
        resolve: (value) => { clearTimeout(timeout); resolve(value) },
        reject: (error) => { clearTimeout(timeout); reject(error) },
      })
      this.socket.send(JSON.stringify({ id, method, params }))
    })
  }

  close() {
    if (this.socket?.readyState === WebSocket.OPEN) this.socket.close()
  }
}

async function evaluate(client, expression) {
  const result = await client.send('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
  })
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.text || 'browser evaluation failed')
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

async function navigate(client, url, expectedText) {
  await client.send('Page.navigate', { url })
  await waitFor(client, `document.readyState === 'complete'`, `${url} document load`)
  await waitFor(client, `document.body && document.body.innerText.includes(${JSON.stringify(expectedText)})`, `${url} content`)
  await delay(1200)
}

async function capture(client, filename) {
  const result = await client.send('Page.captureScreenshot', {
    format: 'png',
    fromSurface: true,
    captureBeyondViewport: false,
  })
  fs.writeFileSync(path.join(outputDirectory, filename), Buffer.from(result.data, 'base64'))
}

fs.mkdirSync(artifactsDirectory, { recursive: true })
fs.rmSync(profileDirectory, { recursive: true, force: true })
fs.mkdirSync(profileDirectory, { recursive: true })
fs.mkdirSync(outputDirectory, { recursive: true })

const port = await reservePort()
const browser = spawn(browserPath, [
  '--headless=new',
  '--disable-gpu',
  '--hide-scrollbars',
  '--no-first-run',
  '--no-default-browser-check',
  '--disable-extensions',
  '--remote-allow-origins=*',
  `--remote-debugging-port=${port}`,
  '--remote-debugging-address=127.0.0.1',
  `--user-data-dir=${profileDirectory}`,
  `--window-size=${viewport.width},${viewport.height}`,
  'about:blank',
], { stdio: 'ignore', windowsHide: true })

let client
try {
  const webSocketURL = await waitForBrowser(port)
  client = new CDPClient(webSocketURL)
  await client.connect()
  await client.send('Page.enable')
  await client.send('Runtime.enable')
  await client.send('Emulation.setDeviceMetricsOverride', {
    width: viewport.width,
    height: viewport.height,
    deviceScaleFactor: 1,
    mobile: false,
  })

  await navigate(client, `${webBase}/login`, '登录运维控制台')
  await evaluate(client, `(() => {
    const username = document.querySelector('#username')
    const password = document.querySelector('#password')
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
    setter.call(username, ${JSON.stringify(username)})
    username.dispatchEvent(new Event('input', { bubbles: true }))
    setter.call(password, ${JSON.stringify(password)})
    password.dispatchEvent(new Event('input', { bubbles: true }))
    document.querySelector('form').requestSubmit()
    return true
  })()`)
  await waitFor(client, `location.pathname !== '/login'`, 'successful login')

  const pages = [
    { name: 'Dashboard', route: '/', expected: '累计规则命中', file: '01-dashboard.png' },
    { name: 'Clusters', route: '/clusters', expected: 'demo-kind-', file: '02-clusters.png' },
    { name: 'Workloads', route: '/workloads', expected: '工作负载', file: '03-workloads.png' },
    { name: 'Diagnoses', route: '/diagnoses', expected: '智能诊断', file: '04-diagnoses.png' },
  ]
  for (const page of pages) {
    await navigate(client, `${webBase}${page.route}`, page.expected)
    await capture(client, page.file)
    process.stdout.write(`Captured ${page.file}\n`)
  }

  const git = spawnSync('git', ['rev-parse', '--verify', 'HEAD'], { cwd: root, encoding: 'utf8', windowsHide: true })
  const metadata = {
    captured_at: new Date().toISOString(),
    web_base: webBase,
    viewport,
    source_revision: git.status === 0 ? git.stdout.trim() : 'uncommitted-baseline',
    pages: pages.map(({ name, route, file }) => ({ name, route, file })),
  }
  fs.writeFileSync(path.join(outputDirectory, 'capture-metadata.json'), `${JSON.stringify(metadata, null, 2)}\n`, 'utf8')
} finally {
  client?.close()
  if (!browser.killed) browser.kill()
  await delay(500)
  if (browser.pid) spawnSync('taskkill', ['/pid', String(browser.pid), '/T', '/F'], { stdio: 'ignore', windowsHide: true })
  fs.rmSync(profileDirectory, { recursive: true, force: true })
}

/**
 * Capture UI baselines for the K8s AIOps console.
 *
 * Uses headless Chrome over CDP (same technique as capture-thesis-screenshots).
 * Captures each view at the configured viewports with `prefers-reduced-motion:
 * reduce` emulated so page rendering is deterministic (CSS animations reset and
 * the ParticleNetwork canvas freezes), then stores the PNG plus a manifest entry
 * (commit, viewport, sha256, motion masks) under docs/ui-baselines/.
 *
 * Usage:
 *   node scripts/capture-ui-baselines.mjs            # capture mode (writes baselines)
 *   node scripts/capture-ui-baselines.mjs --verify   # verify mode (re-capture + compare)
 * Env: AIOPS_UI_BASE (default http://127.0.0.1:18080)
 *      AIOPS_UI_BASELINES_DIR (default <repo>/docs/ui-baselines)
 */
import { spawn, spawnSync } from 'node:child_process'
import fs from 'node:fs'
import net from 'node:net'
import path from 'node:path'
import crypto from 'node:crypto'
import { fileURLToPath } from 'node:url'

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(scriptDirectory, '..')
const verifyMode = process.argv.includes('--verify')
const baseUrl = (process.env.AIOPS_UI_BASE || 'http://127.0.0.1:18080').replace(/\/$/, '')
const baselineDir = path.resolve(process.env.AIOPS_UI_BASELINES_DIR || path.join(root, 'docs', 'ui-baselines'))
const manifestPath = path.join(baselineDir, 'manifest.json')
const imagesDir = path.join(baselineDir, 'images')
const diffThreshold = Number(process.env.AIOPS_UI_DIFF_THRESHOLD || '0.002') // 0.2% tolerated pixels

const CHROME =
  process.env.AIOPS_BROWSER_PATH ||
  (fs.existsSync('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome')
    ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
    : fs.existsSync('/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge')
      ? '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge'
      : null)
if (!CHROME) throw new Error('Google Chrome or Microsoft Edge not found')

const viewportSet = [
  { name: 'desktop-1440x900', width: 1440, height: 900 },
  { name: 'mobile-375x812', width: 375, height: 812 },
]

const views = [
  {
    name: 'login',
    path: '/login',
    readyText: '进入控制台',
    settleMs: 1400,
    masks: ['document.querySelectorAll(".login-signal")[2]?.querySelector("b")'],
  },
]

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
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
      }, 20000)
      this.pending.set(id, {
        resolve: (value) => {
          clearTimeout(timeout)
          resolve(value)
        },
        reject: (error) => {
          clearTimeout(timeout)
          reject(error)
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
      const response = await fetch(`http://127.0.0.1:${port}/json/list`)
      const targets = await response.json()
      const page = targets.find((target) => target.type === 'page')
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
  await waitFor(
    client,
    `document.body && document.body.innerText.includes(${JSON.stringify(expectedText)})`,
    `${url} content`,
  )
}

function rectWithin(rect, viewport) {
  if (!rect || rect.w < 1 || rect.h < 1) return null
  const x = Math.max(0, Math.round(rect.x))
  const y = Math.max(0, Math.round(rect.y))
  const w = Math.min(viewport.width - x, Math.round(rect.w))
  const h = Math.min(viewport.height - y, Math.round(rect.h))
  if (w < 1 || h < 1) return null
  return { x, y, w, h }
}

async function captureEntry(client, view, viewport) {
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
  await navigate(client, `${baseUrl}${view.path}`, view.readyText)
  await delay(view.settleMs || 1200)

  const screenshot = await client.send('Page.captureScreenshot', {
    format: 'png',
    fromSurface: true,
    captureBeyondViewport: false,
  })
  const png = Buffer.from(screenshot.data, 'base64')

  const masks = []
  for (const selector of view.masks || []) {
    const rect = await evaluate(
      client,
      `(() => { const el = ${selector}; if (!el) return null; const r = el.getBoundingClientRect(); return { x: r.x, y: r.y, w: r.width, h: r.height }; })()`,
    )
    const clamped = rectWithin(rect, viewport)
    if (clamped) masks.push(clamped)
  }

  return { png, masks }
}

async function sha256(buffer) {
  return crypto.createHash('sha256').update(buffer).digest('hex')
}

function currentCommit() {
  const result = spawnSync('git', ['rev-parse', '--short', 'HEAD'], { cwd: root, encoding: 'utf8' })
  return result.status === 0 ? result.stdout.trim() : 'unknown'
}

function readBmpPixelData(bmpPath) {
  const buffer = fs.readFileSync(bmpPath)
  const pixelOffset = buffer.readUInt32LE(10)
  const width = buffer.readInt32LE(18)
  const rawHeight = buffer.readInt32LE(22)
  const height = Math.abs(rawHeight)
  const bpp = buffer.readUInt16LE(28)
  if (bpp !== 24 && bpp !== 32) throw new Error(`unsupported BMP bpp ${bpp}`)
  const bytesPerPixel = bpp / 8
  const rowSize = Math.ceil((width * bpp) / 32) * 4
  // sips produces bottom-up rows for positive height
  const bottomUp = rawHeight > 0
  return { width, height, pixelOffset, bytesPerPixel, rowSize, bottomUp, buffer }
}

function pixel(buffer, meta, x, y) {
  const row = meta.bottomUp ? meta.height - 1 - y : y
  const offset = meta.pixelOffset + row * meta.rowSize + x * meta.bytesPerPixel
  const b = buffer[offset]
  const g = buffer[offset + 1]
  const r = buffer[offset + 2]
  return [r, g, b]
}

function toBmp(pngPath) {
  const bmpPath = pngPath.replace(/\.png$/, '.bmp')
  const sips = spawnSync('sips', ['-s', 'format', 'bmp', pngPath, '--out', bmpPath], { encoding: 'utf8' })
  if (sips.status !== 0) throw new Error(`sips failed: ${sips.stderr}`)
  return bmpPath
}

function inMask(x, y, masks) {
  return masks.some((m) => x >= m.x && x < m.x + m.w && y >= m.y && y < m.y + m.h)
}

function comparePngs(baselinePng, currentPng, masks, viewport) {
  const bmpA = toBmp(baselinePng)
  const bmpB = toBmp(currentPng)
  try {
    const metaA = readBmpPixelData(bmpA)
    const metaB = readBmpPixelData(bmpB)
    if (metaA.width !== metaB.width || metaA.height !== metaB.height) {
      return { compatible: false, reason: `dimension mismatch ${metaA.width}x${metaA.height} vs ${metaB.width}x${metaB.height}` }
    }
    const width = metaA.width
    const height = metaA.height
    let compared = 0
    let differing = 0
    let maxChannelDiff = 0
    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x++) {
        if (inMask(x, y, masks)) continue
        compared++
        const pa = pixel(metaA.buffer, metaA, x, y)
        const pb = pixel(metaB.buffer, metaB, x, y)
        const diff = Math.max(Math.abs(pa[0] - pb[0]), Math.abs(pa[1] - pb[1]), Math.abs(pa[2] - pb[2]))
        if (diff > 0) differing++
        if (diff > maxChannelDiff) maxChannelDiff = diff
      }
    }
    return { compatible: true, compared, differing, maxChannelDiff, ratio: differing / compared }
  } finally {
    fs.rmSync(bmpA, { force: true })
    fs.rmSync(bmpB, { force: true })
  }
}

async function main() {
  fs.mkdirSync(imagesDir, { recursive: true })
  const port = await reservePort()
  const profileDir = path.join(root, '.tools', `ui-baseline-profile-${process.pid}`)
  const chrome = spawn(
    CHROME,
    [
      '--headless=new',
      '--disable-gpu',
      '--no-first-run',
      '--no-default-browser-check',
      '--hide-scrollbars',
      `--remote-debugging-port=${port}`,
      `--user-data-dir=${profileDir}`,
      `--window-size=1440,900`,
      'about:blank',
    ],
    { stdio: 'ignore' },
  )
  try {
    const wsUrl = await waitForBrowser(port)
    const client = new CDPClient(wsUrl)
    await client.connect()
    await client.send('Page.enable')
    await client.send('Runtime.enable')

    // Deterministic capture: stub Math.random so particle canvases and any
    // layout randomness render identically across runs (baseline vs verify).
    await client.send('Page.addScriptToEvaluateOnNewDocument', {
      source:
        '(() => { let s = 20260813; const next = () => { s = (s * 1664525 + 1013904223) >>> 0; return s / 4294967296; }; Math.random = next; })();',
    })

    const commit = currentCommit()
    const entries = []

    const captureDir = verifyMode
      ? path.join(root, '.tools', `ui-baseline-verify-${process.pid}`, 'images')
      : imagesDir
    fs.mkdirSync(captureDir, { recursive: true })

    for (const view of views) {
      for (const viewport of viewportSet) {
        const { png, masks } = await captureEntry(client, view, viewport)
        const file = `${view.name}-${viewport.name}.png`
        const filename = path.join(captureDir, file)
        fs.writeFileSync(filename, png)
        const hash = await sha256(png)
        entries.push({
          view: view.name,
          path: view.path,
          viewport: `${viewport.width}x${viewport.height}`,
          file: `images/${file}`,
          sha256: hash,
          bytes: png.length,
          commit,
          motion: 'reduce',
          masks,
        })
        console.log(`captured ${file} (${viewport.width}x${viewport.height}) ${png.length}B sha256=${hash.slice(0, 12)}`)
      }
    }

    if (verifyMode) {
      const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'))
      const baselineByName = new Map(manifest.entries.map((e) => [`${e.view}@${e.viewport}`, e]))
      let failed = false
      for (const entry of entries) {
        const baseline = baselineByName.get(`${entry.view}@${entry.viewport}`)
        if (!baseline) {
          console.log(`NO BASELINE for ${entry.view}@${entry.viewport}`)
          failed = true
          continue
        }
        const captureName = path.basename(baseline.file)
        const baselineFile = path.join(baselineDir, baseline.file)
        const currentFile = path.join(captureDir, captureName)
        if (baseline.sha256 === entry.sha256) {
          console.log(`IDENTICAL  ${entry.view}@${entry.viewport}`)
          continue
        }
        const result = comparePngs(baselineFile, currentFile, baseline.masks || [], {
          width: Number(entry.viewport.split('x')[0]),
          height: Number(entry.viewport.split('x')[1]),
        })
        if (!result.compatible) {
          console.log(`INCOMPATIBLE ${entry.view}@${entry.viewport}: ${result.reason}`)
          failed = true
          continue
        }
        const ok = result.ratio <= diffThreshold
        console.log(
          `${ok ? 'PASS' : 'FAIL'}  ${entry.view}@${entry.viewport} diff=${(result.ratio * 100).toFixed(3)}% (${result.differing}/${result.compared} px) maxΔ=${result.maxChannelDiff}`,
        )
        if (!ok) failed = true
      }
      process.exitCode = failed ? 1 : 0
      return
    }

    const manifest = {
      schema: 1,
      generatedAt: new Date().toISOString(),
      commit,
      baseUrl,
      motion: 'reduce',
      diffThreshold,
      entries,
    }
    fs.writeFileSync(manifestPath, JSON.stringify(manifest, null, 2) + '\n')
    console.log(`manifest -> ${manifestPath} (${entries.length} entries)`)
  } finally {
    chrome.kill()
    fs.rmSync(profileDir, { recursive: true, force: true })
    if (verifyMode) fs.rmSync(path.join(root, '.tools', `ui-baseline-verify-${process.pid}`), { recursive: true, force: true })
  }
}

main().catch((error) => {
  console.error(`ERROR: ${error.message}`)
  process.exit(1)
})

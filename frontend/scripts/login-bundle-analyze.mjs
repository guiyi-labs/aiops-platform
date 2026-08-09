#!/usr/bin/env node
/* global process */
/**
 * Login-page bundle volume analyzer (M93-B2).
 *
 * Reads `dist/assets` and reports the login-page-specific transfer volume:
 * - LoginView JS chunk (raw + gzip), which also embeds ParticleNetwork.vue.
 * - Entry JS/CSS that any first visit must download (index-*.js / index-*.css).
 * - Login-specific CSS byte share within the global stylesheet.
 *
 * Login styles live inside the global `index-*.css` because LoginView has no
 * scoped <style> block. We therefore report:
 *   a) full entry CSS as the honest upper bound for login CSS volume, and
 *   b) a measured login selector share (chars of login-scoped rules) so the
 *      budget is not entirely diluted by header/console rules.
 */

import { readdirSync, readFileSync, existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { Buffer } from 'node:buffer'
import { join } from 'node:path'
import zlib from 'node:zlib'

const assetsDir = join(process.cwd(), 'dist', 'assets')
const outFile = process.argv[2] ?? join(process.cwd(), '.artifacts', 'login-perf', 'login-bundle.json')

function gzipSize(buffer) {
  return zlib.gzipSync(buffer).length
}

function bytesToKiB(bytes) {
  return Number((bytes / 1024).toFixed(2))
}

if (!existsSync(assetsDir)) {
  console.error('dist/assets not found — run `pnpm build` first.')
  process.exit(1)
}

const files = readdirSync(assetsDir)
const jsFiles = files.filter((name) => name.endsWith('.js'))
const cssFiles = files.filter((name) => name.endsWith('.css'))

const loginChunk = jsFiles.filter((name) => /^LoginView-/.test(name))
const entryJs = jsFiles.filter((name) => /^index-.*\.js$/.test(name))
const entryCss = cssFiles.filter((name) => /^index-.*\.css$/.test(name))

function statFiles(names) {
  return names.map((name) => {
    const raw = readFileSync(join(assetsDir, name))
    const gz = gzipSize(raw)
    return {
      file: name,
      rawBytes: raw.length,
      gzipBytes: gz,
      rawKiB: bytesToKiB(raw.length),
      gzipKiB: bytesToKiB(gz),
    }
  })
}

function loginCssShare(cssText) {
  // Rough but deterministic estimate: split the stylesheet into rule blocks
  // and classify them by whether a login-scoped selector is present.
  const selectors = /(^|,)\s*([.#][\w-]*(?:login|particle-network|topo-|hub-|flow-|node-|capabilit)[\w-]*)/gm
  const matches = []
  let match
  while ((match = selectors.exec(cssText)) !== null) {
    matches.push(match.index + match[0].indexOf(match[2]))
  }
  if (matches.length === 0) return { ruleCount: 0, byteShare: 0, percent: 0 }

  // Approximate page bytes owned by login rules using the distance to the next
  // top-level '}' that closes the rule containing the selector.
  const sorted = matches.sort((a, b) => a - b)
  let loginBytes = 0
  for (const index of sorted) {
    const close = cssText.indexOf('}', index)
    if (close === -1) continue
    const block = cssText.slice(index, close + 1)
    loginBytes += Buffer.byteLength(block, 'utf8')
  }
  const totalBytes = Buffer.byteLength(cssText, 'utf8')
  const percent = Number(((loginBytes / totalBytes) * 100).toFixed(2))
  return { ruleCount: matches.length, byteShare: loginBytes, percent }
}

const loginStats = statFiles(loginChunk)
const entryJsStats = statFiles(entryJs)
const entryCssStats = statFiles(entryCss)

const cssText = entryCssStats.length > 0 ? readFileSync(join(assetsDir, entryCssStats[0].file), 'utf8') : ''
const cssShare = loginCssShare(cssText)

const totals = {
  loginJsRawKiB: bytesToKiB(loginStats.reduce((sum, file) => sum + file.rawBytes, 0)),
  loginJsGzipKiB: bytesToKiB(loginStats.reduce((sum, file) => sum + file.gzipBytes, 0)),
  entryJsRawKiB: bytesToKiB(entryJsStats.reduce((sum, file) => sum + file.rawBytes, 0)),
  entryJsGzipKiB: bytesToKiB(entryJsStats.reduce((sum, file) => sum + file.gzipBytes, 0)),
  entryCssRawKiB: bytesToKiB(entryCssStats.reduce((sum, file) => sum + file.rawBytes, 0)),
  entryCssGzipKiB: bytesToKiB(entryCssStats.reduce((sum, file) => sum + file.gzipBytes, 0)),
  loginCssRawKiB: bytesToKiB(cssShare.byteShare),
  loginCssGzipKiB: bytesToKiB(Math.round((cssShare.byteShare / (cssText.length || 1)) * (entryCssStats[0]?.gzipBytes ?? 0))),
}

const report = {
  generatedAt: new Date().toISOString(),
  source: 'dist/assets (production build)',
  stats: {
    loginJs: loginStats,
    entryJs: entryJsStats,
    entryCss: entryCssStats,
    loginCssShare: {
      selectorCount: cssShare.ruleCount,
      byteShare: cssShare.byteShare,
      percentOfEntryCss: cssShare.percent,
    },
    totals,
  },
  note: 'LoginView JS embeds ParticleNetwork.vue (canvas + RAF). Login styles live in global index CSS; entry CSS is the honest upper bound, login selector share is the measured minimum.',
}

console.log('Login-page bundle volume:')
console.log(`  LoginView JS raw:  ${totals.loginJsRawKiB} kB  gzip: ${totals.loginJsGzipKiB} kB`)
console.log(`  Entry JS raw:      ${totals.entryJsRawKiB} kB  gzip: ${totals.entryJsGzipKiB} kB`)
console.log(`  Entry CSS raw:     ${totals.entryCssRawKiB} kB  gzip: ${totals.entryCssGzipKiB} kB`)
console.log(`  Login CSS share:   ${cssShare.percent}% of entry CSS (${totals.loginCssRawKiB} kB)`)

mkdirSync(join(process.cwd(), '.artifacts', 'login-perf'), { recursive: true })
writeFileSync(outFile, JSON.stringify(report, null, 2))
console.log(`Wrote ${outFile}`)
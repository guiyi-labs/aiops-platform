#!/usr/bin/env node
/* global process */

import { Buffer } from 'node:buffer'
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const cwd = process.cwd()
const outDir = join(cwd, '.artifacts', 'style-audit')
const layers = ['base.css', 'console-theme.css', 'motion.css', 'premium-ui.css']
const reports = layers.map((name) => {
  const path = join(cwd, 'src', 'styles', name)
  if (!existsSync(path)) throw new Error(`Missing active style layer: ${path}`)
  const source = readFileSync(path, 'utf8')
  const selectors = selectorCount(source)
  return {
    name,
    bytes: Buffer.byteLength(source),
    lines: source.split(/\r?\n/).length,
    selectorCount: selectors.total,
    uniqueSelectorCount: selectors.unique,
    sha256: createHash('sha256').update(source).digest('hex'),
  }
})

const output = {
  schema: 1,
  version: 'm96-style-baseline-v1',
  generatedAt: new Date().toISOString(),
  importOrder: layers,
  layers: reports,
  totals: {
    bytes: reports.reduce((sum, report) => sum + report.bytes, 0),
    lines: reports.reduce((sum, report) => sum + report.lines, 0),
    selectorCount: reports.reduce((sum, report) => sum + report.selectorCount, 0),
    uniqueSelectorCount: reports.reduce((sum, report) => sum + report.uniqueSelectorCount, 0),
  },
  mode: 'report',
  removedUnreferencedLayer: 'kubesphere-theme.css',
}
mkdirSync(outDir, { recursive: true })
const jsonPath = join(outDir, 'm96-style-baseline-v1.json')
writeFileSync(jsonPath, JSON.stringify(output, null, 2))

const lines = [
  '# M96 active CSS layer baseline',
  '',
  `- Generated: ${output.generatedAt}`,
  `- Import order: ${layers.join(' -> ')}`,
  `- Totals: ${output.totals.bytes} bytes, ${output.totals.lines} lines, ${output.totals.selectorCount} selector occurrences, ${output.totals.uniqueSelectorCount} unique selectors`,
  `- Removed unreferenced layer: ${output.removedUnreferencedLayer}`,
  '- Mode: report; no fail-closed size threshold is set from this first M96 measurement.',
  '',
  '| Layer | Bytes | Lines | Selectors | Unique selectors | SHA-256 |',
  '|---|---:|---:|---:|---:|---|',
  ...reports.map((report) => `| ${report.name} | ${report.bytes} | ${report.lines} | ${report.selectorCount} | ${report.uniqueSelectorCount} | ${report.sha256} |`),
]
const markdownPath = join(outDir, 'm96-style-baseline-v1.md')
writeFileSync(markdownPath, lines.join('\n'), 'utf8')
console.log(`Wrote ${jsonPath}`)
console.log(`Wrote ${markdownPath}`)

function selectorCount(source) {
  const selectors = new Set()
  let total = 0
  let depth = 0
  let prelude = ''
  let comment = false
  for (let index = 0; index < source.length; index += 1) {
    const character = source[index]
    const next = source[index + 1]
    if (comment) {
      if (character === '*' && next === '/') {
        comment = false
        index += 1
      }
      continue
    }
    if (character === '/' && next === '*') {
      comment = true
      index += 1
      continue
    }
    if (character === '{') {
      if (depth === 0 && prelude.trim() && !prelude.trim().startsWith('@')) {
        const current = prelude.trim().split(',').map((item) => item.trim()).filter(Boolean)
        current.forEach((selector) => selectors.add(selector))
        total += current.length
      }
      depth += 1
      prelude = ''
      continue
    }
    if (character === '}') {
      depth = Math.max(0, depth - 1)
      prelude = ''
      continue
    }
    if (depth === 0) prelude += character
  }
  return { total, unique: selectors.size }
}

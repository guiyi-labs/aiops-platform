/**
 * CSS token audit & safe migration for the frontend theme layers.
 *
 * Layers (import order, later wins): base.css → console-theme.css →
 * motion.css → premium-ui.css. Multiple :root blocks merge per property
 * (later wins). This script:
 *
 *   1. Computes the EFFECTIVE value of every custom property (resolving
 *      var(--x) references against the same layer chain).
 *   2. Scans every rule for hardcoded color literals (hex/rgb/rgba),
 *      skipping :root definitions, comments and var(--x, fallback) fallbacks.
 *   3. Classifies literals: MATCHED (a token exists with the exact same
 *      effective value → safe to replace with var()) vs ORPHAN (no token).
 *
 * Usage:
 *   node scripts/audit-css-tokens.mjs              # report only (exit 0)
 *   node scripts/audit-css-tokens.mjs --apply      # apply exact-value replacements
 *   node scripts/audit-css-tokens.mjs --check      # fail if any MATCHED literal remains
 *
 * Env: AIOPS_CSS_ROOT (default <repo>/frontend/src/styles)
 */
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(scriptDirectory, '..')
const stylesDir = path.resolve(process.env.AIOPS_CSS_ROOT || path.join(root, 'frontend', 'src', 'styles'))
const layerOrder = ['base.css', 'console-theme.css', 'motion.css', 'premium-ui.css']
const applyMode = process.argv.includes('--apply')
const checkMode = process.argv.includes('--check')

function stripComments(css) {
  return css.replace(/\/\*[\s\S]*?\*\//g, (m) => ' '.repeat(m.length))
}

function normalizeColor(value) {
  let v = value.trim().replace(/\s+/g, '').toLowerCase()
  // expand hex shorthand #abc → #aabbcc
  const hex = v.match(/^#([0-9a-f]{3})$/)
  if (hex) v = `#${hex[1].split('').map((c) => c + c).join('')}`
  return v
}

function parseRootTokens(css) {
  // Collect :root { ... } blocks with brace-depth tracking
  const tokens = {} // name → raw value (last wins per layer)
  const stripped = stripComments(css)
  let i = 0
  const len = stripped.length
  while (i < len) {
    const brace = stripped.indexOf('{', i)
    if (brace === -1) break
    const selector = stripped.slice(0, brace).trim()
    // find matching close brace
    let depth = 0
    let j = brace
    for (; j < len; j++) {
      if (stripped[j] === '{') depth++
      else if (stripped[j] === '}') {
        depth--
        if (depth === 0) break
      }
    }
    const block = stripped.slice(brace + 1, j)
    if (/:root\s*$/.test(selector)) {
      const re = /(--[\w-]+)\s*:\s*([^;]+);/g
      let m
      while ((m = re.exec(block))) {
        tokens[m[1]] = m[2].trim()
      }
    }
    i = j + 1
  }
  return tokens
}

function resolveTokens(tokens) {
  // resolve var(--x) references (single-level, no cycles expected)
  const resolved = { ...tokens }
  let changed = true
  let guard = 0
  while (changed && guard++ < 10) {
    changed = false
    for (const [name, value] of Object.entries(resolved)) {
      const m = value.match(/^var\(\s*(--[\w-]+)\s*\)$/)
      if (m && resolved[m[1]] !== undefined && resolved[m[1]] !== value) {
        const next = resolved[m[1]]
        if (next !== value) {
          resolved[name] = next
          changed = true
        }
      }
    }
  }
  return resolved
}

function findReplaceableLiterals(css, tokens) {
  // returns [{ start, end, literal, token }]
  const stripped = stripComments(css)
  const results = []
  const literalRe = /rgba?\([^)]*\)|#[0-9a-fA-F]{3,8}/g
  // precompute regions to skip: inside :root blocks and inside var(...) calls
  const skipRegions = []
  // :root blocks
  let i = 0
  const len = stripped.length
  while (i < len) {
    const brace = stripped.indexOf('{', i)
    if (brace === -1) break
    const selector = stripped.slice(0, brace).trim()
    let depth = 0
    let j = brace
    for (; j < len; j++) {
      if (stripped[j] === '{') depth++
      else if (stripped[j] === '}') {
        depth--
        if (depth === 0) break
      }
    }
    if (/:root\s*$/.test(selector)) skipRegions.push([brace, j + 1])
    i = j + 1
  }
  // var(...) call regions
  for (let k = 0; k < len; k++) {
    if (stripped[k] !== 'v') continue
    const m = stripped.slice(k, k + 3)
    if (m !== 'var' && m !== 'VAR') continue
    if (!/\s*\($/.test(stripped.slice(k + 3, k + 5))) continue
    // find matching close paren
    let depth = 0
    let p = k + 3
    for (; p < len; p++) {
      if (stripped[p] === '(') depth++
      else if (stripped[p] === ')') {
        depth--
        if (depth === 0) break
      }
    }
    if (depth === 0) skipRegions.push([k, p + 1])
  }

  const inRegion = (pos) => skipRegions.some(([a, b]) => pos >= a && pos < b)

  let m
  while ((m = literalRe.exec(stripped))) {
    if (inRegion(m.index)) continue
    const literal = m[0]
    const norm = normalizeColor(literal)
    const token = Object.entries(tokens).find(([, value]) => normalizeColor(value) === norm)?.[0]
    if (token) results.push({ start: m.index, end: m.index + m[0].length, literal, token })
  }
  return results
}

function loadLayer() {
  const combined = []
  const allTokens = {}
  for (const file of layerOrder) {
    const css = fs.readFileSync(path.join(stylesDir, file), 'utf8')
    combined.push({ file, css })
    const tokens = parseRootTokens(css)
    for (const [k, v] of Object.entries(tokens)) allTokens[k] = v
  }
  const resolved = resolveTokens(allTokens)
  return { combined, resolved }
}

const { combined, resolved } = loadLayer()

// ---- report ----
const perFile = combined.map(({ file, css }) => {
  const found = findReplaceableLiterals(css, resolved)
  return { file, replaceable: found.length, literals: found }
})

const totalReplaceable = perFile.reduce((s, f) => s + f.replaceable, 0)

if (applyMode) {
  let replaced = 0
  const cssByFile = new Map(combined.map(({ file, css }) => [file, css]))
  for (const { file, literals } of perFile) {
    if (literals.length === 0) continue
    const css = cssByFile.get(file)
    // replace from end to start to keep positions valid
    let out = css
    for (const { start, end, token } of [...literals].reverse()) {
      out = out.slice(0, start) + `var(${token})` + out.slice(end)
      replaced++
    }
    fs.writeFileSync(path.join(stylesDir, file), out)
    console.log(`applied ${literals.length} replacements in ${file}`)
  }
  console.log(`total replaced: ${replaced}`)
  process.exit(0)
}

for (const { file, replaceable, literals } of perFile) {
  console.log(`\n== ${file}: ${replaceable} replaceable literals ==`)
  const byToken = {}
  for (const { literal, token } of literals) {
    byToken[token] = byToken[token] || { count: 0, literals: new Set() }
    byToken[token].count++
    byToken[token].literals.add(literal)
  }
  const top = Object.entries(byToken).sort((a, b) => b[1].count - a[1].count).slice(0, 12)
  for (const [token, info] of top) {
    console.log(`  var(${token})  x${info.count}   (${[...info.literals].join(', ')})`)
  }
  if (replaceable === 0) console.log('  (clean — no exact-value literals outside :root/var())')
}

// orphan summary (top hardcoded values without an exact token)
const orphanCount = {}
for (const { literals } of perFile) {
  const found = new Map()
  for (const { literal } of literals) found.set(literal, (found.get(literal) || 0) + 1)
  for (const [lit, count] of found) orphanCount[lit] = (orphanCount[lit] || 0) + count
}
// also count literals that had no token at all
const allLiterals = []
for (const { file, css } of combined) {
  const stripped = stripComments(css)
  const re = /rgba?\([^)]*\)|#[0-9a-fA-F]{3,8}/g
  let m
  while ((m = re.exec(stripped))) allLiterals.push({ file, literal: m[0] })
}
const hardcoded = {}
for (const { literal } of allLiterals) hardcoded[normalizeColor(literal)] = (hardcoded[normalizeColor(literal)] || 0) + 1
console.log('\n== orphan literals (no exact token) — top 20 ==')
Object.entries(hardcoded)
  .sort((a, b) => b[1] - a[1])
  .slice(0, 20)
  .forEach(([color, count]) => console.log(`  ${color}  x${count}`))

console.log(`\nTOTAL replaceable: ${totalReplaceable}`)
if (checkMode && totalReplaceable > 0) {
  console.error(`FAIL: ${totalReplaceable} exact-value literals should use tokens (run --apply)`)
  process.exit(1)
}
if (checkMode) console.log('PASS: no exact-value literals outside :root/var()')

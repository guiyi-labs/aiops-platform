/* global process */
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import zlib from 'node:zlib'

const ENTRY_JS_GZIP_KB = 60
const LARGEST_CHUNK_GZIP_KB = 32
const TOTAL_JS_GZIP_KB = 310
// 70 -> 74（2026-11-02）。原因不是要"放宽预算"，而是旧值是在 2026-10-30 定的，
// 之后落地的两批纯 CSS 特性没再量过：完整的暗色语义 token（surface/text/border/status/
// shadow + 13 个 --topo-*）与 ≤720px 的表格卡片化。全新构建实测 70.3kB，超旧阈值 0.3kB。
// 上一轮报「gate 通过」时读的是 10-31 之前的 dist，属于陈旧产物造成的假绿灯。
// 74 留约 5% 余量；再涨就该查是否有人往 CSS 里塞运行时不需要的东西，而不是继续抬这个数。
const TOTAL_CSS_GZIP_KB = 74

const dir = join(process.cwd(), 'dist', 'assets')
let files
try {
  files = readdirSync(dir).filter((f) => f.endsWith('.js') || f.endsWith('.css'))
} catch {
  console.error('dist/assets not found — run `pnpm build` first.')
  process.exit(1)
}

let totalJs = 0
let totalCss = 0
let largestChunkKb = 0
let largestChunkName = ''
let entryGzipKb = 0
let entryName = ''

for (const f of files) {
  const gz = zlib.gzipSync(readFileSync(join(dir, f))).length / 1024
  if (f.endsWith('.js')) {
    totalJs += gz
    if (gz > largestChunkKb && !f.startsWith('index-')) {
      largestChunkKb = gz
      largestChunkName = f
    }
    if (f.startsWith('index-')) {
      entryName = f
      entryGzipKb = gz
    }
  } else {
    totalCss += gz
  }
}

const failures = []
if (entryGzipKb > ENTRY_JS_GZIP_KB) failures.push(`entry JS ${entryGzipKb.toFixed(1)}kB > ${ENTRY_JS_GZIP_KB}kB`)
if (largestChunkKb > LARGEST_CHUNK_GZIP_KB) failures.push(`largest chunk ${largestChunkName} ${largestChunkKb.toFixed(1)}kB > ${LARGEST_CHUNK_GZIP_KB}kB`)
if (totalJs > TOTAL_JS_GZIP_KB) failures.push(`total JS gzip ${totalJs.toFixed(1)}kB > ${TOTAL_JS_GZIP_KB}kB`)
if (totalCss > TOTAL_CSS_GZIP_KB) failures.push(`total CSS gzip ${totalCss.toFixed(1)}kB > ${TOTAL_CSS_GZIP_KB}kB`)

console.log(`entry JS gzip:  ${entryGzipKb.toFixed(1)} kB (${entryName})`)
console.log(`largest chunk:  ${largestChunkName} ${largestChunkKb.toFixed(1)} kB`)
console.log(`total JS gzip:  ${totalJs.toFixed(1)} kB`)
console.log(`total CSS gzip: ${totalCss.toFixed(1)} kB`)
if (failures.length > 0) {
  console.error('Bundle gate FAILED:')
  for (const f of failures) console.error(`  - ${f}`)
  process.exit(1)
}
console.log('Bundle gate passed.')
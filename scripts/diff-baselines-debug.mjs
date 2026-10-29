#!/usr/bin/env node
/**
 * 诊断用：比较两批基线截图，输出差异的分布（按行/按列）与包围盒。
 *
 * 存在理由：capture-ui-baselines.mjs --verify 只报「差了多少像素」，不报「差在哪里」。
 * 排查捕获不确定性时，必须知道是页头的时间戳在动、还是主体布局在动，否则只能猜。
 *
 * 用法：node scripts/diff-baselines-debug.mjs <dirA> <dirB> [view] [viewport]
 */
import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'

function toBmp(pngPath) {
  const bmpPath = pngPath.replace(/\.png$/, '.debug.bmp')
  const r = spawnSync('sips', ['-s', 'format', 'bmp', pngPath, '--out', bmpPath], { encoding: 'utf8' })
  if (r.status !== 0) throw new Error(`sips failed: ${r.stderr}`)
  return bmpPath
}

function readBmp(p) {
  const buffer = fs.readFileSync(p)
  const pixelOffset = buffer.readUInt32LE(10)
  const width = buffer.readInt32LE(18)
  const rawHeight = buffer.readInt32LE(22)
  const height = Math.abs(rawHeight)
  const bpp = buffer.readUInt16LE(28)
  const bytesPerPixel = bpp / 8
  const rowSize = Math.ceil((width * bpp) / 32) * 4
  return { width, height, pixelOffset, bytesPerPixel, rowSize, bottomUp: rawHeight > 0, buffer }
}

function px(m, x, y) {
  const row = m.bottomUp ? m.height - 1 - y : y
  const o = m.pixelOffset + row * m.rowSize + x * m.bytesPerPixel
  return [m.buffer[o], m.buffer[o + 1], m.buffer[o + 2]]
}

const [, , dirA, dirB, onlyView, onlyViewport] = process.argv
if (!dirA || !dirB) {
  console.error('usage: node scripts/diff-baselines-debug.mjs <dirA> <dirB> [view] [viewport]')
  process.exit(2)
}

const files = fs.readdirSync(path.join(dirA, 'images')).filter((f) => f.endsWith('.png'))
for (const file of files.sort()) {
  if (onlyView && !file.startsWith(`${onlyView}-`)) continue
  if (onlyViewport && !file.includes(onlyViewport)) continue
  const a = path.join(dirA, 'images', file)
  const b = path.join(dirB, 'images', file)
  if (!fs.existsSync(b)) { console.log(`${file}: missing in B`); continue }

  const bmpA = toBmp(a)
  const bmpB = toBmp(b)
  const mA = readBmp(bmpA)
  const mB = readBmp(bmpB)
  if (mA.width !== mB.width || mA.height !== mB.height) {
    console.log(`${file}: dimension mismatch`)
    continue
  }

  let differing = 0
  let minX = 1e9, minY = 1e9, maxX = -1, maxY = -1
  const rowHits = new Map()
  for (let y = 0; y < mA.height; y++) {
    for (let x = 0; x < mA.width; x++) {
      const pa = px(mA, x, y)
      const pb = px(mB, x, y)
      if (pa[0] !== pb[0] || pa[1] !== pb[1] || pa[2] !== pb[2]) {
        differing++
        if (x < minX) minX = x
        if (x > maxX) maxX = x
        if (y < minY) minY = y
        if (y > maxY) maxY = y
        rowHits.set(y, (rowHits.get(y) || 0) + 1)
      }
    }
  }
  fs.rmSync(bmpA, { force: true })
  fs.rmSync(bmpB, { force: true })

  if (differing === 0) continue
  const total = mA.width * mA.height
  console.log(`\n${file}  diff=${((differing / total) * 100).toFixed(3)}% (${differing}px)`)
  console.log(`  bbox: x=${minX}..${maxX}  y=${minY}..${maxY}`)
  // 行分布聚成连续区间，便于看出「只差页头一行」还是「整块主体」
  const ys = [...rowHits.keys()].sort((p, q) => p - q)
  const bands = []
  for (const y of ys) {
    const last = bands[bands.length - 1]
    if (last && y === last.end + 1) last.end = y
    else bands.push({ start: y, end: y })
  }
  const top = bands
    .map((bd) => ({ ...bd, px: ys.filter((y) => y >= bd.start && y <= bd.end).reduce((s, y) => s + rowHits.get(y), 0) }))
    .sort((p, q) => q.px - p.px)
    .slice(0, 6)
  console.log('  top bands (y range → differing px):')
  for (const bd of top) console.log(`    y ${bd.start}..${bd.end}  ${bd.px}px`)
}
